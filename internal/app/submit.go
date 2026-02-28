package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/net/html"

	"crow.watch/internal/auth"
	"crow.watch/internal/link"
	"crow.watch/internal/store"
)

func (a *App) submitPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tags, err := a.Queries.ListActiveTagsWithCategory(r.Context())
	if err != nil {
		a.serverError(w, r, "list active tags", err)
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab != "text" {
		tab = "link"
	}

	a.render(w, "submit", SubmitPageData{
		BaseData:  a.baseData(r),
		Tab:       tab,
		TagGroups: toTagGroups(tags, current.User.IsModerator),
	})
}

func (a *App) submitStory(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	rawURL := strings.TrimSpace(r.FormValue("url"))
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	tagIDStrs := r.Form["tags"]

	errs := make(map[string]string)

	// Validate title
	if title == "" {
		errs["title"] = "Title is required."
	} else if len(title) > 150 {
		errs["title"] = "Title must be 150 characters or fewer."
	}

	// Validate content: need URL xor body
	hasURL := rawURL != ""
	hasBody := body != ""
	if hasURL && hasBody {
		errs["url"] = "A story must have either a URL or a text body, not both."
	} else if !hasURL && !hasBody {
		errs["url"] = "URL or text body is required."
	}

	// Validate body length
	if hasBody && len(body) > 10000 {
		errs["body"] = "Text body must be 10,000 characters or fewer."
	}

	// Clean URL (only if it's a link post)
	var result link.CleanResult
	if hasURL && !hasBody {
		var err error
		result, err = link.Clean(rawURL)
		if err != nil {
			var ve *link.ValidationError
			if errors.As(err, &ve) {
				errs["url"] = ve.Message
			} else {
				errs["url"] = "Invalid URL."
			}
		}
	}

	// Parse tag IDs
	var tagIDs []int64
	for _, s := range tagIDStrs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			tagIDs = append(tagIDs, id)
		}
	}

	// Infer active tab from form content
	tab := "link"
	if hasBody {
		tab = "text"
	}

	if len(errs) > 0 {
		a.renderSubmitError(w, r, current, tab, rawURL, title, body, tagIDs, errs, "")
		return
	}

	// Load and validate tags
	tags, err := a.Queries.GetTagsByIDs(r.Context(), tagIDs)
	if err != nil {
		a.serverError(w, r, "get tags by ids", err)
		return
	}

	hasNonMedia := false
	for _, tag := range tags {
		if !tag.IsMedia {
			hasNonMedia = true
		}
		if tag.Privileged && !current.User.IsModerator {
			a.renderSubmitError(w, r, current, tab, rawURL, title, body, tagIDs, nil,
				"You do not have permission to use the tag \""+tag.Tag+"\".")
			return
		}
	}

	if !hasNonMedia {
		errs["tags"] = "At least one non-media tag is required."
		a.renderSubmitError(w, r, current, tab, rawURL, title, body, tagIDs, errs, "")
		return
	}

	isText := hasBody

	// Link-specific validation
	var domain store.Domain
	var originID pgtype.Int8
	if !isText {
		// Get or create domain
		var err error
		domain, err = a.getOrCreateDomain(r.Context(), result.Domain)
		if err != nil {
			a.serverError(w, r, "get or create domain", err)
			return
		}
		if domain.Banned {
			a.renderSubmitError(w, r, current, tab, rawURL, title, body, tagIDs, nil,
				"This domain has been banned: "+domain.BanReason)
			return
		}

		// Get or create origin
		if result.Origin != "" {
			origin, err := a.getOrCreateOrigin(r.Context(), domain.ID, result.Origin)
			if err != nil {
				a.serverError(w, r, "get or create origin", err)
				return
			}
			if origin.Banned {
				a.renderSubmitError(w, r, current, tab, rawURL, title, body, tagIDs, nil,
					"This origin has been banned: "+origin.BanReason)
				return
			}
			originID = pgtype.Int8{Int64: origin.ID, Valid: true}
		}

		// Duplicate check
		existing, err := a.Queries.FindRecentByNormalizedURL(r.Context(), pgtype.Text{String: result.Normalized, Valid: true})
		if err == nil {
			a.renderSubmitDuplicate(w, r, current, tab, rawURL, title, body, tagIDs, storyPath(existing.ShortCode, existing.Title))
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			a.serverError(w, r, "check duplicate url", err)
			return
		}
	}

	// Transaction: create story + taggings + increment counts
	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	shortCode := generateShortCode()
	params := store.CreateStoryParams{
		UserID:    current.User.ID,
		Title:     title,
		ShortCode: shortCode,
	}
	if isText {
		params.Body = pgtype.Text{String: body, Valid: true}
	} else {
		params.DomainID = pgtype.Int8{Int64: domain.ID, Valid: true}
		params.OriginID = originID
		params.Url = pgtype.Text{String: result.Cleaned, Valid: true}
		params.NormalizedUrl = pgtype.Text{String: result.Normalized, Valid: true}
	}

	story, err := qtx.CreateStory(r.Context(), params)
	if err != nil {
		a.serverError(w, r, "create story", err)
		return
	}

	for _, tag := range tags {
		if err := qtx.CreateTagging(r.Context(), store.CreateTaggingParams{
			StoryID: story.ID,
			TagID:   tag.ID,
		}); err != nil {
			a.serverError(w, r, "create tagging", err)
			return
		}
	}

	if _, err := qtx.CreateVote(r.Context(), store.CreateVoteParams{
		UserID:  current.User.ID,
		StoryID: story.ID,
	}); err != nil {
		a.serverError(w, r, "auto-upvote story", err)
		return
	}

	if !isText {
		if err := qtx.IncrementDomainStoryCount(r.Context(), domain.ID); err != nil {
			a.serverError(w, r, "increment domain story count", err)
			return
		}

		if originID.Valid {
			if err := qtx.IncrementOriginStoryCount(r.Context(), originID.Int64); err != nil {
				a.serverError(w, r, "increment origin story count", err)
				return
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	if isText {
		http.Redirect(w, r, storyPath(shortCode, title), http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (a *App) renderSubmitError(w http.ResponseWriter, r *http.Request, current auth.AuthenticatedUser, tab, rawURL, title, body string, selectedIDs []int64, errs map[string]string, generalErr string) {
	allTags, _ := a.Queries.ListActiveTagsWithCategory(r.Context())
	a.render(w, "submit", SubmitPageData{
		BaseData:  a.baseData(r),
		Tab:       tab,
		URL:       rawURL,
		Title:     title,
		Body:      body,
		TagGroups: toTagGroups(allTags, current.User.IsModerator),
		Selected:  selectedIDs,
		Errors:    errs,
		Error:     generalErr,
	})
}

func (a *App) renderSubmitDuplicate(w http.ResponseWriter, r *http.Request, current auth.AuthenticatedUser, tab, rawURL, title, body string, selectedIDs []int64, dupURL string) {
	allTags, _ := a.Queries.ListActiveTagsWithCategory(r.Context())
	a.render(w, "submit", SubmitPageData{
		BaseData:     a.baseData(r),
		Tab:          tab,
		URL:          rawURL,
		Title:        title,
		Body:         body,
		TagGroups:    toTagGroups(allTags, current.User.IsModerator),
		Selected:     selectedIDs,
		Error:        "This link has already been submitted recently.",
		DuplicateURL: dupURL,
	})
}

func (a *App) getOrCreateDomain(ctx context.Context, domainName string) (store.Domain, error) {
	d, err := a.Queries.GetDomainByName(ctx, domainName)
	if err == nil {
		return d, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Domain{}, err
	}
	return a.Queries.CreateDomain(ctx, domainName)
}

func (a *App) getOrCreateOrigin(ctx context.Context, domainID int64, originName string) (store.Origin, error) {
	o, err := a.Queries.GetOriginByName(ctx, originName)
	if err == nil {
		return o, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.Origin{}, err
	}
	return a.Queries.CreateOrigin(ctx, store.CreateOriginParams{DomainID: domainID, Origin: originName})
}

func (a *App) fetchTitle(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 2048)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
		return
	}

	result, err := link.Clean(req.URL)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Invalid URL."})
		return
	}

	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: safeTransport(),
	}
	httpReq, err := http.NewRequestWithContext(r.Context(), "GET", result.Cleaned, nil)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Could not fetch URL."})
		return
	}
	httpReq.Header.Set("User-Agent", "crow.watch/1.0 (title fetcher)")
	httpReq.Header.Set("Accept", "text/html")

	resp, err := client.Do(httpReq)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Could not fetch URL."})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "URL returned an error."})
		return
	}

	// Read at most 256KB to find the title
	body := io.LimitReader(resp.Body, 256*1024)
	title := extractTitle(body)
	if title == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "No title found."})
		return
	}
	title = cleanTitle(title, result.Cleaned)

	writeJSON(w, http.StatusOK, map[string]string{"title": title})
}

func extractTitle(r io.Reader) string {
	z := html.NewTokenizer(r)
	inTitle := false
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return ""
		case html.StartTagToken:
			tn, _ := z.TagName()
			if string(tn) == "title" {
				inTitle = true
			}
		case html.TextToken:
			if inTitle {
				return strings.TrimSpace(string(z.Text()))
			}
		case html.EndTagToken:
			if inTitle {
				return ""
			}
		}
	}
}

// cleanTitle removes common site-specific prefixes from page titles.
// For example, GitHub titles like "GitHub - owner/repo: description" become just "description".
func cleanTitle(title, fetchedURL string) string {
	if strings.Contains(fetchedURL, "github.com/") {
		// "GitHub - owner/repo: description" → "description"
		if after, ok := strings.CutPrefix(title, "GitHub - "); ok {
			if idx := strings.Index(after, ": "); idx != -1 {
				return strings.TrimSpace(after[idx+2:])
			}
		}
	}
	return title
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// safeTransport returns an http.Transport that blocks connections to
// private, loopback, and link-local IP addresses to prevent SSRF.
func safeTransport() *http.Transport {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	return &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			var validIP net.IP
			for _, ip := range ips {
				if ip.IP.IsLoopback() || ip.IP.IsPrivate() || ip.IP.IsLinkLocalUnicast() || ip.IP.IsLinkLocalMulticast() || ip.IP.IsUnspecified() {
					return nil, fmt.Errorf("blocked: %s resolves to non-public address %s", host, ip.IP)
				}
				if validIP == nil {
					validIP = ip.IP
				}
			}
			return dialer.DialContext(ctx, network, net.JoinHostPort(validIP.String(), port))
		},
	}
}

func toTagGroups(tags []store.ListActiveTagsWithCategoryRow, isModerator bool) []TagGroup {
	var groups []TagGroup
	groupIdx := make(map[string]int)
	for _, t := range tags {
		if t.Privileged && !isModerator {
			continue
		}
		opt := TagOption{
			ID:          t.ID,
			Tag:         t.Tag,
			Description: t.Description,
			IsMedia:     t.IsMedia,
			Privileged:  t.Privileged,
		}
		cat := t.CategoryName
		if idx, ok := groupIdx[cat]; ok {
			groups[idx].Tags = append(groups[idx].Tags, opt)
		} else {
			groupIdx[cat] = len(groups)
			groups = append(groups, TagGroup{Category: cat, Tags: []TagOption{opt}})
		}
	}
	return groups
}

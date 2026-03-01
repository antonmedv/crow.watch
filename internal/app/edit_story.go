package app

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/auth"
	"crow.watch/internal/link"
	"crow.watch/internal/store"
)

func (a *App) editStoryPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	code := r.PathValue("code")
	if len(code) != 6 {
		http.NotFound(w, r)
		return
	}

	row, err := a.Queries.GetStoryByShortCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get story by short code", err)
		return
	}

	tagRows, err := a.Queries.GetStoryTags(r.Context(), row.ID)
	if err != nil {
		a.serverError(w, r, "get story tags", err)
		return
	}
	var selectedIDs []int64
	for _, t := range tagRows {
		selectedIDs = append(selectedIDs, t.ID)
	}

	allTags, err := a.Queries.ListActiveTagsWithCategory(r.Context())
	if err != nil {
		a.serverError(w, r, "list active tags", err)
		return
	}

	tab := "link"
	if row.Body.Valid {
		tab = "text"
	}

	a.render(w, "submit", SubmitPageData{
		BaseData:  a.baseData(r),
		Tab:       tab,
		Title:     row.Title,
		Body:      row.Body.String,
		URL:       row.Url.String,
		TagGroups: toTagGroups(allTags, current.User.IsModerator),
		Selected:  selectedIDs,
		EditMode:  true,
		EditCode:  code,
	})
}

func (a *App) editStory(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	code := r.PathValue("code")
	if len(code) != 6 {
		http.NotFound(w, r)
		return
	}

	row, err := a.Queries.GetStoryByShortCode(r.Context(), code)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get story by short code", err)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	rawURL := strings.TrimSpace(r.FormValue("url"))
	title := strings.TrimSpace(r.FormValue("title"))
	body := strings.TrimSpace(r.FormValue("body"))
	reason := strings.TrimSpace(r.FormValue("reason"))
	tagIDStrs := r.Form["tags"]

	isLinkPost := row.Url.Valid

	errs := make(map[string]string)

	if title == "" {
		errs["title"] = "Title is required."
	} else if len(title) > 150 {
		errs["title"] = "Title must be 150 characters or fewer."
	}

	if row.Body.Valid {
		if body == "" {
			errs["body"] = "Text body is required for text posts."
		} else if len(body) > 10000 {
			errs["body"] = "Text body must be 10,000 characters or fewer."
		}
	}

	// Validate URL for link posts
	var urlResult link.CleanResult
	if isLinkPost {
		if rawURL == "" {
			errs["url"] = "URL is required."
		} else {
			var err error
			urlResult, err = link.Clean(rawURL)
			if err != nil {
				var ve *link.ValidationError
				if errors.As(err, &ve) {
					errs["url"] = ve.Message
				} else {
					errs["url"] = "Invalid URL."
				}
			}
		}
	}

	if reason == "" {
		errs["reason"] = "Reason is required."
	} else if len(reason) > 500 {
		errs["reason"] = "Reason must be 500 characters or fewer."
	}

	var tagIDs []int64
	for _, s := range tagIDStrs {
		id, err := strconv.ParseInt(s, 10, 64)
		if err == nil {
			tagIDs = append(tagIDs, id)
		}
	}

	if len(errs) > 0 {
		a.renderEditError(w, r, current, code, row, title, body, reason, rawURL, tagIDs, errs, "")
		return
	}

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
	}

	if !hasNonMedia {
		errs["tags"] = "At least one non-media tag is required."
		a.renderEditError(w, r, current, code, row, title, body, reason, rawURL, tagIDs, errs, "")
		return
	}

	// Compute diff
	oldTagRows, err := a.Queries.GetStoryTags(r.Context(), row.ID)
	if err != nil {
		a.serverError(w, r, "get story tags", err)
		return
	}
	var oldTagIDs []int64
	oldTagNames := make(map[int64]string)
	for _, t := range oldTagRows {
		oldTagIDs = append(oldTagIDs, t.ID)
		oldTagNames[t.ID] = t.Tag
	}

	newTagNames := make(map[int64]string)
	for _, t := range tags {
		newTagNames[t.ID] = t.Tag
	}

	titleChanged := title != row.Title
	bodyChanged := row.Body.Valid && body != row.Body.String
	tagsChanged := !equalSortedIDs(oldTagIDs, tagIDs)
	urlChanged := isLinkPost && urlResult.Cleaned != row.Url.String

	if !titleChanged && !bodyChanged && !tagsChanged && !urlChanged {
		http.Redirect(w, r, storyPath(row.ShortCode, row.Title), http.StatusSeeOther)
		return
	}

	metadata := make(map[string]any)
	var actions []string

	if urlChanged {
		actions = append(actions, "story.edit_url")
		metadata["url_before"] = row.Url.String
		metadata["url_after"] = urlResult.Cleaned
	}
	if titleChanged {
		actions = append(actions, "story.edit_title")
		metadata["title_before"] = row.Title
		metadata["title_after"] = title
	}
	if bodyChanged {
		actions = append(actions, "story.edit_body")
	}
	if tagsChanged {
		actions = append(actions, "story.edit_tags")
		var oldNames, newNames []string
		for _, id := range oldTagIDs {
			oldNames = append(oldNames, oldTagNames[id])
		}
		for _, id := range tagIDs {
			if name, ok := newTagNames[id]; ok {
				newNames = append(newNames, name)
			}
		}
		metadata["tags_before"] = oldNames
		metadata["tags_after"] = newNames
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		a.serverError(w, r, "marshal metadata", err)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	if urlChanged {
		domain, err := a.getOrCreateDomain(r.Context(), urlResult.Domain)
		if err != nil {
			a.serverError(w, r, "get or create domain", err)
			return
		}
		if domain.Banned {
			a.renderEditError(w, r, current, code, row, title, body, reason, rawURL, tagIDs, nil,
				"This domain has been banned: "+domain.BanReason)
			return
		}

		urlParams := store.UpdateStoryURLParams{
			Url:           pgtype.Text{String: urlResult.Cleaned, Valid: true},
			NormalizedUrl: pgtype.Text{String: urlResult.Normalized, Valid: true},
			DomainID:      pgtype.Int8{Int64: domain.ID, Valid: true},
			ID:            row.ID,
		}

		if urlResult.Origin != "" {
			origin, err := a.getOrCreateOrigin(r.Context(), domain.ID, urlResult.Origin)
			if err != nil {
				a.serverError(w, r, "get or create origin", err)
				return
			}
			if origin.Banned {
				a.renderEditError(w, r, current, code, row, title, body, reason, rawURL, tagIDs, nil,
					"This origin has been banned: "+origin.BanReason)
				return
			}
			urlParams.OriginID = pgtype.Int8{Int64: origin.ID, Valid: true}
		}

		if err := qtx.UpdateStoryURL(r.Context(), urlParams); err != nil {
			a.serverError(w, r, "update story url", err)
			return
		}
	}

	if titleChanged {
		if err := qtx.UpdateStoryTitle(r.Context(), store.UpdateStoryTitleParams{
			Title: title,
			ID:    row.ID,
		}); err != nil {
			a.serverError(w, r, "update story title", err)
			return
		}
	}

	if bodyChanged {
		if err := qtx.UpdateStoryBody(r.Context(), store.UpdateStoryBodyParams{
			Body: pgtype.Text{String: body, Valid: true},
			ID:   row.ID,
		}); err != nil {
			a.serverError(w, r, "update story body", err)
			return
		}
	}

	if tagsChanged {
		if err := qtx.DeleteTaggingsByStory(r.Context(), row.ID); err != nil {
			a.serverError(w, r, "delete taggings", err)
			return
		}
		for _, tag := range tags {
			if err := qtx.CreateTagging(r.Context(), store.CreateTaggingParams{
				StoryID: row.ID,
				TagID:   tag.ID,
			}); err != nil {
				a.serverError(w, r, "create tagging", err)
				return
			}
		}
	}

	actionStr := strings.Join(actions, ",")
	if _, err := qtx.CreateModerationLog(r.Context(), store.CreateModerationLogParams{
		ModeratorID: current.User.ID,
		Action:      actionStr,
		TargetType:  "story",
		TargetID:    row.ID,
		Reason:      reason,
		Metadata:    metadataJSON,
	}); err != nil {
		a.serverError(w, r, "create moderation log", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	displayTitle := title
	if !titleChanged {
		displayTitle = row.Title
	}
	http.Redirect(w, r, storyPath(row.ShortCode, displayTitle), http.StatusSeeOther)
}

func (a *App) renderEditError(w http.ResponseWriter, r *http.Request, current auth.AuthenticatedUser, code string, row store.GetStoryByShortCodeRow, title, body, reason, rawURL string, selectedIDs []int64, errs map[string]string, generalErr string) {
	allTags, _ := a.Queries.ListActiveTagsWithCategory(r.Context())

	tab := "link"
	if row.Body.Valid {
		tab = "text"
	}

	displayURL := rawURL
	if displayURL == "" {
		displayURL = row.Url.String
	}

	a.render(w, "submit", SubmitPageData{
		BaseData:  a.baseData(r),
		Tab:       tab,
		Title:     title,
		Body:      body,
		URL:       displayURL,
		TagGroups: toTagGroups(allTags, current.User.IsModerator),
		Selected:  selectedIDs,
		Errors:    errs,
		Error:     generalErr,
		EditMode:  true,
		EditCode:  code,
		Reason:    reason,
	})
}

func equalSortedIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	aCopy := make([]int64, len(a))
	bCopy := make([]int64, len(b))
	copy(aCopy, a)
	copy(bCopy, b)
	sort.Slice(aCopy, func(i, j int) bool { return aCopy[i] < aCopy[j] })
	sort.Slice(bCopy, func(i, j int) bool { return bCopy[i] < bCopy[j] })
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}

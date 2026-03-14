package app

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/auth"
	"crow.watch/internal/link"
	"crow.watch/internal/store"
)

// apiKeyUserFromRequest authenticates an API request via the
// "Authorization: Bearer <token>" header. On success it returns the user
// and the api_key ID. On failure it writes a JSON 401 and returns false.
func (a *App) apiKeyUserFromRequest(w http.ResponseWriter, r *http.Request) (store.User, int64, bool) {
	authHeader := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok || token == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Missing or invalid Authorization header."})
		return store.User{}, 0, false
	}

	tokenHash := auth.HashToken(token)
	row, err := a.Queries.GetAPIKeyUserByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid API key."})
			return store.User{}, 0, false
		}
		a.Log.Error("api key lookup", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return store.User{}, 0, false
	}

	if row.BannedAt.Valid || row.DeletedAt.Valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Account is not active."})
		return store.User{}, 0, false
	}

	_ = a.Queries.TouchAPIKey(r.Context(), row.ApiKeyID)

	user := store.User{
		ID:                              row.ID,
		Username:                        row.Username,
		Email:                           row.Email,
		PasswordDigest:                  row.PasswordDigest,
		IsModerator:                     row.IsModerator,
		BannedAt:                        row.BannedAt,
		DeletedAt:                       row.DeletedAt,
		InviterID:                       row.InviterID,
		Campaign:                        row.Campaign,
		PasswordResetTokenHash:          row.PasswordResetTokenHash,
		PasswordResetTokenCreatedAt:     row.PasswordResetTokenCreatedAt,
		EmailConfirmedAt:                row.EmailConfirmedAt,
		EmailConfirmationTokenHash:      row.EmailConfirmationTokenHash,
		EmailConfirmationTokenCreatedAt: row.EmailConfirmationTokenCreatedAt,
		UnconfirmedEmail:                row.UnconfirmedEmail,
		Website:                         row.Website,
		About:                           row.About,
		CreatedAt:                       row.CreatedAt,
		UpdatedAt:                       row.UpdatedAt,
	}
	return user, row.ApiKeyID, true
}

func (a *App) apiListTags(w http.ResponseWriter, r *http.Request) {
	tags, err := a.Queries.ListActiveTagsWithCategory(r.Context())
	if err != nil {
		a.Log.Error("api list tags", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	type tagResponse struct {
		Tag      string `json:"tag"`
		Category string `json:"category,omitempty"`
		IsMedia  bool   `json:"is_media"`
	}

	result := make([]tagResponse, len(tags))
	for i, t := range tags {
		result[i] = tagResponse{
			Tag:      t.Tag,
			Category: t.CategoryName,
			IsMedia:  t.IsMedia,
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func (a *App) apiSubmitStory(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.apiKeyUserFromRequest(w, r)
	if !ok {
		return
	}

	var req struct {
		URL     string   `json:"url"`
		Title   string   `json:"title"`
		Body    string   `json:"body"`
		Tags    []string `json:"tags"`
		Hotness int32    `json:"hotness"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body."})
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)

	type fieldErrors map[string]string
	errs := make(fieldErrors)

	// Validate title
	if req.Title == "" {
		errs["title"] = "Title is required."
	} else if len(req.Title) > 150 {
		errs["title"] = "Title must be 150 characters or fewer."
	}

	// Validate content: URL xor body
	hasURL := req.URL != ""
	hasBody := req.Body != ""
	if hasURL && hasBody {
		errs["url"] = "A story must have either a URL or a text body, not both."
	} else if !hasURL && !hasBody {
		errs["url"] = "URL or text body is required."
	}

	if hasBody && len(req.Body) > 10000 {
		errs["body"] = "Text body must be 10,000 characters or fewer."
	}

	if len(req.Tags) == 0 {
		errs["tags"] = "At least one tag is required."
	}

	// Clean URL early so validation errors can be reported together
	var cleanResult link.CleanResult
	if hasURL && !hasBody {
		var err error
		cleanResult, err = link.Clean(req.URL)
		if err != nil {
			var ve *link.ValidationError
			if errors.As(err, &ve) {
				errs["url"] = ve.Message
			} else {
				errs["url"] = "Invalid URL."
			}
		}
	}

	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": errs})
		return
	}

	// Resolve tag names to tag rows
	lowerNames := make([]string, len(req.Tags))
	for i, t := range req.Tags {
		lowerNames[i] = strings.ToLower(strings.TrimSpace(t))
	}
	tags, err := a.Queries.GetTagsByNames(r.Context(), lowerNames)
	if err != nil {
		a.Log.Error("api get tags by names", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}
	if len(tags) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": fieldErrors{"tags": "No valid tags found."}})
		return
	}

	hasNonMedia := false
	for _, tag := range tags {
		if !tag.IsMedia {
			hasNonMedia = true
		}
		if tag.Privileged && !user.IsModerator {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"errors": fieldErrors{"tags": "You do not have permission to use the tag \"" + tag.Tag + "\"."},
			})
			return
		}
	}
	if !hasNonMedia {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": fieldErrors{"tags": "At least one non-media tag is required."}})
		return
	}

	isText := hasBody

	var domain store.Domain
	var originID pgtype.Int8

	if !isText {
		domain, err = a.Queries.GetOrCreateDomain(r.Context(), cleanResult.Domain)
		if err != nil {
			a.Log.Error("api get or create domain", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
			return
		}
		if domain.Banned {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "This domain has been banned: " + domain.BanReason})
			return
		}

		if cleanResult.Origin != "" {
			origin, err := a.Queries.GetOrCreateOrigin(r.Context(), store.GetOrCreateOriginParams{DomainID: domain.ID, Origin: cleanResult.Origin})
			if err != nil {
				a.Log.Error("api get or create origin", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
				return
			}
			if origin.Banned {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "This origin has been banned: " + origin.BanReason})
				return
			}
			originID = pgtype.Int8{Int64: origin.ID, Valid: true}
		}

		existing, err := a.Queries.FindRecentByNormalizedURL(r.Context(), pgtype.Text{String: cleanResult.Normalized, Valid: true})
		if err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":         "This link has already been submitted recently.",
				"duplicate_url": storyPath(existing.ShortCode, existing.Title),
			})
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			a.Log.Error("api check duplicate url", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
			return
		}
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.Log.Error("api begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	shortCode := generateShortCode()
	params := store.CreateStoryParams{
		UserID:    user.ID,
		Title:     req.Title,
		ShortCode: shortCode,
	}
	if isText {
		params.Body = pgtype.Text{String: req.Body, Valid: true}
	} else {
		params.DomainID = pgtype.Int8{Int64: domain.ID, Valid: true}
		params.OriginID = originID
		params.Url = pgtype.Text{String: cleanResult.Cleaned, Valid: true}
		params.NormalizedUrl = pgtype.Text{String: cleanResult.Normalized, Valid: true}
	}

	story, err := qtx.CreateStory(r.Context(), params)
	if err != nil {
		a.Log.Error("api create story", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	for _, tag := range tags {
		if err := qtx.CreateTagging(r.Context(), store.CreateTaggingParams{
			StoryID: story.ID,
			TagID:   tag.ID,
		}); err != nil {
			a.Log.Error("api create tagging", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
			return
		}
	}

	if _, err := qtx.CreateVote(r.Context(), store.CreateVoteParams{
		UserID:  user.ID,
		StoryID: story.ID,
	}); err != nil {
		a.Log.Error("api auto-upvote story", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	if req.Hotness > 1 {
		if err := qtx.SetStoryUpvotes(r.Context(), store.SetStoryUpvotesParams{
			ID:      story.ID,
			Upvotes: req.Hotness,
		}); err != nil {
			a.Log.Error("api set story upvotes", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
			return
		}
	}

	if !isText {
		if err := qtx.IncrementDomainStoryCount(r.Context(), domain.ID); err != nil {
			a.Log.Error("api increment domain story count", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
			return
		}
		if originID.Valid {
			if err := qtx.IncrementOriginStoryCount(r.Context(), originID.Int64); err != nil {
				a.Log.Error("api increment origin story count", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
				return
			}
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.Log.Error("api commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	a.recordIP(r, user.ID, "story")

	writeJSON(w, http.StatusOK, map[string]string{"url": storyPath(shortCode, req.Title)})
}

func (a *App) apiListNewestStories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stories, err := a.Queries.ListStories(ctx, store.ListStoriesParams{
		HideDeleted: true,
		StoryLimit:  25,
	})
	if err != nil {
		a.Log.Error("api list newest stories", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	type apiStoryTag struct {
		Tag     string `json:"tag"`
		IsMedia bool   `json:"is_media"`
	}
	type apiStory struct {
		ShortCode    string        `json:"short_code"`
		Title        string        `json:"title"`
		URL          string        `json:"url,omitempty"`
		Username     string        `json:"username"`
		Domain       string        `json:"domain,omitempty"`
		Tags         []apiStoryTag `json:"tags"`
		Upvotes      int           `json:"upvotes"`
		CommentCount int           `json:"comment_count"`
		CreatedAt    time.Time     `json:"created_at"`
	}

	result := make([]apiStory, 0, len(stories))
	for _, s := range stories {
		tagRows, err := a.Queries.GetStoryTags(ctx, s.ID)
		if err != nil {
			a.Log.Error("api get story tags", "error", err, "story_id", s.ID)
			continue
		}
		tags := make([]apiStoryTag, len(tagRows))
		for i, t := range tagRows {
			tags[i] = apiStoryTag{Tag: t.Tag, IsMedia: t.IsMedia}
		}

		domain := s.Domain.String
		if s.Origin.Valid {
			domain = s.Origin.String
		}

		result = append(result, apiStory{
			ShortCode:    s.ShortCode,
			Title:        s.Title,
			URL:          s.Url.String,
			Username:     s.Username,
			Domain:       domain,
			Tags:         tags,
			Upvotes:      int(s.Upvotes),
			CommentCount: int(s.CommentCount),
			CreatedAt:    s.CreatedAt.Time,
		})
	}

	writeJSON(w, http.StatusOK, result)
}

type apiCommentNode struct {
	ShortCode string           `json:"short_code"`
	Body      string           `json:"body"`
	Username  string           `json:"username"`
	Depth     int              `json:"depth"`
	Upvotes   int              `json:"upvotes"`
	CreatedAt time.Time        `json:"created_at"`
	Children  []apiCommentNode `json:"children"`
}

func buildAPICommentTree(rows []store.ListCommentsByStoryRow) []apiCommentNode {
	type node struct {
		data     apiCommentNode
		children []*node
	}

	nodeMap := make(map[int64]*node, len(rows))
	var roots []*node

	for _, r := range rows {
		body := r.Body
		if r.DeletedAt.Valid {
			body = "[deleted]"
		}
		n := &node{
			data: apiCommentNode{
				ShortCode: r.ShortCode,
				Body:      body,
				Username:  r.Username,
				Depth:     int(r.Depth),
				Upvotes:   int(r.Upvotes),
				CreatedAt: r.CreatedAt.Time,
			},
		}
		nodeMap[r.ID] = n

		if r.ParentID.Valid {
			if parent, ok := nodeMap[r.ParentID.Int64]; ok {
				parent.children = append(parent.children, n)
			} else {
				roots = append(roots, n)
			}
		} else {
			roots = append(roots, n)
		}
	}

	var flatten func(nodes []*node) []apiCommentNode
	flatten = func(nodes []*node) []apiCommentNode {
		result := make([]apiCommentNode, len(nodes))
		for i, n := range nodes {
			result[i] = n.data
			result[i].Children = flatten(n.children)
		}
		return result
	}

	return flatten(roots)
}

func (a *App) apiListComments(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	if len(code) != 6 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Story not found."})
		return
	}

	story, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: code, Valid: true}})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Story not found."})
			return
		}
		a.Log.Error("api get story", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	rows, err := a.Queries.ListCommentsByStory(r.Context(), story.ID)
	if err != nil {
		a.Log.Error("api list comments", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	writeJSON(w, http.StatusOK, buildAPICommentTree(rows))
}

func (a *App) apiCreateComment(w http.ResponseWriter, r *http.Request) {
	user, _, ok := a.apiKeyUserFromRequest(w, r)
	if !ok {
		return
	}

	code := r.PathValue("code")
	if len(code) != 6 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Story not found."})
		return
	}

	story, err := a.Queries.GetStory(r.Context(), store.GetStoryParams{ShortCode: pgtype.Text{String: code, Valid: true}})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Story not found."})
			return
		}
		a.Log.Error("api get story for comment", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	var req struct {
		Body            string `json:"body"`
		ParentShortCode string `json:"parent_short_code"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body."})
		return
	}

	req.Body = strings.TrimSpace(req.Body)

	type fieldErrors map[string]string
	errs := make(fieldErrors)

	if req.Body == "" {
		errs["body"] = "Comment body is required."
	} else if len(req.Body) > maxCommentLength {
		errs["body"] = "Comment body must be 10,000 characters or fewer."
	}

	var parentID pgtype.Int8
	var depth int32
	if req.ParentShortCode != "" {
		parent, err := a.Queries.GetCommentByShortCode(r.Context(), req.ParentShortCode)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				errs["parent_short_code"] = "Parent comment not found."
			} else {
				a.Log.Error("api get parent comment", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
				return
			}
		} else {
			if parent.StoryID != story.ID {
				errs["parent_short_code"] = "Parent comment does not belong to this story."
			} else if parent.Depth >= int32(maxCommentDepth) {
				errs["parent_short_code"] = "Maximum nesting depth reached."
			} else {
				parentID = pgtype.Int8{Int64: parent.ID, Valid: true}
				depth = parent.Depth + 1
			}
		}
	}

	if len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"errors": errs})
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.Log.Error("api begin comment transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	comment, err := qtx.CreateComment(r.Context(), store.CreateCommentParams{
		StoryID:   story.ID,
		UserID:    user.ID,
		ParentID:  parentID,
		Body:      req.Body,
		Depth:     depth,
		ShortCode: generateShortCode(),
	})
	if err != nil {
		a.Log.Error("api create comment", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	if err := qtx.IncrementStoryCommentCount(r.Context(), story.ID); err != nil {
		a.Log.Error("api increment comment count", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.Log.Error("api commit comment transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error."})
		return
	}

	a.recordIP(r, user.ID, "comment")

	_ = a.Queries.RecalculateStoryDownvotes(r.Context(), story.ID)

	writeJSON(w, http.StatusOK, map[string]string{
		"short_code": comment.ShortCode,
		"url":        storyPath(story.ShortCode, story.Title) + "#c_" + comment.ShortCode,
	})
}

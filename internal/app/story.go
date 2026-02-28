package app

import (
	"errors"
	"net/http"
	"time"

	"crow.watch/internal/auth"
	"crow.watch/internal/markdown"
	"crow.watch/internal/store"
	"github.com/jackc/pgx/v5"
)

func (a *App) showStory(w http.ResponseWriter, r *http.Request) {
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

	// Canonical slug redirect
	canonical := storyPath(row.ShortCode, row.Title)
	if r.URL.Path != canonical {
		http.Redirect(w, r, canonical, http.StatusMovedPermanently)
		return
	}

	tagRows, err := a.Queries.GetStoryTags(r.Context(), row.ID)
	if err != nil {
		a.serverError(w, r, "get story tags", err)
		return
	}
	var tags []StoryTag
	for _, t := range tagRows {
		tags = append(tags, StoryTag{Tag: t.Tag, IsMedia: t.IsMedia})
	}

	var hasUpvoted bool
	var hasStoryFlagged bool
	var hasStoryHidden bool
	var currentUserID int64
	current, loggedIn := auth.UserFromContext(r.Context())
	if loggedIn {
		currentUserID = current.User.ID
		votedIDs, err := a.Queries.GetUserVotes(r.Context(), store.GetUserVotesParams{
			UserID:   current.User.ID,
			StoryIds: []int64{row.ID},
		})
		if err == nil {
			for _, vid := range votedIDs {
				if vid == row.ID {
					hasUpvoted = true
				}
			}
		}
		flaggedIDs, err := a.Queries.GetUserStoryFlags(r.Context(), store.GetUserStoryFlagsParams{
			UserID:   current.User.ID,
			StoryIds: []int64{row.ID},
		})
		if err == nil {
			for _, fid := range flaggedIDs {
				if fid == row.ID {
					hasStoryFlagged = true
				}
			}
		}
		hiddenIDs, err := a.Queries.GetUserHiddenStories(r.Context(), store.GetUserHiddenStoriesParams{
			UserID:   current.User.ID,
			StoryIds: []int64{row.ID},
		})
		if err == nil {
			for _, hid := range hiddenIDs {
				if hid == row.ID {
					hasStoryHidden = true
				}
			}
		}
	}

	storyDomain := row.Domain.String
	if row.Origin.Valid {
		storyDomain = row.Origin.String
	}
	// Fetch story flag breakdown
	var flagCounts []FlagCount
	flagRows, err := a.Queries.GetStoryFlagCounts(r.Context(), row.ID)
	if err != nil {
		a.serverError(w, r, "get story flag counts", err)
		return
	}
	for _, f := range flagRows {
		flagCounts = append(flagCounts, FlagCount{Reason: f.Reason, Count: int(f.Count)})
	}

	item := StoryItem{
		ID:           row.ID,
		ShortCode:    row.ShortCode,
		URL:          row.Url.String,
		Title:        row.Title,
		Domain:       storyDomain,
		Username:     row.Username,
		Tags:         tags,
		Upvotes:      int(row.Upvotes),
		Downvotes:    int(row.Downvotes),
		CommentCount: int(row.CommentCount),
		HasUpvoted:   hasUpvoted,
		HasFlagged:   hasStoryFlagged,
		HasHidden:    hasStoryHidden,
		FlagReasons:  storyFlagReasons,
		FlagCounts:   flagCounts,
		IsText:       row.Body.Valid,
		IsLoggedIn:   loggedIn,
		CreatedAt:    row.CreatedAt.Time,
	}

	// Fetch comments
	commentRows, err := a.Queries.ListCommentsByStory(r.Context(), row.ID)
	if err != nil {
		a.serverError(w, r, "list comments", err)
		return
	}

	// Batch-fetch comment votes, flags, and flag counts
	votedMap := make(map[int64]bool)
	flaggedMap := make(map[int64]bool)
	commentFlagCountsMap := make(map[int64][]FlagCount)
	var lastVisit time.Time

	commentIDs := make([]int64, len(commentRows))
	for i, c := range commentRows {
		commentIDs[i] = c.ID
	}

	if len(commentIDs) > 0 {
		// Fetch flag counts for all comments (visible to everyone)
		if fcRows, err := a.Queries.GetCommentFlagCounts(r.Context(), commentIDs); err == nil {
			for _, fc := range fcRows {
				commentFlagCountsMap[fc.CommentID] = append(commentFlagCountsMap[fc.CommentID], FlagCount{
					Reason: fc.Reason,
					Count:  int(fc.Count),
				})
			}
		}
	}

	if loggedIn && len(commentIDs) > 0 {
		if votedIDs, err := a.Queries.GetUserCommentVotes(r.Context(), store.GetUserCommentVotesParams{
			UserID:     current.User.ID,
			CommentIds: commentIDs,
		}); err == nil {
			for _, id := range votedIDs {
				votedMap[id] = true
			}
		}

		if flaggedIDs, err := a.Queries.GetUserCommentFlags(r.Context(), store.GetUserCommentFlagsParams{
			UserID:     current.User.ID,
			CommentIds: commentIDs,
		}); err == nil {
			for _, id := range flaggedIDs {
				flaggedMap[id] = true
			}
		}

		// Get last visit time for unread detection
		if visit, err := a.Queries.GetStoryVisit(r.Context(), store.GetStoryVisitParams{
			UserID:  current.User.ID,
			StoryID: row.ID,
		}); err == nil {
			lastVisit = visit.Time
		}
	}

	comments := buildCommentTree(commentRows, buildTreeOpts{
		currentUserID:    currentUserID,
		storySubmitterID: row.UserID,
		votedMap:         votedMap,
		flaggedMap:       flaggedMap,
		flagCountsMap:    commentFlagCountsMap,
		lastVisit:        lastVisit,
		isLoggedIn:       loggedIn,
		storyCode:        row.ShortCode,
	})

	// Update story visit AFTER building the tree (so current visit doesn't affect unread status)
	if loggedIn {
		_ = a.Queries.UpsertStoryVisit(r.Context(), store.UpsertStoryVisitParams{
			UserID:  current.User.ID,
			StoryID: row.ID,
		})
	}

	a.render(w, "story", StoryPageData{
		BaseData: a.baseData(r),
		Story:    item,
		Body:     markdown.Render(row.Body.String),
		Comments: comments,
	})
}

package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

func (a *App) userStoriesPage(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	if username == "" {
		http.NotFound(w, r)
		return
	}

	// Validate user exists
	_, err := a.Queries.GetPublicProfile(r.Context(), username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get public profile", err)
		return
	}

	page := parsePage(r)
	data := UserStoriesPageData{
		BaseData:        a.baseData(r),
		ProfileUsername: username,
		CurrentPage:     page,
		PagePath:        fmt.Sprintf("/u/%s/stories/page", username),
	}

	stories, err := a.Queries.ListStoriesByUsername(r.Context(), store.ListStoriesByUsernameParams{
		Username:   username,
		StoryLimit: 500,
	})
	if err != nil {
		a.serverError(w, r, "list stories by username", err)
		return
	}

	// Build story list with tags
	type userStory struct {
		story store.ListStoriesByUsernameRow
		tags  []StoryTag
	}
	var all []userStory
	for _, s := range stories {
		tagRows, err := a.Queries.GetStoryTags(r.Context(), s.ID)
		if err != nil {
			a.Log.Error("get story tags", "error", err, "story_id", s.ID)
			continue
		}
		var tags []StoryTag
		for _, t := range tagRows {
			tags = append(tags, StoryTag{Tag: t.Tag, IsMedia: t.IsMedia})
		}
		all = append(all, userStory{story: s, tags: tags})
	}

	// Collect story IDs for batch queries
	storyIDs := make([]int64, len(all))
	for i, s := range all {
		storyIDs[i] = s.story.ID
	}

	// Batch-fetch user votes, flags, and hidden stories if logged in
	votedMap := make(map[int64]bool)
	flaggedMap := make(map[int64]bool)
	hiddenMap := make(map[int64]bool)
	if current, ok := auth.UserFromContext(r.Context()); ok && len(storyIDs) > 0 {
		votedIDs, err := a.Queries.GetUserVotes(r.Context(), store.GetUserVotesParams{
			UserID:   current.User.ID,
			StoryIds: storyIDs,
		})
		if err != nil {
			a.serverError(w, r, "get user votes", err)
			return
		}
		for _, id := range votedIDs {
			votedMap[id] = true
		}
		flaggedIDs, err := a.Queries.GetUserStoryFlags(r.Context(), store.GetUserStoryFlagsParams{
			UserID:   current.User.ID,
			StoryIds: storyIDs,
		})
		if err != nil {
			a.serverError(w, r, "get user story flags", err)
			return
		}
		for _, id := range flaggedIDs {
			flaggedMap[id] = true
		}
		hiddenIDs, err := a.Queries.GetUserHiddenStories(r.Context(), store.GetUserHiddenStoriesParams{
			UserID:   current.User.ID,
			StoryIds: storyIDs,
		})
		if err != nil {
			a.serverError(w, r, "get user hidden stories", err)
			return
		}
		for _, id := range hiddenIDs {
			hiddenMap[id] = true
		}
	}

	// Paginate
	start := (page - 1) * storiesPerPage
	if start > len(all) {
		start = len(all)
	}
	pageStories := all[start:]
	data.HasMore = len(pageStories) > storiesPerPage
	if len(pageStories) > storiesPerPage {
		pageStories = pageStories[:storiesPerPage]
	}

	isLoggedIn := data.BaseData.IsLoggedIn
	userStoriesIsMod := data.BaseData.IsModerator
	for _, item := range pageStories {
		s := item.story
		domain := s.Domain.String
		if s.Origin.Valid {
			domain = s.Origin.String
		}
		data.Stories = append(data.Stories, StoryItem{
			ID:           s.ID,
			ShortCode:    s.ShortCode,
			URL:          s.Url.String,
			Title:        s.Title,
			Domain:       domain,
			Username:     s.Username,
			Tags:         item.tags,
			Upvotes:      int(s.Upvotes),
			Downvotes:    int(s.Downvotes),
			CommentCount: int(s.CommentCount),
			HasUpvoted:   votedMap[s.ID],
			HasFlagged:   flaggedMap[s.ID],
			HasHidden:    hiddenMap[s.ID],
			FlagReasons:  storyFlagReasons,
			IsText:       s.Body.Valid,
			IsLoggedIn:   isLoggedIn,
			IsModerator:  userStoriesIsMod,
			CreatedAt:    s.CreatedAt.Time,
		})
	}

	a.render(w, "user-stories", data)
}

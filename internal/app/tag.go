package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"crow.watch/internal/auth"
	"crow.watch/internal/rank"
	"crow.watch/internal/store"
)

// tagPage serves the hotness-ranked story listing for a specific tag
// (GET /t/{tag} and GET /t/{tag}/page/{page}).
func (a *App) tagPage(w http.ResponseWriter, r *http.Request) {
	tagName := r.PathValue("tag")
	if tagName == "" {
		http.NotFound(w, r)
		return
	}

	tag, err := a.Queries.GetTagByName(r.Context(), tagName)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get tag by name", err)
		return
	}

	page := parsePage(r)
	data := TagPageData{
		BaseData:       a.baseData(r),
		TagName:        tag.Tag,
		TagDescription: tag.Description,
		CurrentPage:    page,
		PagePath:       fmt.Sprintf("/t/%s/page", tag.Tag),
	}

	stories, err := a.Queries.ListRecentStoriesByTag(r.Context(), store.ListRecentStoriesByTagParams{
		TagID:      tag.ID,
		StoryLimit: 500,
	})
	if err != nil {
		a.serverError(w, r, "list stories by tag", err)
		return
	}

	// Collect story IDs for batch queries
	storyIDs := make([]int64, len(stories))
	for i, s := range stories {
		storyIDs[i] = s.ID
	}

	// Batch-fetch comment ranking data
	commentDataMap := make(map[int64]store.GetCommentRankingDataByStoriesRow)
	if len(storyIDs) > 0 {
		commentData, err := a.Queries.GetCommentRankingDataByStories(r.Context(), storyIDs)
		if err != nil {
			a.serverError(w, r, "get comment ranking data", err)
			return
		}
		for _, cd := range commentData {
			commentDataMap[cd.StoryID] = cd
		}
	}

	// Batch-fetch user votes, flags, and hidden stories if logged in
	votedMap := make(map[int64]bool)
	flaggedMap := make(map[int64]bool)
	hiddenMap := make(map[int64]bool)
	if current, ok := auth.UserFromContext(r.Context()); ok {
		if len(storyIDs) > 0 {
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
	}

	// Build rank inputs
	inputs := make([]rank.StoryInput, 0, len(stories))
	storyMeta := make(map[int64]storyDisplayInfo, len(stories))
	for _, s := range stories {
		tagRows, err := a.Queries.GetStoryTagsWithMod(r.Context(), s.ID)
		if err != nil {
			a.Log.Error("get story tags with mod", "error", err, "story_id", s.ID)
			continue
		}
		var tags []rank.TagInput
		var displayTags []StoryTag
		for _, t := range tagRows {
			tags = append(tags, rank.TagInput{HotnessMod: t.HotnessMod})
			displayTags = append(displayTags, StoryTag{Tag: t.Tag, IsMedia: t.IsMedia})
		}
		upvotes := int(s.Upvotes)
		downvotes := int(s.Downvotes)
		inputs = append(inputs, rank.StoryInput{
			ID:         s.ID,
			CreatedAt:  s.CreatedAt.Time,
			Tags:       tags,
			StoryScore: upvotes - downvotes,
			Comments:   buildCommentInputs(commentDataMap[s.ID]),
		})
		domain := s.Domain.String
		if s.Origin.Valid {
			domain = s.Origin.String
		}
		storyMeta[s.ID] = storyDisplayInfo{
			ShortCode:    s.ShortCode,
			URL:          s.Url.String,
			Title:        s.Title,
			Domain:       domain,
			Username:     s.Username,
			Tags:         displayTags,
			Upvotes:      upvotes,
			Downvotes:    downvotes,
			CommentCount: int(s.CommentCount),
			HasUpvoted:   votedMap[s.ID],
			HasFlagged:   flaggedMap[s.ID],
			HasHidden:    hiddenMap[s.ID],
			IsText:       s.Body.Valid,
			CreatedAt:    s.CreatedAt.Time,
		}
	}

	ranked := rank.RankStories(inputs, rank.DefaultHotnessWindowSeconds)

	// Filter out negative-score stories and user-hidden stories
	var visible []rank.ScoredStory
	for _, s := range ranked {
		meta := storyMeta[s.ID]
		if meta.Upvotes-meta.Downvotes < 0 {
			continue
		}
		if hiddenMap[s.ID] {
			continue
		}
		visible = append(visible, s)
	}

	start := (page - 1) * storiesPerPage
	if start > len(visible) {
		start = len(visible)
	}
	end := start + storiesPerPage
	if end > len(visible) {
		end = len(visible)
	}
	data.HasMore = end < len(visible)

	isLoggedIn := data.BaseData.IsLoggedIn
	tagIsModerator := data.BaseData.IsModerator
	for _, s := range visible[start:end] {
		meta := storyMeta[s.ID]
		data.Stories = append(data.Stories, StoryItem{
			ID:           s.ID,
			ShortCode:    meta.ShortCode,
			URL:          meta.URL,
			Title:        meta.Title,
			Domain:       meta.Domain,
			Username:     meta.Username,
			Tags:         meta.Tags,
			Upvotes:      meta.Upvotes,
			Downvotes:    meta.Downvotes,
			CommentCount: meta.CommentCount,
			HasUpvoted:   meta.HasUpvoted,
			HasFlagged:   meta.HasFlagged,
			HasHidden:    meta.HasHidden,
			FlagReasons:  storyFlagReasons,
			IsText:       meta.IsText,
			IsLoggedIn:   isLoggedIn,
			IsModerator:  tagIsModerator,
			CreatedAt:    meta.CreatedAt,
		})
	}

	a.render(w, "tag", data)
}

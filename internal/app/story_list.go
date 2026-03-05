package app

import (
	"net/http"
	"time"

	"crow.watch/internal/auth"
	"crow.watch/internal/rank"
	"crow.watch/internal/store"
)

type storyListOpts struct {
	rankByHotness  bool
	filterNegScore bool
	filterHidden   bool
}

type storyDisplayInfo struct {
	ShortCode    string
	URL          string
	Title        string
	Domain       string
	Username     string
	Tags         []StoryTag
	Upvotes      int
	Downvotes    int
	CommentCount int
	HasUpvoted   bool
	HasFlagged   bool
	HasHidden    bool
	IsText       bool
	CreatedAt    time.Time
	DeletedAt    *time.Time
}

// loadStoryList fetches stories, applies ranking/filtering/pagination,
// and returns the final StoryItem slice and whether more pages exist.
func (a *App) loadStoryList(r *http.Request, base BaseData, page int, params store.ListStoriesParams, opts storyListOpts) ([]StoryItem, bool, error) {
	ctx := r.Context()

	stories, err := a.Queries.ListStories(ctx, params)
	if err != nil {
		return nil, false, err
	}

	// Collect story IDs for batch queries
	storyIDs := make([]int64, len(stories))
	for i, s := range stories {
		storyIDs[i] = s.ID
	}

	// Batch-fetch user votes, flags, and hidden stories if logged in
	votedMap := make(map[int64]bool)
	flaggedMap := make(map[int64]bool)
	hiddenMap := make(map[int64]bool)
	if current, ok := auth.UserFromContext(ctx); ok && len(storyIDs) > 0 {
		votedIDs, err := a.Queries.GetUserVotes(ctx, store.GetUserVotesParams{
			UserID:   current.User.ID,
			StoryIds: storyIDs,
		})
		if err != nil {
			return nil, false, err
		}
		for _, id := range votedIDs {
			votedMap[id] = true
		}
		flaggedIDs, err := a.Queries.GetUserStoryFlags(ctx, store.GetUserStoryFlagsParams{
			UserID:   current.User.ID,
			StoryIds: storyIDs,
		})
		if err != nil {
			return nil, false, err
		}
		for _, id := range flaggedIDs {
			flaggedMap[id] = true
		}
		hiddenIDs, err := a.Queries.GetUserHiddenStories(ctx, store.GetUserHiddenStoriesParams{
			UserID:   current.User.ID,
			StoryIds: storyIDs,
		})
		if err != nil {
			return nil, false, err
		}
		for _, id := range hiddenIDs {
			hiddenMap[id] = true
		}
	}

	// Fetch tags for each story, build display info and optional rank inputs
	var rankInputs []rank.StoryInput
	if opts.rankByHotness {
		rankInputs = make([]rank.StoryInput, 0, len(stories))
	}
	meta := make(map[int64]storyDisplayInfo, len(stories))
	// Preserve chronological order for non-ranked listings
	var orderedIDs []int64

	for _, s := range stories {
		tagRows, err := a.Queries.GetStoryTags(ctx, s.ID)
		if err != nil {
			a.Log.Error("get story tags", "error", err, "story_id", s.ID)
			continue
		}
		var displayTags []StoryTag
		var rankTags []rank.TagInput
		for _, t := range tagRows {
			displayTags = append(displayTags, StoryTag{Tag: t.Tag, IsMedia: t.IsMedia})
			if opts.rankByHotness {
				rankTags = append(rankTags, rank.TagInput{HotnessMod: t.HotnessMod})
			}
		}

		upvotes := int(s.Upvotes)
		downvotes := int(s.Downvotes)

		if opts.rankByHotness {
			rankInputs = append(rankInputs, rank.StoryInput{
				ID:            s.ID,
				CreatedAt:     s.CreatedAt.Time,
				Tags:          rankTags,
				StoryScore:    upvotes - downvotes,
				CommentsCount: int(s.CommentCount),
			})
		}

		domain := s.Domain.String
		if s.Origin.Valid {
			domain = s.Origin.String
		}
		var deletedAt *time.Time
		if s.DeletedAt.Valid {
			t := s.DeletedAt.Time
			deletedAt = &t
		}

		meta[s.ID] = storyDisplayInfo{
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
			DeletedAt:    deletedAt,
		}
		orderedIDs = append(orderedIDs, s.ID)
	}

	// Determine final ordering
	if opts.rankByHotness {
		ranked := rank.SortStories(rankInputs, rank.DefaultHotnessWindowSeconds)
		orderedIDs = orderedIDs[:0]
		for _, s := range ranked {
			orderedIDs = append(orderedIDs, s.ID)
		}
	}

	// Filter
	var visible []int64
	for _, id := range orderedIDs {
		m := meta[id]
		if opts.filterNegScore && m.Upvotes-m.Downvotes < 0 {
			continue
		}
		if opts.filterHidden && hiddenMap[id] {
			continue
		}
		visible = append(visible, id)
	}

	// Paginate
	start := (page - 1) * storiesPerPage
	if start > len(visible) {
		start = len(visible)
	}
	end := start + storiesPerPage
	if end > len(visible) {
		end = len(visible)
	}
	hasMore := end < len(visible)

	// Build StoryItems
	items := make([]StoryItem, 0, end-start)
	for _, id := range visible[start:end] {
		m := meta[id]
		title := m.Title
		url := m.URL
		domain := m.Domain
		if m.DeletedAt != nil {
			title = "[deleted by moderator]"
			url = ""
			domain = ""
		}
		items = append(items, StoryItem{
			ID:           id,
			ShortCode:    m.ShortCode,
			URL:          url,
			Title:        title,
			Domain:       domain,
			Username:     m.Username,
			Tags:         m.Tags,
			Upvotes:      m.Upvotes,
			Downvotes:    m.Downvotes,
			CommentCount: m.CommentCount,
			HasUpvoted:   m.HasUpvoted,
			HasFlagged:   m.HasFlagged,
			HasHidden:    m.HasHidden,
			FlagReasons:  storyFlagReasons,
			IsText:       m.IsText,
			IsLoggedIn:   base.IsLoggedIn,
			IsModerator:  base.IsModerator,
			CreatedAt:    m.CreatedAt,
			DeletedAt:    m.DeletedAt,
		})
	}

	return items, hasMore, nil
}

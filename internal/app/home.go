package app

import (
	"net/http"
	"strconv"
	"time"

	"crow.watch/internal/auth"
	"crow.watch/internal/rank"
	"crow.watch/internal/store"
)

const storiesPerPage = 25

// home serves the hotness-ranked story listing (GET / and GET /page/{page}).
func (a *App) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		a.notFound(w, r)
		return
	}
	page := parsePage(r)
	data := HomePageData{
		BaseData:    a.baseData(r),
		CurrentPage: page,
		PagePath:    "/page",
	}

	// Fetch hidden tag IDs if logged in
	var hiddenTagIDs []int64
	if current, ok := auth.UserFromContext(r.Context()); ok {
		var err error
		hiddenTagIDs, err = a.Queries.ListUserHiddenTagIDs(r.Context(), current.User.ID)
		if err != nil {
			a.serverError(w, r, "get hidden tags", err)
			return
		}
	}

	stories, err := a.Queries.ListRecentStories(r.Context(), store.ListRecentStoriesParams{
		HiddenTagIds: hiddenTagIDs,
		StoryLimit:   500,
	})
	if err != nil {
		a.serverError(w, r, "list stories for ranking", err)
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
			CreatedAt:    meta.CreatedAt,
		})
	}

	a.render(w, "home", data)
}

// newest serves the chronological story listing (GET /newest and GET /newest/page/{page}).
func (a *App) newest(w http.ResponseWriter, r *http.Request) {
	page := parsePage(r)
	data := HomePageData{
		BaseData:    a.baseData(r),
		CurrentPage: page,
		PagePath:    "/newest/page",
	}

	// Fetch hidden tag IDs if logged in
	var newestHiddenTagIDs []int64
	if current, ok := auth.UserFromContext(r.Context()); ok {
		var err error
		newestHiddenTagIDs, err = a.Queries.ListUserHiddenTagIDs(r.Context(), current.User.ID)
		if err != nil {
			a.serverError(w, r, "get hidden tags", err)
			return
		}
	}

	stories, err := a.Queries.ListRecentStories(r.Context(), store.ListRecentStoriesParams{
		HiddenTagIds: newestHiddenTagIDs,
		StoryLimit:   500,
	})
	if err != nil {
		a.serverError(w, r, "list recent stories", err)
		return
	}

	// Build story list with tags, collect IDs for batch queries
	type newestStory struct {
		story store.ListRecentStoriesRow
		tags  []StoryTag
	}
	var allNewest []newestStory
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
		allNewest = append(allNewest, newestStory{story: s, tags: tags})
	}

	// Collect all story IDs for batch queries
	allNewestIDs := make([]int64, len(allNewest))
	for i, s := range allNewest {
		allNewestIDs[i] = s.story.ID
	}

	// Batch-fetch user votes, flags, and hidden stories if logged in
	newestVotedMap := make(map[int64]bool)
	newestFlaggedMap := make(map[int64]bool)
	newestHiddenMap := make(map[int64]bool)
	if current, ok := auth.UserFromContext(r.Context()); ok && len(allNewestIDs) > 0 {
		votedIDs, err := a.Queries.GetUserVotes(r.Context(), store.GetUserVotesParams{
			UserID:   current.User.ID,
			StoryIds: allNewestIDs,
		})
		if err != nil {
			a.serverError(w, r, "get user votes", err)
			return
		}
		for _, id := range votedIDs {
			newestVotedMap[id] = true
		}
		flaggedIDs, err := a.Queries.GetUserStoryFlags(r.Context(), store.GetUserStoryFlagsParams{
			UserID:   current.User.ID,
			StoryIds: allNewestIDs,
		})
		if err != nil {
			a.serverError(w, r, "get user story flags", err)
			return
		}
		for _, id := range flaggedIDs {
			newestFlaggedMap[id] = true
		}
		hiddenIDs, err := a.Queries.GetUserHiddenStories(r.Context(), store.GetUserHiddenStoriesParams{
			UserID:   current.User.ID,
			StoryIds: allNewestIDs,
		})
		if err != nil {
			a.serverError(w, r, "get user hidden stories", err)
			return
		}
		for _, id := range hiddenIDs {
			newestHiddenMap[id] = true
		}
	}

	// Filter out user-hidden stories (but do NOT filter negative score on newest)
	var filtered []newestStory
	for _, s := range allNewest {
		if newestHiddenMap[s.story.ID] {
			continue
		}
		filtered = append(filtered, s)
	}

	// Paginate the filtered list
	start := (page - 1) * storiesPerPage
	if start > len(filtered) {
		start = len(filtered)
	}
	pageStories := filtered[start:]
	data.HasMore = len(pageStories) > storiesPerPage
	if len(pageStories) > storiesPerPage {
		pageStories = pageStories[:storiesPerPage]
	}

	newestLoggedIn := data.BaseData.IsLoggedIn
	for _, item := range pageStories {
		s := item.story
		newestDomain := s.Domain.String
		if s.Origin.Valid {
			newestDomain = s.Origin.String
		}
		data.Stories = append(data.Stories, StoryItem{
			ID:           s.ID,
			ShortCode:    s.ShortCode,
			URL:          s.Url.String,
			Title:        s.Title,
			Domain:       newestDomain,
			Username:     s.Username,
			Tags:         item.tags,
			Upvotes:      int(s.Upvotes),
			Downvotes:    int(s.Downvotes),
			CommentCount: int(s.CommentCount),
			HasUpvoted:   newestVotedMap[s.ID],
			HasFlagged:   newestFlaggedMap[s.ID],
			HasHidden:    newestHiddenMap[s.ID],
			FlagReasons:  storyFlagReasons,
			IsText:       s.Body.Valid,
			IsLoggedIn:   newestLoggedIn,
			CreatedAt:    s.CreatedAt.Time,
		})
	}

	a.render(w, "home", data)
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
}

func buildCommentInputs(cd store.GetCommentRankingDataByStoriesRow) []rank.CommentInput {
	if cd.Total == 0 {
		return nil
	}
	comments := make([]rank.CommentInput, 0, cd.Total)
	for range cd.BySubmitter {
		comments = append(comments, rank.CommentInput{IsSubmitter: true})
	}
	for range cd.Total - cd.BySubmitter {
		comments = append(comments, rank.CommentInput{})
	}
	return comments
}

func parsePage(r *http.Request) int {
	pageStr := r.PathValue("page")
	if pageStr == "" {
		return 1
	}
	p, err := strconv.Atoi(pageStr)
	if err != nil || p < 1 {
		return 1
	}
	return p
}

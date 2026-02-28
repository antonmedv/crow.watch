package app

import (
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/auth"
	"crow.watch/internal/markdown"
	"crow.watch/internal/rank"
	"crow.watch/internal/store"
)

const (
	maxCommentDepth   = 10
	editWindowMinutes = 5
	maxCommentLength  = 10000
)

var flagReasons = []string{"off-topic", "troll", "unkind", "spam"}

type CommentNode struct {
	ID          int64
	StoryID     int64
	UserID      int64
	ParentID    int64
	Username    string
	Body        template.HTML
	RawBody     string
	Depth       int
	Upvotes     int
	Downvotes   int
	HasUpvoted  bool
	HasFlagged  bool
	IsAuthor    bool
	IsSubmitter bool
	CanEdit     bool
	IsDeleted   bool
	IsUnread    bool
	IsLoggedIn  bool
	IsMaxDepth  bool
	CreatedAt   time.Time
	Children    []*CommentNode
	FlagReasons []string
	FlagCounts  []FlagCount
	StoryCode   string
}

type buildTreeOpts struct {
	currentUserID    int64
	storySubmitterID int64
	votedMap         map[int64]bool
	flaggedMap       map[int64]bool
	flagCountsMap    map[int64][]FlagCount
	lastVisit        time.Time
	isLoggedIn       bool
	storyCode        string
}

func buildCommentTree(rows []store.ListCommentsByStoryRow, opts buildTreeOpts) []*CommentNode {
	nodeMap := make(map[int64]*CommentNode, len(rows))
	var roots []*CommentNode

	// First pass: build all nodes
	for _, r := range rows {
		isDeleted := r.DeletedAt.Valid
		var body template.HTML
		var rawBody string
		if isDeleted {
			body = "<em>[deleted]</em>"
		} else {
			body = markdown.Render(r.Body)
			rawBody = r.Body
		}

		canEdit := !isDeleted &&
			opts.isLoggedIn &&
			r.UserID == opts.currentUserID &&
			time.Since(r.CreatedAt.Time) < editWindowMinutes*time.Minute

		isUnread := opts.isLoggedIn &&
			!opts.lastVisit.IsZero() &&
			r.CreatedAt.Time.After(opts.lastVisit) &&
			r.UserID != opts.currentUserID

		// If lastVisit is zero (first visit) and logged in, nothing is unread
		if opts.lastVisit.IsZero() {
			isUnread = false
		}

		node := &CommentNode{
			ID:          r.ID,
			StoryID:     r.StoryID,
			UserID:      r.UserID,
			Username:    r.Username,
			Body:        body,
			RawBody:     rawBody,
			Depth:       int(r.Depth),
			Upvotes:     int(r.Upvotes),
			Downvotes:   int(r.Downvotes),
			HasUpvoted:  opts.votedMap[r.ID],
			HasFlagged:  opts.flaggedMap[r.ID],
			IsAuthor:    opts.isLoggedIn && r.UserID == opts.currentUserID,
			IsSubmitter: r.UserID == opts.storySubmitterID,
			CanEdit:     canEdit,
			IsDeleted:   isDeleted,
			IsUnread:    isUnread,
			IsLoggedIn:  opts.isLoggedIn,
			IsMaxDepth:  int(r.Depth) >= maxCommentDepth,
			CreatedAt:   r.CreatedAt.Time,
			FlagReasons: flagReasons,
			FlagCounts:  opts.flagCountsMap[r.ID],
			StoryCode:   opts.storyCode,
		}
		if r.ParentID.Valid {
			node.ParentID = r.ParentID.Int64
		}
		nodeMap[r.ID] = node
	}

	// Second pass: link children to parents
	for _, r := range rows {
		node := nodeMap[r.ID]
		if r.ParentID.Valid {
			if parent, ok := nodeMap[r.ParentID.Int64]; ok {
				parent.Children = append(parent.Children, node)
			} else {
				roots = append(roots, node)
			}
		} else {
			roots = append(roots, node)
		}
	}

	// Third pass: sort siblings by Wilson score descending, created_at ASC tiebreak
	sortByWilson := func(nodes []*CommentNode) {
		sort.SliceStable(nodes, func(i, j int) bool {
			si := rank.WilsonScore(nodes[i].Upvotes, nodes[i].Downvotes)
			sj := rank.WilsonScore(nodes[j].Upvotes, nodes[j].Downvotes)
			if si != sj {
				return si > sj
			}
			return nodes[i].CreatedAt.Before(nodes[j].CreatedAt)
		})
	}
	sortByWilson(roots)
	for _, node := range nodeMap {
		if len(node.Children) > 1 {
			sortByWilson(node.Children)
		}
	}

	return roots
}

func (a *App) createComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	code := r.PathValue("code")
	if len(code) != 6 {
		http.NotFound(w, r)
		return
	}

	story, err := a.Queries.GetStoryByShortCode(r.Context(), code)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	parentIDStr := r.FormValue("parent_id")

	if body == "" || len(body) > maxCommentLength {
		http.Redirect(w, r, storyPath(story.ShortCode, story.Title), http.StatusSeeOther)
		return
	}

	var parentID pgtype.Int8
	var depth int32
	if parentIDStr != "" {
		pid, err := strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		parentDepth, err := a.Queries.GetCommentDepth(r.Context(), pid)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if parentDepth >= int32(maxCommentDepth) {
			http.Error(w, "max nesting depth reached", http.StatusBadRequest)
			return
		}
		parentID = pgtype.Int8{Int64: pid, Valid: true}
		depth = parentDepth + 1
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	comment, err := qtx.CreateComment(r.Context(), store.CreateCommentParams{
		StoryID:  story.ID,
		UserID:   current.User.ID,
		ParentID: parentID,
		Body:     body,
		Depth:    depth,
	})
	if err != nil {
		a.serverError(w, r, "create comment", err)
		return
	}

	if err := qtx.IncrementStoryCommentCount(r.Context(), story.ID); err != nil {
		a.serverError(w, r, "increment comment count", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	// Recalculate downvotes: this user's comment may neutralize a hide+flag penalty
	_ = a.Queries.RecalculateStoryDownvotes(r.Context(), story.ID)

	http.Redirect(w, r, storyPath(story.ShortCode, story.Title)+"#comment-"+strconv.FormatInt(comment.ID, 10), http.StatusSeeOther)
}

func (a *App) editComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	comment, err := a.Queries.GetCommentByID(r.Context(), commentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if comment.UserID != current.User.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if time.Since(comment.CreatedAt.Time) >= editWindowMinutes*time.Minute {
		http.Error(w, "edit window has passed", http.StatusForbidden)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" || len(body) > maxCommentLength {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if err := a.Queries.UpdateCommentBody(r.Context(), store.UpdateCommentBodyParams{
		Body: body,
		ID:   commentID,
	}); err != nil {
		a.serverError(w, r, "update comment body", err)
		return
	}

	story, err := a.Queries.GetStoryByID(r.Context(), comment.StoryID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, storyPath(story.ShortCode, story.Title)+"#comment-"+strconv.FormatInt(commentID, 10), http.StatusSeeOther)
}

func (a *App) deleteComment(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	commentID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	comment, err := a.Queries.GetCommentByID(r.Context(), commentID)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if comment.UserID != current.User.ID {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if comment.DeletedAt.Valid {
		http.Error(w, "already deleted", http.StatusBadRequest)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	if err := qtx.SoftDeleteComment(r.Context(), commentID); err != nil {
		a.serverError(w, r, "soft delete comment", err)
		return
	}

	if err := qtx.DecrementStoryCommentCount(r.Context(), comment.StoryID); err != nil {
		a.serverError(w, r, "decrement comment count", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit transaction", err)
		return
	}

	// Recalculate downvotes: deleting a comment may restore a hide+flag penalty
	_ = a.Queries.RecalculateStoryDownvotes(r.Context(), comment.StoryID)

	story, err := a.Queries.GetStoryByID(r.Context(), comment.StoryID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, storyPath(story.ShortCode, story.Title), http.StatusSeeOther)
}

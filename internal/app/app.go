package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"math/rand"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"crow.watch/internal/auth"
	"crow.watch/internal/captcha"
	"crow.watch/internal/email"
	"crow.watch/internal/ratelimit"
	"crow.watch/internal/store"
)

type App struct {
	Pool             *pgxpool.Pool
	Queries          *store.Queries
	Sessions         *auth.SessionManager
	Templates        map[string]*template.Template
	EmailTemplates   map[string]*template.Template
	EmailSender      *email.Sender
	AppURL           string
	StaticFS         fs.FS
	Log              *slog.Logger
	DevMode          bool
	TemplateFS       fs.FS
	DevReload        http.Handler
	LoginIPLimiter   *ratelimit.Limiter
	LoginAcctLimiter *ratelimit.Limiter
	InviteLimiter    *ratelimit.Limiter
	Captcha          *captcha.Store
}

type BaseData struct {
	IsLoggedIn     bool
	IsModerator    bool
	EmailConfirmed bool
	Username       string
	Slogan         string
	DevMode        bool
	UnreadReplies  int64
}

type HomePageData struct {
	BaseData
	Stories     []StoryItem
	CurrentPage int
	HasMore     bool
	PagePath    string // "/page" or "/newest/page" for building pagination links
}

type StoryItem struct {
	ID           int64
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
	FlagReasons  []string
	FlagCounts   []FlagCount
	IsText       bool
	IsLoggedIn   bool
	IsModerator  bool
	CreatedAt    time.Time
}

type StoryTag struct {
	Tag     string
	IsMedia bool
}

type TagPageData struct {
	BaseData
	TagName        string
	TagDescription string
	Stories        []StoryItem
	CurrentPage    int
	HasMore        bool
	PagePath       string // "/t/{tag}/page"
}

type LoginPageData struct {
	BaseData
	Tab        string
	Identifier string
	Error      string
}

type SubmitPageData struct {
	BaseData
	Tab          string
	URL          string
	Title        string
	Body         string
	TagGroups    []TagGroup
	Selected     []int64
	Errors       map[string]string
	Error        string
	DuplicateURL string
	EditMode     bool
	EditCode     string
	Reason       string
}

type TagGroup struct {
	Category string
	Tags     []TagOption
}

type FlagCount struct {
	Reason string
	Count  int
}

type StoryPageData struct {
	BaseData
	Story    StoryItem
	Body     template.HTML
	Comments []*CommentNode
}

type TagOption struct {
	ID          int64
	Tag         string
	Description string
	IsMedia     bool
	Privileged  bool
}

type TagsPageData struct {
	BaseData
	TagGroups    []TagGroup
	HiddenTagIDs []int64
}

type ForgotPasswordPageData struct {
	BaseData
	Email   string
	Error   string
	Success string
}

type ResetPasswordPageData struct {
	BaseData
	Token string
	Error string
}

type AccountPageData struct {
	BaseData
	Tab              string
	Email            string
	About            string
	Website          string
	EmailConfirmed   bool
	UnconfirmedEmail string
	Errors           map[string]string
	Success          string
}

type ConfirmEmailPageData struct {
	BaseData
	Error   string
	Success string
}

type ProfilePageData struct {
	BaseData
	Username    string
	About       string
	Website     string
	IsModerator bool
	StoryCount  int64
	InvitedBy   string
	CreatedAt   time.Time
}

type UserStoriesPageData struct {
	BaseData
	ProfileUsername string
	Stories         []StoryItem
	CurrentPage     int
	HasMore         bool
	PagePath        string
}

type InvitePageData struct {
	BaseData
	Tab         string
	Email       string
	Error       string
	Success     string
	InviteURL   string
	Invitations []InviteRow
}

type InviteRow struct {
	Email              string
	RegisteredUsername string
	Status             string
	CreatedAt          time.Time
}

type RegisterPageData struct {
	BaseData
	FormAction     string
	InviterName    string
	WelcomeMessage string
	Email          string
	Username       string
	Errors         map[string]string
	CaptchaID      string
}

type CampaignsPageData struct {
	BaseData
	Campaigns       []CampaignRow
	Slug            string
	WelcomeMessage  string
	SponsorUsername string
	Error           string
}

type ModerationLogPageData struct {
	BaseData
	Entries     []ModerationLogEntry
	CurrentPage int
	HasMore     bool
}

type ModerationLogEntry struct {
	ID                int64
	ModeratorUsername string
	Action            string
	ActionDescription string
	TargetType        string
	TargetID          int64
	TargetLink        string
	TargetTitle       string
	Reason            string
	CreatedAt         time.Time
}

type CampaignRow struct {
	ID              int64
	Slug            string
	SponsorName     string
	CreatedByName   string
	Active          bool
	RegisteredCount int64
	CreatedAt       time.Time
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	staticHandler := http.FileServerFS(a.StaticFS)
	if a.DevMode {
		staticHandler = noCache(staticHandler)
	} else {
		staticHandler = longCache(staticHandler)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler))
	mux.Handle("GET /favicon.png", http.FileServerFS(a.StaticFS))
	mux.HandleFunc("GET /", a.home)
	mux.HandleFunc("GET /page/{page}", a.home)
	mux.HandleFunc("GET /newest", a.newest)
	mux.HandleFunc("GET /newest/page/{page}", a.newest)
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.logout)
	mux.HandleFunc("GET /submit", a.submitPage)
	mux.HandleFunc("POST /submit", a.submitStory)
	mux.HandleFunc("POST /submit/fetch-title", a.fetchTitle)
	mux.HandleFunc("GET /x/{code}/{slug...}", a.showStory)
	mux.HandleFunc("GET /forgot-password", a.forgotPasswordPage)
	mux.HandleFunc("POST /forgot-password", a.forgotPassword)
	mux.HandleFunc("GET /reset-password", a.resetPasswordPage)
	mux.HandleFunc("POST /reset-password", a.resetPassword)
	mux.HandleFunc("GET /about", a.aboutPage)
	mux.HandleFunc("GET /confirm-email", a.confirmEmail)
	mux.HandleFunc("GET /account", a.accountPage)
	mux.HandleFunc("POST /account/email", a.updateEmail)
	mux.HandleFunc("POST /account/password", a.updatePassword)
	mux.HandleFunc("POST /account/resend-confirmation", a.resendConfirmation)
	mux.HandleFunc("GET /u/{username}", a.profilePage)
	mux.HandleFunc("GET /u/{username}/stories", a.userStoriesPage)
	mux.HandleFunc("GET /u/{username}/stories/page/{page}", a.userStoriesPage)
	mux.HandleFunc("POST /account/profile", a.updateProfile)
	mux.HandleFunc("GET /tags", a.tagsPage)
	mux.HandleFunc("GET /t/{tag}", a.tagPage)
	mux.HandleFunc("GET /t/{tag}/page/{page}", a.tagPage)
	mux.HandleFunc("POST /stories/{id}/upvote", a.upvote)
	mux.HandleFunc("POST /stories/{id}/unvote", a.unvote)
	mux.HandleFunc("POST /stories/{id}/flag", a.flagStory)
	mux.HandleFunc("POST /stories/{id}/unflag", a.unflagStory)
	mux.HandleFunc("POST /stories/{id}/hide", a.hideStory)
	mux.HandleFunc("POST /stories/{id}/unhide", a.unhideStory)
	mux.HandleFunc("POST /tags/{id}/hide", a.hideTag)
	mux.HandleFunc("POST /tags/{id}/unhide", a.unhideTag)
	mux.HandleFunc("POST /x/{code}/comments", a.createComment)
	mux.HandleFunc("POST /comments/{id}/edit", a.editComment)
	mux.HandleFunc("POST /comments/{id}/delete", a.deleteComment)
	mux.HandleFunc("POST /comments/{id}/upvote", a.upvoteComment)
	mux.HandleFunc("POST /comments/{id}/unvote", a.unvoteComment)
	mux.HandleFunc("POST /comments/{id}/flag", a.flagComment)
	mux.HandleFunc("POST /comments/{id}/unflag", a.unflagComment)
	mux.HandleFunc("GET /replies", a.repliesPage)
	mux.HandleFunc("GET /invite", a.invitePage)
	mux.HandleFunc("POST /invite/email", a.inviteByEmail)
	mux.HandleFunc("POST /invite/link", a.inviteByLink)
	mux.HandleFunc("GET /register/{token}", a.registerPage)
	mux.HandleFunc("POST /register/{token}", a.register)
	mux.HandleFunc("GET /mod/campaigns", a.campaignsPage)
	mux.HandleFunc("POST /mod/campaigns", a.createCampaign)
	mux.HandleFunc("POST /mod/campaigns/{id}/toggle", a.toggleCampaign)
	mux.HandleFunc("GET /captcha/{id}", a.serveCaptchaImage)
	mux.HandleFunc("GET /join/{slug}", a.joinPage)
	mux.HandleFunc("POST /join/{slug}", a.joinRegister)
	mux.HandleFunc("GET /x/{code}/edit", a.editStoryPage)
	mux.HandleFunc("POST /x/{code}/edit", a.editStory)
	mux.HandleFunc("GET /mod/log", a.moderationLogPage)
	mux.HandleFunc("GET /mod/log/page/{page}", a.moderationLogPage)

	if a.DevReload != nil {
		mux.Handle("GET /__dev/reload", a.DevReload)
	}

	return a.securityHeaders(a.requestLog(a.Sessions.AuthenticateRequest(mux)))
}

func (a *App) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src 'self' https://fonts.gstatic.com; img-src 'self' https:; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		next.ServeHTTP(w, r)
	})
}

func longCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the underlying ResponseWriter.
func (sr *statusRecorder) Unwrap() http.ResponseWriter {
	return sr.ResponseWriter
}

func (a *App) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sr, r)
		a.Log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", sr.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

func (a *App) serverError(w http.ResponseWriter, r *http.Request, msg string, err error) {
	a.Log.Error(msg, "error", err, "method", r.Method, "path", r.URL.Path)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	a.render(w, "not_found", a.baseData(r))
}

var slogans = []string{
	"as smart as a crow",
	"collecting shiny things",
	"clever by nature",
	"collecting shiny things",
}

func (a *App) baseData(r *http.Request) BaseData {
	if current, ok := auth.UserFromContext(r.Context()); ok {
		slogan := slogans[rand.Intn(len(slogans))]
		var unread int64
		if count, err := a.Queries.CountUnreadReplies(r.Context(), current.User.ID); err == nil {
			unread = count
		}
		return BaseData{
			IsLoggedIn:     true,
			IsModerator:    current.User.IsModerator,
			EmailConfirmed: current.User.EmailConfirmedAt.Valid,
			Username:       current.User.Username,
			Slogan:         slogan,
			DevMode:        a.DevMode,
			UnreadReplies:  unread,
		}
	}
	return BaseData{DevMode: a.DevMode}
}

func (a *App) render(w http.ResponseWriter, name string, data any) {
	var tmpl *template.Template

	if a.DevMode && a.TemplateFS != nil {
		templates, err := ParseTemplates(a.TemplateFS, nil, true)
		if err != nil {
			a.Log.Error("dev template parse", "error", err)
			http.Error(w, "template parse error", http.StatusInternalServerError)
			return
		}
		var ok bool
		tmpl, ok = templates[name]
		if !ok {
			a.Log.Error("template not found", "template", name)
			http.Error(w, "template not found", http.StatusInternalServerError)
			return
		}
	} else {
		var ok bool
		tmpl, ok = a.Templates[name]
		if !ok {
			a.Log.Error("template not found", "template", name)
			http.Error(w, "template not found", http.StatusInternalServerError)
			return
		}
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "base", data); err != nil {
		a.Log.Error("template execute", "error", err, "template", name)
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

func ParseTemplates(fsys fs.FS, staticHashes map[string]string, devMode bool) (map[string]*template.Template, error) {
	funcMap := template.FuncMap{
		"storyPath": func(s StoryItem) string {
			return storyPath(s.ShortCode, s.Title)
		},
		"static": func(path string) string {
			if devMode {
				return "/static/" + path + "?_dev=" + strconv.FormatInt(time.Now().UnixMilli(), 10)
			}
			if q, ok := staticHashes[path]; ok {
				return "/static/" + path + q
			}
			return "/static/" + path
		},
		"inSlice": func(needle int64, haystack []int64) bool {
			for _, v := range haystack {
				if v == needle {
					return true
				}
			}
			return false
		},
		"classes": func(parts ...string) string {
			var out []string
			for _, p := range parts {
				if s := strings.TrimSpace(p); s != "" {
					out = append(out, s)
				}
			}
			return strings.Join(out, " ")
		},
		"when": func(cond bool, val string) string {
			if cond {
				return val
			}
			return ""
		},
		"cond": func(cond bool, yes, no any) any {
			if cond {
				return yes
			}
			return no
		},
		"add":      func(a, b int) int { return a + b },
		"subtract": func(a, b int) int { return a - b },
		"multiply": func(a, b int) int { return a * b },
		"pluralize": func(count int, singular, plural string) string {
			if count == 1 {
				return singular
			}
			return plural
		},
		"timeAgo": func(t time.Time) string {
			d := time.Since(t)
			switch {
			case d < time.Minute:
				return "just now"
			case d < 2*time.Minute:
				return "1 minute ago"
			case d < time.Hour:
				return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
			case d < 2*time.Hour:
				return "1 hour ago"
			case d < 24*time.Hour:
				return fmt.Sprintf("%d hours ago", int(d.Hours()))
			case d < 48*time.Hour:
				return "1 day ago"
			default:
				return fmt.Sprintf("%d days ago", int(d.Hours()/24))
			}
		},
	}

	base, err := template.New("").Funcs(funcMap).ParseFS(fsys, "templates/base.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse base template: %w", err)
	}

	partials, _ := fs.Glob(fsys, "templates/partials/*.tmpl")
	if len(partials) > 0 {
		if _, err := base.ParseFS(fsys, partials...); err != nil {
			return nil, fmt.Errorf("parse partials: %w", err)
		}
	}

	pages, err := fs.Glob(fsys, "templates/pages/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("glob page templates: %w", err)
	}

	templates := make(map[string]*template.Template, len(pages))
	for _, page := range pages {
		name := strings.TrimSuffix(filepath.Base(page), ".tmpl")
		clone, err := base.Clone()
		if err != nil {
			return nil, fmt.Errorf("clone base for %s: %w", name, err)
		}
		if _, err := clone.ParseFS(fsys, page); err != nil {
			return nil, fmt.Errorf("parse page %s: %w", name, err)
		}
		templates[name] = clone
	}

	return templates, nil
}

func HashStatic(fsys fs.FS) (map[string]string, error) {
	hashes := make(map[string]string)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		f, err := fsys.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return err
		}
		hashes[path] = "?v=" + hex.EncodeToString(h.Sum(nil))[:8]
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("hash static files: %w", err)
	}
	return hashes, nil
}

func ParseEmailTemplates(fsys fs.FS) (map[string]*template.Template, error) {
	files, err := fs.Glob(fsys, "templates/email/*.html")
	if err != nil {
		return nil, fmt.Errorf("glob email templates: %w", err)
	}

	templates := make(map[string]*template.Template, len(files))
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), ".html")
		tmpl, err := template.ParseFS(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("parse email template %s: %w", name, err)
		}
		templates[name] = tmpl
	}

	return templates, nil
}

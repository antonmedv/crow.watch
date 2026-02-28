package app

import (
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"testing/fstest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"crow.watch/internal/auth"
	"crow.watch/internal/markdown"
	"crow.watch/web"
)

func mustParseTemplates(t *testing.T) map[string]*template.Template {
	t.Helper()
	staticFS, err := fs.Sub(web.FS, "static")
	require.NoError(t, err)
	hashes, err := HashStatic(staticFS)
	require.NoError(t, err)
	templates, err := ParseTemplates(web.FS, hashes, false)
	require.NoError(t, err)
	return templates
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testApp(t *testing.T) *App {
	t.Helper()
	staticFS, err := fs.Sub(web.FS, "static")
	require.NoError(t, err)
	log := discardLogger()
	sessions := auth.NewSessionManager(nil, "test_session", time.Hour, false, log)
	emailTemplates, err := ParseEmailTemplates(web.FS)
	require.NoError(t, err)
	return &App{
		Sessions:       sessions,
		Templates:      mustParseTemplates(t),
		EmailTemplates: emailTemplates,
		AppURL:         "http://localhost:8080",
		StaticFS:       staticFS,
		Log:            log,
	}
}

func TestParseTemplates(t *testing.T) {
	_ = mustParseTemplates(t)
}

func TestParseTemplatesMissingBase(t *testing.T) {
	fsys := fstest.MapFS{
		"templates/pages/home.tmpl": &fstest.MapFile{Data: []byte(`{{define "title"}}Home{{end}}{{define "content"}}hi{{end}}`)},
	}
	_, err := ParseTemplates(fsys, nil, false)
	assert.Error(t, err)
}

func TestRenderHomeLoggedOut(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{})

	body := w.Body.String()
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Contains(t, body, "<title>Crow Watch</title>")
	assert.Contains(t, body, `href="/login"`)
	assert.Contains(t, body, "Login")
	assert.NotContains(t, body, `href="/submit"`)
}

func TestRenderHomeLoggedIn(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{BaseData: BaseData{IsLoggedIn: true, Username: "alice"}})

	body := w.Body.String()
	assert.Contains(t, body, "alice")
	assert.Contains(t, body, `href="/submit"`)
	assert.Contains(t, body, `href="/account"`)
	assert.NotContains(t, body, `href="/login"`)
}

func TestRenderLoginTab(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "login", LoginPageData{Tab: "login"})

	body := w.Body.String()
	assert.Contains(t, body, "Login | Crow Watch")
	assert.Contains(t, body, `<form method="post" action="/login">`)
	assert.Contains(t, body, `name="identifier"`)
	assert.Contains(t, body, `name="password"`)
}

func TestRenderRegisterTab(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "login", LoginPageData{Tab: "register"})

	body := w.Body.String()
	assert.Contains(t, body, "invite-only")
	assert.NotContains(t, body, `name="password"`)
}

func TestRenderLoginError(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "login", LoginPageData{
		Tab:        "login",
		Identifier: "bob@example.com",
		Error:      "Invalid e-mail/username and/or password.",
	})

	body := w.Body.String()
	assert.Contains(t, body, `role="alert"`)
	assert.Contains(t, body, "Invalid e-mail/username and/or password.")
	assert.Contains(t, body, "bob@example.com")
}

func TestRenderUnknownTemplate(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "nonexistent", nil)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSecurityHeaders(t *testing.T) {
	a := testApp(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := a.securityHeaders(inner)

	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, "max-age=63072000; includeSubDomains", w.Header().Get("Strict-Transport-Security"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "strict-origin-when-cross-origin", w.Header().Get("Referrer-Policy"))
}

func TestStaticFileServing(t *testing.T) {
	a := testApp(t)
	handler := a.Routes()

	r := httptest.NewRequest("GET", "/static/css/base.css", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/css")
	assert.Contains(t, w.Body.String(), "--primary")
}

func TestStaticCSSLinked(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{})

	assert.Regexp(t, regexp.MustCompile(`href="/static/css/base\.css\?v=[0-9a-f]{8}"`), w.Body.String())
}

func TestRenderSubmitForm(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "submit", SubmitPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Tab:      "link",
		TagGroups: []TagGroup{
			{Category: "Topics", Tags: []TagOption{
				{ID: 1, Tag: "programming", Description: "Code and dev"},
			}},
			{Category: "Media", Tags: []TagOption{
				{ID: 2, Tag: "video", IsMedia: true},
			}},
		},
	})

	body := w.Body.String()
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, body, "Submit | Crow Watch")
	assert.Contains(t, body, `name="url"`)
	assert.Contains(t, body, `name="title"`)
	assert.Contains(t, body, "programming")
	assert.Contains(t, body, "video")
	assert.Contains(t, body, `<form method="post" action="/submit">`)
}

func TestRenderSubmitFormWithErrors(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "submit", SubmitPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Tab:      "link",
		URL:      "bad-url",
		Title:    "",
		Errors:   map[string]string{"url": "URL must use http or https", "title": "Title is required."},
		Error:    "Please fix the errors below.",
	})

	body := w.Body.String()
	assert.Contains(t, body, "URL must use http or https")
	assert.Contains(t, body, "Title is required.")
	assert.Contains(t, body, "Please fix the errors below.")
	assert.Contains(t, body, `role="alert"`)
	assert.Contains(t, body, "bad-url")
}

func TestHomeShowsStories(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Stories: []StoryItem{
			{
				ID:         1,
				URL:        "https://example.com/article",
				Title:      "Test Article",
				Domain:     "example.com",
				Username:   "bob",
				Tags:       []StoryTag{{Tag: "programming", IsMedia: false}},
				IsLoggedIn: true,
				CreatedAt:  time.Now().Add(-2 * time.Hour),
			},
		},
	})

	body := w.Body.String()
	assert.Contains(t, body, "Test Article")
	assert.Contains(t, body, "example.com")
	assert.Contains(t, body, "bob")
	assert.Contains(t, body, "programming")
	assert.Contains(t, body, "2 hours ago")
	assert.NotContains(t, body, "You are not signed in.")
}

func TestSubmitPageRedirectsUnauthenticated(t *testing.T) {
	a := testApp(t)
	handler := a.Routes()

	r := httptest.NewRequest("GET", "/submit", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/login", w.Header().Get("Location"))
}

func TestSubmitPostRedirectsUnauthenticated(t *testing.T) {
	a := testApp(t)
	handler := a.Routes()

	r := httptest.NewRequest("POST", "/submit", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/login", w.Header().Get("Location"))
}

func TestNavShowsSubmitLink(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{BaseData: BaseData{IsLoggedIn: true, Username: "alice"}})

	body := w.Body.String()
	assert.Contains(t, body, `href="/submit"`)
	assert.Contains(t, body, "Submit")
}

func TestHomeMediaTagHasSpace(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Stories: []StoryItem{
			{
				ID:         1,
				Title:      "A video",
				URL:        "https://example.com/v",
				Domain:     "example.com",
				Username:   "bob",
				Tags:       []StoryTag{{Tag: "video", IsMedia: true}},
				IsLoggedIn: true,
				CreatedAt:  time.Now(),
			},
		},
	})

	body := w.Body.String()
	assert.Contains(t, body, `class="tag tag--media"`)
}

func TestSubmitFormPreservesSelectedTags(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	data := SubmitPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Tab:      "link",
		TagGroups: []TagGroup{
			{Category: "Topics", Tags: []TagOption{
				{ID: 1, Tag: "programming"},
			}},
			{Category: "Media", Tags: []TagOption{
				{ID: 2, Tag: "video", IsMedia: true},
			}},
		},
		Selected: []int64{2},
	}

	a.render(w, "submit", data)
	body := w.Body.String()
	assert.Contains(t, body, `value="2"`)
	assert.Contains(t, body, `aria-selected="true"`)
}

func TestHomeShowsTextStory(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "home", HomePageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Stories: []StoryItem{
			{
				ID:         42,
				ShortCode:  "abc123",
				Title:      "Ask CW: What editor do you use?",
				Username:   "bob",
				Tags:       []StoryTag{{Tag: "ask"}},
				IsText:     true,
				IsLoggedIn: true,
				CreatedAt:  time.Now().Add(-1 * time.Hour),
			},
		},
	})

	body := w.Body.String()
	assert.Contains(t, body, `/x/abc123/ask_cw_what_editor_do_you_use`)
	assert.Contains(t, body, "Ask CW: What editor do you use?")
	assert.Contains(t, body, "crow.watch")
}

func TestRenderStoryDetailPage(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "story", StoryPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Story: StoryItem{
			ID:         42,
			Title:      "Ask CW: What editor do you use?",
			Username:   "bob",
			Tags:       []StoryTag{{Tag: "ask"}},
			IsText:     true,
			IsLoggedIn: true,
			CreatedAt:  time.Now().Add(-1 * time.Hour),
		},
		Body: markdown.Render("I've been using vim for years but curious what others prefer."),
	})

	body := w.Body.String()
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, body, "Ask CW: What editor do you use?")
	assert.Contains(t, body, "story-body")
	assert.Contains(t, body, "<p>I&#39;ve been using vim for years but curious what others prefer.</p>")
	assert.Contains(t, body, "bob")
	assert.Contains(t, body, "crow.watch")
}

func TestRenderSubmitFormHasBodyField(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "submit", SubmitPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Tab:      "text",
	})

	body := w.Body.String()
	assert.Contains(t, body, `name="body"`)
	assert.Contains(t, body, "<textarea")
}

func TestSubmitLinkTabShowsURLNotBody(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "submit", SubmitPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Tab:      "link",
	})

	body := w.Body.String()
	assert.Contains(t, body, `name="url"`)
	assert.Contains(t, body, "fetch-title-btn")
	assert.NotContains(t, body, `name="body"`)
	assert.NotContains(t, body, "<textarea")
}

func TestSubmitTextTabShowsBodyNotURL(t *testing.T) {
	a := testApp(t)
	w := httptest.NewRecorder()

	a.render(w, "submit", SubmitPageData{
		BaseData: BaseData{IsLoggedIn: true, Username: "alice"},
		Tab:      "text",
	})

	body := w.Body.String()
	assert.Contains(t, body, `name="body"`)
	assert.Contains(t, body, "<textarea")
	assert.NotContains(t, body, `name="url"`)
	assert.NotContains(t, body, "fetch-title-btn")
}

func TestHashStatic(t *testing.T) {
	fsys := fstest.MapFS{
		"css/base.css": &fstest.MapFile{Data: []byte("body{}")},
		"js/app.js":    &fstest.MapFile{Data: []byte("alert(1)")},
	}
	hashes, err := HashStatic(fsys)
	require.NoError(t, err)
	assert.Len(t, hashes, 2)
	assert.Regexp(t, regexp.MustCompile(`^\?v=[0-9a-f]{8}$`), hashes["css/base.css"])
	assert.Regexp(t, regexp.MustCompile(`^\?v=[0-9a-f]{8}$`), hashes["js/app.js"])
}

func TestLongCacheMiddleware(t *testing.T) {
	a := testApp(t)
	handler := a.Routes()

	r := httptest.NewRequest("GET", "/static/css/base.css", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "public, max-age=31536000, immutable", w.Header().Get("Cache-Control"))
}

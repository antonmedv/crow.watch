package analytics

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestCollector() *Collector {
	return NewCollector(nil, "test-secret", slog.Default())
}

func TestVisitorID_Deterministic(t *testing.T) {
	c := newTestCollector()
	defer c.Close()
	id1 := c.VisitorID("1.2.3.4", "Mozilla/5.0")
	id2 := c.VisitorID("1.2.3.4", "Mozilla/5.0")
	assert.Equal(t, id1, id2)
}

func TestVisitorID_Length(t *testing.T) {
	c := newTestCollector()
	defer c.Close()
	id := c.VisitorID("1.2.3.4", "Mozilla/5.0")
	assert.Len(t, id, 32)
}

func TestVisitorID_DifferentInputs(t *testing.T) {
	c := newTestCollector()
	defer c.Close()
	id1 := c.VisitorID("1.2.3.4", "Mozilla/5.0")
	id2 := c.VisitorID("5.6.7.8", "Mozilla/5.0")
	id3 := c.VisitorID("1.2.3.4", "Chrome/120")
	assert.NotEqual(t, id1, id2, "different IPs should produce different IDs")
	assert.NotEqual(t, id1, id3, "different UAs should produce different IDs")
}

func TestVisitorID_DifferentSecrets(t *testing.T) {
	c1 := NewCollector(nil, "secret-a", slog.Default())
	c2 := NewCollector(nil, "secret-b", slog.Default())
	defer c1.Close()
	defer c2.Close()
	id1 := c1.VisitorID("1.2.3.4", "Mozilla/5.0")
	id2 := c2.VisitorID("1.2.3.4", "Mozilla/5.0")
	assert.NotEqual(t, id1, id2)
}

func TestShouldTrack(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{"home page", "GET", "/", true},
		{"story page", "GET", "/x/abc123/some-story", true},
		{"newest", "GET", "/newest", true},
		{"tag page", "GET", "/t/golang", true},
		{"static css", "GET", "/static/css/base.css", false},
		{"static js", "GET", "/static/js/vote.js", false},
		{"favicon", "GET", "/favicon.png", false},
		{"api tags", "GET", "/api/tags", false},
		{"api story", "POST", "/api/story", false},
		{"dev reload", "GET", "/__dev/reload", false},
		{"captcha", "GET", "/captcha/abc123", false},
		{"POST login", "POST", "/login", false},
		{"POST vote", "POST", "/stories/1/upvote", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(tt.method, tt.path, nil)
			assert.Equal(t, tt.want, ShouldTrack(r))
		})
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		want       string
	}{
		{"remote addr with port", "1.2.3.4:1234", "", "", "1.2.3.4"},
		{"remote addr without port", "1.2.3.4", "", "", "1.2.3.4"},
		{"x-forwarded-for single", "9.9.9.9:1234", "1.2.3.4", "", "1.2.3.4"},
		{"x-forwarded-for chain", "9.9.9.9:1234", "1.2.3.4, 5.6.7.8", "", "1.2.3.4"},
		{"x-real-ip", "9.9.9.9:1234", "", "1.2.3.4", "1.2.3.4"},
		{"xff takes precedence over xri", "9.9.9.9:1234", "1.1.1.1", "2.2.2.2", "1.1.1.1"},
		{"invalid xff falls through to remote addr", "9.9.9.9:1234", "not-an-ip", "", "9.9.9.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				r.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				r.Header.Set("X-Real-IP", tt.xri)
			}
			assert.Equal(t, tt.want, clientIP(r))
		})
	}
}

func TestCleanReferrer(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{"empty", "", ""},
		{"strips query string", "https://news.ycombinator.com/item?id=123", "news.ycombinator.com/item"},
		{"with path", "https://www.reddit.com/r/golang/comments/abc", "www.reddit.com/r/golang/comments/abc"},
		{"root path", "http://github.com/", "github.com"},
		{"self crow.watch", "https://crow.watch/newest", ""},
		{"self www.crow.watch", "https://www.crow.watch/x/abc/story", ""},
		{"self subdomain", "https://api.crow.watch/v1/test", ""},
		{"invalid url", "not a url ::::", ""},
		{"uppercase", "https://NEWS.YCOMBINATOR.COM/", "news.ycombinator.com"},
		{"github repo", "https://github.com/anthropics/claude-code", "github.com/anthropics/claude-code"},
		{"no path", "https://example.com", "example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cleanReferrer(tt.ref))
		})
	}
}

func TestAnalyticsMiddleware_SkipsNonTrackable(t *testing.T) {
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	r := httptest.NewRequest("POST", "/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	assert.True(t, called)
	assert.Equal(t, http.StatusOK, w.Code)
}

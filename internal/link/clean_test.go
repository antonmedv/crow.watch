package link

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClean_Validation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"empty", "", "URL is required"},
		{"no scheme", "example.com/page", "URL must use http or https"},
		{"ftp scheme", "ftp://example.com", "URL must use http or https"},
		{"no dot in host", "http://localhost/page", "URL must have a valid hostname"},
		{"too long", "https://example.com/" + string(make([]byte, 240)), "URL must be 250 characters or fewer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Clean(tt.input)
			require.Error(t, err)
			var ve *ValidationError
			require.ErrorAs(t, err, &ve)
			assert.Contains(t, ve.Message, tt.wantErr)
		})
	}
}

func TestClean_TrackingParams(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // check in Cleaned URL
	}{
		{"strips utm_source", "https://example.com/page?utm_source=twitter&ref=123", "ref=123"},
		{"strips multiple utm", "https://example.com/?utm_source=x&utm_medium=y&utm_campaign=z", "https://example.com/"},
		{"strips fbclid", "https://example.com/page?fbclid=abc123", "https://example.com/page"},
		{"strips gclid", "https://example.com/page?gclid=abc&q=test", "q=test"},
		{"strips si", "https://example.com/?si=xyz&v=abc", "v=abc"},
		{"preserves non-tracking", "https://example.com/?q=search&page=2", "q=search&page=2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Contains(t, result.Cleaned, tt.want)
			assert.NotContains(t, result.Cleaned, "utm_")
		})
	}
}

func TestClean_PortNormalization(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"removes :80 from http", "http://example.com:80/page", "example.com"},
		{"removes :443 from https", "https://example.com:443/page", "example.com"},
		{"keeps non-default port", "https://example.com:8080/page", "example.com:8080"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Contains(t, result.Cleaned, tt.want)
		})
	}
}

func TestClean_Normalization(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		wantNormalized string
	}{
		{"forces https", "http://example.com/page", "https://example.com/page"},
		{"removes www", "https://www.example.com/page", "https://example.com/page"},
		{"keeps www if host without it has no dot", "https://www.com/page", "https://www.com/page"},
		{"removes fragment", "https://example.com/page#section", "https://example.com/page"},
		{"removes trailing slash", "https://example.com/page/", "https://example.com/page"},
		{"keeps root slash", "https://example.com/", "https://example.com/"},
		{"removes .html", "https://example.com/page.html", "https://example.com/page"},
		{"removes .htm", "https://example.com/page.htm", "https://example.com/page"},
		{"removes /index", "https://example.com/section/index", "https://example.com/section"},
		{"removes /index.php", "https://example.com/section/index.php", "https://example.com/section"},
		{"removes /index.html", "https://example.com/section/index.html", "https://example.com/section"},
		{"removes /Default.aspx", "https://example.com/section/Default.aspx", "https://example.com/section"},
		{"sorts query params", "https://example.com/page?z=1&a=2", "https://example.com/page?a=2&z=1"},
		{"lowercases host", "https://EXAMPLE.COM/Page", "https://example.com/Page"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.wantNormalized, result.Normalized)
		})
	}
}

func TestClean_ArXiv(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"abs to abs", "https://arxiv.org/abs/2301.12345", "https://arxiv.org/abs/2301.12345"},
		{"pdf to abs", "https://arxiv.org/pdf/2301.12345", "https://arxiv.org/abs/2301.12345"},
		{"html to abs", "https://arxiv.org/html/2301.12345", "https://arxiv.org/abs/2301.12345"},
		{"strips version", "https://arxiv.org/abs/2301.12345v2", "https://arxiv.org/abs/2301.12345"},
		{"pdf with version", "https://arxiv.org/pdf/2301.12345v3", "https://arxiv.org/abs/2301.12345"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Normalized)
		})
	}
}

func TestClean_YouTube(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard watch", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
		{"short url", "https://youtu.be/dQw4w9WgXcQ", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
		{"embed", "https://youtube.com/embed/dQw4w9WgXcQ", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
		{"shorts", "https://youtube.com/shorts/dQw4w9WgXcQ", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
		{"with tracking", "https://www.youtube.com/watch?v=dQw4w9WgXcQ&si=xyz", "https://youtube.com/watch?v=dQw4w9WgXcQ"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Normalized)
		})
	}
}

func TestClean_RFC(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standard rfc", "https://www.rfc-editor.org/rfc/rfc9110", "https://rfc-editor.org/rfc/rfc9110"},
		{"info path", "https://rfc-editor.org/info/rfc9110", "https://rfc-editor.org/rfc/rfc9110"},
		{"with .txt", "https://rfc-editor.org/rfc/rfc9110.txt", "https://rfc-editor.org/rfc/rfc9110"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Normalized)
		})
	}
}

func TestClean_Domain(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple domain", "https://example.com/page", "example.com"},
		{"strips www", "https://www.example.com/page", "example.com"},
		{"subdomain preserved", "https://blog.example.com/page", "blog.example.com"},
		{"youtube domain", "https://youtube.com/watch?v=abc12345678", "youtube.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Domain)
		})
	}
}

func TestClean_Origin(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"github repo", "https://github.com/golang/go/issues/123", "github.com/golang"},
		{"github user only", "https://github.com/golang", "github.com/golang"},
		{"gitlab repo", "https://gitlab.com/user/project/-/merge_requests/1", "gitlab.com/user"},
		{"codeberg repo", "https://codeberg.org/forgejo/forgejo/issues/42", "codeberg.org/forgejo"},
		{"gitea repo", "https://gitea.com/gitea/act_runner", "gitea.com/gitea"},
		{"sourcehut repo", "https://sr.ht/~sircmpwn/builds.sr.ht", "sr.ht/~sircmpwn"},
		{"bitbucket repo", "https://bitbucket.org/atlassian/python-bitbucket/src/main", "bitbucket.org/atlassian"},
		{"twitter user", "https://twitter.com/golang/status/123", "twitter.com/golang"},
		{"x.com user", "https://x.com/golang/status/123", "x.com/golang"},
		{"reddit subreddit", "https://reddit.com/r/golang/comments/abc", "reddit.com/r/golang"},
		{"no origin for generic", "https://example.com/page/subpage", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Clean(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, result.Origin)
		})
	}
}

func TestClean_FullFlow(t *testing.T) {
	result, err := Clean("http://www.Example.COM:80/page/?utm_source=twitter&q=test&fbclid=abc#section")
	require.NoError(t, err)

	assert.Equal(t, "http://www.Example.COM:80/page/?utm_source=twitter&q=test&fbclid=abc#section", result.Original)
	assert.NotContains(t, result.Cleaned, "utm_source")
	assert.NotContains(t, result.Cleaned, "fbclid")
	assert.Equal(t, "https://example.com/page?q=test", result.Normalized)
	assert.Equal(t, "example.com", result.Domain)
	assert.Equal(t, "", result.Origin)
}

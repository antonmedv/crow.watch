package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"basic", "<html><head><title>Hello</title></head></html>", "Hello"},
		{"with whitespace", "<title>  Hello World  </title>", "Hello World"},
		{"empty title", "<title></title>", ""},
		{"no title tag", "<html><body>hi</body></html>", ""},
		{"nested head", "<html><head><meta charset='utf-8'><title>Test Page</title></head></html>", "Test Page"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractTitle(strings.NewReader(tt.html)))
		})
	}
}

func TestParseOEmbedTitle(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantTitle string
		wantErr   bool
	}{
		{
			"success",
			`{"title":"Rick Astley - Never Gonna Give You Up","author_name":"Rick Astley","type":"video"}`,
			"Rick Astley - Never Gonna Give You Up",
			false,
		},
		{
			"unicode title",
			`{"title":"日本語タイトル","type":"video"}`,
			"日本語タイトル",
			false,
		},
		{
			"empty title",
			`{"title":"","author_name":"someone"}`,
			"",
			true,
		},
		{
			"missing title field",
			`{"author_name":"someone"}`,
			"",
			true,
		},
		{
			"invalid json",
			`not json`,
			"",
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, err := parseOEmbedTitle(strings.NewReader(tt.body))
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantTitle, title)
			}
		})
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		url   string
		want  string
	}{
		{
			"github repo",
			"GitHub - anthropics/claude-code: CLI for Claude",
			"https://github.com/anthropics/claude-code",
			"CLI for Claude",
		},
		{
			"github repo long description",
			"GitHub - golang/go: The Go programming language",
			"https://github.com/golang/go",
			"The Go programming language",
		},
		{
			"github no description",
			"GitHub - owner/repo",
			"https://github.com/owner/repo",
			"GitHub - owner/repo",
		},
		{
			"non-github url unchanged",
			"Some Article Title",
			"https://example.com/article",
			"Some Article Title",
		},
		{
			"non-github with colon unchanged",
			"GitHub - not actually github",
			"https://example.com/page",
			"GitHub - not actually github",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, cleanTitle(tt.title, tt.url))
		})
	}
}

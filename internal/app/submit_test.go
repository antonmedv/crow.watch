package app

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
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

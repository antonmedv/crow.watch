package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateShortCode(t *testing.T) {
	code := generateShortCode()
	assert.Len(t, code, 6)
	for _, c := range code {
		assert.Contains(t, shortCodeCharset, string(c))
	}

	// Two codes should (almost certainly) differ
	code2 := generateShortCode()
	assert.NotEqual(t, code, code2)
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Hello World", "hello_world"},
		{"Ask CW: What editor do you use?", "ask_cw_what_editor_do_you_use"},
		{"  Leading/Trailing  ", "leading_trailing"},
		{"Multiple---Hyphens", "multiple_hyphens"},
		{"Special!@#$%Characters", "special_characters"},
		{"", ""},
		{"UPPERCASE", "uppercase"},
		{"a-b-c", "a_b_c"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, slugify(tt.input))
		})
	}
}

func TestSlugifyTruncates(t *testing.T) {
	long := ""
	for i := 0; i < 100; i++ {
		long += "a"
	}
	slug := slugify(long)
	assert.LessOrEqual(t, len(slug), 80)
}

func TestStoryPath(t *testing.T) {
	assert.Equal(t, "/x/abc123/hello_world", storyPath("abc123", "Hello World"))
}

package app

import (
	"crypto/rand"
	"regexp"
	"strings"
)

const shortCodeCharset = "abcdefghijklmnopqrstuvwxyz0123456789"

func generateShortCode() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	for i := range b {
		b[i] = shortCodeCharset[int(b[i])%len(shortCodeCharset)]
	}
	return string(b)
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(title string) string {
	s := strings.ToLower(title)
	s = nonAlnum.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 80 {
		s = s[:80]
		s = strings.TrimRight(s, "_")
	}
	return s
}

func storyPath(code, title string) string {
	return "/x/" + code + "/" + slugify(title)
}

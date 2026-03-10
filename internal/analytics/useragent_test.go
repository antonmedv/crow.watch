package analytics

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseUA_Bots(t *testing.T) {
	tests := []struct {
		name string
		ua   string
	}{
		{"googlebot", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
		{"bingbot", "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)"},
		{"curl", "curl/7.68.0"},
		{"wget", "Wget/1.21"},
		{"empty", ""},
		{"headless chrome", "Mozilla/5.0 (X11; Linux x86_64) HeadlessChrome/90.0"},
		{"spider", "Mozilla/5.0 (compatible; AhrefsBot/7.0; Spider)"},
		{"facebook", "facebookexternalhit/1.1"},
		{"twitterbot", "Twitterbot/1.0"},
		{"applebot", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Applebot/0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParseUA(tt.ua)
			assert.True(t, p.IsBot, "expected bot for %q", tt.ua)
			assert.Equal(t, "bot", p.Device)
		})
	}
}

func TestParseUA_Desktop(t *testing.T) {
	tests := []struct {
		name    string
		ua      string
		browser string
		os      string
	}{
		{
			"chrome windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Chrome", "Windows",
		},
		{
			"firefox linux",
			"Mozilla/5.0 (X11; Linux x86_64; rv:121.0) Gecko/20100101 Firefox/121.0",
			"Firefox", "Linux",
		},
		{
			"safari macos",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15",
			"Safari", "macOS",
		},
		{
			"edge windows",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			"Edge", "Windows",
		},
		{
			"opera macos",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 OPR/106.0.0.0",
			"Opera", "macOS",
		},
		{
			"vivaldi linux",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Vivaldi/6.5",
			"Vivaldi", "Linux",
		},
		{
			"chromeos",
			"Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Chrome", "ChromeOS",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParseUA(tt.ua)
			assert.False(t, p.IsBot)
			assert.Equal(t, "desktop", p.Device)
			assert.Equal(t, tt.browser, p.Browser)
			assert.Equal(t, tt.os, p.OS)
		})
	}
}

func TestParseUA_Mobile(t *testing.T) {
	tests := []struct {
		name    string
		ua      string
		browser string
		os      string
	}{
		{
			"chrome android",
			"Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			"Chrome", "Android",
		},
		{
			"safari iphone",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			"Safari", "iOS",
		},
		{
			"firefox android",
			"Mozilla/5.0 (Android 14; Mobile; rv:121.0) Gecko/121.0 Firefox/121.0",
			"Firefox", "Android",
		},
		{
			"chrome ios (crios)",
			"Mozilla/5.0 (iPhone; CPU iPhone OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.6099.119 Mobile/15E148 Safari/604.1",
			"Chrome", "iOS",
		},
		{
			"samsung browser",
			"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0.0.0 Mobile Safari/537.36",
			"Samsung Browser", "Android",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParseUA(tt.ua)
			assert.False(t, p.IsBot)
			assert.Equal(t, "mobile", p.Device)
			assert.Equal(t, tt.browser, p.Browser)
			assert.Equal(t, tt.os, p.OS)
		})
	}
}

func TestParseUA_Tablet(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		os   string
	}{
		{
			"ipad",
			"Mozilla/5.0 (iPad; CPU OS 17_2 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Mobile/15E148 Safari/604.1",
			"iOS",
		},
		{
			"android tablet (no mobile)",
			"Mozilla/5.0 (Linux; Android 14; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Android",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := ParseUA(tt.ua)
			assert.False(t, p.IsBot)
			assert.Equal(t, "tablet", p.Device)
			assert.Equal(t, tt.os, p.OS)
		})
	}
}

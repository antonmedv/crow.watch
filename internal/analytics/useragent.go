package analytics

import "strings"

// ParsedUA holds the parsed user-agent fields.
type ParsedUA struct {
	Device  string // "desktop", "mobile", "tablet"
	Browser string // "Chrome", "Firefox", "Safari", "Edge", "Opera", etc.
	OS      string // "Windows", "macOS", "Linux", "Android", "iOS"
	IsBot   bool
}

var botKeywords = []string{
	"bot", "crawler", "spider", "headless", "wget", "curl",
	"scraper", "slurp", "mediapartners", "facebookexternalhit",
	"twitterbot", "linkedinbot", "applebot", "duckduckbot",
	"baiduspider", "yandexbot", "sogou", "exabot",
	"ia_archiver", "archive.org_bot",
}

// ParseUA extracts device, browser, OS and bot status from a User-Agent string.
func ParseUA(ua string) ParsedUA {
	lower := strings.ToLower(ua)

	// Bot detection
	for _, kw := range botKeywords {
		if strings.Contains(lower, kw) {
			return ParsedUA{Device: "bot", IsBot: true}
		}
	}

	if ua == "" {
		return ParsedUA{Device: "bot", IsBot: true}
	}

	var p ParsedUA

	// OS detection (order matters)
	switch {
	case strings.Contains(lower, "android"):
		p.OS = "Android"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") || strings.Contains(lower, "ipod"):
		p.OS = "iOS"
	case strings.Contains(lower, "macintosh") || strings.Contains(lower, "mac os x"):
		p.OS = "macOS"
	case strings.Contains(lower, "windows"):
		p.OS = "Windows"
	case strings.Contains(lower, "linux"):
		p.OS = "Linux"
	case strings.Contains(lower, "cros"):
		p.OS = "ChromeOS"
	}

	// Device detection
	switch {
	case strings.Contains(lower, "ipad") || strings.Contains(lower, "tablet"):
		p.Device = "tablet"
	case strings.Contains(lower, "mobile") || strings.Contains(lower, "iphone") ||
		strings.Contains(lower, "ipod") || strings.Contains(lower, "android"):
		if strings.Contains(lower, "android") && !strings.Contains(lower, "mobile") {
			p.Device = "tablet"
		} else {
			p.Device = "mobile"
		}
	default:
		p.Device = "desktop"
	}

	// Browser detection (order matters: Edge before Chrome, Chrome before Safari)
	switch {
	case strings.Contains(lower, "edg/") || strings.Contains(lower, "edga/") || strings.Contains(lower, "edgios/"):
		p.Browser = "Edge"
	case strings.Contains(lower, "opr/") || strings.Contains(lower, "opera"):
		p.Browser = "Opera"
	case strings.Contains(lower, "vivaldi"):
		p.Browser = "Vivaldi"
	case strings.Contains(lower, "brave"):
		p.Browser = "Brave"
	case strings.Contains(lower, "firefox") || strings.Contains(lower, "fxios"):
		p.Browser = "Firefox"
	case strings.Contains(lower, "samsungbrowser"):
		p.Browser = "Samsung Browser"
	case strings.Contains(lower, "crios"):
		p.Browser = "Chrome"
	case strings.Contains(lower, "chrome") || strings.Contains(lower, "chromium"):
		p.Browser = "Chrome"
	case strings.Contains(lower, "safari") && !strings.Contains(lower, "chrome"):
		p.Browser = "Safari"
	}

	return p
}

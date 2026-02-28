package link

import (
	"net/url"
	"regexp"
	"strings"
)

var arxivIDRegex = regexp.MustCompile(`^/(abs|html|pdf)/(\d{4}\.\d{4,5})(v\d+)?`)
var youtubeIDRegex = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func normalizeSite(u *url.URL) {
	host := u.Hostname()

	switch {
	case host == "arxiv.org":
		normalizeArxiv(u)
	case host == "youtube.com" || host == "youtu.be":
		normalizeYouTube(u)
	case host == "rfc-editor.org" || host == "www.rfc-editor.org":
		normalizeRFC(u)
	}
}

func normalizeArxiv(u *url.URL) {
	m := arxivIDRegex.FindStringSubmatch(u.Path)
	if m == nil {
		return
	}
	// m[2] is the ID without version
	u.Path = "/abs/" + m[2]
	u.RawQuery = ""
}

func normalizeYouTube(u *url.URL) {
	var videoID string

	host := u.Hostname()
	if host == "youtu.be" {
		// youtu.be/<id>
		id := strings.TrimPrefix(u.Path, "/")
		if youtubeIDRegex.MatchString(id) {
			videoID = id
		}
	} else {
		// youtube.com paths
		switch {
		case strings.HasPrefix(u.Path, "/embed/"):
			id := strings.TrimPrefix(u.Path, "/embed/")
			id = strings.Split(id, "/")[0]
			if youtubeIDRegex.MatchString(id) {
				videoID = id
			}
		case strings.HasPrefix(u.Path, "/shorts/"):
			id := strings.TrimPrefix(u.Path, "/shorts/")
			id = strings.Split(id, "/")[0]
			if youtubeIDRegex.MatchString(id) {
				videoID = id
			}
		case u.Path == "/watch":
			id := u.Query().Get("v")
			if youtubeIDRegex.MatchString(id) {
				videoID = id
			}
		}
	}

	if videoID == "" {
		return
	}

	u.Host = "youtube.com"
	u.Path = "/watch"
	u.RawQuery = "v=" + videoID
}

func normalizeRFC(u *url.URL) {
	// Match patterns like /rfc/rfc1234, /info/rfc1234, etc.
	path := strings.ToLower(u.Path)

	// Try to extract RFC number
	var rfcNum string
	prefixes := []string{"/rfc/rfc", "/info/rfc", "/rfc/"}
	for _, p := range prefixes {
		if strings.HasPrefix(path, p) {
			rest := strings.TrimPrefix(path, p)
			// Strip any extension or trailing content
			rest = strings.TrimSuffix(rest, ".txt")
			rest = strings.TrimSuffix(rest, ".html")
			rest = strings.TrimSuffix(rest, ".xml")
			if rest != "" && isDigits(rest) {
				rfcNum = rest
				break
			}
		}
	}

	if rfcNum == "" {
		return
	}

	u.Host = "rfc-editor.org"
	u.Path = "/rfc/rfc" + rfcNum
	u.RawQuery = ""
}

func isDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

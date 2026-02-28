package link

import (
	"strings"
)

func extractDomain(host string) string {
	host = strings.ToLower(host)
	// Strip port
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}
	// Strip www.
	if strings.HasPrefix(host, "www.") {
		without := strings.TrimPrefix(host, "www.")
		if strings.Contains(without, ".") {
			host = without
		}
	}
	return host
}

func extractOrigin(host, path string) string {
	domain := extractDomain(host)
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")

	switch domain {
	case "github.com", "gitlab.com", "codeberg.org", "gitea.com", "sr.ht", "bitbucket.org":
		// domain/user
		if len(parts) >= 1 && parts[0] != "" {
			return domain + "/" + parts[0]
		}
	case "twitter.com", "x.com":
		// domain/username
		if len(parts) >= 1 && parts[0] != "" {
			return domain + "/" + parts[0]
		}
	case "reddit.com":
		// domain/r/subreddit
		if len(parts) >= 2 && parts[0] == "r" && parts[1] != "" {
			return domain + "/r/" + parts[1]
		}
	}

	return ""
}

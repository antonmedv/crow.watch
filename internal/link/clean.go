package link

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

type CleanResult struct {
	Original   string
	Cleaned    string
	Normalized string
	Domain     string
	Origin     string
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

var trackingParams = map[string]bool{
	"utm_source":   true,
	"utm_medium":   true,
	"utm_campaign": true,
	"utm_term":     true,
	"utm_content":  true,
	"sk":           true,
	"gclid":        true,
	"fbclid":       true,
	"linkid":       true,
	"pp":           true,
	"si":           true,
	"trk":          true,
}

func Clean(raw string) (CleanResult, error) {
	raw = strings.TrimSpace(raw)

	u, err := validate(raw)
	if err != nil {
		return CleanResult{}, err
	}

	stripTracking(u)
	normalizePort(u)
	cleaned := u.String()

	normalize(u)
	normalizeSite(u)

	// Remove fragment
	u.Fragment = ""
	u.RawFragment = ""

	normalized := u.String()
	domain := extractDomain(u.Host)
	origin := extractOrigin(u.Host, u.Path)

	return CleanResult{
		Original:   raw,
		Cleaned:    cleaned,
		Normalized: normalized,
		Domain:     domain,
		Origin:     origin,
	}, nil
}

func validate(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, &ValidationError{Field: "url", Message: "URL is required"}
	}

	if len(raw) > 250 {
		return nil, &ValidationError{Field: "url", Message: "URL must be 250 characters or fewer"}
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, &ValidationError{Field: "url", Message: "Invalid URL"}
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, &ValidationError{Field: "url", Message: "URL must use http or https"}
	}

	host := u.Hostname()
	if host == "" || !strings.Contains(host, ".") {
		return nil, &ValidationError{Field: "url", Message: "URL must have a valid hostname"}
	}

	return u, nil
}

func stripTracking(u *url.URL) {
	q := u.Query()
	changed := false
	for key := range q {
		if trackingParams[strings.ToLower(key)] {
			q.Del(key)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
}

func normalizePort(u *url.URL) {
	port := u.Port()
	if port == "" {
		return
	}
	scheme := strings.ToLower(u.Scheme)
	if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
		u.Host = u.Hostname()
	}
}

func normalize(u *url.URL) {
	// Force https
	u.Scheme = "https"

	// Lowercase host
	u.Host = strings.ToLower(u.Host)

	// Remove www. (unless host-without-www has no dot)
	host := u.Hostname()
	if strings.HasPrefix(host, "www.") {
		without := strings.TrimPrefix(host, "www.")
		if strings.Contains(without, ".") {
			port := u.Port()
			if port != "" {
				u.Host = without + ":" + port
			} else {
				u.Host = without
			}
		}
	}

	// Remove fragment
	u.Fragment = ""
	u.RawFragment = ""

	// Sort query params
	if u.RawQuery != "" {
		q := u.Query()
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sorted := url.Values{}
		for _, k := range keys {
			for _, v := range q[k] {
				sorted.Add(k, v)
			}
		}
		u.RawQuery = sorted.Encode()
	}

	// Normalize path
	path := u.Path

	// Remove /index, /index.php, /index.html, /index.htm, /Default.aspx
	suffixes := []string{"/index.php", "/index.html", "/index.htm", "/index", "/Default.aspx"}
	for _, s := range suffixes {
		if strings.HasSuffix(path, s) {
			path = strings.TrimSuffix(path, s)
			if path == "" {
				path = "/"
			}
			break
		}
	}

	// Remove .html, .htm extensions
	if strings.HasSuffix(path, ".html") {
		path = strings.TrimSuffix(path, ".html")
	} else if strings.HasSuffix(path, ".htm") {
		path = strings.TrimSuffix(path, ".htm")
	}

	// Remove trailing slash (but keep root /)
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimRight(path, "/")
	}

	u.Path = path
}

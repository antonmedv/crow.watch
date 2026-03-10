package analytics

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"crow.watch/internal/store"
)

const (
	recordBuffer = 1024
	numWorkers   = 2
)

// Collector records page view events.
type Collector struct {
	queries   *store.Queries
	log       *slog.Logger
	secretKey []byte

	mu      sync.Mutex
	daySalt string
	dayDate string

	recordCh chan store.InsertPageViewParams
	wg       sync.WaitGroup
}

// NewCollector creates a new analytics collector.
func NewCollector(queries *store.Queries, secretKey string, log *slog.Logger) *Collector {
	c := &Collector{
		queries:   queries,
		log:       log,
		secretKey: []byte(secretKey),
		recordCh:  make(chan store.InsertPageViewParams, recordBuffer),
	}
	c.rotateSalt()

	c.wg.Add(numWorkers)
	for range numWorkers {
		go c.worker()
	}

	return c
}

// Close drains pending page views and stops workers.
func (c *Collector) Close() {
	close(c.recordCh)
	c.wg.Wait()
}

func (c *Collector) worker() {
	defer c.wg.Done()
	for params := range c.recordCh {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := c.queries.InsertPageView(ctx, params); err != nil {
			c.log.Error("record page view", "error", err, "path", params.Path)
		}
		cancel()
	}
}

func (c *Collector) rotateSalt() {
	today := time.Now().UTC().Format("2006-01-02")
	mac := hmac.New(sha256.New, c.secretKey)
	mac.Write([]byte(today))
	c.daySalt = hex.EncodeToString(mac.Sum(nil))
	c.dayDate = today
}

func (c *Collector) salt() string {
	today := time.Now().UTC().Format("2006-01-02")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dayDate != today {
		c.rotateSalt()
	}
	return c.daySalt
}

// VisitorID computes a privacy-preserving daily visitor hash.
func (c *Collector) VisitorID(ip, userAgent string) string {
	h := sha256.New()
	h.Write([]byte(ip))
	h.Write([]byte(userAgent))
	h.Write([]byte(c.salt()))
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// Record stores a page view event via a buffered worker pool.
func (c *Collector) Record(r *http.Request) {
	ip := clientIP(r)
	ua := r.UserAgent()
	path := r.URL.Path
	referrer := cleanReferrer(r.Header.Get("Referer"))
	parsed := ParseUA(ua)
	visitorID := c.VisitorID(ip, ua)

	select {
	case c.recordCh <- store.InsertPageViewParams{
		Path:      path,
		VisitorID: visitorID,
		Referrer:  referrer,
		Device:    parsed.Device,
		Browser:   parsed.Browser,
		Os:        parsed.OS,
		IsBot:     parsed.IsBot,
	}:
	default:
		c.log.Warn("analytics buffer full, dropping page view", "path", path)
	}
}

// ShouldTrack returns true if the request should be tracked.
func ShouldTrack(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path
	if strings.HasPrefix(path, "/static/") ||
		strings.HasPrefix(path, "/api/") ||
		strings.HasPrefix(path, "/__dev/") ||
		path == "/favicon.png" ||
		strings.HasPrefix(path, "/captcha/") {
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" && net.ParseIP(ip) != nil {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		if ip := strings.TrimSpace(xri); net.ParseIP(ip) != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// cleanReferrer extracts host/path from a raw Referer header value.
// Self-referrals are filtered out. Query strings are stripped for privacy.
func cleanReferrer(rawRef string) string {
	if rawRef == "" {
		return ""
	}
	u, err := url.Parse(rawRef)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" || host == "crow.watch" || strings.HasSuffix(host, ".crow.watch") {
		return ""
	}
	p := u.Path
	if p == "" || p == "/" {
		return host
	}
	return host + p
}

package app

import (
	"context"
	"net"
	"net/http"
	"strings"

	"crow.watch/internal/store"
)

// clientIP extracts the client IP address from a request, checking
// proxy headers before falling back to r.RemoteAddr.
func clientIP(r *http.Request) string {
	// X-Forwarded-For may contain a comma-separated list; take the first.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0]); ip != "" {
			return ip
		}
	}

	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// recordIP upserts a user_ips row in the background so it doesn't slow the request.
func (a *App) recordIP(r *http.Request, userID int64, action string) {
	ip := clientIP(r)
	go func() {
		if err := a.Queries.UpsertUserIP(context.Background(), store.UpsertUserIPParams{
			UserID:    userID,
			IpAddress: ip,
			Action:    action,
		}); err != nil {
			a.Log.Error("record user ip", "error", err, "user_id", userID, "action", action)
		}
	}()
}

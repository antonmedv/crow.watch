package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/store"
)

type contextKey string

const userContextKey contextKey = "authenticated_user"

type SessionManager struct {
	queries    *store.Queries
	cookieName string
	ttl        time.Duration
	secure     bool
	log        *slog.Logger
}

type AuthenticatedUser struct {
	SessionID int64
	User      store.User
}

func NewSessionManager(queries *store.Queries, cookieName string, ttl time.Duration, secure bool, log *slog.Logger) *SessionManager {
	return &SessionManager{queries: queries, cookieName: cookieName, ttl: ttl, secure: secure, log: log}
}

func (m *SessionManager) AuthenticateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(m.cookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		tokenHash := HashToken(cookie.Value)
		sessionUser, err := m.queries.GetSessionUserByTokenHash(r.Context(), tokenHash)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				m.clearCookie(w)
				next.ServeHTTP(w, r)
				return
			}
			m.log.Error("authenticate request", "error", err, "method", r.Method, "path", r.URL.Path)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		if sessionUser.BannedAt.Valid || sessionUser.DeletedAt.Valid || sessionUser.PasswordDigest == "*" {
			_ = m.queries.DeleteSessionByTokenHash(r.Context(), tokenHash)
			m.clearCookie(w)
			next.ServeHTTP(w, r)
			return
		}

		_ = m.queries.TouchSession(r.Context(), sessionUser.SessionID)

		ctxUser := AuthenticatedUser{
			SessionID: sessionUser.SessionID,
			User: store.User{
				ID:                              sessionUser.ID,
				Username:                        sessionUser.Username,
				Email:                           sessionUser.Email,
				PasswordDigest:                  sessionUser.PasswordDigest,
				IsModerator:                     sessionUser.IsModerator,
				BannedAt:                        sessionUser.BannedAt,
				DeletedAt:                       sessionUser.DeletedAt,
				InviterID:                       sessionUser.InviterID,
				PasswordResetTokenHash:          sessionUser.PasswordResetTokenHash,
				PasswordResetTokenCreatedAt:     sessionUser.PasswordResetTokenCreatedAt,
				EmailConfirmedAt:                sessionUser.EmailConfirmedAt,
				EmailConfirmationTokenCreatedAt: sessionUser.EmailConfirmationTokenCreatedAt,
				UnconfirmedEmail:                sessionUser.UnconfirmedEmail,
				Website:                         sessionUser.Website,
				About:                           sessionUser.About,
				CreatedAt:                       sessionUser.CreatedAt,
				UpdatedAt:                       sessionUser.UpdatedAt,
			},
		}

		ctx := context.WithValue(r.Context(), userContextKey, ctxUser)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *SessionManager) Login(w http.ResponseWriter, r *http.Request, user store.User) error {
	rawToken, err := newRawToken()
	if err != nil {
		return err
	}

	err = m.queries.CreateSession(r.Context(), store.CreateSessionParams{
		UserID:    user.ID,
		TokenHash: HashToken(rawToken),
		UserAgent: r.UserAgent(),
		IpAddress: r.RemoteAddr,
		ExpiresAt: pgtype.Timestamptz{
			Time:  time.Now().Add(m.ttl),
			Valid: true,
		},
	})
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    rawToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(m.ttl),
	})

	return nil
}

func (m *SessionManager) Logout(w http.ResponseWriter, r *http.Request) error {
	cookie, err := r.Cookie(m.cookieName)
	if err == nil && cookie.Value != "" {
		_ = m.queries.DeleteSessionByTokenHash(r.Context(), HashToken(cookie.Value))
	}
	m.clearCookie(w)
	return nil
}

func UserFromContext(ctx context.Context) (AuthenticatedUser, bool) {
	user, ok := ctx.Value(userContextKey).(AuthenticatedUser)
	return user, ok
}

func newRawToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (m *SessionManager) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		MaxAge:   -1,
		SameSite: http.SameSiteLaxMode,
	})
}

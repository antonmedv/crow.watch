package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

const resetEmailCooldown = 15 * time.Minute

func (a *App) forgotPasswordPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r)})
}

func (a *App) forgotPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r), Error: "Invalid request."})
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r), Email: email, Error: "Please enter your e-mail address."})
		return
	}

	successMsg := "If an account with that e-mail exists, we sent a password reset link."

	user, err := a.Queries.GetUserByLogin(r.Context(), email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r), Success: successMsg})
			return
		}
		a.serverError(w, r, "get user for password reset", err)
		return
	}

	// Skip banned, deleted, wiped, or unconfirmed-email accounts silently.
	if user.BannedAt.Valid || user.DeletedAt.Valid || user.PasswordDigest == "*" || !user.EmailConfirmedAt.Valid {
		a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r), Success: successMsg})
		return
	}

	// Rate-limit: skip if a reset email was already sent recently.
	if user.PasswordResetTokenCreatedAt.Valid && time.Since(user.PasswordResetTokenCreatedAt.Time) < resetEmailCooldown {
		a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r), Success: successMsg})
		return
	}

	token, err := generateResetToken()
	if err != nil {
		a.serverError(w, r, "generate reset token", err)
		return
	}

	tokenHash := auth.HashToken(token)

	err = a.Queries.SetPasswordResetTokenHash(r.Context(), store.SetPasswordResetTokenHashParams{
		PasswordResetTokenHash: pgtype.Text{String: tokenHash, Valid: true},
		ID:                     user.ID,
	})
	if err != nil {
		a.serverError(w, r, "set password reset token", err)
		return
	}

	resetURL := a.AppURL + "/reset-password?token=" + token

	tmpl, ok := a.EmailTemplates["password_reset"]
	if !ok {
		a.serverError(w, r, "email template not found", errors.New("password_reset template missing"))
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Username string
		ResetURL string
	}{
		Username: user.Username,
		ResetURL: resetURL,
	})
	if err != nil {
		a.serverError(w, r, "render email template", err)
		return
	}

	go func() {
		if sendErr := a.EmailSender.Send(context.Background(), user.Email, "Reset your Crow Watch password", buf.String()); sendErr != nil {
			a.Log.Error("send password reset email", "error", sendErr, "email", user.Email)
		}
	}()

	a.render(w, "forgot_password", ForgotPasswordPageData{BaseData: a.baseData(r), Success: successMsg})
}

func (a *App) resetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Redirect(w, r, "/forgot-password", http.StatusSeeOther)
		return
	}
	a.render(w, "reset_password", ResetPasswordPageData{BaseData: a.baseData(r), Token: token})
}

func (a *App) resetPassword(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.render(w, "reset_password", ResetPasswordPageData{BaseData: a.baseData(r), Error: "Invalid request."})
		return
	}

	token := strings.TrimSpace(r.FormValue("token"))
	password := r.FormValue("password")
	confirmation := r.FormValue("password_confirmation")

	if token == "" {
		http.Redirect(w, r, "/forgot-password", http.StatusSeeOther)
		return
	}

	if password == "" {
		a.render(w, "reset_password", ResetPasswordPageData{BaseData: a.baseData(r), Token: token, Error: "Please enter a new password."})
		return
	}
	if len(password) > 72 {
		a.render(w, "reset_password", ResetPasswordPageData{BaseData: a.baseData(r), Token: token, Error: "Password must be 72 bytes or fewer."})
		return
	}
	if password != confirmation {
		a.render(w, "reset_password", ResetPasswordPageData{BaseData: a.baseData(r), Token: token, Error: "Passwords do not match."})
		return
	}

	tokenHash := auth.HashToken(token)

	user, err := a.Queries.GetUserByPasswordResetTokenHash(r.Context(), pgtype.Text{String: tokenHash, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.render(w, "reset_password", ResetPasswordPageData{BaseData: a.baseData(r), Token: token, Error: "This reset link is invalid or has expired."})
			return
		}
		a.serverError(w, r, "get user by reset token", err)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, "hash password", err)
		return
	}

	err = a.Queries.UpdateUserPasswordByID(r.Context(), store.UpdateUserPasswordByIDParams{
		PasswordDigest: string(hash),
		ID:             user.ID,
	})
	if err != nil {
		a.serverError(w, r, "update password", err)
		return
	}

	if err := a.Queries.ClearPasswordResetTokenHash(r.Context(), user.ID); err != nil {
		a.serverError(w, r, "clear reset token", err)
		return
	}

	// Invalidate all existing sessions so a compromised session can't persist.
	if err := a.Queries.DeleteSessionsByUserID(r.Context(), user.ID); err != nil {
		a.serverError(w, r, "delete sessions", err)
		return
	}

	if err := a.Sessions.Login(w, r, user); err != nil {
		a.serverError(w, r, "session login after password reset", err)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func generateResetToken() (string, error) {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

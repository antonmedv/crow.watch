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
)

const confirmationEmailCooldown = 15 * time.Minute

func (a *App) confirmEmail(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tokenHash := auth.HashToken(token)

	user, err := a.Queries.GetUserByEmailConfirmationTokenHash(r.Context(), pgtype.Text{String: tokenHash, Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.render(w, "confirm_email", ConfirmEmailPageData{
				BaseData: a.baseData(r),
				Error:    "This confirmation link is invalid or has expired.",
			})
			return
		}
		a.serverError(w, r, "get user by email confirmation token", err)
		return
	}

	if err := a.Queries.ConfirmUserEmail(r.Context(), user.ID); err != nil {
		if strings.Contains(err.Error(), "users_email_unique") {
			a.render(w, "confirm_email", ConfirmEmailPageData{
				BaseData: a.baseData(r),
				Error:    "That e-mail address is already taken by another account.",
			})
			return
		}
		a.serverError(w, r, "confirm user email", err)
		return
	}

	a.render(w, "confirm_email", ConfirmEmailPageData{
		BaseData: a.baseData(r),
		Success:  "Your e-mail address has been confirmed.",
	})
}

func (a *App) resendConfirmation(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Rate-limit: skip if a confirmation email was already sent recently.
	if current.User.EmailConfirmationTokenCreatedAt.Valid &&
		time.Since(current.User.EmailConfirmationTokenCreatedAt.Time) < confirmationEmailCooldown {
		a.render(w, "account", AccountPageData{
			BaseData:         a.baseData(r),
			Tab:              "email",
			Email:            current.User.Email,
			About:            current.User.About,
			Website:          current.User.Website,
			EmailConfirmed:   current.User.EmailConfirmedAt.Valid,
			UnconfirmedEmail: current.User.UnconfirmedEmail.String,
			Success:          "A confirmation e-mail was already sent recently. Please check your inbox.",
		})
		return
	}

	targetEmail := current.User.Email
	if current.User.UnconfirmedEmail.Valid && current.User.UnconfirmedEmail.String != "" {
		targetEmail = current.User.UnconfirmedEmail.String
	}

	if err := a.sendConfirmationEmail(r.Context(), current.User, targetEmail); err != nil {
		a.serverError(w, r, "resend confirmation email", err)
		return
	}

	a.render(w, "account", AccountPageData{
		BaseData:         a.baseData(r),
		Tab:              "email",
		Email:            current.User.Email,
		About:            current.User.About,
		Website:          current.User.Website,
		EmailConfirmed:   current.User.EmailConfirmedAt.Valid,
		UnconfirmedEmail: current.User.UnconfirmedEmail.String,
		Success:          "Confirmation e-mail sent to " + targetEmail + ".",
	})
}

func (a *App) sendConfirmationEmail(ctx context.Context, user store.User, targetEmail string) error {
	token, err := generateConfirmationToken()
	if err != nil {
		return err
	}

	tokenHash := auth.HashToken(token)

	err = a.Queries.SetEmailConfirmationToken(ctx, store.SetEmailConfirmationTokenParams{
		EmailConfirmationTokenHash: pgtype.Text{String: tokenHash, Valid: true},
		ID:                         user.ID,
	})
	if err != nil {
		return err
	}

	confirmURL := a.AppURL + "/confirm-email?token=" + token

	tmpl, ok := a.EmailTemplates["email_confirmation"]
	if !ok {
		return errors.New("email_confirmation template missing")
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Username   string
		ConfirmURL string
	}{
		Username:   user.Username,
		ConfirmURL: confirmURL,
	})
	if err != nil {
		return err
	}

	go func() {
		if sendErr := a.EmailSender.Send(context.Background(), targetEmail, "Confirm your Crow Watch email", buf.String()); sendErr != nil {
			a.Log.Error("send confirmation email", "error", sendErr, "email", targetEmail)
		}
	}()

	return nil
}

func (a *App) sendConfirmationEmailForNewUser(ctx context.Context, userID int64, username, email string) {
	user := store.User{ID: userID, Username: username, Email: email}
	if err := a.sendConfirmationEmail(ctx, user, email); err != nil {
		a.Log.Error("send confirmation email for new user", "error", err, "user_id", userID)
	}
}

func generateConfirmationToken() (string, error) {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

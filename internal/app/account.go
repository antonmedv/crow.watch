package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"

	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

func (a *App) accountPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tab := r.URL.Query().Get("tab")
	if tab != "email" && tab != "password" {
		tab = "profile"
	}

	a.render(w, "account", AccountPageData{
		BaseData:         a.baseData(r),
		Tab:              tab,
		Email:            current.User.Email,
		About:            current.User.About,
		Website:          current.User.Website,
		EmailConfirmed:   current.User.EmailConfirmedAt.Valid,
		UnconfirmedEmail: current.User.UnconfirmedEmail.String,
	})
}

func (a *App) updateProfile(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.render(w, "account", AccountPageData{
			BaseData: a.baseData(r),
			Tab:      "profile",
			Email:    current.User.Email,
			About:    current.User.About,
			Website:  current.User.Website,
			Errors:   map[string]string{"about": "Invalid request."},
		})
		return
	}

	website := strings.TrimSpace(r.FormValue("website"))
	about := strings.TrimSpace(r.FormValue("about"))

	errs := make(map[string]string)
	if len(website) > 250 {
		errs["website"] = "Website must be 250 characters or fewer."
	}
	if len(about) > 500 {
		errs["about"] = "About must be 500 characters or fewer."
	}

	if len(errs) > 0 {
		a.render(w, "account", AccountPageData{
			BaseData: a.baseData(r),
			Tab:      "profile",
			Email:    current.User.Email,
			About:    about,
			Website:  website,
			Errors:   errs,
		})
		return
	}

	if err := a.Queries.UpdateUserProfile(r.Context(), store.UpdateUserProfileParams{
		Website: website,
		About:   about,
		ID:      current.User.ID,
	}); err != nil {
		a.serverError(w, r, "update profile", err)
		return
	}

	a.render(w, "account", AccountPageData{
		BaseData: a.baseData(r),
		Tab:      "profile",
		Email:    current.User.Email,
		About:    about,
		Website:  website,
		Success:  "Profile updated.",
	})
}

// verifyPassword checks the provided password against the user's stored digest.
// Returns an error message suitable for display, or "" on success.
func verifyPassword(digest, password string) string {
	if password == "" {
		return "Please enter your current password."
	}
	if bcrypt.CompareHashAndPassword([]byte(digest), []byte(password)) != nil {
		return "Current password is incorrect."
	}
	return ""
}

func (a *App) updateEmail(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.render(w, "account", AccountPageData{
			BaseData:         a.baseData(r),
			Tab:              "email",
			Email:            current.User.Email,
			About:            current.User.About,
			Website:          current.User.Website,
			EmailConfirmed:   current.User.EmailConfirmedAt.Valid,
			UnconfirmedEmail: current.User.UnconfirmedEmail.String,
			Errors:           map[string]string{"email": "Invalid request."},
		})
		return
	}

	newEmail := strings.TrimSpace(r.FormValue("email"))

	renderErr := func(errs map[string]string) {
		a.render(w, "account", AccountPageData{
			BaseData:         a.baseData(r),
			Tab:              "email",
			Email:            current.User.Email,
			About:            current.User.About,
			Website:          current.User.Website,
			EmailConfirmed:   current.User.EmailConfirmedAt.Valid,
			UnconfirmedEmail: current.User.UnconfirmedEmail.String,
			Errors:           errs,
		})
	}

	if newEmail == "" {
		renderErr(map[string]string{"email": "E-mail is required."})
		return
	}

	if msg := verifyPassword(current.User.PasswordDigest, r.FormValue("password")); msg != "" {
		renderErr(map[string]string{"email_password": msg})
		return
	}

	// Check if the new email is already taken.
	taken, err := a.Queries.CheckEmailExists(r.Context(), store.CheckEmailExistsParams{
		Email: newEmail,
		ID:    current.User.ID,
	})
	if err != nil {
		a.serverError(w, r, "check email exists", err)
		return
	}
	if taken {
		renderErr(map[string]string{"email": "That e-mail is already registered."})
		return
	}

	// Store as pending and send confirmation to the new address.
	token, err := generateConfirmationToken()
	if err != nil {
		a.serverError(w, r, "generate confirmation token", err)
		return
	}

	tokenHash := auth.HashToken(token)

	err = a.Queries.SetEmailChangeConfirmationToken(r.Context(), store.SetEmailChangeConfirmationTokenParams{
		EmailConfirmationTokenHash: pgtype.Text{String: tokenHash, Valid: true},
		UnconfirmedEmail:           pgtype.Text{String: newEmail, Valid: true},
		ID:                         current.User.ID,
	})
	if err != nil {
		a.serverError(w, r, "set email change confirmation token", err)
		return
	}

	confirmURL := a.AppURL + "/confirm-email?token=" + token

	tmpl, ok2 := a.EmailTemplates["email_confirmation"]
	if !ok2 {
		a.serverError(w, r, "email template not found", errors.New("email_confirmation template missing"))
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		Username   string
		ConfirmURL string
	}{
		Username:   current.User.Username,
		ConfirmURL: confirmURL,
	})
	if err != nil {
		a.serverError(w, r, "render email template", err)
		return
	}

	go func() {
		if sendErr := a.EmailSender.Send(context.Background(), newEmail, "Confirm your Crow Watch email", buf.String()); sendErr != nil {
			a.Log.Error("send email change confirmation", "error", sendErr, "email", newEmail)
		}
	}()

	a.render(w, "account", AccountPageData{
		BaseData:         a.baseData(r),
		Tab:              "email",
		Email:            current.User.Email,
		About:            current.User.About,
		Website:          current.User.Website,
		EmailConfirmed:   current.User.EmailConfirmedAt.Valid,
		UnconfirmedEmail: newEmail,
		Success:          "Confirmation e-mail sent to " + newEmail + ".",
	})
}

func (a *App) updatePassword(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.render(w, "account", AccountPageData{
			BaseData: a.baseData(r),
			Tab:      "password",
			Email:    current.User.Email,
			About:    current.User.About,
			Website:  current.User.Website,
			Errors:   map[string]string{"current_password": "Invalid request."},
		})
		return
	}

	newPassword := r.FormValue("new_password")
	confirmation := r.FormValue("new_password_confirmation")

	errs := make(map[string]string)

	if msg := verifyPassword(current.User.PasswordDigest, r.FormValue("current_password")); msg != "" {
		errs["current_password"] = msg
	}
	if newPassword == "" {
		errs["new_password"] = "Please enter a new password."
	} else if len(newPassword) > 72 {
		errs["new_password"] = "Password must be 72 bytes or fewer."
	}
	if newPassword != confirmation {
		errs["new_password_confirmation"] = "Passwords do not match."
	}

	if len(errs) > 0 {
		a.render(w, "account", AccountPageData{
			BaseData: a.baseData(r),
			Tab:      "password",
			Email:    current.User.Email,
			About:    current.User.About,
			Website:  current.User.Website,
			Errors:   errs,
		})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, "hash password", err)
		return
	}

	if err := a.Queries.UpdateUserPasswordByID(r.Context(), store.UpdateUserPasswordByIDParams{
		PasswordDigest: string(hash),
		ID:             current.User.ID,
	}); err != nil {
		a.serverError(w, r, "update password", err)
		return
	}

	// Delete all sessions for this user (logout everywhere), then re-login.
	if err := a.Queries.DeleteSessionsByUserID(r.Context(), current.User.ID); err != nil {
		a.serverError(w, r, "delete sessions", err)
		return
	}

	user, err := a.Queries.GetUserByID(r.Context(), current.User.ID)
	if err != nil {
		a.serverError(w, r, "get user after password change", err)
		return
	}

	if err := a.Sessions.Login(w, r, user); err != nil {
		a.serverError(w, r, "session login after password change", err)
		return
	}

	a.render(w, "account", AccountPageData{
		BaseData: a.baseData(r),
		Tab:      "password",
		Email:    user.Email,
		About:    user.About,
		Website:  user.Website,
		Success:  "Password changed. All other sessions have been logged out.",
	})
}

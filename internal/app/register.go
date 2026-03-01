package app

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"crow.watch/internal/auth"
	"crow.watch/internal/captcha"
	"crow.watch/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"
)

var usernameRegexp = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func validateRegistration(username, email, password, passwordConfirmation string) map[string]string {
	errs := make(map[string]string)

	if username == "" {
		errs["username"] = "Username is required."
	} else if len(username) < 2 || len(username) > 20 {
		errs["username"] = "Username must be 2-20 characters."
	} else if !usernameRegexp.MatchString(username) {
		errs["username"] = "Username may only contain letters, numbers, hyphens, and underscores."
	}

	if email == "" {
		errs["email"] = "E-mail is required."
	}

	if password == "" {
		errs["password"] = "Password is required."
	} else if len(password) > 72 {
		errs["password"] = "Password must be 72 bytes or fewer."
	} else if password != passwordConfirmation {
		errs["password_confirmation"] = "Passwords do not match."
	}

	return errs
}

// registerPage handles GET /register/{token} (invitation flow).
func (a *App) registerPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	token := r.PathValue("token")
	tokenHash := auth.HashToken(token)

	invite, err := a.Queries.GetInvitationByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.render(w, "register", RegisterPageData{
				BaseData: a.baseData(r),
				Errors:   map[string]string{"token": "This invitation link is invalid or has expired."},
			})
			return
		}
		a.serverError(w, r, "get invitation by token", err)
		return
	}

	var email string
	if invite.Email.Valid {
		email = invite.Email.String
	}

	a.render(w, "register", RegisterPageData{
		BaseData:    a.baseData(r),
		FormAction:  "/register/" + token,
		InviterName: invite.InviterName,
		Email:       email,
	})
}

// register handles POST /register/{token} (invitation flow).
func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.render(w, "register", RegisterPageData{
			BaseData: a.baseData(r),
			Errors:   map[string]string{"form": "Invalid request."},
		})
		return
	}

	token := r.PathValue("token")
	tokenHash := auth.HashToken(token)
	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	passwordConfirmation := r.FormValue("password_confirmation")

	invite, err := a.Queries.GetInvitationByTokenHash(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.render(w, "register", RegisterPageData{
				BaseData: a.baseData(r),
				Errors:   map[string]string{"token": "This invitation link is invalid or has expired."},
			})
			return
		}
		a.serverError(w, r, "get invitation by token", err)
		return
	}

	renderErr := func(errs map[string]string) {
		a.render(w, "register", RegisterPageData{
			BaseData:    a.baseData(r),
			FormAction:  "/register/" + token,
			InviterName: invite.InviterName,
			Email:       email,
			Username:    username,
			Errors:      errs,
		})
	}

	errs := validateRegistration(username, email, password, passwordConfirmation)
	if len(errs) > 0 {
		renderErr(errs)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, "hash password", err)
		return
	}

	tx, err := a.Pool.Begin(r.Context())
	if err != nil {
		a.serverError(w, r, "begin transaction", err)
		return
	}
	defer tx.Rollback(r.Context())

	qtx := a.Queries.WithTx(tx)

	newUser, err := qtx.CreateUser(r.Context(), store.CreateUserParams{
		Username:       username,
		Email:          email,
		PasswordDigest: string(hash),
		InviterID:      pgtype.Int8{Int64: invite.InviterID, Valid: true},
	})
	if err != nil {
		if errs := uniqueUserErrors(err); len(errs) > 0 {
			renderErr(errs)
			return
		}
		a.serverError(w, r, "create user", err)
		return
	}

	_, err = qtx.ClaimInvitation(r.Context(), store.ClaimInvitationParams{
		UsedByID: pgtype.Int8{Int64: newUser.ID, Valid: true},
		ID:       invite.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			renderErr(map[string]string{"token": "This invitation has already been used."})
			return
		}
		a.serverError(w, r, "claim invitation", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		a.serverError(w, r, "commit registration", err)
		return
	}

	// If the invitation was sent to this email, auto-confirm.
	if invite.Email.Valid && invite.Email.String == email {
		if err := a.Queries.ConfirmUserEmail(r.Context(), newUser.ID); err != nil {
			a.Log.Error("auto-confirm email for invited user", "error", err, "user_id", newUser.ID)
		}
	} else {
		go a.sendConfirmationEmailForNewUser(context.Background(), newUser.ID, newUser.Username, newUser.Email)
	}

	a.loginAndRedirect(w, r, newUser)
}

// joinPage handles GET /join/{slug} (campaign flow).
func (a *App) joinPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	slug := r.PathValue("slug")

	campaign, err := a.Queries.GetActiveCampaignBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get campaign", err)
		return
	}

	captchaID, err := a.Captcha.Generate()
	if err != nil {
		a.serverError(w, r, "generate captcha", err)
		return
	}

	a.render(w, "register", RegisterPageData{
		BaseData:       a.baseData(r),
		FormAction:     "/join/" + campaign.Slug,
		WelcomeMessage: campaign.WelcomeMessage,
		CaptchaID:      captchaID,
	})
}

// joinRegister handles POST /join/{slug} (campaign flow).
func (a *App) joinRegister(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	slug := r.PathValue("slug")

	campaign, err := a.Queries.GetActiveCampaignBySlug(r.Context(), slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, r, "get campaign", err)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	passwordConfirmation := r.FormValue("password_confirmation")

	renderErr := func(errs map[string]string) {
		freshID, _ := a.Captcha.Generate()
		a.render(w, "register", RegisterPageData{
			BaseData:       a.baseData(r),
			FormAction:     "/join/" + campaign.Slug,
			WelcomeMessage: campaign.WelcomeMessage,
			Username:       username,
			Email:          email,
			Errors:         errs,
			CaptchaID:      freshID,
		})
	}

	errs := validateRegistration(username, email, password, passwordConfirmation)

	captchaID := r.FormValue("captcha_id")
	captchaAnswer, _ := strconv.Atoi(r.FormValue("captcha_answer"))
	if !a.Captcha.Validate(captchaID, captchaAnswer) {
		errs["captcha"] = "Incorrect answer. Please try again."
	}

	if len(errs) > 0 {
		renderErr(errs)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.serverError(w, r, "hash password", err)
		return
	}

	newUser, err := a.Queries.CreateUser(r.Context(), store.CreateUserParams{
		Username:       username,
		Email:          email,
		PasswordDigest: string(hash),
		InviterID:      pgtype.Int8{Int64: campaign.SponsorID, Valid: true},
		Campaign:       campaign.Slug,
	})
	if err != nil {
		if errs := uniqueUserErrors(err); len(errs) > 0 {
			renderErr(errs)
			return
		}
		a.serverError(w, r, "create user", err)
		return
	}

	go a.sendConfirmationEmailForNewUser(context.Background(), newUser.ID, newUser.Username, newUser.Email)

	a.loginAndRedirect(w, r, newUser)
}

// serveCaptchaImage renders the CAPTCHA PNG for the given ID.
func (a *App) serveCaptchaImage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ca, cb, ok := a.Captcha.GetChallenge(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	if err := captcha.RenderPNG(w, ca, cb); err != nil {
		a.Log.Error("render captcha", "error", err)
	}
}

// uniqueUserErrors maps unique-constraint violations to field errors.
func uniqueUserErrors(err error) map[string]string {
	errs := make(map[string]string)
	errStr := err.Error()
	if strings.Contains(errStr, "users_username_unique") {
		errs["username"] = "That username is already taken."
	}
	if strings.Contains(errStr, "users_email_unique") {
		errs["email"] = "That e-mail is already registered."
	}
	return errs
}

// loginAndRedirect creates a session for a newly registered user and redirects to /.
func (a *App) loginAndRedirect(w http.ResponseWriter, r *http.Request, newUser store.CreateUserRow) {
	user := store.User{
		ID:       newUser.ID,
		Username: newUser.Username,
		Email:    newUser.Email,
	}
	if err := a.Sessions.Login(w, r, user); err != nil {
		a.serverError(w, r, "session login", err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

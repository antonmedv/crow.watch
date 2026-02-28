package app

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"crow.watch/internal/auth"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tab := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tab")))
	if tab != "register" {
		tab = "login"
	}
	a.render(w, "login", LoginPageData{BaseData: a.baseData(r), Tab: tab})
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.render(w, "login", LoginPageData{BaseData: a.baseData(r), Tab: "login", Error: "Invalid login request."})
		return
	}

	identifier := strings.TrimSpace(r.FormValue("identifier"))
	password := r.FormValue("password")

	rateLimitErr := LoginPageData{BaseData: a.baseData(r), Tab: "login", Identifier: identifier, Error: "Too many login attempts. Please try again later."}

	if a.LoginIPLimiter != nil {
		ip, _, _ := net.SplitHostPort(r.RemoteAddr)
		if ip == "" {
			ip = r.RemoteAddr
		}
		if !a.LoginIPLimiter.Allow(ip) {
			a.render(w, "login", rateLimitErr)
			return
		}
	}

	if a.LoginAcctLimiter != nil {
		if !a.LoginAcctLimiter.Allow(strings.ToLower(identifier)) {
			a.render(w, "login", rateLimitErr)
			return
		}
	}

	user, err := a.Queries.GetUserByLogin(r.Context(), identifier)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.render(w, "login", LoginPageData{BaseData: a.baseData(r), Tab: "login", Identifier: identifier, Error: "Invalid e-mail/username and/or password."})
			return
		}
		a.serverError(w, r, "get user by login", err)
		return
	}

	invalidErr := LoginPageData{BaseData: a.baseData(r), Tab: "login", Identifier: identifier, Error: "Invalid e-mail/username and/or password."}

	if user.BannedAt.Valid || user.DeletedAt.Valid || user.PasswordDigest == "*" {
		a.render(w, "login", invalidErr)
		return
	}
	if len(password) > 72 {
		a.render(w, "login", invalidErr)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordDigest), []byte(password)) != nil {
		a.render(w, "login", invalidErr)
		return
	}

	if err := a.Sessions.Login(w, r, user); err != nil {
		a.serverError(w, r, "session login", err)
		return
	}

	if a.LoginAcctLimiter != nil {
		a.LoginAcctLimiter.Reset(strings.ToLower(identifier))
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	_ = a.Sessions.Logout(w, r)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

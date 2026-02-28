package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func (a *App) invitePage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	tab := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tab")))
	if tab != "link" {
		tab = "email"
	}

	invitations, err := a.Queries.ListInvitationsByUser(r.Context(), current.User.ID)
	if err != nil {
		a.serverError(w, r, "list invitations", err)
		return
	}

	rows := make([]InviteRow, len(invitations))
	for i, inv := range invitations {
		rows[i] = InviteRow{
			CreatedAt: inv.CreatedAt.Time,
		}
		if inv.Email.Valid {
			rows[i].Email = inv.Email.String
		}
		if inv.RegisteredUsername.Valid {
			rows[i].RegisteredUsername = inv.RegisteredUsername.String
			rows[i].Status = "Registered"
		} else if time.Since(inv.CreatedAt.Time) > 24*time.Hour {
			rows[i].Status = "Expired"
		} else {
			rows[i].Status = "Pending"
		}
	}

	a.render(w, "invite", InvitePageData{
		BaseData:    a.baseData(r),
		Tab:         tab,
		Invitations: rows,
	})
}

func (a *App) inviteByEmail(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if a.InviteLimiter != nil {
		if !a.InviteLimiter.Allow(strconv.FormatInt(current.User.ID, 10)) {
			a.renderInvitePage(w, r, "email", "", "", "Too many invitations. Please try again later.")
			return
		}
	}

	if err := r.ParseForm(); err != nil {
		a.renderInvitePage(w, r, "email", "", "", "Invalid request.")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	if email == "" {
		a.renderInvitePage(w, r, "email", email, "", "Please enter an e-mail address.")
		return
	}

	// Check if email is already registered.
	_, err := a.Queries.GetUserByEmail(r.Context(), email)
	if err == nil {
		a.renderInvitePage(w, r, "email", email, "", "A user with that e-mail already exists.")
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.serverError(w, r, "check email exists", err)
		return
	}

	token, err := generateInviteToken()
	if err != nil {
		a.serverError(w, r, "generate invite token", err)
		return
	}

	_, err = a.Queries.CreateInvitation(r.Context(), store.CreateInvitationParams{
		InviterID: current.User.ID,
		Email:     pgtype.Text{String: email, Valid: true},
		TokenHash: auth.HashToken(token),
	})
	if err != nil {
		a.serverError(w, r, "create invitation", err)
		return
	}

	inviteURL := a.AppURL + "/register/" + token

	tmpl, ok := a.EmailTemplates["invitation"]
	if !ok {
		a.serverError(w, r, "email template not found", errors.New("invitation template missing"))
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, struct {
		InviterName string
		InviteUrl   string
	}{
		InviterName: current.User.Username,
		InviteUrl:   inviteURL,
	})
	if err != nil {
		a.serverError(w, r, "render email template", err)
		return
	}

	go func() {
		if sendErr := a.EmailSender.Send(context.Background(), email, current.User.Username+" invited you to Crow Watch", buf.String()); sendErr != nil {
			a.Log.Error("send invitation email", "error", sendErr, "email", email)
		}
	}()

	a.renderInvitePage(w, r, "email", "", "", "")
	return
}

func (a *App) inviteByLink(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if a.InviteLimiter != nil {
		if !a.InviteLimiter.Allow(strconv.FormatInt(current.User.ID, 10)) {
			a.renderInvitePage(w, r, "link", "", "", "Too many invitations. Please try again later.")
			return
		}
	}

	token, err := generateInviteToken()
	if err != nil {
		a.serverError(w, r, "generate invite token", err)
		return
	}

	_, err = a.Queries.CreateInvitation(r.Context(), store.CreateInvitationParams{
		InviterID: current.User.ID,
		TokenHash: auth.HashToken(token),
	})
	if err != nil {
		a.serverError(w, r, "create invitation", err)
		return
	}

	inviteURL := a.AppURL + "/register/" + token
	a.renderInvitePage(w, r, "link", "", inviteURL, "")
}

func (a *App) renderInvitePage(w http.ResponseWriter, r *http.Request, tab, email, inviteURL, errMsg string) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	invitations, _ := a.Queries.ListInvitationsByUser(r.Context(), current.User.ID)
	rows := make([]InviteRow, len(invitations))
	for i, inv := range invitations {
		rows[i] = InviteRow{
			CreatedAt: inv.CreatedAt.Time,
		}
		if inv.Email.Valid {
			rows[i].Email = inv.Email.String
		}
		if inv.RegisteredUsername.Valid {
			rows[i].RegisteredUsername = inv.RegisteredUsername.String
			rows[i].Status = "Registered"
		} else if time.Since(inv.CreatedAt.Time) > 24*time.Hour {
			rows[i].Status = "Expired"
		} else {
			rows[i].Status = "Pending"
		}
	}

	data := InvitePageData{
		BaseData:    a.baseData(r),
		Tab:         tab,
		Email:       email,
		InviteURL:   inviteURL,
		Invitations: rows,
	}
	if errMsg != "" {
		data.Error = errMsg
	} else if inviteURL != "" {
		data.Success = "Invite link generated!"
	} else if tab == "email" && email == "" && errMsg == "" {
		data.Success = "Invitation sent!"
	}

	a.render(w, "invite", data)
}

func generateInviteToken() (string, error) {
	buf := make([]byte, 15)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

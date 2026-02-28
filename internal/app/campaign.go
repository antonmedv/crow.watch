package app

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"crow.watch/internal/auth"
	"crow.watch/internal/store"

	"github.com/jackc/pgx/v5"
)

var slugRegexp = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func (a *App) campaignsPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	campaigns, err := a.Queries.ListCampaigns(r.Context())
	if err != nil {
		a.serverError(w, r, "list campaigns", err)
		return
	}

	rows := make([]CampaignRow, len(campaigns))
	for i, c := range campaigns {
		rows[i] = CampaignRow{
			ID:              c.ID,
			Slug:            c.Slug,
			SponsorName:     c.SponsorName,
			CreatedByName:   c.CreatedByName,
			Active:          c.Active,
			RegisteredCount: c.RegisteredCount,
			CreatedAt:       c.CreatedAt.Time,
		}
	}

	a.render(w, "campaigns", CampaignsPageData{
		BaseData:  a.baseData(r),
		Campaigns: rows,
	})
}

func (a *App) createCampaign(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		a.renderCampaignsPage(w, r, "", "", "", "Invalid request.")
		return
	}

	slug := strings.TrimSpace(r.FormValue("slug"))
	welcomeMessage := strings.TrimSpace(r.FormValue("welcome_message"))
	sponsorUsername := strings.TrimSpace(r.FormValue("sponsor_username"))

	errs := make(map[string]string)

	if slug == "" {
		errs["slug"] = "Slug is required."
	} else if len(slug) < 2 || len(slug) > 30 {
		errs["slug"] = "Slug must be 2-30 characters."
	} else if !slugRegexp.MatchString(slug) {
		errs["slug"] = "Slug may only contain lowercase letters, numbers, and hyphens."
	}

	if sponsorUsername == "" {
		errs["sponsor_username"] = "Sponsor username is required."
	}

	if len(errs) > 0 {
		a.renderCampaignsPage(w, r, slug, welcomeMessage, sponsorUsername, errs["slug"]+errs["sponsor_username"])
		return
	}

	sponsor, err := a.Queries.GetUserByLogin(r.Context(), sponsorUsername)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			a.renderCampaignsPage(w, r, slug, welcomeMessage, sponsorUsername, "Sponsor user not found.")
			return
		}
		a.serverError(w, r, "get sponsor user", err)
		return
	}

	_, err = a.Queries.CreateCampaign(r.Context(), store.CreateCampaignParams{
		Slug:           slug,
		WelcomeMessage: welcomeMessage,
		SponsorID:      sponsor.ID,
		CreatedByID:    current.User.ID,
	})
	if err != nil {
		if strings.Contains(err.Error(), "campaigns_slug_unique") {
			a.renderCampaignsPage(w, r, slug, welcomeMessage, sponsorUsername, "That slug is already taken.")
			return
		}
		a.serverError(w, r, "create campaign", err)
		return
	}

	http.Redirect(w, r, "/mod/campaigns", http.StatusSeeOther)
}

func (a *App) renderCampaignsPage(w http.ResponseWriter, r *http.Request, slug, welcomeMessage, sponsorUsername, errMsg string) {
	campaigns, _ := a.Queries.ListCampaigns(r.Context())
	rows := make([]CampaignRow, len(campaigns))
	for i, c := range campaigns {
		rows[i] = CampaignRow{
			ID:              c.ID,
			Slug:            c.Slug,
			SponsorName:     c.SponsorName,
			CreatedByName:   c.CreatedByName,
			Active:          c.Active,
			RegisteredCount: c.RegisteredCount,
			CreatedAt:       c.CreatedAt.Time,
		}
	}

	a.render(w, "campaigns", CampaignsPageData{
		BaseData:        a.baseData(r),
		Campaigns:       rows,
		Slug:            slug,
		WelcomeMessage:  welcomeMessage,
		SponsorUsername: sponsorUsername,
		Error:           errMsg,
	})
}

func (a *App) toggleCampaign(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/mod/campaigns", http.StatusSeeOther)
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/mod/campaigns", http.StatusSeeOther)
		return
	}

	active := r.FormValue("active") == "true"

	err = a.Queries.SetCampaignActive(r.Context(), store.SetCampaignActiveParams{
		Active: active,
		ID:     id,
	})
	if err != nil {
		a.serverError(w, r, "toggle campaign", err)
		return
	}

	http.Redirect(w, r, "/mod/campaigns", http.StatusSeeOther)
}

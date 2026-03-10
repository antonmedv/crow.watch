package app

import (
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/analytics"
	"crow.watch/internal/auth"
	"crow.watch/internal/store"
)

type AnalyticsPageData struct {
	Base            Base
	Period          string
	Stats           AnalyticsStats
	Chart           []ChartPoint
	ChartMax        int
	UserChart       []UserChartPoint
	UserChartMax    int
	TopPages        []PageStat
	Referrers       []ReferrerGroup
	Devices         []BreakdownItem
	Browsers        []BreakdownItem
	UserActivity    UserActivityStats
	TopContributors []UserStat
	TopCommenters   []UserStat
}

type AnalyticsStats struct {
	Views    int
	Visitors int
}

type UserActivityStats struct {
	ActiveUsers int
	NewUsers    int
	NewStories  int
	NewComments int
}

type UserStat struct {
	Username string
	Count    int
}

type ChartPoint struct {
	Label    string
	Views    int
	Visitors int
}

type UserChartPoint struct {
	Label       string
	ActiveUsers int
}

type PageStat struct {
	Path     string
	Views    int
	Visitors int
}

type ReferrerGroup struct {
	Domain string
	Hits   int
	URLs   []ReferrerURL
}

type ReferrerURL struct {
	Path string
	Hits int
}

type BreakdownItem struct {
	Name    string
	Count   int
	Percent float64
}

func (a *App) analyticsPage(w http.ResponseWriter, r *http.Request) {
	current, ok := auth.UserFromContext(r.Context())
	if !ok || !current.User.IsModerator {
		a.notFound(w, r)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "7d"
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	var data AnalyticsPageData
	data.Base = a.baseData(r)
	data.Period = period

	switch period {
	case "today":
		data.Stats, data.TopPages, data.Referrers, data.Devices, data.Browsers = a.liveAnalytics(r, today)
	case "7d":
		start := today.AddDate(0, 0, -6)
		data.Stats, data.Chart, data.TopPages, data.Referrers = a.rangeAnalytics(r, start, today)
		data.Devices, data.Browsers = a.liveBreakdowns(r, start)
	case "30d":
		start := today.AddDate(0, 0, -29)
		data.Stats, data.Chart, data.TopPages, data.Referrers = a.rangeAnalytics(r, start, today)
		data.Devices, data.Browsers = a.liveBreakdowns(r, start)
	default:
		period = "7d"
		data.Period = period
		start := today.AddDate(0, 0, -6)
		data.Stats, data.Chart, data.TopPages, data.Referrers = a.rangeAnalytics(r, start, today)
		data.Devices, data.Browsers = a.liveBreakdowns(r, start)
	}

	for _, pt := range data.Chart {
		if pt.Views > data.ChartMax {
			data.ChartMax = pt.Views
		}
	}

	var since time.Time
	switch period {
	case "today":
		since = today
	case "30d":
		since = today.AddDate(0, 0, -29)
	default:
		since = today.AddDate(0, 0, -6)
	}
	data.UserActivity, data.TopContributors, data.TopCommenters = a.userAnalytics(r, since)

	if period != "today" {
		data.UserChart = a.userChart(r, since, today)
		for _, pt := range data.UserChart {
			if pt.ActiveUsers > data.UserChartMax {
				data.UserChartMax = pt.ActiveUsers
			}
		}
	}

	a.render(w, "analytics", data)
}

func (a *App) analyticsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if a.Analytics != nil && analytics.ShouldTrack(r) {
			a.Analytics.Record(r)
		}
	})
}

func (a *App) liveAnalytics(r *http.Request, since time.Time) (AnalyticsStats, []PageStat, []ReferrerGroup, []BreakdownItem, []BreakdownItem) {
	sinceTS := pgtype.Timestamptz{Time: since, Valid: true}

	var stats AnalyticsStats
	if row, err := a.Queries.GetLiveStats(r.Context(), sinceTS); err == nil {
		stats = AnalyticsStats{Views: int(row.Views), Visitors: int(row.Visitors)}
	}

	var pages []PageStat
	if rows, err := a.Queries.GetLiveTopPages(r.Context(), store.GetLiveTopPagesParams{
		Since: sinceTS, MaxResults: 10,
	}); err == nil {
		for _, row := range rows {
			pages = append(pages, PageStat{Path: row.Path, Views: int(row.Views), Visitors: int(row.Visitors)})
		}
	}

	var referrers []ReferrerGroup
	if domains, err := a.Queries.GetLiveTopReferrers(r.Context(), store.GetLiveTopReferrersParams{
		Since: sinceTS, MaxResults: 10,
	}); err == nil {
		urls, _ := a.Queries.GetLiveReferrerURLs(r.Context(), sinceTS)
		referrers = buildReferrerGroups(domains, urls)
	}

	var devices []BreakdownItem
	if rows, err := a.Queries.GetLiveDeviceBreakdown(r.Context(), sinceTS); err == nil {
		devices = toBreakdown(rows, func(r store.GetLiveDeviceBreakdownRow) (string, int) { return r.Device, int(r.Count) })
	}

	var browsers []BreakdownItem
	if rows, err := a.Queries.GetLiveBrowserBreakdown(r.Context(), sinceTS); err == nil {
		browsers = toBreakdown(rows, func(r store.GetLiveBrowserBreakdownRow) (string, int) { return r.Browser, int(r.Count) })
	}

	return stats, pages, referrers, devices, browsers
}

func (a *App) rangeAnalytics(r *http.Request, start, end time.Time) (AnalyticsStats, []ChartPoint, []PageStat, []ReferrerGroup) {
	startDate := pgtype.Date{Time: start, Valid: true}
	endDate := pgtype.Date{Time: end, Valid: true}

	var stats AnalyticsStats
	if row, err := a.Queries.GetDailyStatsTotals(r.Context(), store.GetDailyStatsTotalsParams{
		StartDate: startDate, EndDate: endDate,
	}); err == nil {
		stats = AnalyticsStats{Views: int(row.Views), Visitors: int(row.Visitors)}
	}

	// Add today's live stats
	todayStart := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, time.UTC)
	sinceTS := pgtype.Timestamptz{Time: todayStart, Valid: true}
	if row, err := a.Queries.GetLiveStats(r.Context(), sinceTS); err == nil {
		stats.Views += int(row.Views)
		stats.Visitors += int(row.Visitors)
	}

	var chart []ChartPoint
	if rows, err := a.Queries.GetDailyStatsRange(r.Context(), store.GetDailyStatsRangeParams{
		StartDate: startDate, EndDate: endDate,
	}); err == nil {
		dayMap := make(map[string]ChartPoint)
		for _, row := range rows {
			key := row.Date.Time.Format("Jan 2")
			dayMap[key] = ChartPoint{Label: key, Views: int(row.Views), Visitors: int(row.Visitors)}
		}
		for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
			key := d.Format("Jan 2")
			if pt, ok := dayMap[key]; ok {
				chart = append(chart, pt)
			} else {
				chart = append(chart, ChartPoint{Label: key})
			}
		}
	}

	var pages []PageStat
	if rows, err := a.Queries.GetTopPagesRange(r.Context(), store.GetTopPagesRangeParams{
		StartDate: startDate, EndDate: endDate, MaxResults: 10,
	}); err == nil {
		for _, row := range rows {
			pages = append(pages, PageStat{Path: row.Path, Views: int(row.Views), Visitors: int(row.Visitors)})
		}
	}

	var referrers []ReferrerGroup
	if domains, err := a.Queries.GetTopReferrersRange(r.Context(), store.GetTopReferrersRangeParams{
		StartDate: startDate, EndDate: endDate, MaxResults: 10,
	}); err == nil {
		urls, _ := a.Queries.GetReferrerURLsRange(r.Context(), store.GetReferrerURLsRangeParams{
			StartDate: startDate, EndDate: endDate,
		})
		referrers = buildReferrerGroupsRange(domains, urls)
	}

	return stats, chart, pages, referrers
}

func (a *App) liveBreakdowns(r *http.Request, since time.Time) ([]BreakdownItem, []BreakdownItem) {
	sinceTS := pgtype.Timestamptz{Time: since, Valid: true}

	var devices []BreakdownItem
	if rows, err := a.Queries.GetLiveDeviceBreakdown(r.Context(), sinceTS); err == nil {
		devices = toBreakdown(rows, func(r store.GetLiveDeviceBreakdownRow) (string, int) { return r.Device, int(r.Count) })
	}

	var browsers []BreakdownItem
	if rows, err := a.Queries.GetLiveBrowserBreakdown(r.Context(), sinceTS); err == nil {
		browsers = toBreakdown(rows, func(r store.GetLiveBrowserBreakdownRow) (string, int) { return r.Browser, int(r.Count) })
	}

	return devices, browsers
}

func (a *App) userChart(r *http.Request, start, end time.Time) []UserChartPoint {
	startDate := pgtype.Date{Time: start, Valid: true}
	endDate := pgtype.Date{Time: end, Valid: true}

	rows, err := a.Queries.GetDailyUserStatsRange(r.Context(), store.GetDailyUserStatsRangeParams{
		StartDate: startDate, EndDate: endDate,
	})
	if err != nil {
		return nil
	}

	dayMap := make(map[string]int, len(rows))
	for _, row := range rows {
		dayMap[row.Date.Time.Format("Jan 2")] = int(row.ActiveUsers)
	}

	var chart []UserChartPoint
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format("Jan 2")
		chart = append(chart, UserChartPoint{Label: key, ActiveUsers: dayMap[key]})
	}
	return chart
}

func (a *App) userAnalytics(r *http.Request, since time.Time) (UserActivityStats, []UserStat, []UserStat) {
	sinceTS := pgtype.Timestamptz{Time: since, Valid: true}

	var activity UserActivityStats
	if row, err := a.Queries.GetUserActivityStats(r.Context(), sinceTS); err == nil {
		activity = UserActivityStats{
			ActiveUsers: int(row.ActiveUsers),
			NewUsers:    int(row.NewUsers),
			NewStories:  int(row.NewStories),
			NewComments: int(row.NewComments),
		}
	}

	var contributors []UserStat
	if rows, err := a.Queries.GetTopContributors(r.Context(), store.GetTopContributorsParams{
		Since: sinceTS, MaxResults: 10,
	}); err == nil {
		for _, row := range rows {
			contributors = append(contributors, UserStat{Username: row.Username, Count: int(row.Stories)})
		}
	}

	var commenters []UserStat
	if rows, err := a.Queries.GetTopCommenters(r.Context(), store.GetTopCommentersParams{
		Since: sinceTS, MaxResults: 10,
	}); err == nil {
		for _, row := range rows {
			commenters = append(commenters, UserStat{Username: row.Username, Count: int(row.Comments)})
		}
	}

	return activity, contributors, commenters
}

func buildReferrerGroups(domains []store.GetLiveTopReferrersRow, urls []store.GetLiveReferrerURLsRow) []ReferrerGroup {
	urlsByDomain := make(map[string][]ReferrerURL)
	for _, u := range urls {
		urlsByDomain[u.ReferrerDomain] = append(urlsByDomain[u.ReferrerDomain], ReferrerURL{
			Path: strings.TrimPrefix(u.ReferrerUrl, u.ReferrerDomain),
			Hits: int(u.Hits),
		})
	}
	groups := make([]ReferrerGroup, 0, len(domains))
	for _, d := range domains {
		groups = append(groups, ReferrerGroup{
			Domain: d.ReferrerDomain,
			Hits:   int(d.Hits),
			URLs:   urlsByDomain[d.ReferrerDomain],
		})
	}
	return groups
}

func buildReferrerGroupsRange(domains []store.GetTopReferrersRangeRow, urls []store.GetReferrerURLsRangeRow) []ReferrerGroup {
	urlsByDomain := make(map[string][]ReferrerURL)
	for _, u := range urls {
		urlsByDomain[u.ReferrerDomain] = append(urlsByDomain[u.ReferrerDomain], ReferrerURL{
			Path: strings.TrimPrefix(u.ReferrerUrl, u.ReferrerDomain),
			Hits: int(u.Hits),
		})
	}
	groups := make([]ReferrerGroup, 0, len(domains))
	for _, d := range domains {
		groups = append(groups, ReferrerGroup{
			Domain: d.ReferrerDomain,
			Hits:   int(d.Hits),
			URLs:   urlsByDomain[d.ReferrerDomain],
		})
	}
	return groups
}

func toBreakdown[T any](rows []T, extract func(T) (string, int)) []BreakdownItem {
	var total int
	items := make([]BreakdownItem, 0, len(rows))
	for _, r := range rows {
		name, count := extract(r)
		total += count
		items = append(items, BreakdownItem{Name: name, Count: count})
	}
	if total > 0 {
		for i := range items {
			items[i].Percent = float64(items[i].Count) / float64(total) * 100
		}
	}
	return items
}

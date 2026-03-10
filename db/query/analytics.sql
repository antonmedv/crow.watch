-- name: InsertPageView :exec
INSERT INTO page_views (path, visitor_id, referrer, device, browser, os, is_bot)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: GetLiveStats :one
SELECT
    COUNT(*)::int AS views,
    COUNT(DISTINCT visitor_id)::int AS visitors
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot;

-- name: GetLiveTopPages :many
SELECT
    path,
    COUNT(*)::int AS views,
    COUNT(DISTINCT visitor_id)::int AS visitors
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
GROUP BY path
ORDER BY views DESC
LIMIT @max_results::int;

-- name: GetLiveTopReferrers :many
SELECT
    referrer AS referrer_domain,
    COUNT(*)::int AS hits
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND referrer != ''
GROUP BY referrer
ORDER BY hits DESC
LIMIT @max_results::int;

-- name: GetLiveDeviceBreakdown :many
SELECT
    device,
    COUNT(*)::int AS count
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND device != ''
GROUP BY device
ORDER BY count DESC;

-- name: GetLiveBrowserBreakdown :many
SELECT
    browser,
    COUNT(*)::int AS count
FROM page_views
WHERE created_at >= @since::timestamptz
  AND NOT is_bot
  AND browser != ''
GROUP BY browser
ORDER BY count DESC;

-- name: GetDailyStatsRange :many
SELECT
    date,
    SUM(views)::int AS views,
    SUM(visitors)::int AS visitors
FROM daily_stats
WHERE date >= @start_date::date AND date <= @end_date::date
GROUP BY date
ORDER BY date;

-- name: GetDailyStatsTotals :one
SELECT
    COALESCE(SUM(views), 0)::int AS views,
    COALESCE(SUM(visitors), 0)::int AS visitors
FROM daily_stats
WHERE date >= @start_date::date AND date <= @end_date::date;

-- name: GetTopPagesRange :many
SELECT
    path,
    SUM(views)::int AS views,
    SUM(visitors)::int AS visitors
FROM daily_stats
WHERE date >= @start_date::date AND date <= @end_date::date
GROUP BY path
ORDER BY views DESC
LIMIT @max_results::int;

-- name: GetTopReferrersRange :many
SELECT
    referrer_domain,
    SUM(hits)::int AS hits
FROM daily_referrers
WHERE date >= @start_date::date AND date <= @end_date::date
GROUP BY referrer_domain
ORDER BY hits DESC
LIMIT @max_results::int;

-- name: AggregatePageViews :exec
INSERT INTO daily_stats (date, path, views, visitors)
SELECT
    @target_date::date,
    path,
    COUNT(*)::int,
    COUNT(DISTINCT visitor_id)::int
FROM page_views
WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz
  AND NOT is_bot
GROUP BY path
ON CONFLICT (date, path) DO UPDATE
SET views = EXCLUDED.views, visitors = EXCLUDED.visitors;

-- name: AggregateReferrers :exec
INSERT INTO daily_referrers (date, referrer_domain, path, hits)
SELECT
    @target_date::date,
    referrer,
    path,
    COUNT(*)::int
FROM page_views
WHERE created_at >= @day_start::timestamptz AND created_at < @day_end::timestamptz
  AND NOT is_bot
  AND referrer != ''
GROUP BY referrer, path
ON CONFLICT (date, referrer_domain, path) DO UPDATE
SET hits = EXCLUDED.hits;

-- name: PurgePageViews :execrows
DELETE FROM page_views
WHERE created_at < @before::timestamptz;

package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/store"
)

const purgeRetention = 365 * 24 * time.Hour

// Aggregate rolls up raw page_views into daily_stats and daily_referrers for the given date.
func Aggregate(ctx context.Context, queries *store.Queries, date time.Time) error {
	dayStart := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	targetDate := pgtype.Date{Time: dayStart, Valid: true}
	start := pgtype.Timestamptz{Time: dayStart, Valid: true}
	end := pgtype.Timestamptz{Time: dayEnd, Valid: true}

	if err := queries.AggregatePageViews(ctx, store.AggregatePageViewsParams{
		TargetDate: targetDate,
		DayStart:   start,
		DayEnd:     end,
	}); err != nil {
		return fmt.Errorf("aggregate page views: %w", err)
	}

	if err := queries.AggregateReferrers(ctx, store.AggregateReferrersParams{
		TargetDate: targetDate,
		DayStart:   start,
		DayEnd:     end,
	}); err != nil {
		return fmt.Errorf("aggregate referrers: %w", err)
	}

	return nil
}

// Purge removes raw page_views older than the given duration.
func Purge(ctx context.Context, queries *store.Queries, olderThan time.Duration) (int64, error) {
	before := pgtype.Timestamptz{Time: time.Now().UTC().Add(-olderThan), Valid: true}
	return queries.PurgePageViews(ctx, before)
}

// RunDailyAggregation runs aggregation and purge once on startup for yesterday,
// then every hour checks if a new day has started and aggregates the previous day.
func RunDailyAggregation(queries *store.Queries, log *slog.Logger, stop <-chan struct{}) {
	lastAggregated := ""

	run := func() {
		yesterday := time.Now().UTC().AddDate(0, 0, -1)
		key := yesterday.Format("2006-01-02")
		if key == lastAggregated {
			return
		}

		ctx := context.Background()

		if err := Aggregate(ctx, queries, yesterday); err != nil {
			log.Error("analytics aggregate", "error", err, "date", key)
			return
		}
		log.Info("analytics aggregated", "date", key)
		lastAggregated = key

		deleted, err := Purge(ctx, queries, purgeRetention)
		if err != nil {
			log.Error("analytics purge", "error", err)
			return
		}
		if deleted > 0 {
			log.Info("analytics purged", "deleted", deleted)
		}
	}

	// Run immediately on startup.
	run()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run()
		case <-stop:
			return
		}
	}
}

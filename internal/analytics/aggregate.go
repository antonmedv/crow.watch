package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"crow.watch/internal/store"
)

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

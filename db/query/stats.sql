-- name: GetSiteStats :one
SELECT
    (SELECT count(*) FROM users WHERE banned_at IS NULL AND deleted_at IS NULL)::bigint AS total_users,
    (SELECT count(*) FROM users WHERE created_at >= CURRENT_DATE AND banned_at IS NULL AND deleted_at IS NULL)::bigint AS users_today,
    (SELECT count(*) FROM users WHERE created_at >= CURRENT_DATE - INTERVAL '7 days' AND banned_at IS NULL AND deleted_at IS NULL)::bigint AS users_this_week,
    (SELECT count(*) FROM stories WHERE deleted_at IS NULL)::bigint AS total_stories,
    (SELECT count(*) FROM stories WHERE created_at >= CURRENT_DATE AND deleted_at IS NULL)::bigint AS stories_today,
    (SELECT count(*) FROM stories WHERE created_at >= CURRENT_DATE - INTERVAL '7 days' AND deleted_at IS NULL)::bigint AS stories_this_week,
    (SELECT count(*) FROM comments WHERE deleted_at IS NULL)::bigint AS total_comments,
    (SELECT count(*) FROM comments WHERE created_at >= CURRENT_DATE AND deleted_at IS NULL)::bigint AS comments_today,
    (SELECT count(*) FROM comments WHERE created_at >= CURRENT_DATE - INTERVAL '7 days' AND deleted_at IS NULL)::bigint AS comments_this_week,
    (SELECT count(DISTINCT user_id) FROM sessions WHERE last_seen_at >= CURRENT_DATE AND expires_at > now())::bigint AS active_users_today,
    (SELECT count(DISTINCT user_id) FROM sessions WHERE last_seen_at >= CURRENT_DATE - INTERVAL '7 days' AND expires_at > now())::bigint AS active_users_this_week;

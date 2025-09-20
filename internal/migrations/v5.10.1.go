package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_10_1 refreshes dashboard charts materialized view with sends/replies/bounces datasets.
func V5_10_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        DROP MATERIALIZED VIEW IF EXISTS mat_dashboard_charts;
        CREATE MATERIALIZED VIEW mat_dashboard_charts AS
            WITH
            click_bounds AS (
                SELECT COALESCE(MAX(TIMEZONE('UTC', created_at)::DATE), CURRENT_DATE) AS to_date
                FROM link_clicks
            ),
            click_rows AS (
                SELECT COUNT(*) AS count, TIMEZONE('UTC', created_at)::DATE AS date
                FROM link_clicks, click_bounds
                WHERE TIMEZONE('UTC', created_at)::DATE BETWEEN (click_bounds.to_date - INTERVAL '30 DAY') AND click_bounds.to_date
                GROUP BY date
                ORDER BY date
            ),
            clicks AS (
                SELECT COALESCE(JSON_AGG(ROW_TO_JSON(click_rows)), '[]'::JSON) AS data FROM click_rows
            ),
            view_bounds AS (
                SELECT COALESCE(MAX(TIMEZONE('UTC', created_at)::DATE), CURRENT_DATE) AS to_date
                FROM campaign_views
            ),
            view_rows AS (
                SELECT COUNT(*) AS count, TIMEZONE('UTC', created_at)::DATE AS date
                FROM campaign_views, view_bounds
                WHERE TIMEZONE('UTC', created_at)::DATE BETWEEN (view_bounds.to_date - INTERVAL '30 DAY') AND view_bounds.to_date
                GROUP BY date
                ORDER BY date
            ),
            views AS (
                SELECT COALESCE(JSON_AGG(ROW_TO_JSON(view_rows)), '[]'::JSON) AS data FROM view_rows
            ),
            send_bounds AS (
                SELECT COALESCE(MAX(TIMEZONE('UTC', created_at)::DATE), CURRENT_DATE) AS to_date
                FROM campaign_sends
            ),
            send_rows AS (
                SELECT COUNT(*) AS count, TIMEZONE('UTC', created_at)::DATE AS date
                FROM campaign_sends, send_bounds
                WHERE TIMEZONE('UTC', created_at)::DATE BETWEEN (send_bounds.to_date - INTERVAL '30 DAY') AND send_bounds.to_date
                GROUP BY date
                ORDER BY date
            ),
            sends AS (
                SELECT COALESCE(JSON_AGG(ROW_TO_JSON(send_rows)), '[]'::JSON) AS data FROM send_rows
            ),
            reply_rows AS (
                SELECT COUNT(*) AS count, TIMEZONE('UTC', reply_at)::DATE AS date
                FROM campaign_sends, send_bounds
                WHERE reply_at IS NOT NULL
                  AND TIMEZONE('UTC', reply_at)::DATE BETWEEN (send_bounds.to_date - INTERVAL '30 DAY') AND send_bounds.to_date
                GROUP BY date
                ORDER BY date
            ),
            replies AS (
                SELECT COALESCE(JSON_AGG(ROW_TO_JSON(reply_rows)), '[]'::JSON) AS data FROM reply_rows
            ),
            bounce_bounds AS (
                SELECT COALESCE(MAX(TIMEZONE('UTC', created_at)::DATE), CURRENT_DATE) AS to_date
                FROM bounces
            ),
            bounce_rows AS (
                SELECT COUNT(*) AS count, TIMEZONE('UTC', created_at)::DATE AS date
                FROM bounces, bounce_bounds
                WHERE TIMEZONE('UTC', created_at)::DATE BETWEEN (bounce_bounds.to_date - INTERVAL '30 DAY') AND bounce_bounds.to_date
                GROUP BY date
                ORDER BY date
            ),
            bounces AS (
                SELECT COALESCE(JSON_AGG(ROW_TO_JSON(bounce_rows)), '[]'::JSON) AS data FROM bounce_rows
            )
            SELECT NOW() AS updated_at,
                   JSON_BUILD_OBJECT(
                       'link_clicks', (SELECT data FROM clicks),
                       'campaign_views', (SELECT data FROM views),
                       'campaign_sends', (SELECT data FROM sends),
                       'campaign_replies', (SELECT data FROM replies),
                       'campaign_bounces', (SELECT data FROM bounces)
                   ) AS data;
        DROP INDEX IF EXISTS mat_dashboard_charts_idx; CREATE UNIQUE INDEX mat_dashboard_charts_idx ON mat_dashboard_charts (updated_at);
    `)
    return err
}

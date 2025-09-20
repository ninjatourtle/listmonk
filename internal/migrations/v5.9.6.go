package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_6 adds optional per-campaign rate window fields
func V5_9_6(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    if _, err := db.Exec(`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS rate_window_count INT NOT NULL DEFAULT 0;`); err != nil {
        return err
    }
    if _, err := db.Exec(`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS rate_window_minutes INT NOT NULL DEFAULT 0;`); err != nil {
        return err
    }
    // Campaign schedule table (window-based)
    if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS campaign_schedule (
            id BIGSERIAL PRIMARY KEY,
            campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
            due_at TIMESTAMPTZ NOT NULL,
            quota INT NOT NULL DEFAULT 0,
            used INT NOT NULL DEFAULT 0,
            status TEXT NOT NULL DEFAULT 'pending', -- pending|done|error
            error TEXT NULL,
            created_at TIMESTAMPTZ DEFAULT NOW(),
            updated_at TIMESTAMPTZ DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_camp_sched_camp_due ON campaign_schedule(campaign_id, due_at);
        CREATE INDEX IF NOT EXISTS idx_camp_sched_status ON campaign_schedule(status);
    `); err != nil {
        return err
    }
    return nil
}

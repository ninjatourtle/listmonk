package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_6_0 adds a campaign_sends table to record individual send events.
func V5_6_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS campaign_sends (
            id            BIGSERIAL PRIMARY KEY,
            campaign_id   INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
            subscriber_id INTEGER NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE ON UPDATE CASCADE,
            step          INTEGER NULL,
            subject       TEXT NOT NULL DEFAULT '',
            from_email    TEXT NOT NULL DEFAULT '',
            created_at    TIMESTAMPTZ DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_campaign_sends_created_at ON campaign_sends(created_at DESC);
        CREATE INDEX IF NOT EXISTS idx_campaign_sends_campaign_id ON campaign_sends(campaign_id);
    `)
    return err
}


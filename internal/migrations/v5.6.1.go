package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_6_1 adds success and error columns to campaign_sends to record failures.
func V5_6_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        ALTER TABLE campaign_sends
            ADD COLUMN IF NOT EXISTS success BOOLEAN NOT NULL DEFAULT TRUE,
            ADD COLUMN IF NOT EXISTS error TEXT NULL;
    `)
    return err
}


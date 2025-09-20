package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_9 adds subscriber_id to campaign_schedule for preassigning recipients.
func V5_9_9(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS subscriber_id INT NULL`); err != nil {
        return err
    }
    return nil
}


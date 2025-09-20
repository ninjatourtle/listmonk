package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_8 adds email_from, email_to, subject columns to campaign_schedule for auditing sent slots.
func V5_9_8(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS email_from TEXT NULL`); err != nil {
        return err
    }
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS email_to TEXT NULL`); err != nil {
        return err
    }
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS subject TEXT NULL`); err != nil {
        return err
    }
    return nil
}


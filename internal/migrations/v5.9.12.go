package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_12 adds subject field to warmcloud_schedule for storing email subject from campaign.
func V5_9_12(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Add subject field to warmcloud_schedule table
    if _, err := db.Exec(`ALTER TABLE warmcloud_schedule ADD COLUMN IF NOT EXISTS subject TEXT NULL`); err != nil {
        return err
    }

    // Create index on subject for better performance
    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wcs_subject ON warmcloud_schedule(subject)`); err != nil {
        return err
    }

    return nil
}

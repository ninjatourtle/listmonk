package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_13 adds persona metadata to warmcloud_schedule slots.
func V5_9_13(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    if _, err := db.Exec(`ALTER TABLE warmcloud_schedule ADD COLUMN IF NOT EXISTS persona TEXT NULL`); err != nil {
        return err
    }
    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wcs_persona ON warmcloud_schedule(persona)`); err != nil {
        return err
    }
    return nil
}


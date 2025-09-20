package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_5 adds an optional email column to warmcloud_schedule to record recipient for done slots.
func V5_9_5(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`ALTER TABLE warmcloud_schedule ADD COLUMN IF NOT EXISTS email TEXT NULL;`)
    return err
}


package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_1 adds from_name column to campaigns for dedicated sender name.
func V5_9_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS from_name TEXT NULL;`)
    return err
}


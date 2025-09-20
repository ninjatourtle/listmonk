package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_0 adds persona_domain column to campaigns for persona-based sending.
func V5_9_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS persona_domain TEXT NULL;`)
    return err
}


package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_7 adds persona_account to campaigns
func V5_9_7(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS persona_account TEXT NULL;`)
    return err
}


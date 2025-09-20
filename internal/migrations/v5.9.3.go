package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_3 adds status column to smtp_send_events to track sent/queued.
func V5_9_3(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`ALTER TABLE smtp_send_events ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'sent';`)
    return err
}


package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_2 adds from_email to campaign_sequence_progress to keep a stable sender per subscriber for sequences.
func V5_9_2(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`ALTER TABLE campaign_sequence_progress ADD COLUMN IF NOT EXISTS from_email TEXT NULL;`)
    return err
}


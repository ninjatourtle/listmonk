package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_5_1 makes campaign_sequence_progress.next_send_at nullable to allow terminal steps.
func V5_5_1(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        ALTER TABLE campaign_sequence_progress
            ALTER COLUMN next_send_at DROP NOT NULL;
    `)
    return err
}


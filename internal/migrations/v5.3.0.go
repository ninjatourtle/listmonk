package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_3_0 adds validation fields to subscribers and email_domain_labels table.
func V5_3_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Add columns if not exist (Postgres supports ADD COLUMN IF NOT EXISTS).
    if _, err := db.Exec(`
        ALTER TABLE subscribers
            ADD COLUMN IF NOT EXISTS validation_status TEXT NULL,
            ADD COLUMN IF NOT EXISTS validation_reason TEXT NULL,
            ADD COLUMN IF NOT EXISTS validated_at TIMESTAMPTZ NULL,
            ADD COLUMN IF NOT EXISTS email_type TEXT NULL;
        CREATE INDEX IF NOT EXISTS idx_subs_validated_at ON subscribers(validated_at);
    `); err != nil {
        return err
    }

    // Create labels table if it doesn't exist.
    if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS email_domain_labels (
            domain          TEXT PRIMARY KEY,
            label           TEXT NOT NULL,
            disposable      BOOLEAN NOT NULL DEFAULT FALSE,
            free            BOOLEAN NOT NULL DEFAULT FALSE,
            public_private  TEXT NULL,
            updated_at      TIMESTAMPTZ DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_email_domain_labels_label ON email_domain_labels(label);
    `); err != nil {
        return err
    }

    return nil
}


package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_2_0 adds smtp_stats table to persist per-SMTP daily send counts.
func V5_2_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Create table if it doesn't exist (idempotent).
    if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS smtp_stats (
            name TEXT NOT NULL,
            day  DATE NOT NULL,
            sent BIGINT NOT NULL DEFAULT 0,
            PRIMARY KEY(name, day)
        );
    `); err != nil {
        return err
    }

    return nil
}


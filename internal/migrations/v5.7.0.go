package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_7_0 adds warmcloud_events table to persist WarmCloud warming events.
func V5_7_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS warmcloud_events (
            id           BIGSERIAL PRIMARY KEY,
            created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            server       TEXT NOT NULL,
            email        TEXT NOT NULL,
            subject      TEXT NOT NULL DEFAULT '',
            submitted    BOOLEAN NOT NULL DEFAULT FALSE,
            error        TEXT NULL,
            submit_token TEXT UNIQUE
        );
        CREATE INDEX IF NOT EXISTS idx_warmcloud_events_created_at ON warmcloud_events(created_at DESC);
        CREATE INDEX IF NOT EXISTS idx_warmcloud_events_server ON warmcloud_events(server);
        CREATE INDEX IF NOT EXISTS idx_warmcloud_events_email ON warmcloud_events(email);
    `)
    return err
}


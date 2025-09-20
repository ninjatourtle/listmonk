package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_8_0 adds smtp_send_events table to persist generic SMTP send events per server.
func V5_8_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS smtp_send_events (
            id         BIGSERIAL PRIMARY KEY,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            server     TEXT NOT NULL,
            email      TEXT NOT NULL,
            subject    TEXT NOT NULL DEFAULT ''
        );
        CREATE INDEX IF NOT EXISTS idx_smtp_send_events_created_at ON smtp_send_events(created_at DESC);
        CREATE INDEX IF NOT EXISTS idx_smtp_send_events_server ON smtp_send_events(server);
        CREATE INDEX IF NOT EXISTS idx_smtp_send_events_email ON smtp_send_events(email);
    `)
    return err
}


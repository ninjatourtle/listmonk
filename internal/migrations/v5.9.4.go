package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_4 adds warmcloud_schedule table for per-server scheduled warming slots.
func V5_9_4(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS warmcloud_schedule (
            id BIGSERIAL PRIMARY KEY,
            server TEXT NOT NULL,
            due_at TIMESTAMPTZ NOT NULL,
            status TEXT NOT NULL DEFAULT 'pending', -- pending|done|error
            error TEXT NULL,
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
            updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_wcs_due ON warmcloud_schedule (due_at);
        CREATE INDEX IF NOT EXISTS idx_wcs_server_due ON warmcloud_schedule (server, due_at);
        CREATE INDEX IF NOT EXISTS idx_wcs_status_due ON warmcloud_schedule (status, due_at);
        -- Ensure we don't insert duplicate slots for a server at the same time
        DO $$ BEGIN
            ALTER TABLE warmcloud_schedule ADD CONSTRAINT uq_wcs_server_due UNIQUE(server, due_at);
        EXCEPTION WHEN duplicate_table THEN
            NULL;
        END $$;
        `)
    return err
}


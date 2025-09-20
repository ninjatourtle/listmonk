package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_5_0 introduces per-subscriber sequence progress table for sequence campaigns.
func V5_5_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Create table if not exists with necessary indexes.
    _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS campaign_sequence_progress (
            campaign_id   INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
            subscriber_id INTEGER NOT NULL REFERENCES subscribers(id) ON DELETE CASCADE ON UPDATE CASCADE,
            current_step  INTEGER NOT NULL DEFAULT 0,
            next_send_at  TIMESTAMPTZ NOT NULL,
            last_sent_at  TIMESTAMPTZ NULL,
            status        TEXT NOT NULL DEFAULT 'pending',
            PRIMARY KEY (campaign_id, subscriber_id)
        );
        CREATE INDEX IF NOT EXISTS idx_seq_prog_camp_next ON campaign_sequence_progress(campaign_id, next_send_at);
        CREATE INDEX IF NOT EXISTS idx_seq_prog_status ON campaign_sequence_progress(status);
    `)
    return err
}


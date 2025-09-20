package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_10_0 introduces reply tracking tables and columns.
func V5_10_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    if _, err := db.Exec(`
        ALTER TABLE campaign_sends
            ADD COLUMN IF NOT EXISTS reply_token TEXT NOT NULL DEFAULT '',
            ADD COLUMN IF NOT EXISTS message_id TEXT NOT NULL DEFAULT '',
            ADD COLUMN IF NOT EXISTS reply_at TIMESTAMPTZ NULL;
        CREATE INDEX IF NOT EXISTS idx_campaign_sends_reply_token ON campaign_sends(reply_token);
        CREATE INDEX IF NOT EXISTS idx_campaign_sends_message_id ON campaign_sends(message_id);
    `); err != nil {
        return err
    }

    if _, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS campaign_replies (
            id BIGSERIAL PRIMARY KEY,
            campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE ON UPDATE CASCADE,
            subscriber_id INTEGER NULL REFERENCES subscribers(id) ON DELETE SET NULL ON UPDATE CASCADE,
            send_id BIGINT NULL REFERENCES campaign_sends(id) ON DELETE SET NULL ON UPDATE CASCADE,
            email TEXT NOT NULL,
            reply_token TEXT NULL,
            message_id TEXT NULL,
            in_reply_to TEXT NULL,
            subject TEXT NOT NULL DEFAULT '',
            snippet TEXT NOT NULL DEFAULT '',
            headers JSONB NOT NULL DEFAULT '{}',
            created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
        );
        CREATE INDEX IF NOT EXISTS idx_campaign_replies_campaign ON campaign_replies(campaign_id, created_at DESC);
        CREATE INDEX IF NOT EXISTS idx_campaign_replies_token ON campaign_replies(reply_token);
        CREATE INDEX IF NOT EXISTS idx_campaign_replies_message_id ON campaign_replies(message_id);
    `); err != nil {
        return err
    }

    return nil
}


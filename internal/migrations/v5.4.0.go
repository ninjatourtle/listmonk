package migrations

import (
    "log"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_4_0 adds 'sequence' campaign type and the 'sequence' JSONB column on campaigns.
func V5_4_0(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Add new enum value and column if not exists.
    if _, err := db.Exec(`
        DO $$
        BEGIN
            IF NOT EXISTS (
                SELECT 1 FROM pg_type t
                JOIN pg_enum e ON t.oid = e.enumtypid
                WHERE t.typname = 'campaign_type' AND e.enumlabel = 'sequence'
            ) THEN
                ALTER TYPE campaign_type ADD VALUE 'sequence';
            END IF;
        END$$;

        ALTER TABLE campaigns
            ADD COLUMN IF NOT EXISTS sequence JSONB NOT NULL DEFAULT '[]';
    `); err != nil {
        return err
    }

    return nil
}


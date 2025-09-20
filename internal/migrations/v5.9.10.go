package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_10 adds email_to, email_from, and subject fields to campaign_schedule for pre-assigned recipient information.
func V5_9_10(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Add email_to field
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS email_to TEXT NULL`); err != nil {
        return err
    }
    
    // Add email_from field
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS email_from TEXT NULL`); err != nil {
        return err
    }
    
    // Add subject field
    if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD COLUMN IF NOT EXISTS subject TEXT NULL`); err != nil {
        return err
    }
    
    // Add foreign key constraint for subscriber_id if it doesn't exist
    // Check if constraint already exists before adding it
    var constraintExists bool
    err := db.Get(&constraintExists, `
        SELECT EXISTS (
            SELECT 1 FROM information_schema.table_constraints 
            WHERE constraint_name = 'fk_campaign_schedule_subscriber_id' 
            AND table_name = 'campaign_schedule'
        )
    `)
    if err != nil {
        return err
    }
    
    if !constraintExists {
        if _, err := db.Exec(`ALTER TABLE campaign_schedule ADD CONSTRAINT fk_campaign_schedule_subscriber_id FOREIGN KEY (subscriber_id) REFERENCES subscribers(id) ON DELETE SET NULL ON UPDATE CASCADE`); err != nil {
            return err
        }
    }
    
    // Create index on subscriber_id for better performance
    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_camp_sched_subscriber_id ON campaign_schedule(subscriber_id)`); err != nil {
        return err
    }
    
    return nil
}

package migrations

import (
    "log"
    "github.com/jmoiron/sqlx"
    "github.com/knadh/koanf/v2"
    "github.com/knadh/stuffbin"
)

// V5_9_11 adds from_email field to warmcloud_schedule for storing sender email address.
func V5_9_11(db *sqlx.DB, fs stuffbin.FileSystem, ko *koanf.Koanf, lo *log.Logger) error {
    // Add from_email field to warmcloud_schedule table
    if _, err := db.Exec(`ALTER TABLE warmcloud_schedule ADD COLUMN IF NOT EXISTS from_email TEXT NULL`); err != nil {
        return err
    }
    
    // Create index on from_email for better performance
    if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wcs_from_email ON warmcloud_schedule(from_email)`); err != nil {
        return err
    }
    
    return nil
}

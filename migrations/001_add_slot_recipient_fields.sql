-- Migration to add recipient fields to campaign_schedule table
-- This migration adds fields for pre-assigned recipient information

-- Add new columns to campaign_schedule table
ALTER TABLE campaign_schedule 
ADD COLUMN IF NOT EXISTS subscriber_id INTEGER NULL REFERENCES subscribers(id) ON DELETE SET NULL ON UPDATE CASCADE,
ADD COLUMN IF NOT EXISTS email_to TEXT NULL,
ADD COLUMN IF NOT EXISTS email_from TEXT NULL,
ADD COLUMN IF NOT EXISTS subject TEXT NULL;

-- Create index on subscriber_id for better performance
CREATE INDEX IF NOT EXISTS idx_camp_sched_subscriber_id ON campaign_schedule(subscriber_id);

package core

import (
    "github.com/knadh/listmonk/models"
)

// QueryRecentSends returns recent send events optionally filtered by campaign ID.
func (c *Core) QueryRecentSends(campaignID, limit int) ([]models.SendLog, error) {
    out := []models.SendLog{}
    if err := c.q.QueryRecentSends.Select(&out, campaignID, limit); err != nil {
        return nil, err
    }
    return out, nil
}


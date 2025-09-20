package core

import (
    "net/http"
    "strings"

    "github.com/knadh/listmonk/models"
    "github.com/labstack/echo/v4"
)

// RecordReply persists a reply event and marks the corresponding send as replied.
func (c *Core) RecordReply(r models.Reply) error {
    // Normalize Message-Id formats: strip angle brackets if present.
    norm := func(s string) string {
        s = strings.TrimSpace(s)
        s = strings.TrimPrefix(s, "<")
        s = strings.TrimSuffix(s, ">")
        return s
    }

    var (
        sendID      int64
        campID      int
        subscriberID int
    )

    // 1) Try by In-Reply-To -> campaign_sends.message_id
    inReplyTo := norm(r.InReplyTo)
    if inReplyTo != "" {
        var row struct{ ID int64; CampaignID, SubscriberID int }
        if err := c.q.GetCampaignSendByMessageID.Get(&row, inReplyTo); err == nil && row.ID > 0 {
            sendID, campID, subscriberID = row.ID, row.CampaignID, row.SubscriberID
        }
    }

    if sendID == 0 {
        return echo.NewHTTPError(http.StatusBadRequest, c.i18n.T("globals.messages.invalidData"))
    }

    // Insert reply row
    var replyID int64
    if err := c.q.InsertCampaignReply.Get(&replyID,
        campID,
        subscriberID,
        sendID,
        r.Email,
        r.ReplyToken,
        r.MessageID,
        r.InReplyTo,
        r.Subject,
        r.Snippet,
        r.Headers,
        r.CreatedAt,
    ); err != nil {
        c.log.Printf("error inserting reply: %v", err)
        return echo.NewHTTPError(http.StatusInternalServerError, c.i18n.T("globals.messages.internalError"))
    }

    // Mark send as replied
    if _, err := c.q.UpdateCampaignSendReplied.Exec(sendID, r.CreatedAt); err != nil {
        c.log.Printf("error updating send replied_at: %v", err)
    }

    _ = replyID
    return nil
}



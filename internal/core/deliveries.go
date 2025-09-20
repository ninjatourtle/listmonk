package core

import (
    "net/http"
    "strings"

    "github.com/knadh/listmonk/models"
    "github.com/labstack/echo/v4"
)

var deliveriesSortFields = []string{"email", "name", "status", "sent_at", "action_at", "opened_at", "clicked_at", "bounced_at", "replied_at"}

// QueryCampaignDeliveries returns per-subscriber delivery status for a campaign with pagination.
func (c *Core) QueryCampaignDeliveries(campID int, status, orderBy, order string, offset, limit int) ([]models.Delivery, int, error) {
    if !strSliceContains(orderBy, deliveriesSortFields) {
        orderBy = "email"
    }
    if order != SortAsc && order != SortDesc {
        order = SortAsc
    }

    out := []models.Delivery{}
    stmt := strings.ReplaceAll(c.q.QueryCampaignDeliveries, "%order%", orderBy+" "+order)
    if err := c.db.Select(&out, stmt, campID, offset, limit, status); err != nil {
        c.log.Printf("error fetching campaign deliveries: %v", err)
        return nil, 0, echo.NewHTTPError(http.StatusInternalServerError,
            c.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.campaign}", "error", pqErrMsg(err)))
    }

    total := 0
    if len(out) > 0 {
        total = out[0].Total
    }

    return out, total, nil
}

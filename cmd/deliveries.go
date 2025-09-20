package main

import (
    "net/http"

    "github.com/knadh/listmonk/internal/auth"
    "github.com/knadh/listmonk/models"
    "github.com/labstack/echo/v4"
)

// GetCampaignDeliveries returns per-subscriber delivery status for a campaign.
func (a *App) GetCampaignDeliveries(c echo.Context) error {
    id := getID(c)

    // Check campaign permissions (view).
    if err := a.checkCampaignPerm(auth.PermTypeGet, id, c); err != nil {
        return err
    }

    pg := a.pg.NewFromURL(c.Request().URL.Query())

    status := c.QueryParam("status")
    orderBy := c.QueryParam("order_by")
    order := c.QueryParam("order")

    res, total, err := a.core.QueryCampaignDeliveries(id, status, orderBy, order, pg.Offset, pg.Limit)
    if err != nil {
        return err
    }

    // Paginate response.
    if len(res) == 0 {
        return c.JSON(http.StatusOK, okResp{models.PageResults{Results: []models.Delivery{}}})
    }

    out := models.PageResults{
        Results: res,
        Total:   total,
        Page:    pg.Page,
        PerPage: pg.PerPage,
    }

    return c.JSON(http.StatusOK, okResp{out})
}


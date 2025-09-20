package main

import (
    "net/http"
    "strconv"

    "github.com/labstack/echo/v4"
)

// GetRecentSends returns recent send events across campaigns or for a specific campaign.
func (a *App) GetRecentSends(c echo.Context) error {
    campID, _ := strconv.Atoi(c.QueryParam("campaign_id"))
    limit, _ := strconv.Atoi(c.QueryParam("limit"))
    if limit <= 0 || limit > 1000 {
        limit = 100
    }
    rows, err := a.core.QueryRecentSends(campID, limit)
    if err != nil {
        return err
    }
    return c.JSON(http.StatusOK, okResp{rows})
}


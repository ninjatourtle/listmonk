package main

import (
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/labstack/echo/v4"
)

// GetCampaignSchedule returns schedule windows for a campaign/day.
func (a *App) GetCampaignSchedule(c echo.Context) error {
    campID, _ := strconv.Atoi(c.QueryParam("campaign_id"))
    dateStr := strings.TrimSpace(c.QueryParam("date"))
    if dateStr == "" {
        dateStr = time.Now().Format("2006-01-02")
    }
    limit, _ := strconv.Atoi(c.QueryParam("limit"))
    if limit <= 0 {
        limit = 500
    }
    offset, _ := strconv.Atoi(c.QueryParam("offset"))
    statusFilter := strings.TrimSpace(c.QueryParam("status"))

    day, err := time.Parse("2006-01-02", dateStr)
    if err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid date")
    }

    type row struct {
        ID             int64     `db:"id" json:"id"`
        CampaignID     int       `db:"campaign_id" json:"campaign_id"`
        CampaignName   string    `db:"campaign_name" json:"campaign_name"`
        CampaignStatus string    `db:"campaign_status" json:"campaign_status"`
        DueAt          time.Time `db:"due_at" json:"due_at"`
        Quota          int       `db:"quota" json:"quota"`
        Used           int       `db:"used" json:"used"`
        Status         string    `db:"status" json:"status"`
        EmailTo        string    `db:"email_to" json:"email_to"`
        EmailFrom      string    `db:"email_from" json:"email_from"`
        Subject        string    `db:"subject" json:"subject"`
        Error          string    `db:"error" json:"error"`
    }

    rows := []row{}
    if err := a.queries.CampSchedListByDay.Select(&rows, campID, day, offset, limit); err != nil {
        return err
    }

    if statusFilter != "" {
        filtered := rows[:0]
        for _, r := range rows {
            if strings.EqualFold(r.Status, statusFilter) {
                filtered = append(filtered, r)
            }
        }
        rows = filtered
    }

    return c.JSON(http.StatusOK, okResp{rows})
}

// RescheduleCampaignSlot updates due_at and resets status.
func (a *App) RescheduleCampaignSlot(c echo.Context) error {
    id := getID(c)
    dueStr := c.FormValue("due_at")
    if dueStr == "" { return echo.NewHTTPError(http.StatusBadRequest, "missing due_at") }
    t, err := time.Parse("2006-01-02 15:04", dueStr)
    if err != nil {
        return echo.NewHTTPError(http.StatusBadRequest, "invalid due_at")
    }
    if _, err := a.queries.CampSchedReschedule.Exec(id, t); err != nil { return err }
    return c.JSON(http.StatusOK, okResp{true})
}

func (a *App) DeleteCampaignSlot(c echo.Context) error {
    id := getID(c)
    if _, err := a.queries.CampSchedDelete.Exec(id); err != nil { return err }
    return c.JSON(http.StatusOK, okResp{true})
}

func (a *App) SkipCampaignSlot(c echo.Context) error {
    id := getID(c)
    if _, err := a.queries.CampSchedSkip.Exec(id); err != nil { return err }
    return c.JSON(http.StatusOK, okResp{true})
}

package main

import (
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/knadh/listmonk/internal/messenger/email"
    "github.com/labstack/echo/v4"
)

// GetWarmCloudSends returns the recent warmcloud send/submit events.
func (a *App) GetWarmCloudSends(c echo.Context) error {
    // Pagination
    lim := 100
    if s := c.QueryParam("limit"); s != "" {
        if n, err := strconv.Atoi(s); err == nil && n > 0 {
            lim = n
        }
    }
    page := 1
    if s := c.QueryParam("page"); s != "" {
        if n, err := strconv.Atoi(s); err == nil && n > 0 {
            page = n
        }
    }
    off := (page - 1) * lim

    type row struct {
        CreatedAt string  `db:"created_at" json:"created_at"`
        Server    string  `db:"server" json:"server"`
        Email     string  `db:"email" json:"email"`
        Subject   string  `db:"subject" json:"subject"`
        Submitted bool    `db:"submitted" json:"submitted"`
        Error     *string `db:"error" json:"error"`
    }
    // Optional filters: server (exact name), today (bool), from/to (YYYY-MM-DD)
    server := strings.TrimSpace(c.QueryParam("server"))
    today := c.QueryParam("today") == "1"
    from := strings.TrimSpace(c.QueryParam("from"))
    to := strings.TrimSpace(c.QueryParam("to"))

    // Build WHERE dynamically with positional params
    where := []string{"1=1"}
    args := []any{}
    if server != "" {
        where = append(where, "server = $"+strconv.Itoa(len(args)+1))
        args = append(args, server)
    }
    if today {
        where = append(where, "created_at::date = CURRENT_DATE")
    }
    if from != "" {
        where = append(where, "created_at::date >= $"+strconv.Itoa(len(args)+1))
        args = append(args, from)
    }
    if to != "" {
        where = append(where, "created_at::date <= $"+strconv.Itoa(len(args)+1))
        args = append(args, to)
    }
    // pagination params
    args = append(args, lim, off)
    query := `SELECT created_at, server, email, subject, submitted, error
              FROM warmcloud_events
              WHERE ` + strings.Join(where, " AND ") + `
              ORDER BY created_at DESC
              LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
    var out []row
    if err := a.db.Select(&out, query, args...); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    return c.JSON(http.StatusOK, okResp{out})
}

// GetWarmCloudSchedule returns schedule slots for a server/day.
func (a *App) GetWarmCloudSchedule(c echo.Context) error {
    server := strings.TrimSpace(c.QueryParam("server"))
    if server == "" { return echo.NewHTTPError(http.StatusBadRequest, "missing server") }
    
    // Normalize server name to match internal naming convention
    if !strings.HasPrefix(server, "email-") {
        server = "email-" + server
    }
    dayStr := strings.TrimSpace(c.QueryParam("date"))
    var day time.Time
    var err error
    if dayStr == "" {
        day = time.Now()
    } else {
        // Parse date in UTC to avoid timezone issues
        day, err = time.ParseInLocation("2006-01-02", dayStr, time.UTC)
        if err != nil { return echo.NewHTTPError(http.StatusBadRequest, "invalid date") }
    }
    lim := 1000
    if s := c.QueryParam("limit"); s != "" { if n, err := strconv.Atoi(s); err == nil && n > 0 { lim = n } }
    off := 0
    if s := c.QueryParam("offset"); s != "" { if n, err := strconv.Atoi(s); err == nil && n >= 0 { off = n } }

    type row struct {
        ID int64 `db:"id" json:"id"`
        Server string `db:"server" json:"server"`
        DueAt time.Time `db:"due_at" json:"due_at"`
        Status string `db:"status" json:"status"`
        Error string `db:"error" json:"error"`
        Email string `json:"email"`
        Subject string `db:"subject" json:"subject"`
        Submitted bool `json:"submitted"`
        EmailFrom string `db:"from_email" json:"email_from"`
        Persona string `db:"persona" json:"persona"`
    }
    var out []row
    rows, err := a.queries.WarmSchedListByDay.Queryx(server, day.Format("2006-01-02"), lim, off)
    if err != nil {
        // Log the error for debugging
        a.log.Printf("Error querying warmcloud schedule for server %s, day %s: %v", server, day.Format("2006-01-02"), err)
        return echo.NewHTTPError(http.StatusInternalServerError, "Failed to load schedule data")
    }
    defer rows.Close()
    for rows.Next() {
        var r row
        if err := rows.StructScan(&r); err != nil {
            return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
        }
        // For non-pending, infer email+subject and submitted flag from warmcloud_events close to due_at.
        if r.Status != "pending" {
            var em, subj string
            var subm bool
            // Prefer an exact email match if the schedule slot recorded it; else, use the closest event in the window.
            if strings.TrimSpace(r.Email) != "" {
                q := `SELECT email, subject, submitted FROM warmcloud_events
                      WHERE server = $1 AND email = $2 AND created_at BETWEEN $3 AND $4
                      ORDER BY created_at ASC LIMIT 1`
                if err := a.db.QueryRowx(q, r.Server, r.Email, r.DueAt.Add(-10*time.Minute), r.DueAt.Add(10*time.Minute)).Scan(&em, &subj, &subm); err == nil {
                    if r.Email == "" { r.Email = em }
                    if r.Subject == "" { r.Subject = subj }
                    r.Submitted = subm
                }
            } else {
                q := `SELECT email, subject, submitted FROM warmcloud_events
                      WHERE server = $1 AND created_at BETWEEN $2 AND $3
                      ORDER BY created_at ASC LIMIT 1`
                if err := a.db.QueryRowx(q, r.Server, r.DueAt.Add(-10*time.Minute), r.DueAt.Add(10*time.Minute)).Scan(&em, &subj, &subm); err == nil {
                    if r.Email == "" { r.Email = em }
                    if r.Subject == "" { r.Subject = subj }
                    r.Submitted = subm
                }
            }
        }
        
        // EmailFrom is now loaded directly from database via from_email field
        out = append(out, r)
    }
    return c.JSON(http.StatusOK, okResp{out})
}

// RescheduleWarmCloudSlot updates due_at for a slot and resets status to pending.
func (a *App) RescheduleWarmCloudSlot(c echo.Context) error {
    idStr := c.Param("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil || id <= 0 { return echo.NewHTTPError(http.StatusBadRequest, "invalid id") }
    var req struct { DueAt string `json:"due_at"` }
    if err := c.Bind(&req); err != nil { return echo.NewHTTPError(http.StatusBadRequest, "invalid payload") }
    if strings.TrimSpace(req.DueAt) == "" { return echo.NewHTTPError(http.StatusBadRequest, "missing due_at") }
    // Support both ISO and "YYYY-MM-DD HH:MM"
    var due time.Time
    if t, err := time.Parse(time.RFC3339, req.DueAt); err == nil { due = t } else {
        // try common local format
        if t2, err2 := time.Parse("2006-01-02 15:04", req.DueAt); err2 == nil { due = t2 } else {
            return echo.NewHTTPError(http.StatusBadRequest, "invalid due_at format")
        }
    }
    // Update
    if _, err := a.db.Exec(`UPDATE warmcloud_schedule SET due_at = $2, status = 'pending', updated_at = NOW() WHERE id = $1`, id, due); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    return c.JSON(http.StatusOK, okResp{"ok"})
}

// DeleteWarmCloudSlot removes a slot by id.
func (a *App) DeleteWarmCloudSlot(c echo.Context) error {
    idStr := c.Param("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil || id <= 0 { return echo.NewHTTPError(http.StatusBadRequest, "invalid id") }
    if _, err := a.db.Exec(`DELETE FROM warmcloud_schedule WHERE id = $1`, id); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    return c.JSON(http.StatusOK, okResp{"ok"})
}

// SkipWarmCloudSlot marks a slot as done without sending.
func (a *App) SkipWarmCloudSlot(c echo.Context) error {
    idStr := c.Param("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil || id <= 0 { return echo.NewHTTPError(http.StatusBadRequest, "invalid id") }
    if _, err := a.queries.WarmSchedMarkDone.Exec(id); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    return c.JSON(http.StatusOK, okResp{"ok"})
}

// FinishWarmCloudToday marks all pending slots for today as done for a server.
func (a *App) FinishWarmCloudToday(c echo.Context) error {
    server := normalizeSMTPName(strings.TrimSpace(c.QueryParam("server")))
    if server == "" {
        return echo.NewHTTPError(http.StatusBadRequest, "missing server")
    }

    today := time.Now().Format("2006-01-02")
    if _, err := a.queries.WarmSchedMarkAllTodayDone.Exec(server, today); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }

    if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
        em.WarmingFinishToday(server)
    }

    return c.JSON(http.StatusOK, okResp{"ok"})
}

// DisableWarmCloud removes only unsent emails from schedule for a server.
func (a *App) DisableWarmCloud(c echo.Context) error {
    server := strings.TrimSpace(c.QueryParam("server"))
    if server == "" { return echo.NewHTTPError(http.StatusBadRequest, "missing server") }
    
    // Remove only unsent (pending) emails from schedule
    if _, err := a.queries.WarmSchedDeleteUnsentOnly.Exec(server); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    
    return c.JSON(http.StatusOK, okResp{"ok"})
}

// EnableWarmCloud enables warming and creates schedule for today if needed.
func (a *App) EnableWarmCloud(c echo.Context) error {
    server := strings.TrimSpace(c.QueryParam("server"))
    if server == "" { return echo.NewHTTPError(http.StatusBadRequest, "missing server") }
    
    // This will trigger the warming manager to create schedule automatically
    // The actual logic is handled in the warming manager's scheduleCoordinator
    return c.JSON(http.StatusOK, okResp{"ok"})
}

// ClearWarmCloudSchedule removes all schedule entries for a server.
func (a *App) ClearWarmCloudSchedule(c echo.Context) error {
    server := strings.TrimSpace(c.QueryParam("server"))
    if server == "" { return echo.NewHTTPError(http.StatusBadRequest, "missing server") }
    
    // Delete all schedule entries for this server
    if _, err := a.queries.WarmSchedDeleteAll.Exec(server); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    
    return c.JSON(http.StatusOK, okResp{"ok"})
}

package main

import (
    "net/http"
    "strconv"
    "strings"

    "github.com/labstack/echo/v4"
)

// GetSmtpSends returns recent SMTP sends with optional filters.
func (a *App) GetSmtpSends(c echo.Context) error {
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

    server := strings.TrimSpace(c.QueryParam("server"))
    today := c.QueryParam("today") == "1"
    from := strings.TrimSpace(c.QueryParam("from"))
    to := strings.TrimSpace(c.QueryParam("to"))
    status := strings.TrimSpace(c.QueryParam("status"))

    type row struct {
        CreatedAt string  `db:"created_at" json:"created_at"`
        Server    string  `db:"server" json:"server"`
        Email     string  `db:"email" json:"email"`
        Subject   string  `db:"subject" json:"subject"`
        Status    string  `db:"status" json:"status"`
    }
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
    if status != "" {
        where = append(where, "status = $"+strconv.Itoa(len(args)+1))
        args = append(args, status)
    }
    // Append limit/offset
    args = append(args, lim, off)
    query := `SELECT created_at, server, email, subject, status
              FROM smtp_send_events
              WHERE ` + strings.Join(where, " AND ") + `
              ORDER BY created_at DESC
              LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
    var out []row
    if err := a.db.Select(&out, query, args...); err != nil {
        return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
    }
    return c.JSON(http.StatusOK, okResp{out})
}

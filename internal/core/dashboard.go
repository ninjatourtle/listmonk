package core

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx/types"
	"github.com/labstack/echo/v4"
)

// GetDashboardCharts returns chart data points to render on the dashboard.
type recentView struct {
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	Email        *string   `db:"email" json:"email"`
	Name         *string   `db:"name" json:"name"`
	CampaignID   int       `db:"campaign_id" json:"campaign_id"`
	CampaignName string    `db:"campaign_name" json:"campaign_name"`
}

type recentClick struct {
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
	Email        *string   `db:"email" json:"email"`
	Name         *string   `db:"name" json:"name"`
	CampaignID   int       `db:"campaign_id" json:"campaign_id"`
	CampaignName string    `db:"campaign_name" json:"campaign_name"`
	URL          *string   `db:"url" json:"url"`
}

func (c *Core) GetDashboardCharts() (types.JSONText, error) {
	_ = c.refreshCache(matDashboardCharts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCharts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", pqErrMsg(err)))
	}

	// Unmarshal the charts JSON to append recent items.
	var obj map[string]any
	if err := json.Unmarshal(out, &obj); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", err.Error()))
	}

	const lim = 10
	recViews := []recentView{}
	if err := c.q.GetRecentCampaignViews.Select(&recViews, lim); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "recent views", "error", pqErrMsg(err)))
	}
	recClicks := []recentClick{}
	if err := c.q.GetRecentLinkClicks.Select(&recClicks, lim); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "recent clicks", "error", pqErrMsg(err)))
	}

	obj["recent_views"] = recViews
	obj["recent_clicks"] = recClicks

	b, err := json.Marshal(obj)
	if err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard charts", "error", err.Error()))
	}
	return types.JSONText(b), nil
}

// GetDashboardCounts returns stats counts to show on the dashboard.
func (c *Core) GetDashboardCounts() (types.JSONText, error) {
	_ = c.refreshCache(matDashboardCounts, false)

	var out types.JSONText
	if err := c.q.GetDashboardCounts.Get(&out); err != nil {
		return nil, echo.NewHTTPError(http.StatusInternalServerError,
			c.i18n.Ts("globals.messages.errorFetching", "name", "dashboard stats", "error", pqErrMsg(err)))
	}

	return out, nil
}

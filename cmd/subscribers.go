package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"net/textproto"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/events"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/internal/subimporter"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
	"github.com/lib/pq"
)

const (
	dummyUUID = "00000000-0000-0000-0000-000000000000"
)

// subQueryReq is a "catch all" struct for reading various
// subscriber related requests.
type subQueryReq struct {
	Search             string `json:"search"`
	Query              string `json:"query"`
	ListIDs            []int  `json:"list_ids"`
	TargetListIDs      []int  `json:"target_list_ids"`
	SubscriberIDs      []int  `json:"ids"`
	Action             string `json:"action"`
	Status             string `json:"status"`
	SubscriptionStatus string `json:"subscription_status"`
	All                bool   `json:"all"`
}

// subOptin contains the data that's passed to the double opt-in e-mail template.
type subOptin struct {
	models.Subscriber

	OptinURL string
	UnsubURL string
	Lists    []models.List
}

var (
	dummySubscriber = models.Subscriber{
		Email:   "demo@listmonk.app",
		Name:    "Demo Subscriber",
		UUID:    dummyUUID,
		Attribs: models.JSON{"city": "Bengaluru"},
	}
)

// GetSubscriber handles the retrieval of a single subscriber by ID.
func (a *App) GetSubscriber(c echo.Context) error {
	user := auth.GetUser(c)

	// Check if the user has access to at least one of the lists on the subscriber.
	id := getID(c)
	if err := a.hasSubPerm(user, []int{id}); err != nil {
		return err
	}

	// Fetch the subscriber from the DB.
	out, err := a.core.GetSubscriber(id, "", "")
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// ValidateSubscriberEmail validates an e-mail via syntax, MX, and SMTP RCPT probe.
func (a *App) ValidateSubscriberEmail(c echo.Context) error {
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}
	email := strings.TrimSpace(req.Email)
	addr, err := mail.ParseAddress(email)
	syntaxOK := (err == nil && strings.Contains(addr.Address, "@"))
	user, domain := "", ""
	if syntaxOK {
		p := strings.SplitN(addr.Address, "@", 2)
		user, domain = p[0], p[1]
	}

	// MX lookup
	mxOK := false
	var mxHosts []string
	if domain != "" {
		if mx, err := net.LookupMX(domain); err == nil && len(mx) > 0 {
			sort.Slice(mx, func(i, j int) bool { return mx[i].Pref < mx[j].Pref })
			for _, m := range mx {
				mxHosts = append(mxHosts, strings.TrimSuffix(m.Host, "."))
			}
			mxOK = true
		}
	}

	// SMTP probe
	type line struct{ Dir, Msg string }
	var dialog []line
	deliveryOK := false
	if mxOK {
		helo := "validate.echo25.email"
		from := a.cfg.FromEmail
		if from == "" && domain != "" {
			from = "validator@" + domain
		}
		for _, host := range mxHosts {
			d := net.Dialer{Timeout: 10 * time.Second}
			conn, err := d.Dial("tcp", net.JoinHostPort(host, "25"))
			if err != nil {
				continue
			}
			_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
			r := bufio.NewReader(conn)
			// Banner
			if b, err := r.ReadString('\n'); err == nil {
				dialog = append(dialog, line{"recv", strings.TrimSpace(b)})
			}
			// EHLO
			fmt.Fprintf(conn, "EHLO %s\r\n", helo)
			dialog = append(dialog, line{"send", "EHLO " + helo})
			if resp, err := r.ReadString('\n'); err == nil {
				dialog = append(dialog, line{"recv", strings.TrimSpace(resp)})
			}
			// MAIL FROM
			fmt.Fprintf(conn, "MAIL FROM:<%s>\r\n", from)
			dialog = append(dialog, line{"send", "MAIL FROM:<" + from + ">"})
			if resp, err := r.ReadString('\n'); err == nil {
				dialog = append(dialog, line{"recv", strings.TrimSpace(resp)})
			}
			// RCPT TO
			fmt.Fprintf(conn, "RCPT TO:<%s>\r\n", addr.Address)
			dialog = append(dialog, line{"send", "RCPT TO:<" + addr.Address + ">"})
			if resp, err := r.ReadString('\n'); err == nil {
				dialog = append(dialog, line{"recv", strings.TrimSpace(resp)})
				if len(resp) >= 3 && (resp[:3] == "250" || resp[:3] == "251") {
					deliveryOK = true
				}
			}
			fmt.Fprintf(conn, "QUIT\r\n")
			_ = conn.Close()
			if deliveryOK {
				break
			}
		}
	}

	// Heuristics
	freeDomains := map[string]bool{"gmail.com": true, "yahoo.com": true, "yandex.ru": true, "mail.ru": true, "outlook.com": true, "hotmail.com": true}
	tempDomains := map[string]bool{"mailinator.com": true, "tempmail.com": true, "10minutemail.com": true}
	isFree := freeDomains[strings.ToLower(domain)]
	isTemp := tempDomains[strings.ToLower(domain)]

	out := map[string]any{
		"user":        user,
		"domain":      domain,
		"syntax_ok":   syntaxOK,
		"mx_ok":       mxOK,
		"mx_hosts":    mxHosts,
		"delivery_ok": deliveryOK,
		"free":        isFree,
		"temporary":   isTemp,
		"dialog":      dialog,
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// mxCache caches MX hosts per domain for the lifetime of a request.
type mxCache struct {
	mu sync.Mutex
	m  map[string][]string
}

func newMXCache() *mxCache { return &mxCache{m: make(map[string][]string)} }
func (c *mxCache) get(domain string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.m[domain]
	return v, ok
}
func (c *mxCache) set(domain string, hosts []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[domain] = hosts
}

type validateEmailOpts struct {
	DialTimeout    time.Duration
	Deadline       time.Duration
	MaxMX          int // 0 = all
	MXCache        *mxCache
	OverallTimeout time.Duration
	Reporter       func(stage string, data map[string]any)
}

// helper: validate email using syntax, MX and RCPT probe with options.
func (a *App) validateEmailFull(email string) (syntaxOK, mxOK, deliveryOK bool) {
	// Defaults to preserve original behaviour for single checks.
	return a.validateEmailFullWithOpts(context.Background(), email, validateEmailOpts{
		DialTimeout:    10 * time.Second,
		Deadline:       15 * time.Second,
		MaxMX:          0,
		MXCache:        nil,
		OverallTimeout: 20 * time.Second,
		Reporter:       nil,
	})
}

func (a *App) validateEmailFullWithOpts(ctx context.Context, email string, opt validateEmailOpts) (syntaxOK, mxOK, deliveryOK bool) {
	if opt.OverallTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.OverallTimeout)
		defer cancel()
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(email))
	if err != nil || !strings.Contains(addr.Address, "@") {
		return false, false, false
	}
	parts := strings.SplitN(addr.Address, "@", 2)
	domain := parts[1]

	// MX lookup with simple in-request cache.
	var mxHosts []string
	if opt.Reporter != nil {
		opt.Reporter("mx_lookup_start", map[string]any{"domain": domain})
	}
	if opt.MXCache != nil {
		if v, ok := opt.MXCache.get(domain); ok {
			mxHosts = v
		}
	}
	if len(mxHosts) == 0 {
		if mx, err := net.DefaultResolver.LookupMX(ctx, domain); err == nil && len(mx) > 0 {
			sort.Slice(mx, func(i, j int) bool { return mx[i].Pref < mx[j].Pref })
			for _, m := range mx {
				mxHosts = append(mxHosts, strings.TrimSuffix(m.Host, "."))
			}
			if opt.MXCache != nil {
				opt.MXCache.set(domain, mxHosts)
			}
		}
	}
	if len(mxHosts) > 0 {
		mxOK = true
	}
	if opt.Reporter != nil {
		opt.Reporter("mx_lookup_done", map[string]any{"domain": domain, "mx_hosts": mxHosts, "mx_ok": mxOK})
	}
	syntaxOK = true

	if mxOK {
		helo := "validate.echo25.email"
		from := a.cfg.FromEmail
		if from == "" {
			from = "validator@" + domain
		}

		limit := len(mxHosts)
		if opt.MaxMX > 0 && opt.MaxMX < limit {
			limit = opt.MaxMX
		}
		for _, host := range mxHosts[:limit] {
			// Respect context cancellation.
			select {
			case <-ctx.Done():
				return syntaxOK, mxOK, deliveryOK
			default:
			}
			if opt.Reporter != nil {
				opt.Reporter("smtp_probe_start", map[string]any{"host": host})
			}
			d := net.Dialer{Timeout: opt.DialTimeout}
			conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, "25"))
			if err != nil {
				continue
			}
			_ = conn.SetDeadline(time.Now().Add(opt.Deadline))
			r := bufio.NewReader(conn)
			_, _ = r.ReadString('\n')
			fmt.Fprintf(conn, "EHLO %s\r\n", helo)
			_, _ = r.ReadString('\n')
			fmt.Fprintf(conn, "MAIL FROM:<%s>\r\n", from)
			_, _ = r.ReadString('\n')
			fmt.Fprintf(conn, "RCPT TO:<%s>\r\n", addr.Address)
			if resp, err := r.ReadString('\n'); err == nil {
				if len(resp) >= 3 && (resp[:3] == "250" || resp[:3] == "251") {
					deliveryOK = true
				}
				if opt.Reporter != nil {
					opt.Reporter("smtp_probe_rcpt", map[string]any{"host": host, "response": strings.TrimSpace(resp)})
				}
			}
			fmt.Fprintf(conn, "QUIT\r\n")
			_ = conn.Close()
			if deliveryOK {
				break
			}
		}
	}
	return
}

// ValidateSubscribersBulk validates a list of subscribers by ID and blocklists bad ones.
func (a *App) ValidateSubscribersBulk(c echo.Context) error {
	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}
	if len(req.SubscriberIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.errorNoIDs"))
	}

	// Concurrency controls. Base on SMTP settings (sum of enabled max_conns) to scale workers.
	total := len(req.SubscriberIDs)
	concurrency := 8
	if s, err := a.core.GetSettings(); err == nil {
		sum := 0
		for _, srv := range s.SMTP {
			if srv.Enabled && srv.MaxConns > 0 {
				sum += srv.MaxConns
			}
		}
		if sum > 0 {
			concurrency = sum
		}
		if concurrency > 64 {
			concurrency = 64
		} // hard cap to avoid overload
	}
	if total < concurrency {
		concurrency = total
	}

	ctx := c.Request().Context()
	cache := newMXCache()
	opts := validateEmailOpts{DialTimeout: 5 * time.Second, Deadline: 8 * time.Second, MaxMX: 3, MXCache: cache, OverallTimeout: 20 * time.Second}

	// Work item definition must appear before channel usage.
	type job struct {
		id    int
		email string
	}
	tasks := make(chan job, concurrency)
	results := make(chan int, concurrency)
	var wg sync.WaitGroup

	// Progress counters and event ticker.
	var processed int64
	var invalidCount int64
	done := make(chan struct{})

	// Start event.
	_ = a.events.Publish(events.Event{Type: "validation", Message: "started", Data: map[string]any{"total": total, "concurrency": concurrency}})

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = a.events.Publish(events.Event{Type: "validation", Message: "progress", Data: map[string]any{
					"total":     total,
					"processed": atomic.LoadInt64(&processed),
					"invalid":   atomic.LoadInt64(&invalidCount),
				}})
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Preload domain labels from DB (optional best-effort).
	domainSet := map[string]struct{}{}
	jobs := make([]job, 0, total)
	for _, id := range req.SubscriberIDs {
		sub, err := a.core.GetSubscriber(id, "", "")
		if err != nil {
			continue
		}
		jobs = append(jobs, job{id: id, email: sub.Email})
		parts := strings.SplitN(sub.Email, "@", 2)
		if len(parts) == 2 {
			domainSet[strings.ToLower(parts[1])] = struct{}{}
		}
	}
	domains := make([]string, 0, len(domainSet))
	for d := range domainSet {
		domains = append(domains, d)
	}

	type dlabel struct {
		Label         string
		Disposable    bool
		Free          bool
		PublicPrivate string
	}
	domLabels := map[string]dlabel{}
	if len(domains) > 0 {
		rows, err := a.queries.GetEmailDomainLabels.Queryx(pq.Array(domains))
		if err == nil {
			defer rows.Close()
			var domain, label, pubpriv string
			var disposable, free bool
			for rows.Next() {
				if err := rows.Scan(&domain, &label, &disposable, &free, &pubpriv); err == nil {
					domLabels[strings.ToLower(domain)] = dlabel{Label: label, Disposable: disposable, Free: free, PublicPrivate: pubpriv}
				}
			}
		}
	}

	worker := func() {
		defer wg.Done()
		for j := range tasks {
			// Abort quickly if request is cancelled.
			select {
			case <-ctx.Done():
				return
			default:
			}
			email := j.email
			// Reporter for per-item steps.
			rep := func(stage string, data map[string]any) {
				if data == nil {
					data = map[string]any{}
				}
				data["email"] = email
				data["id"] = j.id
				_ = a.events.Publish(events.Event{Type: "validation", Message: stage, Data: data})
			}
			o := opts
			o.Reporter = rep
			// Item start
			rep("item_start", nil)
			started := time.Now()
			syn, mx, deliv := a.validateEmailFullWithOpts(ctx, email, o)

			// Classify by domain label if present.
			parts := strings.SplitN(email, "@", 2)
			domain := ""
			if len(parts) == 2 {
				domain = strings.ToLower(parts[1])
			}
			lbl := domLabels[domain]
			emailType := lbl.Label
			if emailType == "" {
				if lbl.Free {
					emailType = "free"
				}
				if lbl.Disposable {
					emailType = "disposable"
				}
			}
			reason := ""
			status := "valid"
			switch {
			case !syn:
				status = "invalid"
				reason = "syntax"
			case !mx:
				status = "invalid"
				reason = "no_mx"
			case !deliv:
				status = "invalid"
				reason = "rcpt_reject"
			}

			// Persist validation fields (best effort).
			_, _ = a.queries.UpdateSubscriberValidation.Exec(j.id, status, reason, emailType, time.Now())

			if status != "valid" {
				atomic.AddInt64(&invalidCount, 1)
				results <- j.id
			}
			atomic.AddInt64(&processed, 1)
			rep("item_done", map[string]any{"status": status, "reason": reason, "elapsed_ms": time.Since(started).Milliseconds()})
		}
	}

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}

	go func() {
		for _, j := range jobs {
			// Stop feeding if client aborts.
			select {
			case <-ctx.Done():
				break
			default:
			}
			tasks <- j
		}
		close(tasks)
	}()

	// Collector goroutine to close results when workers finish.
	go func() { wg.Wait(); close(results); close(done) }()

	invalid := make([]int, 0)
	for id := range results {
		invalid = append(invalid, id)
	}

	if len(invalid) > 0 {
		if err := a.core.BlocklistSubscribers(invalid); err != nil {
			return err
		}
	}
	_ = a.events.Publish(events.Event{Type: "validation", Message: "finished", Data: map[string]any{
		"total":       total,
		"processed":   atomic.LoadInt64(&processed),
		"invalid":     len(invalid),
		"blocklisted": len(invalid),
	}})
	return c.JSON(http.StatusOK, okResp{map[string]any{
		"total":       total,
		"invalid":     len(invalid),
		"blocklisted": len(invalid),
		"invalid_ids": invalid,
	}})
}

// ValidateSubscribersByQuery validates subscribers matching a query/filter (or all) and blocklists invalid ones.
// Accepts the same payload shape as other query-based bulk ops: search, query, list_ids, subscription_status, all.
func (a *App) ValidateSubscribersByQuery(c echo.Context) error {
	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	// If all=true, ignore search/query filters.
	if req.All {
		req.Search = ""
		req.Query = ""
	} else {
		req.Query = formatSQLExp(req.Query)
	}

	// Get total for progress (cheap 1-row query to obtain count).
	_, total, err := a.core.QuerySubscribers(strings.TrimSpace(req.Search), req.Query, req.ListIDs, req.SubscriptionStatus, "desc", "subscribers.id", 0, 1)
	if err != nil {
		return err
	}

	// Concurrency controls based on SMTP settings.
	concurrency := 8
	if s, err := a.core.GetSettings(); err == nil {
		sum := 0
		for _, srv := range s.SMTP {
			if srv.Enabled && srv.MaxConns > 0 {
				sum += srv.MaxConns
			}
		}
		if sum > 0 {
			concurrency = sum
		}
		if concurrency > 64 {
			concurrency = 64
		}
	}
	if total > 0 && total < concurrency {
		concurrency = total
	}

	ctx := c.Request().Context()
	cache := newMXCache()
	opts := validateEmailOpts{DialTimeout: 5 * time.Second, Deadline: 8 * time.Second, MaxMX: 3, MXCache: cache, OverallTimeout: 20 * time.Second}

	type job struct {
		id    int
		email string
	}
	tasks := make(chan job, concurrency)
	results := make(chan int, concurrency)
	var wg sync.WaitGroup

	// Progress events.
	var processed int64
	var invalidCount int64
	done := make(chan struct{})
	_ = a.events.Publish(events.Event{Type: "validation", Message: "started", Data: map[string]any{"total": total, "concurrency": concurrency}})

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = a.events.Publish(events.Event{Type: "validation", Message: "progress", Data: map[string]any{
					"total":     total,
					"processed": atomic.LoadInt64(&processed),
					"invalid":   atomic.LoadInt64(&invalidCount),
				}})
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	// Domain labels cache
	type dlabel struct {
		Label         string
		Disposable    bool
		Free          bool
		PublicPrivate string
	}
	domLabels := map[string]dlabel{}
	loadDomainLabel := func(domain string) dlabel {
		if v, ok := domLabels[domain]; ok {
			return v
		}
		rows, err := a.queries.GetEmailDomainLabels.Queryx(pq.Array([]string{domain}))
		if err == nil {
			defer rows.Close()
			var d, label, pubpriv string
			var disposable, free bool
			for rows.Next() {
				if err := rows.Scan(&d, &label, &disposable, &free, &pubpriv); err == nil {
					domLabels[strings.ToLower(d)] = dlabel{Label: label, Disposable: disposable, Free: free, PublicPrivate: pubpriv}
				}
			}
		}
		return domLabels[domain]
	}

	worker := func() {
		defer wg.Done()
		for j := range tasks {
			select {
			case <-ctx.Done():
				return
			default:
			}
			email := j.email
			rep := func(stage string, data map[string]any) {
				if data == nil {
					data = map[string]any{}
				}
				data["email"] = email
				data["id"] = j.id
				_ = a.events.Publish(events.Event{Type: "validation", Message: stage, Data: data})
			}
			o := opts
			o.Reporter = rep
			rep("item_start", nil)
			started := time.Now()
			syn, mx, deliv := a.validateEmailFullWithOpts(ctx, email, o)

			domain := ""
			if p := strings.SplitN(email, "@", 2); len(p) == 2 {
				domain = strings.ToLower(p[1])
			}
			lbl := loadDomainLabel(domain)
			emailType := lbl.Label
			if emailType == "" {
				if lbl.Free {
					emailType = "free"
				}
				if lbl.Disposable {
					emailType = "disposable"
				}
			}
			reason := ""
			status := "valid"
			switch {
			case !syn:
				status = "invalid"
				reason = "syntax"
			case !mx:
				status = "invalid"
				reason = "no_mx"
			case !deliv:
				status = "invalid"
				reason = "rcpt_reject"
			}
			_, _ = a.queries.UpdateSubscriberValidation.Exec(j.id, status, reason, emailType, time.Now())
			if status != "valid" {
				atomic.AddInt64(&invalidCount, 1)
				results <- j.id
			}
			atomic.AddInt64(&processed, 1)
			rep("item_done", map[string]any{"status": status, "reason": reason, "elapsed_ms": time.Since(started).Milliseconds()})
		}
	}

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}

	// Producer: iterate all matching subscribers in batches using ExportSubscribers.
	go func() {
		exp, err := a.core.ExportSubscribers(strings.TrimSpace(req.Search), req.Query, nil, req.ListIDs, req.SubscriptionStatus, a.cfg.DBBatchSize)
		if err != nil {
			close(tasks)
			return
		}
		for {
			batch, err := exp()
			if err != nil || len(batch) == 0 {
				break
			}
			for _, r := range batch {
				select {
				case <-ctx.Done():
					break
				default:
				}
				tasks <- job{id: r.ID, email: r.Email}
			}
		}
		close(tasks)
	}()

	go func() { wg.Wait(); close(results); close(done) }()

	invalid := make([]int, 0)
	for id := range results {
		invalid = append(invalid, id)
	}
	if len(invalid) > 0 {
		if err := a.core.BlocklistSubscribers(invalid); err != nil {
			return err
		}
	}
	_ = a.events.Publish(events.Event{Type: "validation", Message: "finished", Data: map[string]any{
		"total":       total,
		"processed":   atomic.LoadInt64(&processed),
		"invalid":     len(invalid),
		"blocklisted": len(invalid),
	}})
	return c.JSON(http.StatusOK, okResp{map[string]any{"total": total, "invalid": len(invalid), "blocklisted": len(invalid)}})
}

// QuerySubscribers handles querying subscribers based on an arbitrary SQL expression.
func (a *App) QuerySubscribers(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Filter list IDs by permission.
	listIDs, err := a.filterListQueryByPerm("list_id", c.QueryParams(), user)
	if err != nil {
		return err
	}

	// Does the user have the subscribers:sql_query permission?
	query := formatSQLExp(c.FormValue("query"))
	if query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	var (
		searchStr = strings.TrimSpace(c.FormValue("search"))
		subStatus = c.FormValue("subscription_status")
		order     = c.FormValue("order")
		orderBy   = c.FormValue("order_by")
		pg        = a.pg.NewFromURL(c.Request().URL.Query())
	)

	// Query subscribers from the DB.
	res, total, err := a.core.QuerySubscribers(searchStr, query, listIDs, subStatus, order, orderBy, pg.Offset, pg.Limit)
	if err != nil {
		return err
	}

	out := models.PageResults{
		Query:   query,
		Search:  searchStr,
		Results: res,
		Total:   total,
		Page:    pg.Page,
		PerPage: pg.PerPage,
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// ExportSubscribers handles querying subscribers based on an arbitrary SQL expression.
func (a *App) ExportSubscribers(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Filter list IDs by permission.
	listIDs, err := a.filterListQueryByPerm("list_id", c.QueryParams(), user)
	if err != nil {
		return err
	}

	// Export only specific subscriber IDs?
	subIDs, err := getQueryInts("id", c.QueryParams())
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
	}

	// Filter by subscription status
	subStatus := c.QueryParam("subscription_status")

	// Does the user have the subscribers:sql_query permission?
	var (
		searchStr = strings.TrimSpace(c.FormValue("search"))
		query     = formatSQLExp(c.FormValue("query"))
	)
	if query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Get the batched export iterator.
	exp, err := a.core.ExportSubscribers(searchStr, query, subIDs, listIDs, subStatus, a.cfg.DBBatchSize)
	if err != nil {
		return err
	}

	var (
		hdr = c.Response().Header()
		wr  = csv.NewWriter(c.Response())
	)

	hdr.Set(echo.HeaderContentType, echo.MIMEOctetStream)
	hdr.Set("Content-type", "text/csv")
	hdr.Set(echo.HeaderContentDisposition, "attachment; filename="+"subscribers.csv")
	hdr.Set("Content-Transfer-Encoding", "binary")
	hdr.Set("Cache-Control", "no-cache")
	wr.Write([]string{"uuid", "email", "name", "attributes", "status", "created_at", "updated_at"})

loop:
	// Iterate in batches until there are no more subscribers to export.
	for {
		out, err := exp()
		if err != nil {
			return err
		}
		if len(out) == 0 {
			break
		}

		for _, r := range out {
			if err = wr.Write([]string{r.UUID, r.Email, r.Name, r.Attribs, r.Status,
				r.CreatedAt.Time.String(), r.UpdatedAt.Time.String()}); err != nil {
				a.log.Printf("error streaming CSV export: %v", err)
				break loop
			}
		}

		// Flush CSV to stream after each batch.
		wr.Flush()
	}

	return nil
}

// CreateSubscriber handles the creation of a new subscriber.
func (a *App) CreateSubscriber(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Get and validate fields.
	var req subimporter.SubReq
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Validate fields.
	req, err := a.importer.ValidateFields(req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	// Filter lists against the current user's permitted lists.
	listIDs := user.FilterListsByPerm(auth.PermTypeManage, req.Lists)

	// Insert the subscriber into the DB.
	sub, _, err := a.core.InsertSubscriber(req.Subscriber, listIDs, nil, req.PreconfirmSubs)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{sub})
}

// UpdateSubscriber handles modification of a subscriber.
func (a *App) UpdateSubscriber(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Get and validate fields.
	req := struct {
		models.Subscriber
		Lists          []int `json:"lists"`
		PreconfirmSubs bool  `json:"preconfirm_subscriptions"`
	}{}
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Sanitize and validate the email field.
	if em, err := a.importer.SanitizeEmail(req.Email); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	} else {
		req.Email = em
	}

	if req.Name != "" && !strHasLen(req.Name, 1, stdInputMaxLen) {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.invalidName"))
	}

	// Filter lists against the current user's permitted lists.
	listIDs := user.FilterListsByPerm(auth.PermTypeManage, req.Lists)

	// Update the subscriber in the DB.
	id := getID(c)
	out, _, err := a.core.UpdateSubscriberWithLists(id, req.Subscriber, listIDs, nil, req.PreconfirmSubs, true)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// SubscriberSendOptin sends an optin confirmation e-mail to a subscriber.
func (a *App) SubscriberSendOptin(c echo.Context) error {
	// Fetch the subscriber.
	id := getID(c)
	out, err := a.core.GetSubscriber(id, "", "")
	if err != nil {
		return err
	}

	// Trigger the opt-in confirmation e-mail hook.
	if _, err := a.fnOptinNotify(out, nil); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, a.i18n.T("subscribers.errorSendingOptin"))
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// BlocklistSubscriber handles the blocklisting of a given subscriber.
func (a *App) BlocklistSubscriber(c echo.Context) error {
	// Update the subscribers in the DB.
	id := getID(c)
	if err := a.core.BlocklistSubscribers([]int{id}); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// BlocklistSubscribers handles the blocklisting of one or more subscribers.
func (a *App) BlocklistSubscribers(c echo.Context) error {
	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(req.SubscriberIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "ids"))
	}

	// Update the subscribers in the DB.
	if err := a.core.BlocklistSubscribers(req.SubscriberIDs); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// ManageSubscriberLists handles bulk addition or removal of subscribers
// from or to one or more target lists.
// It takes either an ID in the URI, or a list of IDs in the request body.
func (a *App) ManageSubscriberLists(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	// Is it an /:id call?
	var (
		pID    = c.Param("id")
		subIDs []int
	)
	if pID != "" {
		id, _ := strconv.Atoi(pID)
		if id < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
		}
		subIDs = append(subIDs, id)
	}

	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(req.SubscriberIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.errorNoIDs"))
	}
	if len(subIDs) == 0 {
		subIDs = req.SubscriberIDs
	}
	if len(req.TargetListIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.errorNoListsGiven"))
	}

	// Filter lists against the current user's permitted lists.
	listIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, req.TargetListIDs)

	// User doesn't have the required list permissions.
	if len(listIDs) == 0 {
		return echo.NewHTTPError(http.StatusForbidden,
			a.i18n.Ts("globals.messages.permissionDenied", "name", "lists"))
	}

	// Run the action in the DB.
	var err error
	switch req.Action {
	case "add":
		err = a.core.AddSubscriptions(subIDs, listIDs, req.Status)
	case "remove":
		err = a.core.DeleteSubscriptions(subIDs, listIDs)
	case "unsubscribe":
		err = a.core.UnsubscribeLists(subIDs, listIDs, nil)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.invalidAction"))
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteSubscriber handles deletion of a single subscriber.
func (a *App) DeleteSubscriber(c echo.Context) error {
	// Delete the subscribers from the DB.
	id := getID(c)
	if err := a.core.DeleteSubscribers([]int{id}, nil); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteSubscribers handles bulk deletion of one or more subscribers.
func (a *App) DeleteSubscribers(c echo.Context) error {
	// Multiple IDs.
	ids, err := parseStringIDs(c.Request().URL.Query()["id"])
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", err.Error()))
	}
	if len(ids) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorInvalidIDs", "error", "ids"))
	}

	// Delete the subscribers from the DB.
	if err := a.core.DeleteSubscribers(ids, nil); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteSubscribersByQuery bulk deletes based on an
// arbitrary SQL expression.
func (a *App) DeleteSubscribersByQuery(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return err
	}

	req.Search = strings.TrimSpace(req.Search)
	req.Query = formatSQLExp(req.Query)
	if req.All {
		// If the "all" flag is set, ignore any subquery that may be present.
		req.Search = ""
		req.Query = ""
	} else if req.Search == "" && req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "query"))
	}

	// Does the user have the subscribers:sql_query permission?
	if req.Query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Delete the subscribers from the DB.
	if err := a.core.DeleteSubscribersByQuery(req.Search, req.Query, req.ListIDs, req.SubscriptionStatus); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// BlocklistSubscribersByQuery bulk blocklists subscribers
// based on an arbitrary SQL expression.
func (a *App) BlocklistSubscribersByQuery(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return err
	}

	req.Search = strings.TrimSpace(req.Search)
	req.Query = formatSQLExp(req.Query)
	if req.All {
		// If the "all" flag is set, ignore any subquery that may be present.
		req.Search = ""
		req.Query = ""
	} else if req.Search == "" && req.Query == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidFields", "name", "query"))
	}
	// Does the user have the subscribers:sql_query permission?
	if req.Query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Update the subscribers in the DB.
	if err := a.core.BlocklistSubscribersByQuery(req.Search, req.Query, req.ListIDs, req.SubscriptionStatus); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// ManageSubscriberListsByQuery bulk adds/removes/unsubscribes subscribers
// from one or more lists based on an arbitrary SQL expression.
func (a *App) ManageSubscriberListsByQuery(c echo.Context) error {
	// Get the authenticated user.
	user := auth.GetUser(c)

	var req subQueryReq
	if err := c.Bind(&req); err != nil {
		return err
	}
	if len(req.TargetListIDs) == 0 {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.T("subscribers.errorNoListsGiven"))
	}

	req.Search = strings.TrimSpace(req.Search)
	req.Query = formatSQLExp(req.Query)

	// Does the user have the subscribers:sql_query permission?
	if req.Query != "" {
		if !user.HasPerm(auth.PermSubscribersSqlQuery) {
			return echo.NewHTTPError(http.StatusForbidden,
				a.i18n.Ts("globals.messages.permissionDenied", "name", auth.PermSubscribersSqlQuery))
		}
	}

	// Filter lists against the current user's permitted lists.
	sourceListIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, req.ListIDs)
	targetListIDs := user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, req.TargetListIDs)

	// Run the action in the DB.
	var err error
	switch req.Action {
	case "add":
		err = a.core.AddSubscriptionsByQuery(req.Search, req.Query, sourceListIDs, targetListIDs, req.Status, req.SubscriptionStatus)
	case "remove":
		err = a.core.DeleteSubscriptionsByQuery(req.Search, req.Query, sourceListIDs, targetListIDs, req.SubscriptionStatus)
	case "unsubscribe":
		err = a.core.UnsubscribeListsByQuery(req.Search, req.Query, sourceListIDs, targetListIDs, req.SubscriptionStatus)
	default:
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("subscribers.invalidAction"))
	}

	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// DeleteSubscriberBounces deletes all the bounces on a subscriber.
func (a *App) DeleteSubscriberBounces(c echo.Context) error {
	// Delete the bounces from the DB.
	id := getID(c)
	if err := a.core.DeleteSubscriberBounces(id, ""); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{true})
}

// ExportSubscriberData pulls the subscriber's profile,
// list subscriptions, campaign views and clicks and produces
// a JSON report. This is a privacy feature and depends on the
// configuration in a.Constants.Privacy.
func (a *App) ExportSubscriberData(c echo.Context) error {
	// Get the subscriber's data. A single query that gets the profile,
	// list subscriptions, campaign views, and link clicks. Names of
	// private lists are replaced with "Private list".
	id := getID(c)
	_, b, err := a.exportSubscriberData(id, "", a.cfg.Privacy.Exportable)
	if err != nil {
		a.log.Printf("error exporting subscriber data: %s", err)
		return echo.NewHTTPError(http.StatusInternalServerError,
			a.i18n.Ts("globals.messages.errorFetching", "name", "{globals.terms.subscribers}", "error", err.Error()))
	}

	// Set headers to force the browser to prompt for download.
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Content-Disposition", `attachment; filename="data.json"`)
	return c.Blob(http.StatusOK, "application/json", b)
}

// exportSubscriberData collates the data of a subscriber including profile,
// subscriptions, campaign_views, link_clicks (if they're enabled in the config)
// and returns a formatted, indented JSON payload. Either takes a numeric id
// and an empty subUUID or takes 0 and a string subUUID.
func (a *App) exportSubscriberData(id int, subUUID string, exportables map[string]bool) (models.SubscriberExportProfile, []byte, error) {
	data, err := a.core.GetSubscriberProfileForExport(id, subUUID)
	if err != nil {
		return data, nil, err
	}

	// Filter out the non-exportable items.
	if _, ok := exportables["profile"]; !ok {
		data.Profile = nil
	}
	if _, ok := exportables["subscriptions"]; !ok {
		data.Subscriptions = nil
	}
	if _, ok := exportables["campaign_views"]; !ok {
		data.CampaignViews = nil
	}
	if _, ok := exportables["link_clicks"]; !ok {
		data.LinkClicks = nil
	}

	// Marshal the data into an indented payload.
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		a.log.Printf("error marshalling subscriber export data: %v", err)
		return data, nil, err
	}

	return data, b, nil
}

// hasSubPerm checks whether the current user has permission to access the given list
// of subscriber IDs.
func (a *App) hasSubPerm(u auth.User, subIDs []int) error {
	allPerm, listIDs := u.GetPermittedLists(auth.PermTypeGet | auth.PermTypeManage)

	// User has blanket get_all|manage_all permission.
	if allPerm {
		return nil
	}

	// Check whether the subscribers have the list IDs permitted to the user.
	res, err := a.core.HasSubscriberLists(subIDs, listIDs)
	if err != nil {
		return err
	}

	for id, has := range res {
		if !has {
			return echo.NewHTTPError(http.StatusForbidden, a.i18n.Ts("globals.messages.permissionDenied", "name", fmt.Sprintf("subscriber: %d", id)))
		}
	}

	return nil
}

// filterListQueryByPerm filters the list IDs in the query params and returns the list IDs to which the user has access.
func (a *App) filterListQueryByPerm(param string, qp url.Values, user auth.User) ([]int, error) {
	var listIDs []int

	// If there are incoming list query params, filter them by permission.
	if qp.Has(param) {
		ids, err := getQueryInts(param, qp)
		if err != nil {
			return nil, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidID"))
		}

		listIDs = user.FilterListsByPerm(auth.PermTypeGet|auth.PermTypeManage, ids)
	}

	// There are no incoming params. If the user doesn't have permission to get all subscribers,
	// filter by the lists they have access to.
	if len(listIDs) == 0 {
		if _, ok := user.PermissionsMap[auth.PermSubscribersGetAll]; !ok {
			if len(user.GetListIDs) > 0 {
				listIDs = user.GetListIDs
			} else {
				// User doesn't have access to any lists.
				listIDs = []int{-1}
			}
		}
	}

	return listIDs, nil
}

// formatSQLExp does basic sanitisation on arbitrary
// SQL query expressions coming from the frontend.
func formatSQLExp(q string) string {
	q = strings.TrimSpace(q)
	if len(q) == 0 {
		return ""
	}

	// Remove semicolon suffix.
	if q[len(q)-1] == ';' {
		q = q[:len(q)-1]
	}
	return q
}

// makeOptinNotifyHook returns an enclosed callback that sends optin confirmation e-mails.
// This is plugged into the 'core' package to send optin confirmations when a new subscriber is
// created via `core.CreateSubscriber()`.
func makeOptinNotifyHook(unsubHeader bool, u *UrlConfig, q *models.Queries, i *i18n.I18n) func(sub models.Subscriber, listIDs []int) (int, error) {
	return func(sub models.Subscriber, listIDs []int) (int, error) {
		// Fetch double opt-in lists from the given list IDs.
		// Get the list of subscription lists where the subscriber hasn't confirmed.
		var lists = []models.List{}
		if err := q.GetSubscriberLists.Select(&lists, sub.ID, nil, pq.Array(listIDs), nil, models.SubscriptionStatusUnconfirmed, models.ListOptinDouble); err != nil {
			lo.Printf("error fetching lists for opt-in: %s", err)
			return 0, err
		}

		// None.
		if len(lists) == 0 {
			return 0, nil
		}

		var (
			out      = subOptin{Subscriber: sub, Lists: lists}
			qListIDs = url.Values{}
		)

		// Construct the opt-in URL with list IDs.
		for _, l := range out.Lists {
			qListIDs.Add("l", l.UUID)
		}
		out.OptinURL = fmt.Sprintf(u.OptinURL, sub.UUID, qListIDs.Encode())
		out.UnsubURL = fmt.Sprintf(u.UnsubURL, dummyUUID, sub.UUID)

		// Unsub headers.
		hdr := textproto.MIMEHeader{}
		hdr.Set(models.EmailHeaderSubscriberUUID, sub.UUID)

		// Attach List-Unsubscribe headers?
		if unsubHeader {
			unsubURL := fmt.Sprintf(u.UnsubURL, dummyUUID, sub.UUID)
			hdr.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
			hdr.Set("List-Unsubscribe", `<`+unsubURL+`>`)
		}

		// Send the e-mail.
		if err := notifs.Notify([]string{sub.Email}, i.T("subscribers.optinSubject"), notifs.TplSubscriberOptin, out, hdr); err != nil {
			lo.Printf("error sending opt-in e-mail for subscriber %d (%s): %s", sub.ID, sub.UUID, err)
			return 0, err
		}

		return len(lists), nil
	}
}

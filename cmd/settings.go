package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gdgvda/cron"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx/types"
	"github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

const pwdMask = "•"

type aboutHost struct {
	OS       string `json:"os"`
	Machine  string `json:"arch"`
	Hostname string `json:"hostname"`
}

type aboutSystem struct {
	NumCPU  int    `json:"num_cpu"`
	AllocMB uint64 `json:"memory_alloc_mb"`
	OSMB    uint64 `json:"memory_from_os_mb"`
}

type about struct {
	Version   string         `json:"version"`
	Build     string         `json:"build"`
	GoVersion string         `json:"go_version"`
	GoArch    string         `json:"go_arch"`
	Database  types.JSONText `json:"database"`
	System    aboutSystem    `json:"system"`
	Host      aboutHost      `json:"host"`
}

var (
	reAlphaNum = regexp.MustCompile(`[^a-z0-9\-]`)
)

// GetSettings returns settings from the DB.
func (a *App) GetSettings(c echo.Context) error {
	s, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Empty out passwords.
	for i := range s.SMTP {
		s.SMTP[i].Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.SMTP[i].Password))
	}
	for i := range s.BounceBoxes {
		s.BounceBoxes[i].Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BounceBoxes[i].Password))
	}
	for i := range s.Messengers {
		s.Messengers[i].Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.Messengers[i].Password))
	}

	s.UploadS3AwsSecretAccessKey = strings.Repeat(pwdMask, utf8.RuneCountInString(s.UploadS3AwsSecretAccessKey))
	s.SendgridKey = strings.Repeat(pwdMask, utf8.RuneCountInString(s.SendgridKey))
	s.BouncePostmark.Password = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BouncePostmark.Password))
	s.BounceForwardEmail.Key = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BounceForwardEmail.Key))
	s.BouncePowermta.Key = strings.Repeat(pwdMask, utf8.RuneCountInString(s.BouncePowermta.Key))
	s.SecurityCaptcha.HCaptcha.Secret = strings.Repeat(pwdMask, utf8.RuneCountInString(s.SecurityCaptcha.HCaptcha.Secret))
	s.OIDC.ClientSecret = strings.Repeat(pwdMask, utf8.RuneCountInString(s.OIDC.ClientSecret))

	// Mask IMAP/POP passwords in personas
	for pi := range s.Personas {
		for ai := range s.Personas[pi].Accounts {
			if s.Personas[pi].Accounts[ai].IMAPPassword != "" {
				s.Personas[pi].Accounts[ai].IMAPPassword = strings.Repeat(pwdMask, utf8.RuneCountInString(s.Personas[pi].Accounts[ai].IMAPPassword))
			}
			if s.Personas[pi].Accounts[ai].POPPassword != "" {
				s.Personas[pi].Accounts[ai].POPPassword = strings.Repeat(pwdMask, utf8.RuneCountInString(s.Personas[pi].Accounts[ai].POPPassword))
			}
		}
	}

	return c.JSON(http.StatusOK, okResp{s})
}

// UpdateSettings returns settings from the DB.
func (a *App) UpdateSettings(c echo.Context) error {
	// Unmarshal and marshal the fields once to sanitize the settings blob.
	var set models.Settings
	if err := c.Bind(&set); err != nil {
		return err
	}

	// Get the existing settings.
	cur, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Validate and sanitize postback Messenger names along with SMTP names
	// (where each SMTP is also considered as a standalone messenger).
	// Duplicates are disallowed and "email" is a reserved name.
	names := map[string]bool{emailMsgr: true}

	// There should be at least one SMTP block that's enabled.
	has := false
	for i, s := range set.SMTP {
		if s.Enabled {
			has = true
		}

		// Sanitize and normalize the SMTP server name.
		name := reAlphaNum.ReplaceAllString(strings.ToLower(strings.TrimSpace(s.Name)), "-")
		if name != "" {
			if !strings.HasPrefix(name, "email-") {
				name = "email-" + name
			}

			if _, ok := names[name]; ok {
				return echo.NewHTTPError(http.StatusBadRequest,
					a.i18n.Ts("settings.duplicateMessengerName", "name", name))
			}

			names[name] = true
		}
		set.SMTP[i].Name = name

		// Assign a UUID. The frontend only sends a password when the user explicitly
		// changes the password. In other cases, the existing password in the DB
		// is copied while updating the settings and the UUID is used to match
		// the incoming array of SMTP blocks with the array in the DB.
		if s.UUID == "" {
			set.SMTP[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		// Ensure the HOST is trimmed of any whitespace.
		// This is a common mistake when copy-pasting SMTP settings.
		set.SMTP[i].Host = strings.TrimSpace(s.Host)

		// If there's no password coming in from the frontend, copy the existing
		// password by matching the UUID.
		if s.Password == "" {
			for _, c := range cur.SMTP {
				if s.UUID == c.UUID {
					set.SMTP[i].Password = c.Password
				}
			}
		}
	}
	if !has {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.errorNoSMTP"))
	}

	// Always remove the trailing slash from the app root URL.
	set.AppRootURL = strings.TrimRight(set.AppRootURL, "/")

	// Bounce boxes.
	for i, s := range set.BounceBoxes {
		// Assign a UUID. The frontend only sends a password when the user explicitly
		// changes the password. In other cases, the existing password in the DB
		// is copied while updating the settings and the UUID is used to match
		// the incoming array of blocks with the array in the DB.
		if s.UUID == "" {
			set.BounceBoxes[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		// Ensure the HOST is trimmed of any whitespace.
		// This is a common mistake when copy-pasting SMTP settings.
		set.BounceBoxes[i].Host = strings.TrimSpace(s.Host)

		if d, _ := time.ParseDuration(s.ScanInterval); d.Minutes() < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.bounces.invalidScanInterval"))
		}

		// If there's no password coming in from the frontend, copy the existing
		// password by matching the UUID.
		if s.Password == "" {
			for _, c := range cur.BounceBoxes {
				if s.UUID == c.UUID {
					set.BounceBoxes[i].Password = c.Password
				}
			}
		}
	}

	for i, m := range set.Messengers {
		// UUID to keep track of password changes similar to the SMTP logic above.
		if m.UUID == "" {
			set.Messengers[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		if m.Password == "" {
			for _, c := range cur.Messengers {
				if m.UUID == c.UUID {
					set.Messengers[i].Password = c.Password
				}
			}
		}

		name := reAlphaNum.ReplaceAllString(strings.ToLower(m.Name), "")
		if _, ok := names[name]; ok {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("settings.duplicateMessengerName", "name", name))
		}
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.invalidMessengerName"))
		}

		set.Messengers[i].Name = name
		names[name] = true
	}

	// S3 password?
	if set.UploadS3AwsSecretAccessKey == "" {
		set.UploadS3AwsSecretAccessKey = cur.UploadS3AwsSecretAccessKey
	}
	if set.SendgridKey == "" {
		set.SendgridKey = cur.SendgridKey
	}
	if set.BouncePostmark.Password == "" {
		set.BouncePostmark.Password = cur.BouncePostmark.Password
	}
	if set.BounceForwardEmail.Key == "" {
		set.BounceForwardEmail.Key = cur.BounceForwardEmail.Key
	}
	if set.BouncePowermta.Key == "" {
		set.BouncePowermta.Key = cur.BouncePowermta.Key
	}
	if set.SecurityCaptcha.HCaptcha.Secret == "" {
		set.SecurityCaptcha.HCaptcha.Secret = cur.SecurityCaptcha.HCaptcha.Secret
	}
	if set.OIDC.ClientSecret == "" {
		set.OIDC.ClientSecret = cur.OIDC.ClientSecret
	}

	// OIDC user auto-creation is enabled. Validate.
	if set.OIDC.AutoCreateUsers {
		if set.OIDC.DefaultUserRoleID.Int < auth.SuperAdminRoleID {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", a.i18n.T("settings.security.OIDCDefaultRole")))
		}
	}

	for n, v := range set.UploadExtensions {
		set.UploadExtensions[n] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "."))
	}

	// Domain blocklist / allowlist.
	doms := make([]string, 0, len(set.DomainBlocklist))
	for _, d := range set.DomainBlocklist {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			doms = append(doms, d)
		}
	}
	set.DomainBlocklist = doms

	doms = make([]string, 0, len(set.DomainAllowlist))
	for _, d := range set.DomainAllowlist {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			doms = append(doms, d)
		}
	}
	set.DomainAllowlist = doms

	// Validate slow query caching cron.
	if set.CacheSlowQueries {
		if _, err := cron.ParseStandard(set.CacheSlowQueriesInterval); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidData")+": slow query cron: "+err.Error())
		}
	}

	// Personas: sanitize domains, accounts, suppressions and SMTP lists. Also preserve IMAP passwords
	// masked by the UI (bullets) or when empty.
	if len(set.Personas) > 0 {
		// Build a map of existing SMTP names for validation.
		validSMTP := map[string]bool{}
		for _, s := range set.SMTP {
			if s.Name != "" {
				validSMTP[s.Name] = true
			}
		}

		// Current settings for password carryover
		curMap := map[string]map[string]string{}
		for _, p := range cur.Personas {
			am := map[string]string{}
			for _, a := range p.Accounts {
				am[strings.ToLower(a.Name)] = a.IMAPPassword
			}
			curMap[strings.ToLower(p.Domain)] = am
		}

		cleaned := make([]models.Persona, 0, len(set.Personas))
		seenDom := map[string]bool{}
		for _, p := range set.Personas {
			d := strings.ToLower(strings.TrimSpace(p.Domain))
			if d == "" || seenDom[d] {
				continue
			}
			seenDom[d] = true

			// SMTP allow-list: keep only known names and dedup
			smtpSeen := map[string]bool{}
			smtpOut := make([]string, 0, len(p.SMTP))
			for _, n := range p.SMTP {
				nm := strings.TrimSpace(strings.ToLower(n))
				// stored names are already normalized in update above; keep exact case from set.SMTP
				// try to match ignoring case
				var match string
				for vn := range validSMTP {
					if strings.EqualFold(vn, nm) {
						match = vn
						break
					}
				}
				if match != "" && !smtpSeen[match] {
					smtpSeen[match] = true
					smtpOut = append(smtpOut, match)
				}
			}

			// Accounts
			accSeen := map[string]bool{}
			accOut := make([]models.PersonaAccount, 0, len(p.Accounts))
			for _, a := range p.Accounts {
				name := strings.ToLower(strings.TrimSpace(a.Name))
				if name == "" || accSeen[name] {
					continue
				}
				// validate local part: a-z0-9._-
				ok := true
				for _, r := range name {
					if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
						continue
					}
					ok = false
					break
				}
				if !ok {
					continue
				}
				accSeen[name] = true

				// Carry over IMAP/POP passwords if masked or empty.
				pwd := strings.TrimSpace(a.IMAPPassword)
				if pwd == "" || strings.Count(pwd, pwdMask) == utf8.RuneCountInString(pwd) {
					if curPwd := curMap[d][name]; curPwd != "" {
						pwd = curPwd
					}
				}

				accOut = append(accOut, models.PersonaAccount{
					Name:              name,
					IMAPEnabled:       a.IMAPEnabled,
					IMAPHost:          strings.TrimSpace(a.IMAPHost),
					IMAPPort:          a.IMAPPort,
					IMAPUsername:      strings.TrimSpace(a.IMAPUsername),
					IMAPPassword:      pwd,
					IMAPTLS:           a.IMAPTLS,
					IMAPFolder:        strings.TrimSpace(a.IMAPFolder),
					IMAPCheckInterval: strings.TrimSpace(a.IMAPCheckInterval),
					POPEnabled:        a.POPEnabled,
					POPHost:           strings.TrimSpace(a.POPHost),
					POPPort:           a.POPPort,
					POPUsername:       strings.TrimSpace(a.POPUsername),
					POPPassword:       strings.TrimSpace(a.POPPassword),
					POPTLSEnabled:     a.POPTLSEnabled,
					POPTLSSkipVerify:  a.POPTLSSkipVerify,
					POPCheckInterval:  strings.TrimSpace(a.POPCheckInterval),
				})
			}

			// Suppressions normalize + dedup
			supSeen := map[string]bool{}
			supOut := make([]string, 0, len(p.Suppressions))
			for _, s := range p.Suppressions {
				em := strings.ToLower(strings.TrimSpace(s))
				if em == "" || supSeen[em] {
					continue
				}
				if _, err := mail.ParseAddress(em); err != nil {
					continue
				}
				supSeen[em] = true
				supOut = append(supOut, em)
			}

			cleaned = append(cleaned, models.Persona{
				Domain:       d,
				Label:        strings.TrimSpace(p.Label),
				SMTP:         smtpOut,
				Accounts:     accOut,
				Suppressions: supOut,
			})
		}
		set.Personas = cleaned
	}

	// Update the settings in the DB.
	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}

	// If there are any active campaigns, don't do an auto reload and
	// warn the user on the frontend.
	if a.manager.HasRunningCampaigns() {
		a.Lock()
		a.needsRestart = true
		a.Unlock()

		return c.JSON(http.StatusOK, okResp{struct {
			NeedsRestart bool `json:"needs_restart"`
		}{true}})
	}

	// No running campaigns. Reload the a.
	go func() {
		<-time.After(time.Millisecond * 500)
		a.chReload <- syscall.SIGHUP
	}()

	return c.JSON(http.StatusOK, okResp{true})
}

// GetLogs returns the log entries stored in the log buffer.
func (a *App) GetLogs(c echo.Context) error {
	return c.JSON(http.StatusOK, okResp{a.bufLog.Lines()})
}

// TestSMTPSettings returns the log entries stored in the log buffer.
func (a *App) TestSMTPSettings(c echo.Context) error {
	// Copy the raw JSON post body.
	reqBody, err := io.ReadAll(c.Request().Body)
	if err != nil {
		a.log.Printf("error reading SMTP test: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	// Load the JSON into koanf to parse SMTP settings properly including timestrings.
	ko := koanf.New(".")
	if err := ko.Load(rawbytes.Provider(reqBody), json.Parser()); err != nil {
		a.log.Printf("error unmarshalling SMTP test request: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	req := email.Server{}
	if err := ko.UnmarshalWithConf("", &req, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		a.log.Printf("error scanning SMTP test request: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	to := ko.String("email")
	if to == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.missingFields", "name", "email"))
	}

	// Initialize a new SMTP pool with strict timeouts to avoid UI 504 timeouts.
	req.MaxConns = 1
	req.IdleTimeout = 2 * time.Second
	req.PoolWaitTimeout = 2 * time.Second
	msgr, err := email.New("", req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorCreating", "name", "SMTP", "error", err.Error()))
	}

	// Render the test email template body.
	var b bytes.Buffer
	if err := notifs.Tpls.ExecuteTemplate(&b, "smtp-test", nil); err != nil {
		a.log.Printf("error compiling notification template '%s': %v", "smtp-test", err)
		return err
	}

	m := models.Message{}
	m.From = a.cfg.FromEmail
	m.To = []string{to}
	m.Subject = a.i18n.T("settings.smtp.testConnection")
	m.Body = b.Bytes()
	// Bound the send to a max duration to prevent gateway 504s.
	errCh := make(chan error, 1)
	go func() { errCh <- msgr.Push(m) }()
	select {
	case err := <-errCh:
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
	case <-time.After(12 * time.Second):
		return echo.NewHTTPError(http.StatusGatewayTimeout, "SMTP test timed out (check host/port/TLS)")
	}

	return c.JSON(http.StatusOK, okResp{a.bufLog.Lines()})
}

func (a *App) GetAboutInfo(c echo.Context) error {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	out := a.about
	out.System.AllocMB = mem.Alloc / 1024 / 1024
	out.System.OSMB = mem.Sys / 1024 / 1024

	return c.JSON(http.StatusOK, out)
}

// CheckSMTPServer performs a basic connectivity and DNS check for an SMTP server.
func (a *App) CheckSMTPServer(c echo.Context) error {
	name := c.Param("name")
	// Normalize like UpdateSMTPServer
	normalizedName := reAlphaNum.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	if normalizedName != "" && !strings.HasPrefix(normalizedName, "email-") {
		normalizedName = "email-" + normalizedName
	}

	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}
	var found bool
	var idx int
	for i := range set.SMTP {
		if set.SMTP[i].Name == normalizedName {
			found = true
			idx = i
			break
		}
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "smtp not found")
	}

	s := set.SMTP[idx]
	// Derive host domain (from host/hello_hostname/username)
	hostDomain := s.HelloHostname
	if hostDomain == "" {
		if addr, err := mail.ParseAddress(s.Username); err == nil {
			if parts := strings.Split(addr.Address, "@"); len(parts) == 2 {
				hostDomain = parts[1]
			}
		}
	}
	if hostDomain == "" {
		hostDomain = strings.TrimPrefix(s.Host, "smtp.")
	}
	// Derive From domain: query param takes precedence, then From header, then same as host.
	fromDomain := strings.TrimSpace(c.QueryParam("from_domain"))
	if fromDomain == "" {
		// EmailHeaders is an array of maps.
		for _, hm := range s.EmailHeaders {
			if v, ok := hm["From"]; ok {
				if addr, err := mail.ParseAddress(v); err == nil {
					if parts := strings.Split(addr.Address, "@"); len(parts) == 2 {
						fromDomain = parts[1]
						break
					}
				}
			}
		}
	}
	if fromDomain == "" {
		fromDomain = hostDomain
	}

	// Connect
	connOK := false
	banner := ""
	var connErr string
	if s.Host != "" && s.Port != 0 {
		d := net.Dialer{Timeout: 10 * time.Second}
		conn, err := d.Dial("tcp", net.JoinHostPort(s.Host, strconv.Itoa(s.Port)))
		if err == nil {
			connOK = true
			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			r := bufio.NewReader(conn)
			line, _ := r.ReadString('\n')
			banner = strings.TrimSpace(line)
			_ = conn.Close()
		} else {
			connErr = err.Error()
		}
	}

	// DNS checks
	spfOK := false
	dmarcOK := false
	var spfTxt, dmarcTxt string
	var spfErr, dmarcErr string
	if fromDomain != "" {
		if txts, err := net.LookupTXT(fromDomain); err == nil {
			for _, t := range txts {
				if strings.Contains(strings.ToLower(t), "v=spf1") {
					spfOK = true
					spfTxt = t
					break
				}
			}
		} else {
			spfErr = err.Error()
		}
		if txts, err := net.LookupTXT("_dmarc." + fromDomain); err == nil {
			for _, t := range txts {
				if strings.Contains(strings.ToLower(t), "v=dmarc1") {
					dmarcOK = true
					dmarcTxt = t
					break
				}
			}
		} else {
			dmarcErr = err.Error()
		}
	}

	out := map[string]any{
		"name":             name,
		"host":             s.Host,
		"port":             s.Port,
		"domain":           fromDomain,
		"host_domain":      hostDomain,
		"connection_ok":    connOK,
		"banner":           banner,
		"connection_error": connErr,
		"spf_ok":           spfOK,
		"spf_record":       spfTxt,
		"spf_error":        spfErr,
		"dmarc_ok":         dmarcOK,
		"dmarc_record":     dmarcTxt,
		"dmarc_error":      dmarcErr,
		"dkim_info":        "DKIM проверка требует селектор; укажите селектор для детальной проверки",
	}
	return c.JSON(http.StatusOK, okResp{out})
}

type smtpStat struct {
	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name"`
	SentTotal      int64  `json:"sent_total"`
	SentToday      int64  `json:"sent_today"`
	DailyLimit     int    `json:"daily_limit"`
	SendPause      string `json:"send_pause"`

	// WarmCloud settings
	WarmCloudEnabled             bool   `json:"warmcloud_enabled"`
	WarmCloudToken               string `json:"warmcloud_token,omitempty"`
	WarmCloudUserID              int    `json:"warmcloud_user_id"`
	WarmCloudRefID               string `json:"warmcloud_ref_id"`
	WarmCloudBig                 string `json:"warmcloud_big"`
	WarmCloudDailyLimit          int    `json:"warmcloud_daily_limit"`
	WarmCloudDailyStep           int    `json:"warmcloud_daily_step"`
	WarmCloudCampaignID          int    `json:"warmcloud_campaign_id"`
	WarmCloudReadPercent         int    `json:"warmcloud_read_percent"`
	WarmCloudMaxPerTick          int    `json:"warmcloud_max_per_tick"`
	WarmCloudSleepBetween        string `json:"warmcloud_sleep_between"`
	WarmCloudRateCount           int    `json:"warmcloud_rate_count"`
	WarmCloudRateWindowMinutes   int    `json:"warmcloud_rate_window_minutes"`
	WarmCloudRateJitterPct       int    `json:"warmcloud_rate_jitter_pct"`
	WarmCloudWorkingHoursEnabled bool   `json:"warmcloud_working_hours_enabled"`
	WarmCloudWorkingHours        string `json:"warmcloud_working_hours"`

	// Warming progress
	WarmCloudSentToday      int `json:"warmcloud_sent_today"`
	WarmCloudCurrentLimit   int `json:"warmcloud_current_limit"`
	WarmCloudSubmittedToday int `json:"warmcloud_submitted_today"`
	WarmCloudTargetSubmit   int `json:"warmcloud_target_submit"`
	WarmCloudPendingSlots   int `json:"warmcloud_pending_slots"`
}

// GetSMTPStats returns stats for configured SMTP servers, persisted in DB.
func (a *App) GetSMTPStats(c echo.Context) error {
	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	out := make([]smtpStat, 0, len(set.SMTP))
	for _, cfg := range set.SMTP {
		// Display name without email- prefix.
		disp := cfg.Name
		if strings.HasPrefix(disp, "email-") {
			disp = strings.TrimPrefix(disp, "email-")
		}

		st := smtpStat{
			Name:           disp,
			NormalizedName: cfg.Name,
			DailyLimit:     cfg.DailyLimit,
			SendPause:      cfg.SendPause,
			// Effective enablement respects global gate.
			WarmCloudEnabled:             (cfg.WarmCloudEnabled && set.WarmCloudGlobalEnabled),
			WarmCloudToken:               cfg.WarmCloudToken,
			WarmCloudUserID:              cfg.WarmCloudUserID,
			WarmCloudRefID:               cfg.WarmCloudRefID,
			WarmCloudBig:                 cfg.WarmCloudBig,
			WarmCloudDailyLimit:          cfg.WarmCloudDailyLimit,
			WarmCloudDailyStep:           cfg.WarmCloudDailyStep,
			WarmCloudCampaignID:          cfg.WarmCloudCampaignID,
			WarmCloudReadPercent:         cfg.WarmCloudReadPercent,
			WarmCloudMaxPerTick:          cfg.WarmCloudMaxPerTick,
			WarmCloudSleepBetween:        cfg.WarmCloudSleepBetween,
			WarmCloudRateCount:           cfg.WarmCloudRateCount,
			WarmCloudRateWindowMinutes:   cfg.WarmCloudRateWindowMinutes,
			WarmCloudRateJitterPct:       cfg.WarmCloudRateJitterPct,
			WarmCloudWorkingHoursEnabled: cfg.WarmCloudWorkingHoursEnabled,
			WarmCloudWorkingHours:        cfg.WarmCloudWorkingHours,
		}

		// If server has a normalized name, fetch persisted counters.
		if cfg.Name != "" {
			var today, total int64
			if err := a.queries.SmtpGetToday.Get(&today, cfg.Name); err == nil {
				st.SentToday = today
			}
			if err := a.queries.SmtpGetTotal.Get(&total, cfg.Name); err == nil {
				st.SentTotal = total
			}

			// Load persisted WarmCloud counts (namespaced key) as fallback across restarts.
			var wcToday int64
			if err := a.queries.SmtpGetToday.Get(&wcToday, "warmcloud:"+cfg.Name); err == nil {
				if int(wcToday) > st.WarmCloudSentToday {
					st.WarmCloudSentToday = int(wcToday)
				}
			}
			var wcReadToday int64
			if err := a.queries.SmtpGetToday.Get(&wcReadToday, "warmcloud_read:"+cfg.Name); err == nil {
				st.WarmCloudSubmittedToday = int(wcReadToday)
			}

			// For warming progress, pick from in-memory messenger servers.
			// If there are duplicate instances (group vs individual), pick the max values.
			for _, m := range a.messengers {
				em, ok := m.(*email.Emailer)
				if !ok {
					continue
				}
				for _, srv := range em.Servers() {
					if srv.Name == cfg.Name {
						if srv.WarmCloudSentToday > st.WarmCloudSentToday {
							st.WarmCloudSentToday = srv.WarmCloudSentToday
						}
						if srv.WarmCloudCurrentLimit > st.WarmCloudCurrentLimit {
							st.WarmCloudCurrentLimit = srv.WarmCloudCurrentLimit
						}
						if srv.WarmCloudSubmittedToday > st.WarmCloudSubmittedToday {
							st.WarmCloudSubmittedToday = srv.WarmCloudSubmittedToday
						}
					}
				}
			}
			// Pending scheduled slots today (if scheduling is enabled)
			var pend int64
			if err := a.queries.WarmSchedCountPendingToday.Get(&pend, cfg.Name); err == nil {
				st.WarmCloudPendingSlots = int(pend)
			}
		}

		// Compute target confirmations for today.
		limitForToday := st.WarmCloudCurrentLimit
		if limitForToday == 0 {
			limitForToday = st.WarmCloudDailyLimit
		}
		if st.WarmCloudReadPercent > 0 {
			st.WarmCloudTargetSubmit = (limitForToday*st.WarmCloudReadPercent + 99) / 100
		}

		out = append(out, st)
	}

	return c.JSON(http.StatusOK, okResp{out})
}

// GetPersonas returns the personas array from settings.
func (a *App) GetPersonas(c echo.Context) error {
	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{set.Personas})
}

type personasUpdateReq struct {
	Personas []models.Persona `json:"personas"`
}

// UpdatePersonas replaces the personas array in settings.
func (a *App) UpdatePersonas(c echo.Context) error {
	var req personasUpdateReq
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Load current settings, update personas, persist.
	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Normalize and validate (same logic as UpdateSettings personas block)
	norm := make([]models.Persona, 0, len(req.Personas))
	seen := map[string]bool{}
	for _, p := range req.Personas {
		d := strings.ToLower(strings.TrimSpace(p.Domain))
		if d == "" {
			continue
		}
		if seen[d] {
			continue
		}
		seen[d] = true

		// Accounts
		accSeen := map[string]bool{}
		accs := make([]models.PersonaAccount, 0, len(p.Accounts))
		for _, a := range p.Accounts {
			lp := strings.ToLower(strings.TrimSpace(a.Name))
			if lp == "" || accSeen[lp] {
				continue
			}
			ok := true
			for _, r := range lp {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
					continue
				}
				ok = false
				break
			}
			if !ok {
				continue
			}
			accSeen[lp] = true
			accs = append(accs, models.PersonaAccount{
				Name:              lp,
				IMAPEnabled:       a.IMAPEnabled,
				IMAPHost:          strings.TrimSpace(a.IMAPHost),
				IMAPPort:          a.IMAPPort,
				IMAPUsername:      strings.TrimSpace(a.IMAPUsername),
				IMAPPassword:      strings.TrimSpace(a.IMAPPassword),
				IMAPTLS:           a.IMAPTLS,
				IMAPFolder:        strings.TrimSpace(a.IMAPFolder),
				IMAPCheckInterval: strings.TrimSpace(a.IMAPCheckInterval),
				POPEnabled:        a.POPEnabled,
				POPHost:           strings.TrimSpace(a.POPHost),
				POPPort:           a.POPPort,
				POPUsername:       strings.TrimSpace(a.POPUsername),
				POPPassword:       strings.TrimSpace(a.POPPassword),
				POPTLSEnabled:     a.POPTLSEnabled,
				POPTLSSkipVerify:  a.POPTLSSkipVerify,
				POPCheckInterval:  strings.TrimSpace(a.POPCheckInterval),
			})
		}

		// Dedup suppressions and lowercase.
		supSeen := map[string]bool{}
		sups := make([]string, 0, len(p.Suppressions))
		for _, s := range p.Suppressions {
			em := strings.ToLower(strings.TrimSpace(s))
			if em == "" || supSeen[em] {
				continue
			}
			// Very light email shape check.
			if _, err := mail.ParseAddress(em); err != nil {
				continue
			}
			supSeen[em] = true
			sups = append(sups, em)
		}

		// SMTP list dedup (kept as-is here; UpdateSettings does stricter validation)
		smtpSeen := map[string]bool{}
		smtp := make([]string, 0, len(p.SMTP))
		for _, n := range p.SMTP {
			nm := strings.TrimSpace(n)
			if nm == "" || smtpSeen[nm] {
				continue
			}
			smtpSeen[nm] = true
			smtp = append(smtp, nm)
		}

		norm = append(norm, models.Persona{Domain: d, Label: strings.TrimSpace(p.Label), SMTP: smtp, Accounts: models.PersonaAccounts(accs), Suppressions: sups})
	}

	set.Personas = norm
	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}

type personaUpdateReq struct {
	Label        *string                 `json:"label"`
	SMTP         *[]string               `json:"smtp"`
	Accounts     []models.PersonaAccount `json:"accounts"`
	Suppressions []string                `json:"suppressions"`
}

// UpdatePersonaByDomain updates a single persona (matched by :domain).
func (a *App) UpdatePersonaByDomain(c echo.Context) error {
	domain := strings.ToLower(strings.TrimSpace(c.Param("domain")))
	if domain == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	var req personaUpdateReq
	if err := c.Bind(&req); err != nil {
		return err
	}

	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	updated := false
	for i, p := range set.Personas {
		if strings.EqualFold(p.Domain, domain) {
			if req.Label != nil {
				set.Personas[i].Label = strings.TrimSpace(*req.Label)
			}

			if req.Accounts != nil {
				// Build old account map for password carryover
				old := map[string]models.PersonaAccount{}
				for _, oa := range set.Personas[i].Accounts {
					old[strings.ToLower(oa.Name)] = oa
				}

				accSeen := map[string]bool{}
				accs := make([]models.PersonaAccount, 0, len(req.Accounts))
				for _, a := range req.Accounts {
					lp := strings.ToLower(strings.TrimSpace(a.Name))
					if lp == "" || accSeen[lp] {
						continue
					}
					ok := true
					for _, r := range lp {
						if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
							continue
						}
						ok = false
						break
					}
					if !ok {
						continue
					}
					accSeen[lp] = true

					pwd := strings.TrimSpace(a.IMAPPassword)
					if pwd == "" || strings.Count(pwd, pwdMask) == utf8.RuneCountInString(pwd) {
						if oa, ok := old[lp]; ok {
							pwd = oa.IMAPPassword
						}
					}

					accs = append(accs, models.PersonaAccount{
						Name:              lp,
						IMAPEnabled:       a.IMAPEnabled,
						IMAPHost:          strings.TrimSpace(a.IMAPHost),
						IMAPPort:          a.IMAPPort,
						IMAPUsername:      strings.TrimSpace(a.IMAPUsername),
						IMAPPassword:      pwd,
						IMAPTLS:           a.IMAPTLS,
						IMAPFolder:        strings.TrimSpace(a.IMAPFolder),
						IMAPCheckInterval: strings.TrimSpace(a.IMAPCheckInterval),
					})
				}
				set.Personas[i].Accounts = models.PersonaAccounts(accs)
			}

			if req.SMTP != nil {
				// Assign as-is; full validation is part of UpdateSettings flow.
				smtpSeen := map[string]bool{}
				out := make([]string, 0, len(*req.SMTP))
				for _, n := range *req.SMTP {
					nm := strings.TrimSpace(n)
					if nm == "" || smtpSeen[nm] {
						continue
					}
					smtpSeen[nm] = true
					out = append(out, nm)
				}
				set.Personas[i].SMTP = out
			}

			if req.Suppressions != nil {
				supSeen := map[string]bool{}
				sups := make([]string, 0, len(req.Suppressions))
				for _, s := range req.Suppressions {
					em := strings.ToLower(strings.TrimSpace(s))
					if em == "" || supSeen[em] {
						continue
					}
					if _, err := mail.ParseAddress(em); err != nil {
						continue
					}
					supSeen[em] = true
					sups = append(sups, em)
				}
				set.Personas[i].Suppressions = sups
			}
			updated = true
			break
		}
	}
	if !updated {
		// If persona doesn't exist, append a new one with minimal info.
		np := models.Persona{Domain: domain}
		if req.Label != nil {
			np.Label = strings.TrimSpace(*req.Label)
		}
		// Accounts
		accSeen := map[string]bool{}
		for _, a := range req.Accounts {
			lp := strings.ToLower(strings.TrimSpace(a.Name))
			if lp == "" || accSeen[lp] {
				continue
			}
			ok := true
			for _, r := range lp {
				if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
					continue
				}
				ok = false
				break
			}
			if !ok {
				continue
			}
			accSeen[lp] = true
			np.Accounts = append(np.Accounts, models.PersonaAccount{
				Name:              lp,
				IMAPEnabled:       a.IMAPEnabled,
				IMAPHost:          strings.TrimSpace(a.IMAPHost),
				IMAPPort:          a.IMAPPort,
				IMAPUsername:      strings.TrimSpace(a.IMAPUsername),
				IMAPPassword:      strings.TrimSpace(a.IMAPPassword),
				IMAPTLS:           a.IMAPTLS,
				IMAPFolder:        strings.TrimSpace(a.IMAPFolder),
				IMAPCheckInterval: strings.TrimSpace(a.IMAPCheckInterval),
			})
		}
		// Suppressions
		supSeen := map[string]bool{}
		for _, s := range req.Suppressions {
			em := strings.ToLower(strings.TrimSpace(s))
			if em == "" || supSeen[em] {
				continue
			}
			if _, err := mail.ParseAddress(em); err != nil {
				continue
			}
			supSeen[em] = true
			np.Suppressions = append(np.Suppressions, em)
		}
		set.Personas = append(set.Personas, np)
	}

	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}

// DeletePersonaByDomain deletes a persona by domain.
func (a *App) DeletePersonaByDomain(c echo.Context) error {
	domain := strings.ToLower(strings.TrimSpace(c.Param("domain")))
	if domain == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}
	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}
	out := make([]models.Persona, 0, len(set.Personas))
	removed := false
	for _, p := range set.Personas {
		if strings.EqualFold(p.Domain, domain) {
			removed = true
			continue
		}
		out = append(out, p)
	}
	if !removed {
		return c.JSON(http.StatusOK, okResp{true})
	}
	set.Personas = out
	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, okResp{true})
}

type smtpUpdate struct {
	DailyLimit *int    `json:"daily_limit"`
	SendPause  *string `json:"send_pause"`

	// WarmCloud settings (per-server)
	WarmCloudEnabled             *bool   `json:"warmcloud_enabled"`
	WarmCloudToken               *string `json:"warmcloud_token"`
	WarmCloudUserID              *int    `json:"warmcloud_user_id"`
	WarmCloudRefID               *string `json:"warmcloud_ref_id"`
	WarmCloudBig                 *string `json:"warmcloud_big"`
	WarmCloudDailyLimit          *int    `json:"warmcloud_daily_limit"`
	WarmCloudDailyStep           *int    `json:"warmcloud_daily_step"`
	WarmCloudCampaignID          *int    `json:"warmcloud_campaign_id"`
	WarmCloudReadPercent         *int    `json:"warmcloud_read_percent"`
	WarmCloudMaxPerTick          *int    `json:"warmcloud_max_per_tick"`
	WarmCloudSleepBetween        *string `json:"warmcloud_sleep_between"`
	WarmCloudRateCount           *int    `json:"warmcloud_rate_count"`
	WarmCloudRateWindowMinutes   *int    `json:"warmcloud_rate_window_minutes"`
	WarmCloudRateJitterPct       *int    `json:"warmcloud_rate_jitter_pct"`
	WarmCloudWorkingHoursEnabled *bool   `json:"warmcloud_working_hours_enabled"`
	WarmCloudWorkingHours        *string `json:"warmcloud_working_hours"`
}

// UpdateSMTPServer updates per-server limits.
func (a *App) UpdateSMTPServer(c echo.Context) error {
	name := c.Param("name")
	var req smtpUpdate
	if err := c.Bind(&req); err != nil {
		return err
	}

	// Normalize the name to match the format used in settings
	normalizedName := reAlphaNum.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	if normalizedName != "" && !strings.HasPrefix(normalizedName, "email-") {
		normalizedName = "email-" + normalizedName
	}

	// Update settings in DB.
	msgr := a.emailMsgr.(*email.Emailer)
	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}
	toggledOn := false
	toggledOff := false
	rebuildSchedule := false
	newCurrentLimit := 0
	// If global WarmCloud is disabled, disallow enabling at server level.
	if !set.WarmCloudGlobalEnabled && req.WarmCloudEnabled != nil && *req.WarmCloudEnabled {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.warmcloud.globalDisabled"))
	}
	found := false
	for i := range set.SMTP {
		if set.SMTP[i].Name == normalizedName {
			prevEnabled := set.SMTP[i].WarmCloudEnabled
			if req.DailyLimit != nil {
				set.SMTP[i].DailyLimit = *req.DailyLimit
			}
			if req.SendPause != nil {
				set.SMTP[i].SendPause = *req.SendPause
			}

			// Update WarmCloud settings (only provided fields)
			if req.WarmCloudEnabled != nil {
				set.SMTP[i].WarmCloudEnabled = *req.WarmCloudEnabled
				if *req.WarmCloudEnabled && !prevEnabled {
					toggledOn = true
				}
				if !*req.WarmCloudEnabled && prevEnabled {
					toggledOff = true
				}
			}
			if req.WarmCloudToken != nil {
				set.SMTP[i].WarmCloudToken = *req.WarmCloudToken
			}
			if req.WarmCloudUserID != nil {
				set.SMTP[i].WarmCloudUserID = *req.WarmCloudUserID
			}
			if req.WarmCloudRefID != nil {
				set.SMTP[i].WarmCloudRefID = *req.WarmCloudRefID
			}
			if req.WarmCloudBig != nil {
				set.SMTP[i].WarmCloudBig = *req.WarmCloudBig
			}
			if req.WarmCloudDailyLimit != nil {
				set.SMTP[i].WarmCloudDailyLimit = *req.WarmCloudDailyLimit
			}
			if req.WarmCloudDailyStep != nil {
				set.SMTP[i].WarmCloudDailyStep = *req.WarmCloudDailyStep
			}
			if req.WarmCloudCampaignID != nil {
				set.SMTP[i].WarmCloudCampaignID = *req.WarmCloudCampaignID
			}
			if req.WarmCloudReadPercent != nil {
				set.SMTP[i].WarmCloudReadPercent = *req.WarmCloudReadPercent
			}
			if req.WarmCloudMaxPerTick != nil {
				set.SMTP[i].WarmCloudMaxPerTick = *req.WarmCloudMaxPerTick
			}
			if req.WarmCloudSleepBetween != nil {
				set.SMTP[i].WarmCloudSleepBetween = *req.WarmCloudSleepBetween
			}
			if req.WarmCloudRateCount != nil {
				set.SMTP[i].WarmCloudRateCount = *req.WarmCloudRateCount
			}
			if req.WarmCloudRateWindowMinutes != nil {
				set.SMTP[i].WarmCloudRateWindowMinutes = *req.WarmCloudRateWindowMinutes
			}
			if req.WarmCloudRateJitterPct != nil {
				set.SMTP[i].WarmCloudRateJitterPct = *req.WarmCloudRateJitterPct
			}
			if req.WarmCloudWorkingHoursEnabled != nil {
				set.SMTP[i].WarmCloudWorkingHoursEnabled = *req.WarmCloudWorkingHoursEnabled
			}
			if req.WarmCloudWorkingHours != nil {
				set.SMTP[i].WarmCloudWorkingHours = *req.WarmCloudWorkingHours
			}

			if toggledOn {
				base := set.SMTP[i].WarmCloudDailyLimit
				if req.WarmCloudDailyLimit != nil && *req.WarmCloudDailyLimit > 0 {
					base = *req.WarmCloudDailyLimit
				}
				if base > 0 {
					newCurrentLimit = base
				}
			} else if req.WarmCloudDailyLimit != nil && *req.WarmCloudDailyLimit > 0 {
				newCurrentLimit = *req.WarmCloudDailyLimit
			}

			found = true
			break
		}
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "smtp not found")
	}
	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}

	// Update in-memory server.
	for _, s := range msgr.Servers() {
		if s.Name == normalizedName {
			if req.DailyLimit != nil {
				s.DailyLimit = *req.DailyLimit
			}
			if req.SendPause != nil {
				s.SendPause = *req.SendPause
			}

			if req.WarmCloudEnabled != nil {
				s.WarmCloudEnabled = *req.WarmCloudEnabled
			}
			if req.WarmCloudToken != nil {
				s.WarmCloudToken = *req.WarmCloudToken
			}
			if req.WarmCloudUserID != nil {
				s.WarmCloudUserID = *req.WarmCloudUserID
			}
			if req.WarmCloudRefID != nil {
				s.WarmCloudRefID = *req.WarmCloudRefID
			}
			if req.WarmCloudBig != nil {
				s.WarmCloudBig = *req.WarmCloudBig
			}
			if req.WarmCloudDailyLimit != nil {
				s.WarmCloudDailyLimit = *req.WarmCloudDailyLimit
			}
			if req.WarmCloudDailyStep != nil {
				s.WarmCloudDailyStep = *req.WarmCloudDailyStep
			}
			if req.WarmCloudCampaignID != nil {
				s.WarmCloudCampaignID = *req.WarmCloudCampaignID
			}
			if req.WarmCloudReadPercent != nil {
				s.WarmCloudReadPercent = *req.WarmCloudReadPercent
			}
			if req.WarmCloudMaxPerTick != nil {
				s.WarmCloudMaxPerTick = *req.WarmCloudMaxPerTick
			}
			if req.WarmCloudSleepBetween != nil {
				s.WarmCloudSleepBetween = *req.WarmCloudSleepBetween
			}
			if req.WarmCloudRateCount != nil {
				s.WarmCloudRateCount = *req.WarmCloudRateCount
			}
			if req.WarmCloudRateWindowMinutes != nil {
				s.WarmCloudRateWindowMinutes = *req.WarmCloudRateWindowMinutes
			}
			if req.WarmCloudRateJitterPct != nil {
				s.WarmCloudRateJitterPct = *req.WarmCloudRateJitterPct
			}
			if req.WarmCloudWorkingHoursEnabled != nil {
				s.WarmCloudWorkingHoursEnabled = *req.WarmCloudWorkingHoursEnabled
			}
			if req.WarmCloudWorkingHours != nil {
				s.WarmCloudWorkingHours = *req.WarmCloudWorkingHours
			}

			rebuildSchedule = s.WarmCloudEnabled

			if newCurrentLimit > 0 {
				s.WarmCloudCurrentLimit = newCurrentLimit
				if _, err := a.queries.SmtpSetToday.Exec("warmcloud_limit:"+normalizedName, newCurrentLimit); err != nil {
					a.log.Printf("error persisting warmcloud limit for %s: %v", normalizedName, err)
				}
			}
			break
		}
	}

	// Rebuild near-future schedule for this server to reflect updated settings.
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		if toggledOff {
			if _, err := a.queries.WarmSchedDeleteUnsentOnly.Exec(normalizedName); err != nil {
				a.log.Printf("error deleting pending warmcloud slots for %s: %v", normalizedName, err)
			}
		}
		if rebuildSchedule {
			em.WarmingRebuildSchedule(normalizedName)
		}
	}

	return c.JSON(http.StatusOK, okResp{"ok"})
}

// UpdateWarmCloudGlobalSettings updates global WarmCloud settings
func (a *App) UpdateWarmCloudGlobalSettings(c echo.Context) error {
	var req struct {
		WarmCloudGlobalEnabled           bool   `json:"warmcloud.global_enabled"`
		WarmCloudGlobalToken             string `json:"warmcloud.global_token"`
		WarmCloudGlobalUserID            int    `json:"warmcloud.global_user_id"`
		WarmCloudGlobalRefID             string `json:"warmcloud.global_ref_id"`
		WarmCloudGlobalBig               string `json:"warmcloud.global_big"`
		WarmCloudGlobalDefaultLimit      int    `json:"warmcloud.global_default_limit"`
		WarmCloudGlobalDefaultStep       int    `json:"warmcloud.global_default_step"`
		WarmCloudGlobalDefaultCampaignID int    `json:"warmcloud.global_default_campaign_id"`
		WarmCloudGlobalWorkingHours      string `json:"warmcloud.global_working_hours"`
		WarmCloudGlobalReadPercent       int    `json:"warmcloud.global_read_percent"`
		WarmCloudGlobalMaxPerTick        int    `json:"warmcloud.global_max_per_tick"`
		WarmCloudGlobalSleepBetween      string `json:"warmcloud.global_sleep_between"`
	}

	if err := c.Bind(&req); err != nil {
		return err
	}

	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Update global WarmCloud settings
	set.WarmCloudGlobalEnabled = req.WarmCloudGlobalEnabled
	set.WarmCloudGlobalToken = req.WarmCloudGlobalToken
	set.WarmCloudGlobalUserID = req.WarmCloudGlobalUserID
	set.WarmCloudGlobalRefID = req.WarmCloudGlobalRefID
	set.WarmCloudGlobalBig = req.WarmCloudGlobalBig
	set.WarmCloudGlobalDefaultLimit = req.WarmCloudGlobalDefaultLimit
	set.WarmCloudGlobalDefaultStep = req.WarmCloudGlobalDefaultStep
	set.WarmCloudGlobalDefaultCampaignID = req.WarmCloudGlobalDefaultCampaignID
	set.WarmCloudGlobalWorkingHours = req.WarmCloudGlobalWorkingHours
	set.WarmCloudGlobalReadPercent = req.WarmCloudGlobalReadPercent
	set.WarmCloudGlobalMaxPerTick = req.WarmCloudGlobalMaxPerTick
	set.WarmCloudGlobalSleepBetween = req.WarmCloudGlobalSleepBetween

	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}

	return c.JSON(http.StatusOK, okResp{"ok"})
}

// GetWarmCloudStatus returns per-server warming status for UI controls.
func (a *App) GetWarmCloudStatus(c echo.Context) error {
	em, ok := a.emailMsgr.(*email.Emailer)
	if !ok || em == nil {
		return c.JSON(http.StatusOK, okResp{map[string]any{}})
	}
	st := em.GetWarmingStatus()
	// Convert keys to display names (trim email- prefix if present)
	out := map[string]any{}
	for k, v := range st {
		disp := k
		if strings.HasPrefix(disp, "email-") {
			disp = strings.TrimPrefix(disp, "email-")
		}
		// Augment with schedule data: next due and pending today.
		if m, ok2 := v.(map[string]interface{}); ok2 {
			// next due in the future (UI display)
			{
				var due time.Time
				q := `SELECT due_at FROM warmcloud_schedule WHERE server = $1 AND status = 'pending' AND due_at > NOW() ORDER BY due_at ASC LIMIT 1`
				if err := a.db.Get(&due, q, k); err == nil {
					m["next_at"] = due
				}
			}
			// pending count today
			var n int64
			if err := a.queries.WarmSchedCountPendingToday.Get(&n, k); err == nil {
				m["pending_today"] = n
			}
			out[disp] = m
		} else {
			out[disp] = v
		}
	}
	return c.JSON(http.StatusOK, okResp{out})
}

// normalizeSMTPName ensures consistency with settings/internal naming.
func normalizeSMTPName(name string) string {
	s := reAlphaNum.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	if s != "" && !strings.HasPrefix(s, "email-") {
		s = "email-" + s
	}
	return s
}

// PauseWarmCloud pauses warming for a server.
func (a *App) PauseWarmCloud(c echo.Context) error {
	n := normalizeSMTPName(c.Param("name"))
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		em.PauseWarming(n)
	}
	return c.JSON(http.StatusOK, okResp{"ok"})
}

// ResumeWarmCloud resumes warming for a server.
func (a *App) ResumeWarmCloud(c echo.Context) error {
	n := normalizeSMTPName(c.Param("name"))
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		em.ResumeWarming(n)
	}
	return c.JSON(http.StatusOK, okResp{"ok"})
}

// ForceWarmCloudTick triggers a one-off run.
func (a *App) ForceWarmCloudTick(c echo.Context) error {
	n := normalizeSMTPName(c.Param("name"))
	started := false
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		started = em.ForceWarmingTick(n)
	}
	return c.JSON(http.StatusOK, okResp{map[string]bool{"started": started}})
}

// ResetWarmCloudDaily resets daily counters for a server.
func (a *App) ResetWarmCloudDaily(c echo.Context) error {
	n := normalizeSMTPName(c.Param("name"))
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		em.ResetWarmingDaily(n)
	}
	return c.JSON(http.StatusOK, okResp{"ok"})
}

// ResetWarmCloudDailyAll resets daily counters for all servers.
func (a *App) ResetWarmCloudDailyAll(c echo.Context) error {
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		em.ResetWarmingDailyAll()
	}
	return c.JSON(http.StatusOK, okResp{"ok"})
}

// RebuildWarmCloudSchedule deletes future slots and replans the immediate window for a server.
func (a *App) RebuildWarmCloudSchedule(c echo.Context) error {
	n := normalizeSMTPName(c.Param("name"))
	if em, ok := a.emailMsgr.(*email.Emailer); ok && em != nil {
		em.WarmingRebuildSchedule(n)
	}
	return c.JSON(http.StatusOK, okResp{"ok"})
}

// GetWarmCloudBalance queries WarmCloud for the current account balance using global settings.
func (a *App) GetWarmCloudBalance(c echo.Context) error {
	set, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	if !set.WarmCloudGlobalEnabled {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.warmcloud.notEnabled"))
	}
	if set.WarmCloudGlobalToken == "" || set.WarmCloudGlobalUserID == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.warmcloud.credentialsMissing"))
	}

	wc := email.NewWarmCloudClient(set.WarmCloudGlobalToken, set.WarmCloudGlobalUserID, set.WarmCloudGlobalRefID)
	// Slightly shorter timeout for a UI action
	wc.WithHTTPClient(&http.Client{Timeout: 15 * time.Second})

	bal, err := wc.GetBalance()
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, err.Error())
	}
	return c.JSON(http.StatusOK, okResp{map[string]any{"balance": bal}})
}

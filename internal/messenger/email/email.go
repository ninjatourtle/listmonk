package email

import (
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net/smtp"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"github.com/knadh/listmonk/models"
	"github.com/knadh/smtppool/v2"
)

const (
	MessengerName = "email"

	hdrReturnPath = "Return-Path"
	hdrBcc        = "Bcc"
	hdrCc         = "Cc"
)

var ErrDailyLimitReached = errors.New("smtp daily limit reached")

// Server represents an SMTP server's credentials.
type Server struct {
	// Name is a unique identifier for the server.
	Name          string            `json:"name"`
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	AuthProtocol  string            `json:"auth_protocol"`
	TLSType       string            `json:"tls_type"`
	TLSSkipVerify bool              `json:"tls_skip_verify"`
	EmailHeaders  map[string]string `json:"email_headers"`

	// Optional limits.
	DailyLimit int    `json:"daily_limit"`
	SendPause  string `json:"send_pause"`

	// WarmCloud API settings
	WarmCloudEnabled bool   `json:"warmcloud_enabled"`
	WarmCloudToken   string `json:"warmcloud_token,omitempty"`
	WarmCloudUserID  int    `json:"warmcloud_user_id"`
	WarmCloudRefID   string `json:"warmcloud_ref_id"`
	// WarmCloudBig selects provider type for /get: 'm' (mail.ru) or 'g' (gmail.com)
	WarmCloudBig          string `json:"warmcloud_big"`
	WarmCloudDailyLimit   int    `json:"warmcloud_daily_limit"`
	WarmCloudDailyStep    int    `json:"warmcloud_daily_step"`
	WarmCloudCampaignID   int    `json:"warmcloud_campaign_id"`
	WarmCloudReadPercent  int    `json:"warmcloud_read_percent"`
	WarmCloudMaxPerTick   int    `json:"warmcloud_max_per_tick"`
	WarmCloudSleepBetween string `json:"warmcloud_sleep_between"`
	// Rate limiting: N emails per M minutes (+ optional jitter)
	WarmCloudRateCount         int `json:"warmcloud_rate_count"`
	WarmCloudRateWindowMinutes int `json:"warmcloud_rate_window_minutes"`
	WarmCloudRateJitterPct     int `json:"warmcloud_rate_jitter_pct"`
	// Per-server working hours override
	WarmCloudWorkingHoursEnabled bool   `json:"warmcloud_working_hours_enabled"`
	WarmCloudWorkingHours        string `json:"warmcloud_working_hours"`

	// Warming progress
	WarmCloudSentToday      int `json:"warmcloud_sent_today"`
	WarmCloudCurrentLimit   int `json:"warmcloud_current_limit"`
	WarmCloudSubmittedToday int `json:"warmcloud_submitted_today"`

	SentTotal int64 `json:"-"`
	SentToday int64 `json:"-"`
	lastDay   string

	// Rest of the options are embedded directly from the smtppool lib.
	// The JSON tag is for config unmarshal to work.
	//lint:ignore SA5008 ,squash is needed by koanf/mapstructure config unmarshal.
	smtppool.Opt `json:",squash"`

	pool *smtppool.Pool
}

// Emailer is the SMTP e-mail messenger.
type Emailer struct {
	servers  []*Server
	name     string
	warming  *WarmingManager
	settings *models.Settings

	// onSent is an optional hook invoked after a successful send with the
	// selected server's normalized name. Used by callers to record stats.
	onSent func(serverName string)

	// warmEventHook is invoked when a warming event occurs (send/submit), for persistence.
	warmEventHook func(WarmingEvent)
	// smtpEventHook is invoked on every successful SMTP send (non-warmcloud), for persistence.
	smtpEventHook func(server, to, subject string)
	// warmLimitHook persists per-server current warm limit for the day.
	warmLimitHook func(server string, limit int)

	// schedule hooks backed by the app/DB
	schedule struct {
		insert                       func(server string, dueAt time.Time) error
		insertWithFromEmail          func(server string, dueAt time.Time, fromEmail string) error
		insertWithFromEmailSubject   func(server string, dueAt time.Time, fromEmail string, subject string, persona string) error
		deleteFuture                 func(server string) error
		nextDueByServer              func(server string) (int64, time.Time, bool)
		nextDueByServerWithFromEmail func(server string) (int64, time.Time, string, bool)
		markDone                     func(id int64) error
		markDoneEmail                func(id int64, email string) error
		markError                    func(id int64, msg string) error
		markRead                     func(id int64) error
		markReadEmail                func(id int64, email string) error
		updateFromEmail              func(id int64, fromEmail string) error
		updatePersona                func(id int64, persona string) error
		countPendingToday            func(server string) (int64, error)
		deleteDayPending             func(server string, day time.Time) error
		countPendingWindow           func(server string, from, to time.Time) (int64, error)
		getSentToday                 func(server string, day time.Time) (int64, error)
		markAllTodayDone             func(server string, day time.Time) error
		deleteUnsentOnly             func(server string) error
		deleteAll                    func(server string) error
		listDay                      func(server string, day time.Time) ([]ScheduleSlot, error)
	}
}

// ScheduleSlot represents a single warmcloud schedule entry exposed to the warming manager.
type ScheduleSlot struct {
	ID        int64
	DueAt     time.Time
	Status    string
	FromEmail string
	Persona   string
}

// Servers returns the list of configured servers.
func (e *Emailer) Servers() []*Server {
	return e.servers
}

// New returns an SMTP e-mail Messenger backend with the given SMTP servers.
// Group indicates whether the messenger represents a group of SMTP servers (1 or more)
// that are used as a round-robin pool, or a single server.
func New(name string, servers ...Server) (*Emailer, error) {
	e := &Emailer{
		servers: make([]*Server, 0, len(servers)),
		name:    name,
	}

	for _, srv := range servers {
		s := srv

		var auth smtp.Auth
		switch s.AuthProtocol {
		case "cram":
			auth = smtp.CRAMMD5Auth(s.Username, s.Password)
		case "plain":
			auth = smtp.PlainAuth("", s.Username, s.Password, s.Host)
		case "login":
			auth = &smtppool.LoginAuth{Username: s.Username, Password: s.Password}
		case "", "none":
		default:
			return nil, fmt.Errorf("unknown SMTP auth type '%s'", s.AuthProtocol)
		}
		s.Opt.Auth = auth

		// TLS config.
		s.Opt.SSL = smtppool.SSLNone
		if s.TLSType != "none" {
			s.TLSConfig = &tls.Config{}
			if s.TLSSkipVerify {
				s.TLSConfig.InsecureSkipVerify = s.TLSSkipVerify
			} else {
				s.TLSConfig.ServerName = s.Host
			}

			// SSL/TLS, not STARTTLS.
			switch s.TLSType {
			case "TLS":
				s.Opt.SSL = smtppool.SSLTLS
			case "STARTTLS":
				s.Opt.SSL = smtppool.SSLSTARTTLS
			}
		}

		pool, err := smtppool.New(s.Opt)
		if err != nil {
			return nil, err
		}

		s.pool = pool
		e.servers = append(e.servers, &s)
	}

	// Initialize warming manager only for the group messenger (name == "email").
	// Individual per-SMTP messengers should not start their own managers to avoid duplicates.
	if name == MessengerName {
		e.warming = NewWarmingManager(e)
		e.warming.Start()
	}

	return e, nil
}

// Name returns the messenger's name.
func (e *Emailer) Name() string {
	return e.name
}

// SetOnSentHook sets a callback invoked after a successful send with the
// normalized SMTP server name. If the server has an empty name, the hook
// is not called.
func (e *Emailer) SetOnSentHook(fn func(serverName string)) {
	e.onSent = fn
}

// SetWarmEventHook wires a persistence callback for warming events.
func (e *Emailer) SetWarmEventHook(fn func(WarmingEvent)) {
	e.warmEventHook = fn
}

// SetSMTPEventHook wires persistence for generic SMTP sends.
func (e *Emailer) SetSMTPEventHook(fn func(server, to, subject string)) {
	e.smtpEventHook = fn
}

// SetWarmLimitHook wires a persistence callback for storing today's current warm limit per server.
func (e *Emailer) SetWarmLimitHook(fn func(server string, limit int)) {
	e.warmLimitHook = fn
}

// SetSettings sets the application settings for access to personas and other configuration.
func (e *Emailer) SetSettings(settings *models.Settings) {
	e.settings = settings
}

// SetScheduleHooks wires DB-backed schedule operations into the emailer.
func (e *Emailer) SetScheduleHooks(
	insert func(string, time.Time) error,
	insertWithFromEmail func(string, time.Time, string) error,
	insertWithFromEmailSubject func(string, time.Time, string, string, string) error,
	deleteFuture func(string) error,
	nextDueByServer func(string) (int64, time.Time, bool),
	nextDueByServerWithFromEmail func(string) (int64, time.Time, string, bool),
	markDone func(int64) error,
	markDoneEmail func(int64, string) error,
	markError func(int64, string) error,
	markRead func(int64) error,
	markReadEmail func(int64, string) error,
	updateFromEmail func(int64, string) error,
	updatePersona func(int64, string) error,
	countPendingToday func(string) (int64, error),
	deleteDayPending func(string, time.Time) error,
	countPendingWindow func(string, time.Time, time.Time) (int64, error),
	getSentToday func(string, time.Time) (int64, error),
	markAllTodayDone func(string, time.Time) error,
	deleteUnsentOnly func(string) error,
	deleteAll func(string) error,
	listDay func(string, time.Time) ([]ScheduleSlot, error),
) {
	e.schedule.insert = insert
	e.schedule.insertWithFromEmail = insertWithFromEmail
	e.schedule.insertWithFromEmailSubject = insertWithFromEmailSubject
	e.schedule.deleteFuture = deleteFuture
	e.schedule.nextDueByServer = nextDueByServer
	e.schedule.nextDueByServerWithFromEmail = nextDueByServerWithFromEmail
	e.schedule.markDone = markDone
	e.schedule.markDoneEmail = markDoneEmail
	e.schedule.markError = markError
	e.schedule.markRead = markRead
	e.schedule.markReadEmail = markReadEmail
	e.schedule.updateFromEmail = updateFromEmail
	e.schedule.updatePersona = updatePersona
	e.schedule.countPendingToday = countPendingToday
	e.schedule.deleteDayPending = deleteDayPending
	e.schedule.countPendingWindow = countPendingWindow
	e.schedule.getSentToday = getSentToday
	e.schedule.markAllTodayDone = markAllTodayDone
	e.schedule.deleteUnsentOnly = deleteUnsentOnly
	e.schedule.deleteAll = deleteAll
	e.schedule.listDay = listDay
}

// Stop stops the emailer and its warming manager
func (e *Emailer) Stop() {
	if e.warming != nil {
		e.warming.Stop()
	}
}

// SetCampaignFetcher wires a campaign fetcher into the warming manager.
func (e *Emailer) SetCampaignFetcher(fn func(int) (*models.Campaign, error)) {
	if e.warming != nil {
		e.warming.SetCampaignFetcher(fn)
	}
}

// SetWorkingHours wires working hours spec into the warming manager.
func (e *Emailer) SetWorkingHours(spec string) {
	if e.warming != nil {
		e.warming.SetWorkingHours(spec)
	}
}

// GetWarmingStatus returns the warming status
func (e *Emailer) GetWarmingStatus() map[string]interface{} {
	if e.warming != nil {
		return e.warming.GetWarmingStatus()
	}
	return make(map[string]interface{})
}

// PauseWarming pauses warming for a specific server name.
func (e *Emailer) PauseWarming(serverName string) {
	if e.warming != nil {
		e.warming.Pause(serverName)
	}
}

// ResumeWarming resumes warming for a specific server name.
func (e *Emailer) ResumeWarming(serverName string) {
	if e.warming != nil {
		e.warming.Resume(serverName)
	}
}

// ForceWarmingTick triggers a one-off warming run if possible. Returns true if started.
func (e *Emailer) ForceWarmingTick(serverName string) bool {
	if e.warming != nil {
		return e.warming.ForceTick(serverName)
	}
	return false
}

// ResetWarmingDaily resets daily counters and advances limit for a server.
func (e *Emailer) ResetWarmingDaily(serverName string) {
	if e.warming != nil {
		e.warming.ResetDaily(serverName)
	}
}

// ResetWarmingDailyAll resets daily counters for all servers.
func (e *Emailer) ResetWarmingDailyAll() {
	if e.warming != nil {
		e.warming.ResetAllDaily()
	}
}

// WarmingRebuildSchedule deletes future pending slots and plans next window for the given server.
func (e *Emailer) WarmingRebuildSchedule(serverName string) {
	if e.warming == nil {
		return
	}
	if e.schedule.deleteFuture != nil {
		_ = e.schedule.deleteFuture(serverName)
	}
	if err := e.warming.RebuildSchedule(serverName, time.Now(), true); err != nil {
		elogf("WarmCloud: ошибка пересоздания расписания для %s: %v", serverName, err)
	}
}

// WarmingRebuildScheduleDay rebuilds schedule for the specified day (date part of t)
func (e *Emailer) WarmingRebuildScheduleDay(serverName string, t time.Time) {
	if e.warming == nil {
		return
	}
	if err := e.warming.RebuildSchedule(serverName, t, true); err != nil {
		elogf("WarmCloud: ошибка пересоздания расписания на день для %s: %v", serverName, err)
	}
}

// WarmingFinishToday marks the rest of today's schedule entries as completed in memory.
func (e *Emailer) WarmingFinishToday(serverName string) {
	if e.warming != nil {
		e.warming.MarkFinishedToday(serverName)
	}
}

// Push pushes a message to the server.
func (e *Emailer) Push(m models.Message) error {
	// If there are more than one SMTP servers, send to a random
	// one from the list.
	var (
		ln  = len(e.servers)
		srv *Server
	)
	if ln > 1 {
		srv = e.servers[rand.Intn(ln)]
	} else {
		srv = e.servers[0]
	}

	// Reset daily counter if needed.
	today := time.Now().Format("2006-01-02")
	if srv.lastDay != today {
		atomic.StoreInt64(&srv.SentToday, 0)
		srv.lastDay = today
	}

	if srv.DailyLimit > 0 && atomic.LoadInt64(&srv.SentToday) >= int64(srv.DailyLimit) {
		return ErrDailyLimitReached
	}

	if srv.SendPause != "" {
		if d, err := time.ParseDuration(srv.SendPause); err == nil {
			time.Sleep(d)
		}
	}

	// Are there attachments?
	var files []smtppool.Attachment
	if m.Attachments != nil {
		files = make([]smtppool.Attachment, 0, len(m.Attachments))
		for _, f := range m.Attachments {
			a := smtppool.Attachment{
				Filename: f.Name,
				Header:   f.Header,
				Content:  make([]byte, len(f.Content)),
			}
			copy(a.Content, f.Content)
			files = append(files, a)
		}
	}

	// Create the email.
	em := smtppool.Email{
		From:        m.From,
		To:          m.To,
		Subject:     m.Subject,
		Attachments: files,
	}

	em.Headers = textproto.MIMEHeader{}

	// Attach SMTP level headers.
	for k, v := range srv.EmailHeaders {
		em.Headers.Set(k, v)
	}

	// Attach e-mail level headers.
	for k, v := range m.Headers {
		em.Headers.Set(k, v[0])
	}

	// Ensure Message-Id uses the configured Hello hostname (if set).
	// Many MTAs default to the system/Docker hostname when generating Message-Id.
	// If the SMTP "hello_hostname" is configured, prefer that for the domain part.
	if em.Headers.Get(models.EmailHeaderMessageId) == "" && srv.Opt.HelloHostname != "" {
		// Derive a safe domain from hello hostname (strip ports and whitespace).
		domain := strings.TrimSpace(strings.ToLower(srv.Opt.HelloHostname))
		if i := strings.Index(domain, ":"); i > -1 {
			domain = domain[:i]
		}
		// Generate a reasonably unique ID. Format: <ts.rand@domain>
		mid := fmt.Sprintf("<%d.%d@%s>", time.Now().UnixNano(), rand.Int63(), domain)
		em.Headers.Set(models.EmailHeaderMessageId, mid)
	}

	// If the `Return-Path` header is set, it should be set as the
	// the SMTP envelope sender (via the Sender field of the email struct).
	if sender := em.Headers.Get(hdrReturnPath); sender != "" {
		em.Sender = sender
		em.Headers.Del(hdrReturnPath)
	}

	// If the `Bcc` header is set, it should be set on the Envelope
	if bcc := em.Headers.Get(hdrBcc); bcc != "" {
		for _, part := range strings.Split(bcc, ",") {
			em.Bcc = append(em.Bcc, strings.TrimSpace(part))
		}
		em.Headers.Del(hdrBcc)
	}

	// If the `Cc` header is set, it should be set on the Envelope
	if cc := em.Headers.Get(hdrCc); cc != "" {
		for _, part := range strings.Split(cc, ",") {
			em.Cc = append(em.Cc, strings.TrimSpace(part))
		}
		em.Headers.Del(hdrCc)
	}

	switch m.ContentType {
	case "plain":
		em.Text = []byte(m.Body)
	default:
		em.HTML = m.Body
		if len(m.AltBody) > 0 {
			em.Text = m.AltBody
		}
	}

	if err := srv.pool.Send(em); err != nil {
		return err
	}
	atomic.AddInt64(&srv.SentTotal, 1)
	atomic.AddInt64(&srv.SentToday, 1)

	// Skip journaling and counters for system notifications (eg: admin notifs)
	// marked via header so they don't pollute user-visible SMTP stats.
	isSystem := false
	if m.Headers != nil {
		hv := strings.TrimSpace(m.Headers.Get("X-Listmonk-System"))
		if hv != "" && (hv == "1" || strings.EqualFold(hv, "true")) {
			isSystem = true
		}
	}
	if e.onSent != nil && srv.Name != "" && !isSystem {
		// Fire and forget; the caller can do any persistence/logging.
		e.onSent(srv.Name)
	}
	if e.smtpEventHook != nil && srv.Name != "" && !isSystem {
		for _, to := range m.To {
			e.smtpEventHook(srv.Name, to, m.Subject)
		}
	}
	return nil
}

// RestoreSentToday sets the in-memory counter for the given server to the provided value
// and marks the current day so the counter is not reset on the next send.
func (e *Emailer) RestoreSentToday(server string, sent int64) {
	nowDay := time.Now().Format("2006-01-02")
	for _, srv := range e.servers {
		if srv.Name != server {
			continue
		}
		atomic.StoreInt64(&srv.SentToday, sent)
		srv.lastDay = nowDay
		return
	}
}

// Flush flushes the message queue to the server.
func (e *Emailer) Flush() error {
	return nil
}

// Close closes the SMTP pools.
func (e *Emailer) Close() error {
	for _, s := range e.servers {
		s.pool.Close()
	}
	return nil
}

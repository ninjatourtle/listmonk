package manager

import (
	"errors"
	"fmt"
	"html/template"
	"log"
	"math/rand"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"maps"

	"github.com/Masterminds/sprig/v3"
	"github.com/knadh/listmonk/internal/i18n"
	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/models"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	// BaseTPL is the name of the base template.
	BaseTPL = "base"

	// ContentTpl is the name of the compiled message.
	ContentTpl = "content"

	dummyUUID = "00000000-0000-0000-0000-000000000000"
)

// Store represents a data backend, such as a database,
// that provides subscriber and campaign records.
type Store interface {
	NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error)
	NextSubscribers(campID, limit int) ([]models.Subscriber, error)
	NextSequenceSubscribers(campID, limit int) ([]models.Subscriber, error)
	GetCampaign(campID int) (*models.Campaign, error)
	GetAttachment(mediaID int) (models.Attachment, error)
	UpdateCampaignStatus(campID int, status string) error
	UpdateCampaignCounts(campID int, toSend int, sent int, lastSubID int) error
	GetSequenceProgress(campID int, subscriberID int) (int, time.Time, string, string, bool, error)
	AdvanceSequenceProgress(campID int, subscriberID int, step int, nextAt *time.Time, done bool) error
	SetSequenceFromEmail(campID int, subscriberID int, fromEmail string) error
	GetEarliestSequenceNext(campID int) (*time.Time, error)
	UpdateCampaignSendAt(campID int, t time.Time, status string) error
	RecordSend(campID int, subscriberID int, step int, subject string, fromEmail string, success bool, errMsg string, replyToken string, messageID string) error
	RecordImmediateBounce(sub models.Subscriber, camp *models.Campaign, smtpErr error) error
	CreateLink(url string) (string, error)
	BlocklistSubscriber(id int64) error
	DeleteSubscriber(id int64) error

	// Settings access for persona-based sending
	GetSettings() (*models.Settings, error)

	// Campaign schedule windows
	CampSchedInsert(campID int, due time.Time, quota int) error
	CampSchedInsertWithRecipient(campID int, due time.Time, quota int, subscriberID int, emailTo, emailFrom, subject string) error
	CampSchedDeleteFuture(campID int) error
	CampSchedDeleteOutdated(campID int) error
	CampSchedNextDue(campID int) (int64, time.Time, int, int, int, string, string, string, bool, error) // id, due, quota, used, subscriber_id, email_to, email_from, subject
	CampSchedConsume(id int64, n int) (int, int, string, error)                                         // quota, used, status
	CampSchedMarkDoneEmail(id int64, to, from, subject string) error
	AssignSlotSubscriber(id int64, subscriberID int, email string) error
	GetCampaignSubscribers(campID, lastID, maxID int, campType string, limit int) ([]models.Subscriber, error)
	GetSubscriberByID(id int) (models.Subscriber, error)
	UpdateCampaignMaxSubscriberID(campID, maxSubID int) error
}

// Messenger is an interface for a generic messaging backend,
// for instance, e-mail, SMS etc.
type Messenger interface {
	Name() string
	Push(models.Message) error
	Flush() error
	Close() error
}

// CampStats contains campaign stats like per minute send rate.
type CampStats struct {
	SendRate int
}

// Manager handles the scheduling, processing, and queuing of campaigns
// and message pushes.
type Manager struct {
	cfg        Config
	store      Store
	i18n       *i18n.I18n
	messengers map[string]Messenger
	fnNotify   func(subject string, data any) error
	log        *log.Logger

	randMu sync.Mutex
	rng    *rand.Rand

	// Campaigns that are currently running.
	pipes    map[int]*pipe
	pipesMut sync.RWMutex

	// Campaign scheduling locks to prevent concurrent slot seeding
	schedLocks    map[int]*sync.Mutex
	schedLocksMut sync.RWMutex

	tpls    map[int]*models.Template
	tplsMut sync.RWMutex

	// Links generated using Track() are cached here so as to not query
	// the database for the link UUID for every message sent. This has to
	// be locked as it may be used externally when previewing campaigns.
	links    map[string]string
	linksMut sync.RWMutex

	nextPipes chan *pipe
	campMsgQ  chan CampaignMessage
	msgQ      chan models.Message

	// Sliding window keeps track of the total number of messages sent in a period
	// and on reaching the specified limit, waits until the window is over before
	// sending further messages.
	slidingCount int
	slidingStart time.Time

	tplFuncs template.FuncMap
}

// CampaignMessage represents an instance of campaign message to be pushed out,
// specific to a subscriber, via the campaign's messenger.
type CampaignMessage struct {
	Campaign   *models.Campaign
	Subscriber models.Subscriber

	from     string
	to       string
	subject  string
	body     []byte
	altBody  []byte
	unsubURL string

	pipe *pipe

	// For sequence campaigns
	StepIndex  int
	NextStepAt *time.Time

	// Optional: schedule slot id associated with this message
	SlotID int64
}

// Config has parameters for configuring the manager.
type Config struct {
	// Number of subscribers to pull from the DB in a single iteration.
	BatchSize             int
	Concurrency           int
	MessageRate           int
	MaxSendErrors         int
	SlidingWindow         bool
	SlidingWindowDuration time.Duration
	SlidingWindowRate     int
	RequeueOnError        bool
	FromEmail             string
	IndividualTracking    bool
	LinkTrackURL          string
	UnsubURL              string
	OptinURL              string
	MessageURL            string
	ViewTrackURL          string
	ArchiveURL            string
	RootURL               string
	UnsubHeader           bool

	// Interval to scan the DB for active campaign checkpoints.
	ScanInterval time.Duration

	// ScanCampaigns indicates whether this instance of manager will scan the DB
	// for active campaigns and process them.
	// This can be used to run multiple instances of listmonk
	// (exposed to the internet, private etc.) where only one does campaign
	// processing while the others handle other kinds of traffic.
	ScanCampaigns bool
}

var pushTimeout = time.Second * 3

// New returns a new instance of Mailer.
func New(cfg Config, store Store, i *i18n.I18n, l *log.Logger) *Manager {
	if cfg.BatchSize < 1 {
		cfg.BatchSize = 1000
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 1
	}
	if cfg.MessageRate < 1 {
		cfg.MessageRate = 1
	}

	m := &Manager{
		cfg:   cfg,
		store: store,
		i18n:  i,
		fnNotify: func(subject string, data any) error {
			return notifs.NotifySystem(subject, notifs.TplCampaignStatus, data, nil)
		},
		log:          l,
		messengers:   make(map[string]Messenger),
		pipes:        make(map[int]*pipe),
		schedLocks:   make(map[int]*sync.Mutex),
		tpls:         make(map[int]*models.Template),
		links:        make(map[string]string),
		nextPipes:    make(chan *pipe, 1000),
		campMsgQ:     make(chan CampaignMessage, cfg.Concurrency*cfg.MessageRate*2),
		msgQ:         make(chan models.Message, cfg.Concurrency*cfg.MessageRate*2),
		slidingStart: time.Now(),
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	m.tplFuncs = m.makeGnericFuncMap()

	return m
}

// getCampaignSchedLock returns a mutex for the given campaign ID to prevent concurrent slot seeding
func (m *Manager) getCampaignSchedLock(campID int) *sync.Mutex {
	m.schedLocksMut.Lock()
	defer m.schedLocksMut.Unlock()

	if lock, exists := m.schedLocks[campID]; exists {
		return lock
	}

	lock := &sync.Mutex{}
	m.schedLocks[campID] = lock
	return lock
}

// computeNextSequenceTime determines the absolute time for the subscriber's next step
// based on the current message's step and the campaign's sequence. Returns (nil, true)
// if the sequence is complete for the subscriber.
func computeNextSequenceTime(msg CampaignMessage) (*time.Time, bool) {
	if msg.Campaign == nil || msg.Campaign.Type != models.CampaignTypeSequence {
		return nil, true
	}
	// Parse sequence JSON.
	seq, err := models.ParseSequence(msg.Campaign.Sequence)
	if err != nil {
		return nil, true
	}
	nextIdx := msg.StepIndex + 1
	if nextIdx >= len(seq.Steps) {
		return nil, true
	}
	st := seq.Steps[nextIdx]
	// Compute delay in minutes
	mult := 1
	switch strings.ToLower(st.DelayUnit) {
	case "hours":
		mult = 60
	case "days":
		mult = 60 * 24
	}
	delayMins := st.DelayValue * mult
	t := time.Now().Add(time.Duration(delayMins) * time.Minute)
	return &t, false
}

// AddMessenger adds a Messenger messaging backend to the manager.
func (m *Manager) AddMessenger(msg Messenger) error {
	id := msg.Name()
	if _, ok := m.messengers[id]; ok {
		return fmt.Errorf("messenger '%s' is already loaded", id)
	}
	m.messengers[id] = msg

	return nil
}

// PushMessage pushes an arbitrary non-campaign Message to be sent out by the workers.
// It times out if the queue is busy.
func (m *Manager) PushMessage(msg models.Message) error {
	t := time.NewTicker(pushTimeout)
	defer t.Stop()

	select {
	case m.msgQ <- msg:
	case <-t.C:
		m.log.Printf("message push timed out: '%s'", msg.Subject)
		return errors.New("message push timed out")
	}

	return nil
}

// PushCampaignMessage pushes a campaign messages into a queue to be sent out by the workers.
// It times out if the queue is busy.
func (m *Manager) PushCampaignMessage(msg CampaignMessage) error {
	t := time.NewTicker(pushTimeout)
	defer t.Stop()

	// Load any media/attachments.
	if err := m.attachMedia(msg.Campaign); err != nil {
		return err
	}

	select {
	case m.campMsgQ <- msg:
	case <-t.C:
		m.log.Printf("message push timed out: '%s'", msg.Subject())
		return errors.New("message push timed out")
	}

	return nil
}

// HasMessenger checks if a given messenger is registered.
func (m *Manager) HasMessenger(id string) bool {
	_, ok := m.messengers[id]

	return ok
}

// HasRunningCampaigns checks if there are any active campaigns.
func (m *Manager) HasRunningCampaigns() bool {
	m.pipesMut.Lock()
	defer m.pipesMut.Unlock()

	return len(m.pipes) > 0
}

// GetCampaignStats returns campaign statistics.
func (m *Manager) GetCampaignStats(id int) CampStats {
	n := 0

	m.pipesMut.Lock()
	if c, ok := m.pipes[id]; ok {
		n = int(c.rate.Rate())
	}
	m.pipesMut.Unlock()

	return CampStats{SendRate: n}
}

// Run is a blocking function (that should be invoked as a goroutine)
// that scans the data source at regular intervals for pending campaigns,
// and queues them for processing. The process queue fetches batches of
// subscribers and pushes messages to them for each queued campaign
// until all subscribers are exhausted, at which point, a campaign is marked
// as "finished".
func (m *Manager) Run() {
	if m.cfg.ScanCampaigns {
		// Periodically scan campaigns and push running campaigns to nextPipes
		// to fetch subscribers from the campaign.
		go m.scanCampaigns(m.cfg.ScanInterval)
	}

	// Spawn N message workers.
	for i := 0; i < m.cfg.Concurrency; i++ {
		go m.worker()
	}

	// Indefinitely wait on the pipe queue to fetch the next set of subscribers
	// for any active campaigns.
	for p := range m.nextPipes {
		has, err := p.NextSubscribers()
		if err != nil {
			m.log.Printf("error processing campaign batch (%s): %v", p.camp.Name, err)
			continue
		}

		if has {
			// There are more subscribers to fetch. Queue again.
			select {
			case m.nextPipes <- p:
			default:
			}
		} else {
			// The pipe is created with a +1 on the waitgroup pseudo counter
			// so that it immediately waits. Subsequently, every message created
			// is incremented in the counter in pipe.newMessage(), and when it's'
			// processed (or ignored when a campaign is paused or cancelled),
			// the count is's reduced in worker().
			//
			// This marks down the original non-message +1, causing the waitgroup
			// to be released and the pipe to end, triggering the pg.Wait()
			// in newPipe() that calls pipe.cleanup().
			p.wg.Done()
		}
	}
}

// CacheTpl caches a template for ad-hoc use. This is currently only used by tx templates.
func (m *Manager) CacheTpl(id int, tpl *models.Template) {
	m.tplsMut.Lock()
	m.tpls[id] = tpl
	m.tplsMut.Unlock()
}

// DeleteTpl deletes a cached template.
func (m *Manager) DeleteTpl(id int) {
	m.tplsMut.Lock()
	delete(m.tpls, id)
	m.tplsMut.Unlock()
}

func (m *Manager) randInt63() int64 {
	m.randMu.Lock()
	defer m.randMu.Unlock()
	return m.rng.Int63()
}

// GetTpl returns a cached template.
func (m *Manager) GetTpl(id int) (*models.Template, error) {
	m.tplsMut.RLock()
	tpl, ok := m.tpls[id]
	m.tplsMut.RUnlock()

	if !ok {
		return nil, fmt.Errorf("template %d not found", id)
	}

	return tpl, nil
}

// TemplateFuncs returns the template functions to be applied into
// compiled campaign templates.
func (m *Manager) TemplateFuncs(c *models.Campaign) template.FuncMap {
	f := template.FuncMap{
		"TrackLink": func(url string, msg *CampaignMessage) string {
			subUUID := msg.Subscriber.UUID
			if !m.cfg.IndividualTracking {
				subUUID = dummyUUID
			}

			return m.trackLink(url, msg.Campaign.UUID, subUUID)
		},
		"TrackView": func(msg *CampaignMessage) template.HTML {
			subUUID := msg.Subscriber.UUID
			if !m.cfg.IndividualTracking {
				subUUID = dummyUUID
			}

			return template.HTML(fmt.Sprintf(`<img src="%s" alt="" />`,
				fmt.Sprintf(m.cfg.ViewTrackURL, msg.Campaign.UUID, subUUID)))
		},
		"UnsubscribeURL": func(msg *CampaignMessage) string {
			return msg.unsubURL
		},
		"ManageURL": func(msg *CampaignMessage) string {
			return msg.unsubURL + "?manage=true"
		},
		"OptinURL": func(msg *CampaignMessage) string {
			// Add list IDs.
			// TODO: Show private lists list on optin e-mail
			return fmt.Sprintf(m.cfg.OptinURL, msg.Subscriber.UUID, "")
		},
		"MessageURL": func(msg *CampaignMessage) string {
			return fmt.Sprintf(m.cfg.MessageURL, c.UUID, msg.Subscriber.UUID)
		},
		"ArchiveURL": func() string {
			return m.cfg.ArchiveURL
		},
		"RootURL": func() string {
			return m.cfg.RootURL
		},
	}

	maps.Copy(f, m.tplFuncs)

	return f
}

func (m *Manager) GenericTemplateFuncs() template.FuncMap {
	return m.tplFuncs
}

// StopCampaign marks a running campaign as stopped so that all its queued messages are ignored.
func (m *Manager) StopCampaign(id int) {
	m.pipesMut.RLock()
	if p, ok := m.pipes[id]; ok {
		p.Stop(false)
	}
	m.pipesMut.RUnlock()
}

// Close closes and exits the campaign manager.
func (m *Manager) Close() {
	close(m.nextPipes)
	close(m.msgQ)
}

// scanCampaigns is a blocking function that periodically scans the data source
// for campaigns to process and dispatches them to the manager. It feeds campaigns
// into nextPipes.
func (m *Manager) scanCampaigns(tick time.Duration) {
	t := time.NewTicker(tick)
	defer t.Stop()

	// Periodically scan the data source for campaigns to process.
	for range t.C {
		ids, counts := m.getCurrentCampaigns()
		campaigns, err := m.store.NextCampaigns(ids, counts)
		if err != nil {
			m.log.Printf("error fetching campaigns: %v", err)
			continue
		}

		for _, c := range campaigns {
			// Create a new pipe that'll handle this campaign's states.
			p, err := m.newPipe(c)
			if err != nil {
				m.log.Printf("error processing campaign (%s): %v", c.Name, err)
				continue
			}
			m.log.Printf("start processing campaign (%s)", c.Name)

			// If subscriber processing is busy, move on. Blocking and waiting
			// can end up in a race condition where the waiting campaign's
			// state in the data source has changed.
			select {
			case m.nextPipes <- p:
			default:
			}
		}
	}
}

// worker is a blocking function that perpetually listents to events (message) on different
// queues and processes them.
func (m *Manager) worker() {
	// Counter to keep track of the message / sec rate limit.
	numMsg := 0
	for {
		select {
		// Campaign message.
		case msg, ok := <-m.campMsgQ:
			if !ok {
				return
			}

			// If the campaign has ended or stopped, ignore the message.
			if msg.pipe != nil && msg.pipe.stopped.Load() {
				// Reduce the message counter on the pipe.
				msg.pipe.wg.Done()
				continue
			}

			if msg.pipe != nil {
				defer msg.pipe.wg.Done()
			}

			// Pause on hitting the message rate.
			if numMsg >= m.cfg.MessageRate {
				time.Sleep(time.Second)
				numMsg = 0
			}
			numMsg++

			// Outgoing message.
			out := models.Message{
				From:        msg.from,
				To:          []string{msg.to},
				Subject:     msg.subject,
				ContentType: msg.Campaign.ContentType,
				Body:        msg.body,
				AltBody:     msg.altBody,
				Subscriber:  msg.Subscriber,
				Campaign:    msg.Campaign,
				Attachments: msg.Campaign.Attachments,
			}

			h := textproto.MIMEHeader{}
			h.Set(models.EmailHeaderCampaignUUID, msg.Campaign.UUID)
			h.Set(models.EmailHeaderSubscriberUUID, msg.Subscriber.UUID)

			// Attach List-Unsubscribe headers?
			if m.cfg.UnsubHeader {
				h.Set("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
				h.Set("List-Unsubscribe", `<`+msg.unsubURL+`>`)
			}

			// Attach any custom headers.
			if len(msg.Campaign.Headers) > 0 {
				for _, set := range msg.Campaign.Headers {
					for hdr, val := range set {
						h.Add(hdr, val)
					}
				}
			}

			// Set the headers.
			out.Headers = h

			// Ensure we have a stable Message-Id so replies can be matched via In-Reply-To/References.
			// If not present, generate one using the domain from the From address.
			messageID := strings.TrimSpace(out.Headers.Get(models.EmailHeaderMessageId))
			if messageID == "" {
				// Derive domain from From address
				_, fromEmail := parseFromNameEmail(msg.from)
				domain := ""
				if i := strings.LastIndex(fromEmail, "@"); i > -1 && i+1 < len(fromEmail) {
					domain = strings.ToLower(strings.TrimSpace(fromEmail[i+1:]))
				}
				if domain == "" {
					domain = "localhost"
				}
				messageID = fmt.Sprintf("<%d.%d@%s>", time.Now().UnixNano(), m.randInt63(), domain)
				out.Headers.Set(models.EmailHeaderMessageId, messageID)
			}

			// For sequence campaigns with personas, persist stable from_email per subscriber on first set.
			if msg.Campaign.Type == models.CampaignTypeSequence && msg.from != "" {
				// Extract email part
				_, email := parseFromNameEmail(msg.from)
				if email != "" {
					_ = m.store.SetSequenceFromEmail(msg.Campaign.ID, msg.Subscriber.ID, email)
				}
			}

			// Push the message to the messenger.
			err := m.messengers[msg.Campaign.Messenger].Push(out)
			if err != nil && errors.Is(err, email.ErrDailyLimitReached) {
				if msg.pipe != nil {
					msg.pipe.pauseForDailyLimit()
					msg.pipe.wg.Done()
				}
				continue
			}
			if err != nil {
				m.log.Printf("error sending message in campaign %s: subscriber %d: %v", msg.Campaign.Name, msg.Subscriber.ID, err)
				// Record failed send
				_ = m.store.RecordSend(msg.Campaign.ID, msg.Subscriber.ID, -1, msg.Subject(), msg.from, false, err.Error(), "", "")
				// Also record a bounce using bounce settings.
				_ = m.store.RecordImmediateBounce(msg.Subscriber, msg.Campaign, err)
			}

			// Increment the send rate or the error counter if there was an error.
			if msg.pipe != nil {
				if err != nil {
					// Call the error callback, which keeps track of the error count
					// and stops the campaign if the error count exceeds the threshold.
					msg.pipe.OnError()
				} else {
					id := uint64(msg.Subscriber.ID)
					if id > msg.pipe.lastID.Load() {
						msg.pipe.lastID.Store(uint64(msg.Subscriber.ID))
						msg.pipe.wg.Done()
					}
					msg.pipe.rate.Incr(1)
					msg.pipe.sent.Add(1)
					// For sequence campaigns, advance per-subscriber progress.
					if msg.Campaign.Type == models.CampaignTypeSequence {
						// Compute next step time.
						nextAt, done := computeNextSequenceTime(msg)
						if e := m.store.AdvanceSequenceProgress(msg.Campaign.ID, msg.Subscriber.ID, msg.StepIndex+1, nextAt, done); e != nil {
							m.log.Printf("error advancing sequence progress: %v", e)
						}
					}

					// Record send event for recent sends page.
					stepNum := -1
					if msg.Campaign.Type == models.CampaignTypeSequence {
						stepNum = msg.StepIndex + 1
					}
					_ = m.store.RecordSend(msg.Campaign.ID, msg.Subscriber.ID, stepNum, msg.Subject(), msg.from, true, "", "", messageID)
					if msg.SlotID > 0 {
						_ = m.store.CampSchedMarkDoneEmail(msg.SlotID, msg.to, msg.from, msg.Subject())
					}
				}
			}

		// Arbitrary message.
		case msg, ok := <-m.msgQ:
			if !ok {
				return
			}

			// Push the message to the messenger.
			if err := m.messengers[msg.Messenger].Push(msg); err != nil {
				m.log.Printf("error sending message '%s': %v", msg.Subject, err)
			}
		}
	}
}

// getCurrentCampaigns returns the IDs of campaigns currently being processed
// and their sent counts.
func (m *Manager) getCurrentCampaigns() ([]int64, []int64) {
	// Needs to return an empty slice in case there are no campaigns.
	m.pipesMut.RLock()
	defer m.pipesMut.RUnlock()

	var (
		ids    = make([]int64, 0, len(m.pipes))
		counts = make([]int64, 0, len(m.pipes))
	)
	for _, p := range m.pipes {
		ids = append(ids, int64(p.camp.ID))

		// Get the sent counts for campaigns and reset them to 0
		// as in the database, they're stored cumulatively (sent += $newSent).
		counts = append(counts, p.sent.Swap(0))
	}

	return ids, counts
}

// trackLink register a URL and return its UUID to be used in message templates
// for tracking links.
func (m *Manager) trackLink(url, campUUID, subUUID string) string {
	url = strings.ReplaceAll(url, "&amp;", "&")

	m.linksMut.RLock()
	if uu, ok := m.links[url]; ok {
		m.linksMut.RUnlock()
		return fmt.Sprintf(m.cfg.LinkTrackURL, uu, campUUID, subUUID)
	}
	m.linksMut.RUnlock()

	// Register link.
	uu, err := m.store.CreateLink(url)
	if err != nil {
		m.log.Printf("error registering tracking for link '%s': %v", url, err)

		// If the registration fails, fail over to the original URL.
		return url
	}

	m.linksMut.Lock()
	m.links[url] = uu
	m.linksMut.Unlock()

	return fmt.Sprintf(m.cfg.LinkTrackURL, uu, campUUID, subUUID)
}

// sendNotif sends a notification to registered admin e-mails.
func (m *Manager) sendNotif(c *models.Campaign, status, reason string) error {
	var (
		subject = fmt.Sprintf("%s: %s", cases.Title(language.Und).String(status), c.Name)
		data    = map[string]any{
			"ID":     c.ID,
			"Name":   c.Name,
			"Status": status,
			"Sent":   c.Sent,
			"ToSend": c.ToSend,
			"Reason": reason,
		}
	)

	return m.fnNotify(subject, data)
}

// makeGnericFuncMap returns a generic template func map with custom template
// functions and sprig template functions.
func (m *Manager) makeGnericFuncMap() template.FuncMap {
	funcs := template.FuncMap{
		"Date": func(layout string) string {
			if layout == "" {
				layout = time.ANSIC
			}
			return time.Now().Format(layout)
		},
		"L": func() *i18n.I18n {
			return m.i18n
		},
		"Safe": func(safeHTML string) template.HTML {
			return template.HTML(safeHTML)
		},
	}

	// Copy spring functions.
	sprigFuncs := sprig.GenericFuncMap()
	delete(sprigFuncs, "env")
	delete(sprigFuncs, "expandenv")

	maps.Copy(funcs, sprigFuncs)

	return funcs
}

// attachMedia loads any media/attachments from the media store and attaches
// the byte blobs to the campaign.
func (m *Manager) attachMedia(c *models.Campaign) error {
	// Load any media/attachments.
	for _, mid := range []int64(c.MediaIDs) {
		a, err := m.store.GetAttachment(int(mid))
		if err != nil {
			return fmt.Errorf("error fetching attachment %d on campaign %s: %v", mid, c.Name, err)
		}

		c.Attachments = append(c.Attachments, a)
	}

	return nil
}

// MakeAttachmentHeader is a helper function that returns a
// textproto.MIMEHeader tailored for attachments, primarily
// email. If no encoding is given, base64 is assumed.
func MakeAttachmentHeader(filename, encoding, contentType string) textproto.MIMEHeader {
	if encoding == "" {
		encoding = "base64"
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", "attachment; filename="+filename)
	h.Set("Content-Type", fmt.Sprintf("%s; name=\""+filename+"\"", contentType))
	h.Set("Content-Transfer-Encoding", encoding)
	return h
}

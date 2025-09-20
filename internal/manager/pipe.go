package manager

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/knadh/listmonk/models"
	"github.com/paulbellamy/ratecounter"
	null "gopkg.in/volatiletech/null.v6"
)

type pipe struct {
	camp       *models.Campaign
	rate       *ratecounter.RateCounter
	wg         *sync.WaitGroup
	sent       atomic.Int64
	lastID     atomic.Uint64
	errors     atomic.Uint64
	stopped    atomic.Bool
	withErrors atomic.Bool

	m *Manager

	// persona-based rotation
	personaAccounts []string
	personaIdx      atomic.Uint32
	personaName     string

	// Per-campaign sliding window state
	slidingCount int
	slidingStart time.Time

	// Campaign schedule window state
	sched struct {
		id           int64
		due          time.Time
		quota        int
		used         int
		ready        bool
		subscriberID int
		emailFrom    string
	}

	// Current slot ID for message creation
	currentSlotID int64
}

// newPipe adds a campaign to the process queue.
func (m *Manager) newPipe(c *models.Campaign) (*pipe, error) {
	// Validate messenger.
	if _, ok := m.messengers[c.Messenger]; !ok {
		m.store.UpdateCampaignStatus(c.ID, models.CampaignStatusCancelled)
		return nil, fmt.Errorf("unknown messenger %s on campaign %s", c.Messenger, c.Name)
	}

	// Load the template.
	if err := c.CompileTemplate(m.TemplateFuncs(c)); err != nil {
		return nil, err
	}

	// Load any media/attachments.
	if err := m.attachMedia(c); err != nil {
		return nil, err
	}

	// Add the campaign to the active map.
	p := &pipe{
		camp: c,
		rate: ratecounter.NewRateCounter(time.Minute),
		wg:   &sync.WaitGroup{},
		m:    m,
	}

	// Initialize persona rotation if persona_domain is set on the campaign.
	if c.PersonaDomain.Valid && strings.TrimSpace(c.PersonaDomain.String) != "" {
		if set, err := m.store.GetSettings(); err == nil {
			dom := strings.ToLower(strings.TrimSpace(c.PersonaDomain.String))
			for _, per := range set.Personas {
				if strings.EqualFold(per.Domain, dom) {
					// Build account emails local@domain
					for _, acc := range per.Accounts {
						name := strings.TrimSpace(acc.Name)
						if name == "" {
							continue
						}
						p.personaAccounts = append(p.personaAccounts, name+"@"+per.Domain)
					}
					break
				}
			}
		}
		// Prefer explicit FromName; fallback to name parsed from FromEmail.
		if c.FromName.Valid && strings.TrimSpace(c.FromName.String) != "" {
			p.personaName = strings.TrimSpace(c.FromName.String)
		} else {
			p.personaName, _ = parseFromNameEmail(c.FromEmail)
		}
	}

	// Increment the waitgroup so that Wait() blocks immediately. This is necessary
	// as a campaign pipe is created first and subscribers/messages under it are
	// fetched asynchronolusly later. The messages each add to the wg and that
	// count is used to determine the exhaustion/completion of all messages.
	p.wg.Add(1)

	go func() {
		// Wait for all the messages in the campaign to be processed
		// (successfully or skipped after errors or cancellation).
		p.wg.Wait()

		p.cleanup()
	}()

	// If per-campaign scheduling is configured, pre-build schedule as individual email slots per window.
	if c.RateWindowCount > 0 && c.RateWindowMinutes > 0 {
		// Use campaign-specific lock to prevent concurrent slot seeding
		lock := m.getCampaignSchedLock(c.ID)
		lock.Lock()
		defer lock.Unlock()

		// Clear future pending windows and outdated slots to avoid duplicates and stale data
		_ = m.store.CampSchedDeleteFuture(c.ID)
		_ = m.store.CampSchedDeleteOutdated(c.ID)

		// Plan all remaining slots with pre-assigned recipients
		if err := p.planAllRemainingSlots(); err != nil {
			m.log.Printf("error planning campaign slots (%s): %v", c.Name, err)
		}
	}

	m.pipesMut.Lock()
	m.pipes[c.ID] = p
	m.pipesMut.Unlock()
	return p, nil
}

// planAllRemainingSlots plans all remaining slots for the campaign with pre-assigned recipients
func (p *pipe) planAllRemainingSlots() error {
	c := p.camp

	// Calculate remaining recipients
	remaining := c.ToSend - c.Sent
	if remaining <= 0 {
		return nil
	}

	// Get start time - if campaign was paused for a long time, start from now
	start := time.Now()
	if c.SendAt.Valid && c.SendAt.Time.After(start) {
		start = c.SendAt.Time
	}

	// If campaign was paused for more than 1 hour, start from now to avoid stale slots
	// This ensures fresh subscriber lists and persona settings
	if c.StartedAt.Valid {
		pauseDuration := start.Sub(c.StartedAt.Time)
		if pauseDuration > time.Hour {
			start = time.Now()
		}
	}

	// Get all remaining subscribers for this campaign
	subscribers, err := p.m.store.GetCampaignSubscribers(c.ID, c.LastSubscriberID, c.MaxSubscriberID, c.Type, remaining)
	if err != nil {
		return fmt.Errorf("error fetching campaign subscribers: %v", err)
	}

	if len(subscribers) == 0 {
		return nil
	}

	// Update campaign counters to reflect current subscriber state
	// This ensures we have fresh subscriber IDs after a long pause
	if len(subscribers) > 0 {
		maxSubID := subscribers[len(subscribers)-1].ID
		if maxSubID > c.MaxSubscriberID {
			// Update max_subscriber_id in the database
			_ = p.m.store.UpdateCampaignMaxSubscriberID(c.ID, maxSubID)
		}
	}

	// Calculate scheduling parameters
	window := time.Duration(c.RateWindowMinutes) * time.Minute
	totalSlots := len(subscribers)

	// Generate planned from addresses for persona rotation
	plannedFroms := p.generatePlannedFromAddresses(totalSlots)

	// Create slots for all subscribers
	dueTimes := slotTimePlan(start, window, totalSlots, c.RateWindowCount, time.Now())
	if len(dueTimes) != totalSlots {
		return fmt.Errorf("slot time planner returned %d entries for %d subscribers", len(dueTimes), totalSlots)
	}

	for i, subscriber := range subscribers {
		due := dueTimes[i]
		plannedSubject := p.generatePlannedSubject(subscriber)

		if err := p.m.store.CampSchedInsertWithRecipient(
			c.ID,
			due,
			1,
			subscriber.ID,
			subscriber.Email,
			plannedFroms[i],
			plannedSubject,
		); err != nil {
			return fmt.Errorf("error creating slot for subscriber %d: %v", subscriber.ID, err)
		}
	}

	return nil
}

// generatePlannedFromAddresses generates planned from addresses for all slots
func (p *pipe) generatePlannedFromAddresses(count int) []string {
	c := p.camp
	froms := make([]string, count)

	// If persona is not used, use campaign's from_email
	if len(p.personaAccounts) == 0 {
		for i := 0; i < count; i++ {
			froms[i] = c.FromEmail
		}
		return froms
	}

	// If specific persona account is set, use it
	if c.PersonaAccount.Valid && strings.TrimSpace(c.PersonaAccount.String) != "" {
		addr := strings.TrimSpace(c.PersonaAccount.String)
		for i := 0; i < count; i++ {
			if p.personaName != "" {
				froms[i] = p.personaName + " <" + addr + ">"
			} else {
				froms[i] = addr
			}
		}
		return froms
	}

	// Rotate through persona accounts
	for i := 0; i < count; i++ {
		idx := i % len(p.personaAccounts)
		addr := p.personaAccounts[idx]
		if p.personaName != "" {
			froms[i] = p.personaName + " <" + addr + ">"
		} else {
			froms[i] = addr
		}
	}

	return froms
}

// generatePlannedSubject generates planned subject for a subscriber
func (p *pipe) generatePlannedSubject(subscriber models.Subscriber) string {
	// For now, return the campaign subject as-is
	// In the future, this could be enhanced to process templates with subscriber attributes
	return p.camp.Subject
}

// NextSubscribers processes the next batch of subscribers in a given campaign.
// It returns a bool indicating whether any subscribers were processed
// in the current batch or not. A false indicates that all subscribers
// have been processed, or that a campaign has been paused or cancelled.
func (p *pipe) NextSubscribers() (bool, error) {
	// Fetch the next batch of subscribers from a 'running' campaign.
	var (
		subs []models.Subscriber
		err  error
	)
	// Decide if per-campaign sliding is enabled; else fallback to global
	campHasSliding := p.camp.RateWindowCount > 0 && p.camp.RateWindowMinutes > 0
	// Is there a global sliding window limit configured?
	hasSliding := p.m.cfg.SlidingWindow &&
		p.m.cfg.SlidingWindowRate > 0 &&
		p.m.cfg.SlidingWindowDuration.Seconds() > 1

	if campHasSliding {
		if !p.sched.ready || p.sched.used >= p.sched.quota {
			id, dueAt, quota, used, slotSubscriberID, _, slotEmailFrom, _, ok, e := p.m.store.CampSchedNextDue(p.camp.ID)
			if e != nil {
				return false, e
			}
			if !ok {
				if err := p.planAllRemainingSlots(); err != nil {
					return false, err
				}
				p.wg.Add(1)
				p.enqueueAfter(time.Second)
				return false, nil
			}

			p.sched.id = id
			p.sched.due = dueAt
			p.sched.quota = quota
			p.sched.used = used
			p.sched.ready = true
			p.sched.subscriberID = slotSubscriberID
			p.sched.emailFrom = slotEmailFrom
		}

		subscriberID := p.sched.subscriberID
		if subscriberID > 0 {
			sub, err := p.m.store.GetSubscriberByID(subscriberID)
			if err != nil {
				return false, fmt.Errorf("error fetching pre-assigned subscriber %d: %v", subscriberID, err)
			}
			subs = []models.Subscriber{sub}
		} else {
			subs, err = p.m.store.NextSubscribers(p.camp.ID, 1)
			if err != nil {
				return false, err
			}
			if len(subs) == 0 {
				return false, nil
			}
			if err := p.m.store.AssignSlotSubscriber(p.sched.id, subs[0].ID, subs[0].Email); err != nil {
				return false, err
			}
			p.sched.subscriberID = subs[0].ID
		}

		if delay := time.Until(p.sched.due); delay > 0 {
			p.wg.Add(1)
			p.enqueueAfter(delay)
			return false, nil
		}

		p.currentSlotID = p.sched.id
	} else {
		if p.camp.Type == models.CampaignTypeSequence {
			subs, err = p.m.store.NextSequenceSubscribers(p.camp.ID, p.m.cfg.BatchSize)
		} else {
			subs, err = p.m.store.NextSubscribers(p.camp.ID, p.m.cfg.BatchSize)
		}
		if err != nil {
			return false, fmt.Errorf("error fetching campaign subscribers (%s): %v", p.camp.Name, err)
		}
		if len(subs) == 0 {
			return false, nil
		}
	}

	// Push messages.
	for _, s := range subs {
		msg, err := p.newMessage(s)
		if err != nil {
			p.m.log.Printf("error rendering message (%s) (%s): %v", p.camp.Name, s.Email, err)
			continue
		}

		// Use pre-assigned values from slot if available, otherwise use persona logic
		if campHasSliding {
			if p.sched.emailFrom != "" {
				msg.from = p.sched.emailFrom
			} else {
				p.applyPersonaFrom(&msg)
			}

			// consume slot credit before enqueueing
			_, used, status, err := p.m.store.CampSchedConsume(p.sched.id, 1)
			if err != nil {
				return false, err
			}
			p.sched.used = used
			if status == "done" {
				p.sched.ready = false
				p.sched.subscriberID = 0
				p.sched.emailFrom = ""
			}
			msg.SlotID = p.sched.id
		} else {
			p.applyPersonaFrom(&msg)
		}

		// Check if scheduling or sliding window is active BEFORE enqueueing the send.
		if !campHasSliding && hasSliding {
			// Global window
			for {
				diff := time.Since(p.m.slidingStart)
				if diff >= p.m.cfg.SlidingWindowDuration {
					p.m.slidingStart = time.Now()
					p.m.slidingCount = 0
					// re-evaluate immediately
					continue
				}
				if p.m.slidingCount >= p.m.cfg.SlidingWindowRate {
					wait := p.m.cfg.SlidingWindowDuration - diff
					p.m.log.Printf("messages exceeded (%d) for the window (%v since %s). Sleeping for %s.",
						p.m.slidingCount,
						p.m.cfg.SlidingWindowDuration,
						p.m.slidingStart.Format(time.RFC822Z),
						wait.Round(time.Second))
					p.m.slidingCount = 0
					time.Sleep(wait)
					continue
				}
				// allowed to send
				p.m.slidingCount++
				break
			}
		}

		// Push the message to the queue while blocking and waiting until
		// the queue is drained.
		p.m.campMsgQ <- msg
	}

	return true, nil
}

func (p *pipe) enqueueAfter(delay time.Duration) {
	if delay < time.Millisecond {
		delay = time.Millisecond
	}

	go func() {
		timer := time.NewTimer(delay)
		defer timer.Stop()

		<-timer.C
		if p.stopped.Load() {
			return
		}

		for {
			if p.stopped.Load() {
				return
			}
			select {
			case p.m.nextPipes <- p:
				return
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
}

func (p *pipe) applyPersonaFrom(msg *CampaignMessage) {
	if msg.from != "" {
		return
	}

	if len(p.personaAccounts) == 0 {
		return
	}

	addr := ""
	if p.camp.PersonaAccount.Valid && strings.TrimSpace(p.camp.PersonaAccount.String) != "" {
		addr = strings.TrimSpace(p.camp.PersonaAccount.String)
	} else {
		idx := int(p.personaIdx.Add(1) - 1)
		addr = p.personaAccounts[idx%len(p.personaAccounts)]
	}

	if p.personaName != "" {
		msg.from = p.personaName + " <" + addr + ">"
	} else {
		msg.from = addr
	}
}

func (p *pipe) pauseForDailyLimit() {
	if p == nil {
		return
	}
	if p.stopped.Load() {
		return
	}

	p.Stop(false)

	if err := p.m.store.UpdateCampaignStatus(p.camp.ID, models.CampaignStatusPaused); err != nil {
		p.m.log.Printf("error pausing campaign (%s) on daily limit: %v", p.camp.Name, err)
	} else {
		p.m.log.Printf("campaign (%s) paused: SMTP daily limit reached", p.camp.Name)
		_ = p.m.sendNotif(p.camp, models.CampaignStatusPaused, "SMTP daily limit reached")
	}
}

// parseFromNameEmail extracts display name and email from a typical "Name <email>" or "email" string.
func parseFromNameEmail(in string) (string, string) {
	in = strings.TrimSpace(in)
	if in == "" {
		return "", ""
	}
	lt := strings.Index(in, "<")
	gt := strings.Index(in, ">")
	if lt >= 0 && gt > lt {
		name := strings.Trim(strings.TrimSpace(in[:lt]), `"`)
		email := strings.TrimSpace(in[lt+1 : gt])
		return name, email
	}
	return "", in
}

// OnError keeps track of the number of errors that occur while sending messages
// and pauses the campaign if the error threshold is met.
func (p *pipe) OnError() {
	if p.m.cfg.MaxSendErrors < 1 {
		return
	}

	// If the error threshold is met, pause the campaign.
	count := p.errors.Add(1)
	if int(count) < p.m.cfg.MaxSendErrors {
		return
	}

	p.Stop(true)
	p.m.log.Printf("error count exceeded %d. pausing campaign %s", p.m.cfg.MaxSendErrors, p.camp.Name)
}

// Stop "marks" a campaign as stopped. It doesn't actually stop the processing
// of messages. That happens when every queued message in the campaign is processed,
// marking .wg, the waitgroup counter as done. That triggers cleanup().
func (p *pipe) Stop(withErrors bool) {
	// Already stopped.
	if p.stopped.Load() {
		return
	}

	if withErrors {
		p.withErrors.Store(true)
	}

	p.stopped.Store(true)
}

// newMessage returns a campaign message while internally incrementing the
// number of messages in the pipe wait group so that the status of every
// message can be atomically tracked.
func (p *pipe) newMessage(s models.Subscriber) (CampaignMessage, error) {
	// For sequence, build a per-step campaign clone with step content.
	if p.camp.Type == models.CampaignTypeSequence {
		stepIndex, _, _, storedFrom, _, err := p.m.store.GetSequenceProgress(p.camp.ID, s.ID)
		if err != nil {
			return CampaignMessage{}, err
		}
		// Parse sequence JSON.
		seq, err := models.ParseSequence(p.camp.Sequence)
		if err != nil {
			return CampaignMessage{}, err
		}
		if stepIndex < 0 || stepIndex >= len(seq.Steps) {
			return CampaignMessage{}, fmt.Errorf("invalid sequence step index %d", stepIndex)
		}

		// Optimistically reserve this subscriber for the current step to avoid
		// duplicate selection in the same scan cycle.
		lockAt := time.Now().Add(2 * time.Minute)
		if err := p.m.store.AdvanceSequenceProgress(p.camp.ID, s.ID, stepIndex, &lockAt, false); err != nil {
			return CampaignMessage{}, err
		}

		// Clone campaign with step content.
		cp := *p.camp
		st := seq.Steps[stepIndex]
		cp.Subject = st.Subject
		if st.Content != nil {
			cp.ContentType = st.Content.ContentType
			cp.Body = st.Content.Body
			cp.BodySource = null.NewString(st.Content.BodySource, st.Content.BodySource != "")
			if st.Content.TemplateID > 0 {
				cp.TemplateID = null.NewInt(int(st.Content.TemplateID), true)
			}
		}
		if st.AltBody != nil {
			cp.AltBody = null.NewString(*st.AltBody, true)
		} else {
			cp.AltBody.Valid = false
		}

		if err := cp.CompileTemplate(p.m.TemplateFuncs(&cp)); err != nil {
			return CampaignMessage{}, err
		}
		msg, err := p.m.NewCampaignMessage(&cp, s)
		if err != nil {
			return msg, err
		}
		msg.pipe = p
		msg.StepIndex = stepIndex

		// If persona is in use for sequences, prefer stored from_email for this subscriber.
		if len(p.personaAccounts) > 0 {
			if storedFrom != "" {
				if p.personaName != "" {
					msg.from = p.personaName + " <" + storedFrom + ">"
				} else {
					msg.from = storedFrom
				}
			}
		}
		// NextStepAt is computed in worker when advancing.
		msg.SlotID = p.currentSlotID
		p.wg.Add(1)
		return msg, nil
	}

	msg, err := p.m.NewCampaignMessage(p.camp, s)
	if err != nil {
		return msg, err
	}

	msg.pipe = p
	msg.SlotID = p.currentSlotID
	p.wg.Add(1)

	return msg, nil
}

// cleanup finishes the campaign and updates the campaign status in the DB
// and also triggers a notification to the admin. This only triggers once
// a pipe's wg counter is fully exhausted, draining all messages in its queue.
func (p *pipe) cleanup() {
	defer func() {
		p.m.pipesMut.Lock()
		delete(p.m.pipes, p.camp.ID)
		p.m.pipesMut.Unlock()
	}()

	// Update campaign's 'sent' count with any residual not yet flushed by scanner.
	// Use atomic swap to avoid races with periodic scans that also flush counts.
	residual := int(p.sent.Swap(0))
	if err := p.m.store.UpdateCampaignCounts(p.camp.ID, 0, residual, int(p.lastID.Load())); err != nil {
		p.m.log.Printf("error updating campaign counts (%s): %v", p.camp.Name, err)
	}

	// The campaign was auto-paused due to errors.
	if p.withErrors.Load() {
		if err := p.m.store.UpdateCampaignStatus(p.camp.ID, models.CampaignStatusPaused); err != nil {
			p.m.log.Printf("error updating campaign (%s) status to %s: %v", p.camp.Name, models.CampaignStatusPaused, err)
		} else {
			p.m.log.Printf("set campaign (%s) to %s", p.camp.Name, models.CampaignStatusPaused)
		}

		_ = p.m.sendNotif(p.camp, models.CampaignStatusPaused, "Too many errors")
		return
	}

	// The campaign was manually stopped (pause, cancel).
	if p.stopped.Load() {
		p.m.log.Printf("stop processing campaign (%s)", p.camp.Name)
		return
	}

	// Campaign wasn't manually stopped and subscribers were naturally exhausted.
	// Fetch the up-to-date campaign status from the DB.
	c, err := p.m.store.GetCampaign(p.camp.ID)
	if err != nil {
		p.m.log.Printf("error fetching campaign (%s) for ending: %v", p.camp.Name, err)
		return
	}

	if p.camp.Type == models.CampaignTypeSequence {
		// For sequence campaigns, see if there is a next due time to schedule.
		nextAt, err := p.m.store.GetEarliestSequenceNext(p.camp.ID)
		if err != nil {
			p.m.log.Printf("error fetching earliest sequence time (%s): %v", p.camp.Name, err)
			return
		}
		if nextAt != nil {
			if err := p.m.store.UpdateCampaignSendAt(p.camp.ID, *nextAt, models.CampaignStatusScheduled); err != nil {
				p.m.log.Printf("error scheduling next sequence step for (%s): %v", p.camp.Name, err)
			} else {
				p.m.log.Printf("campaign (%s) scheduled for next sequence at %s", p.camp.Name, nextAt.Format(time.RFC3339))
			}
		} else {
			// No more steps pending; finish campaign
			if err := p.m.store.UpdateCampaignStatus(p.camp.ID, models.CampaignStatusFinished); err != nil {
				p.m.log.Printf("error finishing campaign (%s): %v", p.camp.Name, err)
			} else {
				p.m.log.Printf("campaign (%s) finished", p.camp.Name)
			}
		}
	} else {
		// If a running campaign has exhausted subscribers, it's finished.
		if c.Status == models.CampaignStatusRunning || c.Status == models.CampaignStatusScheduled {
			c.Status = models.CampaignStatusFinished
			if err := p.m.store.UpdateCampaignStatus(p.camp.ID, models.CampaignStatusFinished); err != nil {
				p.m.log.Printf("error finishing campaign (%s): %v", p.camp.Name, err)
			} else {
				p.m.log.Printf("campaign (%s) finished", p.camp.Name)
			}
		} else {
			p.m.log.Printf("finish processing campaign (%s)", p.camp.Name)
		}
	}

	// Notify admin.
	_ = p.m.sendNotif(c, c.Status, "")
}

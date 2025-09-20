package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/manager"
	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
	"strings"
	"time"
)

// store implements DataSource over the primary
// database.
type store struct {
	queries *models.Queries
	core    *core.Core
	media   media.Store
}

type runningCamp struct {
	CampaignID       int    `db:"campaign_id"`
	CampaignType     string `db:"campaign_type"`
	LastSubscriberID int    `db:"last_subscriber_id"`
	MaxSubscriberID  int    `db:"max_subscriber_id"`
	ListID           int    `db:"list_id"`
}

func newManagerStore(q *models.Queries, c *core.Core, m media.Store) *store {
	return &store{
		queries: q,
		core:    c,
		media:   m,
	}
}

// NextCampaigns retrieves active campaigns ready to be processed excluding
// campaigns that are also being processed. Additionally, it takes a map of campaignID:sentCount
// of campaigns that are being processed and updates them in the DB.
func (s *store) NextCampaigns(currentIDs []int64, sentCounts []int64) ([]*models.Campaign, error) {
	var out []*models.Campaign
	err := s.queries.NextCampaigns.Select(&out, pq.Int64Array(currentIDs), pq.Int64Array(sentCounts))
	return out, err
}

// NextSubscribers retrieves a subset of subscribers of a given campaign.
// Since batches are processed sequentially, the retrieval is ordered by ID,
// and every batch takes the last ID of the last batch and fetches the next
// batch above that.
func (s *store) NextSubscribers(campID, limit int) ([]models.Subscriber, error) {
	var camps []runningCamp
	if err := s.queries.GetRunningCampaign.Select(&camps, campID); err != nil {
		return nil, err
	}

	var listIDs []int
	for _, c := range camps {
		listIDs = append(listIDs, c.ListID)
	}

	if len(listIDs) == 0 {
		return nil, nil
	}

	var out []models.Subscriber
	err := s.queries.NextCampaignSubscribers.Select(&out, camps[0].CampaignID, camps[0].CampaignType, camps[0].LastSubscriberID, camps[0].MaxSubscriberID, pq.Array(listIDs), limit)
	return out, err
}

// NextSequenceSubscribers returns due subscribers for a sequence campaign.
func (s *store) NextSequenceSubscribers(campID, limit int) ([]models.Subscriber, error) {
	var out []models.Subscriber
	err := s.queries.NextSequenceSubscribers.Select(&out, campID, limit)
	return out, err
}

// GetSequenceProgress returns current step and next time for a subscriber in a campaign.
func (s *store) GetSequenceProgress(campID int, subscriberID int) (int, time.Time, string, string, bool, error) {
	var row struct {
		CurrentStep int            `db:"current_step"`
		NextSendAt  time.Time      `db:"next_send_at"`
		Status      string         `db:"status"`
		FromEmail   sql.NullString `db:"from_email"`
	}
	err := s.queries.GetSequenceProgress.Get(&row, campID, subscriberID)
	if err != nil {
		if err == sql.ErrNoRows {
			camp, e := s.GetCampaign(campID)
			if e != nil {
				return 0, time.Time{}, "", "", false, e
			}
			if camp.SendAt.Valid {
				return 0, camp.SendAt.Time, "pending", "", false, nil
			}
			return 0, time.Time{}, "pending", "", false, nil
		}
		return 0, time.Time{}, "", "", false, err
	}
	fe := ""
	if row.FromEmail.Valid {
		fe = row.FromEmail.String
	}
	return row.CurrentStep, row.NextSendAt, row.Status, fe, true, nil
}

// AdvanceSequenceProgress upserts the next step or marks as done for a subscriber.
func (s *store) AdvanceSequenceProgress(campID int, subscriberID int, step int, nextAt *time.Time, done bool) error {
	status := "pending"
	if done {
		status = "done"
	}
	if nextAt == nil {
		_, err := s.queries.AdvanceSequenceProgress.Exec(campID, subscriberID, step, nil, status)
		return err
	}
	_, err := s.queries.AdvanceSequenceProgress.Exec(campID, subscriberID, step, *nextAt, status)
	return err
}

func (s *store) SetSequenceFromEmail(campID int, subscriberID int, fromEmail string) error {
	_, err := s.queries.SetSequenceFromEmail.Exec(campID, subscriberID, fromEmail)
	return err
}

// GetEarliestSequenceNext returns the earliest next_send_at among pending progress rows or campaign.send_at if in future.
func (s *store) GetEarliestSequenceNext(campID int) (*time.Time, error) {
	var t sql.NullTime
	if err := s.queries.GetEarliestSequenceNext.Get(&t, campID); err != nil {
		return nil, err
	}
	if t.Valid {
		return &t.Time, nil
	}
	return nil, nil
}

// UpdateCampaignSendAt updates campaign send_at and optionally status.
func (s *store) UpdateCampaignSendAt(campID int, tt time.Time, status string) error {
	_, err := s.queries.UpdateCampaignSendAt.Exec(campID, tt, status)
	return err
}

// RecordSend inserts a send log entry.
func (s *store) RecordSend(campID int, subscriberID int, step int, subject string, fromEmail string, success bool, errMsg string, replyToken string, messageID string) error {
	var stepPtr *int
	if step >= 0 {
		stepPtr = &step
	}
	_, err := s.queries.InsertCampaignSend.Exec(campID, subscriberID, stepPtr, subject, fromEmail, success, errMsg, replyToken, messageID)
	return err
}

// RecordImmediateBounce logs a bounce immediately on SMTP send error using Core's bounce settings.
func (s *store) RecordImmediateBounce(sub models.Subscriber, camp *models.Campaign, smtpErr error) error {
	if smtpErr == nil {
		return nil
	}
	// Map error to bounce type.
	typ := "soft"
	es := smtpErr.Error()
	if strings.HasPrefix(es, "5") || strings.Contains(es, " 5.") || strings.Contains(strings.ToLower(es), "rejected") {
		typ = "hard"
	}
	meta, _ := json.Marshal(map[string]string{"error": es})
	b := models.Bounce{
		SubscriberUUID: sub.UUID,
		Email:          sub.Email,
		CampaignUUID:   camp.UUID,
		Type:           typ,
		Source:         "smtp",
		Meta:           json.RawMessage(meta),
		CreatedAt:      time.Now(),
	}
	return s.core.RecordBounce(b)
}

// GetCampaign fetches a campaign from the database.
func (s *store) GetCampaign(campID int) (*models.Campaign, error) {
	var out = &models.Campaign{}
	err := s.queries.GetCampaign.Get(out, campID, nil, nil, "default")
	return out, err
}

// UpdateCampaignStatus updates a campaign's status.
func (s *store) UpdateCampaignStatus(campID int, status string) error {
	_, err := s.queries.UpdateCampaignStatus.Exec(campID, status)
	return err
}

// UpdateCampaignCounts updates a campaign's status.
func (s *store) UpdateCampaignCounts(campID int, toSend int, sent int, lastSubID int) error {
	_, err := s.queries.UpdateCampaignCounts.Exec(campID, toSend, sent, lastSubID)
	return err
}

// GetAttachment fetches a media attachment blob.
func (s *store) GetAttachment(mediaID int) (models.Attachment, error) {
	m, err := s.core.GetMedia(mediaID, "", "", s.media)
	if err != nil {
		return models.Attachment{}, err
	}

	b, err := s.media.GetBlob(m.URL)
	if err != nil {
		return models.Attachment{}, err
	}

	return models.Attachment{
		Name:    m.Filename,
		Content: b,
		Header:  manager.MakeAttachmentHeader(m.Filename, "base64", m.ContentType),
	}, nil
}

// CreateLink registers a URL with a UUID for tracking clicks and returns the UUID.
func (s *store) CreateLink(url string) (string, error) {
	// Create a new UUID for the URL. If the URL already exists in the DB
	// the UUID in the database is returned.
	uu, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	var out string
	if err := s.queries.CreateLink.Get(&out, uu, url); err != nil {
		return "", err
	}

	return out, nil
}

// GetSettings fetches global settings to access personas for persona-based campaigns.
func (s *store) GetSettings() (*models.Settings, error) {
	set, err := s.core.GetSettings()
	if err != nil {
		return nil, err
	}
	return &set, nil
}

// ============= Campaign schedule (window-based) =============
func (s *store) CampSchedInsert(campID int, due time.Time, quota int) error {
	_, err := s.queries.CampSchedInsert.Exec(campID, due, quota, nil, nil, nil, nil)
	return err
}

func (s *store) CampSchedInsertWithRecipient(campID int, due time.Time, quota int, subscriberID int, emailTo, emailFrom, subject string) error {
	_, err := s.queries.CampSchedInsert.Exec(campID, due, quota, subscriberID, emailTo, emailFrom, subject)
	return err
}

func (s *store) CampSchedDeleteFuture(campID int) error {
	_, err := s.queries.CampSchedDeleteFuture.Exec(campID)
	return err
}

func (s *store) CampSchedDeleteOutdated(campID int) error {
	_, err := s.queries.CampSchedDeleteOutdated.Exec(campID)
	return err
}

func (s *store) CampSchedNextDue(campID int) (int64, time.Time, int, int, int, string, string, string, bool, error) {
	var row struct {
		ID           int64          `db:"id"`
		DueAt        time.Time      `db:"due_at"`
		Quota        int            `db:"quota"`
		Used         int            `db:"used"`
		SubscriberID *int           `db:"subscriber_id"`
		EmailTo      sql.NullString `db:"email_to"`
		EmailFrom    sql.NullString `db:"email_from"`
		Subject      sql.NullString `db:"subject"`
	}
	if err := s.queries.CampSchedNextDue.Get(&row, campID); err != nil {
		if err == sql.ErrNoRows {
			return 0, time.Time{}, 0, 0, 0, "", "", "", false, nil
		}
		return 0, time.Time{}, 0, 0, 0, "", "", "", false, err
	}

	subscriberID := 0
	if row.SubscriberID != nil {
		subscriberID = *row.SubscriberID
	}

	emailTo := ""
	if row.EmailTo.Valid {
		emailTo = row.EmailTo.String
	}

	emailFrom := ""
	if row.EmailFrom.Valid {
		emailFrom = row.EmailFrom.String
	}

	subject := ""
	if row.Subject.Valid {
		subject = row.Subject.String
	}

	return row.ID, row.DueAt, row.Quota, row.Used, subscriberID, emailTo, emailFrom, subject, true, nil
}

func (s *store) CampSchedConsume(id int64, n int) (int, int, string, error) {
	var row struct {
		ID     int64     `db:"id"`
		DueAt  time.Time `db:"due_at"`
		Quota  int       `db:"quota"`
		Used   int       `db:"used"`
		Status string    `db:"status"`
	}
	if err := s.queries.CampSchedConsume.Get(&row, id, n); err != nil {
		return 0, 0, "", err
	}
	return row.Quota, row.Used, row.Status, nil
}

func (s *store) CampSchedMarkDoneEmail(id int64, to, from, subject string) error {
	_, err := s.queries.CampSchedMarkDoneEmail.Exec(id, to, from, subject)
	return err
}

func (s *store) AssignSlotSubscriber(id int64, subscriberID int, email string) error {
	_, err := s.queries.CampSchedAssignSubscriber.Exec(id, subscriberID, email)
	return err
}

func (s *store) GetCampaignSubscribers(campID, lastID, maxID int, campType string, limit int) ([]models.Subscriber, error) {
	var out []models.Subscriber
	err := s.queries.CampSchedGetCampaignSubscribers.Select(&out, campID, lastID, maxID, campType, limit)
	return out, err
}

func (s *store) UpdateCampaignMaxSubscriberID(campID, maxSubID int) error {
	_, err := s.queries.UpdateCampaignMaxSubscriberID.Exec(campID, maxSubID)
	return err
}

func (s *store) GetSubscriberByID(id int) (models.Subscriber, error) {
	var sub models.Subscriber
	if err := s.queries.GetSubscriber.Get(&sub, id, "", ""); err != nil {
		return models.Subscriber{}, err
	}
	return sub, nil
}

// RecordBounce records a bounce event and returns the bounce count.
func (s *store) RecordBounce(b models.Bounce) (int64, int, error) {
	var res = struct {
		SubscriberID int64 `db:"subscriber_id"`
		Num          int   `db:"num"`
	}{}

	err := s.queries.UpdateCampaignStatus.Select(&res,
		b.SubscriberUUID,
		b.Email,
		b.CampaignUUID,
		b.Type,
		b.Source,
		b.Meta)

	return res.SubscriberID, res.Num, err
}

// BlocklistSubscriber blocklists a subscriber permanently.
func (s *store) BlocklistSubscriber(id int64) error {
	_, err := s.queries.BlocklistSubscribers.Exec(pq.Int64Array{id})
	return err
}

// DeleteSubscriber deletes a subscriber from the DB.
func (s *store) DeleteSubscriber(id int64) error {
	_, err := s.queries.DeleteSubscribers.Exec(pq.Int64Array{id})
	return err
}

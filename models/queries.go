package models

import (
	"context"
	"database/sql"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Queries contains all prepared SQL queries.
type Queries struct {
	GetDashboardCharts *sqlx.Stmt `query:"get-dashboard-charts"`
	GetDashboardCounts *sqlx.Stmt `query:"get-dashboard-counts"`

	InsertSubscriber                *sqlx.Stmt `query:"insert-subscriber"`
	UpsertSubscriber                *sqlx.Stmt `query:"upsert-subscriber"`
	UpsertBlocklistSubscriber       *sqlx.Stmt `query:"upsert-blocklist-subscriber"`
	GetSubscriber                   *sqlx.Stmt `query:"get-subscriber"`
	HasSubscriberLists              *sqlx.Stmt `query:"has-subscriber-list"`
	GetSubscribersByEmails          *sqlx.Stmt `query:"get-subscribers-by-emails"`
	GetSubscriberLists              *sqlx.Stmt `query:"get-subscriber-lists"`
	GetSubscriptions                *sqlx.Stmt `query:"get-subscriptions"`
	GetSubscriberListsLazy          *sqlx.Stmt `query:"get-subscriber-lists-lazy"`
	UpdateSubscriber                *sqlx.Stmt `query:"update-subscriber"`
	UpdateSubscriberValidation      *sqlx.Stmt `query:"update-subscriber-validation"`
	UpdateSubscriberWithLists       *sqlx.Stmt `query:"update-subscriber-with-lists"`
	BlocklistSubscribers            *sqlx.Stmt `query:"blocklist-subscribers"`
	AddSubscribersToLists           *sqlx.Stmt `query:"add-subscribers-to-lists"`
	DeleteSubscriptions             *sqlx.Stmt `query:"delete-subscriptions"`
	DeleteUnconfirmedSubscriptions  *sqlx.Stmt `query:"delete-unconfirmed-subscriptions"`
	ConfirmSubscriptionOptin        *sqlx.Stmt `query:"confirm-subscription-optin"`
	UnsubscribeSubscribersFromLists *sqlx.Stmt `query:"unsubscribe-subscribers-from-lists"`
	DeleteSubscribers               *sqlx.Stmt `query:"delete-subscribers"`
	GetEmailDomainLabels            *sqlx.Stmt `query:"get-email-domain-labels"`
	DeleteBlocklistedSubscribers    *sqlx.Stmt `query:"delete-blocklisted-subscribers"`
	DeleteOrphanSubscribers         *sqlx.Stmt `query:"delete-orphan-subscribers"`
	UnsubscribeByCampaign           *sqlx.Stmt `query:"unsubscribe-by-campaign"`
	ExportSubscriberData            *sqlx.Stmt `query:"export-subscriber-data"`

	// Non-prepared arbitrary subscriber queries.
	QuerySubscribers                       string     `query:"query-subscribers"`
	QuerySubscribersCount                  string     `query:"query-subscribers-count"`
	QuerySubscribersCountAll               *sqlx.Stmt `query:"query-subscribers-count-all"`
	QuerySubscribersForExport              string     `query:"query-subscribers-for-export"`
	QuerySubscribersTpl                    string     `query:"query-subscribers-template"`
	DeleteSubscribersByQuery               string     `query:"delete-subscribers-by-query"`
	AddSubscribersToListsByQuery           string     `query:"add-subscribers-to-lists-by-query"`
	BlocklistSubscribersByQuery            string     `query:"blocklist-subscribers-by-query"`
	DeleteSubscriptionsByQuery             string     `query:"delete-subscriptions-by-query"`
	UnsubscribeSubscribersFromListsByQuery string     `query:"unsubscribe-subscribers-from-lists-by-query"`

	CreateList      *sqlx.Stmt `query:"create-list"`
	QueryLists      string     `query:"query-lists"`
	GetLists        *sqlx.Stmt `query:"get-lists"`
	GetListsByOptin *sqlx.Stmt `query:"get-lists-by-optin"`
	GetListTypes    *sqlx.Stmt `query:"get-list-types"`
	UpdateList      *sqlx.Stmt `query:"update-list"`
	UpdateListsDate *sqlx.Stmt `query:"update-lists-date"`
	DeleteLists     *sqlx.Stmt `query:"delete-lists"`

	CreateCampaign        *sqlx.Stmt `query:"create-campaign"`
	QueryCampaigns        string     `query:"query-campaigns"`
	GetCampaign           *sqlx.Stmt `query:"get-campaign"`
	GetCampaignForPreview *sqlx.Stmt `query:"get-campaign-for-preview"`
	GetCampaignStats      *sqlx.Stmt `query:"get-campaign-stats"`
	GetCampaignStatus     *sqlx.Stmt `query:"get-campaign-status"`
	GetArchivedCampaigns  *sqlx.Stmt `query:"get-archived-campaigns"`
	CampaignHasLists      *sqlx.Stmt `query:"campaign-has-lists"`

	// These two queries are read as strings and based on settings.individual_tracking=on/off,
	// are interpolated and copied to view and click counts. Same query, different tables.
	GetCampaignAnalyticsCounts string     `query:"get-campaign-analytics-counts"`
	GetCampaignViewCounts      *sqlx.Stmt `query:"get-campaign-view-counts"`
	GetCampaignClickCounts     *sqlx.Stmt `query:"get-campaign-click-counts"`
	GetCampaignLinkCounts      *sqlx.Stmt `query:"get-campaign-link-counts"`
	GetCampaignBounceCounts    *sqlx.Stmt `query:"get-campaign-bounce-counts"`
	DeleteCampaignViews        *sqlx.Stmt `query:"delete-campaign-views"`
	DeleteCampaignLinkClicks   *sqlx.Stmt `query:"delete-campaign-link-clicks"`
	GetRecentCampaignViews     *sqlx.Stmt `query:"get-recent-campaign-views"`
	GetRecentLinkClicks        *sqlx.Stmt `query:"get-recent-link-clicks"`

	// Campaign deliveries (per-recipient status)
	QueryCampaignDeliveries string `query:"query-campaign-deliveries"`

	NextCampaigns           *sqlx.Stmt `query:"next-campaigns"`
	GetRunningCampaign      *sqlx.Stmt `query:"get-running-campaign"`
	NextCampaignSubscribers *sqlx.Stmt `query:"next-campaign-subscribers"`
	// Sequence (per-subscriber progress)
	NextSequenceSubscribers      *sqlx.Stmt `query:"next-sequence-subscribers"`
	GetSequenceProgress          *sqlx.Stmt `query:"get-sequence-progress"`
	SetSequenceFromEmail         *sqlx.Stmt `query:"set-sequence-from-email"`
	AdvanceSequenceProgress      *sqlx.Stmt `query:"advance-sequence-progress"`
	GetEarliestSequenceNext      *sqlx.Stmt `query:"get-earliest-sequence-next"`
	UpdateCampaignSendAt         *sqlx.Stmt `query:"update-campaign-send-at"`
	GetOneCampaignSubscriber     *sqlx.Stmt `query:"get-one-campaign-subscriber"`
	UpdateCampaign               *sqlx.Stmt `query:"update-campaign"`
	UpdateCampaignPersona        *sqlx.Stmt `query:"update-campaign-persona"`
	UpdateCampaignFromName       *sqlx.Stmt `query:"update-campaign-from-name"`
	UpdateCampaignPersonaAccount *sqlx.Stmt `query:"update-campaign-persona-account"`
	UpdateCampaignStatus         *sqlx.Stmt `query:"update-campaign-status"`
	UpdateCampaignCounts         *sqlx.Stmt `query:"update-campaign-counts"`
	UpdateCampaignArchive        *sqlx.Stmt `query:"update-campaign-archive"`
	RegisterCampaignView         *sqlx.Stmt `query:"register-campaign-view"`
	DeleteCampaign               *sqlx.Stmt `query:"delete-campaign"`

	InsertMedia *sqlx.Stmt `query:"insert-media"`
	GetMedia    *sqlx.Stmt `query:"get-media"`
	QueryMedia  *sqlx.Stmt `query:"query-media"`
	DeleteMedia *sqlx.Stmt `query:"delete-media"`

	CreateTemplate     *sqlx.Stmt `query:"create-template"`
	GetTemplates       *sqlx.Stmt `query:"get-templates"`
	UpdateTemplate     *sqlx.Stmt `query:"update-template"`
	SetDefaultTemplate *sqlx.Stmt `query:"set-default-template"`
	DeleteTemplate     *sqlx.Stmt `query:"delete-template"`

	CreateLink        *sqlx.Stmt `query:"create-link"`
	RegisterLinkClick *sqlx.Stmt `query:"register-link-click"`

	// Sends log
	InsertCampaignSend          *sqlx.Stmt `query:"insert-campaign-send"`
	UpdateCampaignSendReplied   *sqlx.Stmt `query:"update-campaign-send-replied"`
	GetCampaignSendByReplyToken *sqlx.Stmt `query:"get-campaign-send-by-reply-token"`
	GetCampaignSendByMessageID  *sqlx.Stmt `query:"get-campaign-send-by-message-id"`
	InsertCampaignReply         *sqlx.Stmt `query:"insert-campaign-reply"`
	QueryRecentSends            *sqlx.Stmt `query:"query-recent-sends"`

	GetSettings    *sqlx.Stmt `query:"get-settings"`
	UpdateSettings *sqlx.Stmt `query:"update-settings"`
	UpsertSettings *sqlx.Stmt `query:"upsert-settings"`

	// GetStats *sqlx.Stmt `query:"get-stats"`
	RecordBounce                *sqlx.Stmt `query:"record-bounce"`
	QueryBounces                string     `query:"query-bounces"`
	BlocklistBouncedSubscribers *sqlx.Stmt `query:"blocklist-bounced-subscribers"`
	DeleteBounces               *sqlx.Stmt `query:"delete-bounces"`
	DeleteBouncesBySubscriber   *sqlx.Stmt `query:"delete-bounces-by-subscriber"`
	GetDBInfo                   string     `query:"get-db-info"`

	CreateUser        *sqlx.Stmt `query:"create-user"`
	UpdateUser        *sqlx.Stmt `query:"update-user"`
	UpdateUserProfile *sqlx.Stmt `query:"update-user-profile"`
	UpdateUserLogin   *sqlx.Stmt `query:"update-user-login"`
	DeleteUsers       *sqlx.Stmt `query:"delete-users"`
	GetUsers          *sqlx.Stmt `query:"get-users"`
	GetUser           *sqlx.Stmt `query:"get-user"`
	GetAPITokens      *sqlx.Stmt `query:"get-api-tokens"`
	LoginUser         *sqlx.Stmt `query:"login-user"`

	CreateRole            *sqlx.Stmt `query:"create-role"`
	GetUserRoles          *sqlx.Stmt `query:"get-user-roles"`
	GetListRoles          *sqlx.Stmt `query:"get-list-roles"`
	UpdateRole            *sqlx.Stmt `query:"update-role"`
	DeleteRole            *sqlx.Stmt `query:"delete-role"`
	UpsertListPermissions *sqlx.Stmt `query:"upsert-list-permissions"`
	DeleteListPermission  *sqlx.Stmt `query:"delete-list-permission"`

	// SMTP stats
	SmtpIncrement *sqlx.Stmt `query:"smtp-increment"`
	SmtpGetToday  *sqlx.Stmt `query:"smtp-get-today"`
	SmtpSetToday  *sqlx.Stmt `query:"smtp-set-today"`
	SmtpGetTotal  *sqlx.Stmt `query:"smtp-get-total"`
	SmtpGetLatest *sqlx.Stmt `query:"smtp-get-latest"`

	// WarmCloud schedule
	WarmSchedInsert                     *sqlx.Stmt `query:"warmcloud-sched-insert"`
	WarmSchedDeleteFuture               *sqlx.Stmt `query:"warmcloud-sched-delete-future"`
	WarmSchedNextDueByServer            *sqlx.Stmt `query:"warmcloud-sched-next-due-by-server"`
	WarmSchedNextDueWithFromEmail       *sqlx.Stmt `query:"warmcloud-sched-next-due-with-from-email"`
	WarmSchedMarkDone                   *sqlx.Stmt `query:"warmcloud-sched-mark-done"`
	WarmSchedMarkDoneEmail              *sqlx.Stmt `query:"warmcloud-sched-mark-done-email"`
	WarmSchedMarkError                  *sqlx.Stmt `query:"warmcloud-sched-mark-error"`
	WarmSchedMarkRead                   *sqlx.Stmt `query:"warmcloud-sched-mark-read"`
	WarmSchedMarkReadEmail              *sqlx.Stmt `query:"warmcloud-sched-mark-read-email"`
	WarmSchedUpdateFromEmail            *sqlx.Stmt `query:"warmcloud-sched-update-from-email"`
	WarmSchedCountPendingToday          *sqlx.Stmt `query:"warmcloud-sched-count-pending-today"`
	WarmSchedCountPendingWindow         *sqlx.Stmt `query:"warmcloud-sched-count-pending-window"`
	WarmSchedListByDay                  *sqlx.Stmt `query:"warmcloud-sched-list-by-day"`
	WarmSchedDeleteDayPending           *sqlx.Stmt `query:"warmcloud-sched-delete-day-pending"`
	WarmSchedInsertWithFromEmail        *sqlx.Stmt `query:"warmcloud-sched-insert-with-from-email"`
	WarmSchedInsertWithFromEmailSubject *sqlx.Stmt `query:"warmcloud-sched-insert-with-from-email-subject"`
	WarmSchedListDayBasic               *sqlx.Stmt `query:"warmcloud-sched-list-day-basic"`
	WarmSchedGetSentToday               *sqlx.Stmt `query:"warmcloud-sched-get-sent-today"`
	WarmSchedMarkAllTodayDone           *sqlx.Stmt `query:"warmcloud-sched-mark-all-today-done"`
	WarmSchedDeleteUnsentOnly           *sqlx.Stmt `query:"warmcloud-sched-delete-unsent-only"`
	WarmSchedDeleteAll                  *sqlx.Stmt `query:"warmcloud-sched-delete-all"`
	WarmSchedUpdatePersona              *sqlx.Stmt `query:"warmcloud-sched-update-persona"`

	//

	// Campaign schedule (window-based)
	CampSchedInsert                 *sqlx.Stmt `query:"camp-sched-insert"`
	CampSchedDeleteFuture           *sqlx.Stmt `query:"camp-sched-delete-future"`
	CampSchedDeleteOutdated         *sqlx.Stmt `query:"camp-sched-delete-outdated"`
	CampSchedNextDue                *sqlx.Stmt `query:"camp-sched-next-due"`
	CampSchedConsume                *sqlx.Stmt `query:"camp-sched-consume"`
	CampSchedListByDay              *sqlx.Stmt `query:"camp-sched-list-by-day"`
	CampSchedReschedule             *sqlx.Stmt `query:"camp-sched-reschedule"`
	CampSchedDelete                 *sqlx.Stmt `query:"camp-sched-delete"`
	CampSchedSkip                   *sqlx.Stmt `query:"camp-sched-skip"`
	CampSchedMarkDoneEmail          *sqlx.Stmt `query:"camp-sched-mark-done-email"`
	CampSchedAssignSubscriber       *sqlx.Stmt `query:"camp-sched-assign-subscriber"`
	CampSchedGetCampaignSubscribers *sqlx.Stmt `query:"camp-sched-get-campaign-subscribers"`
	UpdateCampaignMaxSubscriberID   *sqlx.Stmt `query:"update-campaign-max-subscriber-id"`
}

// compileSubscriberQueryTpl takes an arbitrary WHERE expressions
// to filter subscribers from the subscribers table and prepares a query
// out of it using the raw `query-subscribers-template` query template.
// While doing this, a readonly transaction is created and the query is
// dry run on it to ensure that it is indeed readonly.
func (q *Queries) compileSubscriberQueryTpl(searchStr, queryExp string, db *sqlx.DB, subStatus string) (string, error) {
	tx, err := db.BeginTxx(context.Background(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// There's an arbitrary query condition.
	cond := "TRUE"
	if queryExp != "" {
		cond = queryExp
	}

	// Perform the dry run.
	stmt := strings.ReplaceAll(q.QuerySubscribersTpl, "%query%", cond)
	if _, err := tx.Exec(stmt, true, pq.Int64Array{}, subStatus, searchStr); err != nil {
		return "", err
	}

	return stmt, nil
}

// compileSubscriberQueryTpl takes an arbitrary WHERE expressions and a subscriber
// query template that depends on the filter (eg: delete by query, blocklist by query etc.)
// combines and executes them.
func (q *Queries) ExecSubQueryTpl(searchStr, queryExp, baseQueryTpl string, listIDs []int, db *sqlx.DB, subStatus string, args ...any) error {
	// Perform a dry run.
	filterExp, err := q.compileSubscriberQueryTpl(searchStr, queryExp, db, subStatus)
	if err != nil {
		return err
	}

	if len(listIDs) == 0 {
		listIDs = []int{}
	}

	// Insert the subscriber filter query into the target query.
	stmt := strings.ReplaceAll(baseQueryTpl, "%query%", filterExp)

	// First argument is the boolean indicating if the query is a dry run.
	a := append([]any{false, pq.Array(listIDs), subStatus, searchStr}, args...)

	// Execute the query on the DB.
	if _, err := db.Exec(stmt, a...); err != nil {
		return err
	}
	return nil
}

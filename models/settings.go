package models

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/volatiletech/null.v6"
)

// Settings represents the app settings stored in the DB.
type Settings struct {
	AppSiteName                   string   `json:"app.site_name"`
	AppRootURL                    string   `json:"app.root_url"`
	AppLogoURL                    string   `json:"app.logo_url"`
	AppFaviconURL                 string   `json:"app.favicon_url"`
	AppFromEmail                  string   `json:"app.from_email"`
	AppNotifyEmails               []string `json:"app.notify_emails"`
	EnablePublicSubPage           bool     `json:"app.enable_public_subscription_page"`
	EnablePublicArchive           bool     `json:"app.enable_public_archive"`
	EnablePublicArchiveRSSContent bool     `json:"app.enable_public_archive_rss_content"`
	SendOptinConfirmation         bool     `json:"app.send_optin_confirmation"`
	CheckUpdates                  bool     `json:"app.check_updates"`
	AppLang                       string   `json:"app.lang"`

	ReplyEnabled bool   `json:"reply.enabled"`
	ReplyDomain  string `json:"reply.domain"`

	AppBatchSize             int    `json:"app.batch_size"`
	AppConcurrency           int    `json:"app.concurrency"`
	AppMaxSendErrors         int    `json:"app.max_send_errors"`
	AppMessageRate           int    `json:"app.message_rate"`
	CacheSlowQueries         bool   `json:"app.cache_slow_queries"`
	CacheSlowQueriesInterval string `json:"app.cache_slow_queries_interval"`

	AppMessageSlidingWindow         bool   `json:"app.message_sliding_window"`
	AppMessageSlidingWindowDuration string `json:"app.message_sliding_window_duration"`
	AppMessageSlidingWindowRate     int    `json:"app.message_sliding_window_rate"`

	PrivacyIndividualTracking bool     `json:"privacy.individual_tracking"`
	PrivacyUnsubHeader        bool     `json:"privacy.unsubscribe_header"`
	PrivacyAllowBlocklist     bool     `json:"privacy.allow_blocklist"`
	PrivacyAllowPreferences   bool     `json:"privacy.allow_preferences"`
	PrivacyAllowExport        bool     `json:"privacy.allow_export"`
	PrivacyAllowWipe          bool     `json:"privacy.allow_wipe"`
	PrivacyExportable         []string `json:"privacy.exportable"`
	PrivacyRecordOptinIP      bool     `json:"privacy.record_optin_ip"`
	DomainBlocklist           []string `json:"privacy.domain_blocklist"`
	DomainAllowlist           []string `json:"privacy.domain_allowlist"`

	SecurityCaptcha struct {
		Altcha struct {
			Enabled    bool `json:"enabled"`
			Complexity int  `json:"complexity"`
		} `json:"altcha"`
		HCaptcha struct {
			Enabled bool   `json:"enabled"`
			Key     string `json:"key"`
			Secret  string `json:"secret"`
		} `json:"hcaptcha"`
	} `json:"security.captcha"`

	OIDC struct {
		Enabled           bool     `json:"enabled"`
		ProviderURL       string   `json:"provider_url"`
		ProviderName      string   `json:"provider_name"`
		ClientID          string   `json:"client_id"`
		ClientSecret      string   `json:"client_secret"`
		AutoCreateUsers   bool     `json:"auto_create_users"`
		DefaultUserRoleID null.Int `json:"default_user_role_id"`
		DefaultListRoleID null.Int `json:"default_list_role_id"`
	} `json:"security.oidc"`

	UploadProvider             string   `json:"upload.provider"`
	UploadExtensions           []string `json:"upload.extensions"`
	UploadFilesystemUploadPath string   `json:"upload.filesystem.upload_path"`
	UploadFilesystemUploadURI  string   `json:"upload.filesystem.upload_uri"`
	UploadS3URL                string   `json:"upload.s3.url"`
	UploadS3PublicURL          string   `json:"upload.s3.public_url"`
	UploadS3AwsAccessKeyID     string   `json:"upload.s3.aws_access_key_id"`
	UploadS3AwsDefaultRegion   string   `json:"upload.s3.aws_default_region"`
	UploadS3AwsSecretAccessKey string   `json:"upload.s3.aws_secret_access_key,omitempty"`
	UploadS3Bucket             string   `json:"upload.s3.bucket"`
	UploadS3BucketDomain       string   `json:"upload.s3.bucket_domain"`
	UploadS3BucketPath         string   `json:"upload.s3.bucket_path"`
	UploadS3BucketType         string   `json:"upload.s3.bucket_type"`
	UploadS3Expiry             string   `json:"upload.s3.expiry"`

	SMTP []struct {
		Name          string              `json:"name"`
		UUID          string              `json:"uuid"`
		Enabled       bool                `json:"enabled"`
		Host          string              `json:"host"`
		HelloHostname string              `json:"hello_hostname"`
		Port          int                 `json:"port"`
		AuthProtocol  string              `json:"auth_protocol"`
		Username      string              `json:"username"`
		Password      string              `json:"password,omitempty"`
		EmailHeaders  []map[string]string `json:"email_headers"`
		MaxConns      int                 `json:"max_conns"`
		MaxMsgRetries int                 `json:"max_msg_retries"`
		IdleTimeout   string              `json:"idle_timeout"`
		WaitTimeout   string              `json:"wait_timeout"`
		TLSType       string              `json:"tls_type"`
		TLSSkipVerify bool                `json:"tls_skip_verify"`
		DailyLimit    int                 `json:"daily_limit"`
		SendPause     string              `json:"send_pause"`

		// WarmCloud API settings
		WarmCloudEnabled      bool   `json:"warmcloud_enabled"`
		WarmCloudToken        string `json:"warmcloud_token,omitempty"`
		WarmCloudUserID       int    `json:"warmcloud_user_id"`
		WarmCloudRefID        string `json:"warmcloud_ref_id"`
		WarmCloudBig          string `json:"warmcloud_big"`
		WarmCloudDailyLimit   int    `json:"warmcloud_daily_limit"`
		WarmCloudDailyStep    int    `json:"warmcloud_daily_step"`
		WarmCloudCampaignID   int    `json:"warmcloud_campaign_id"`
		WarmCloudReadPercent  int    `json:"warmcloud_read_percent"`
		WarmCloudMaxPerTick   int    `json:"warmcloud_max_per_tick"`
		WarmCloudSleepBetween string `json:"warmcloud_sleep_between"`
		// Rate limiting: send N emails per M minutes (jitter optional)
		WarmCloudRateCount         int `json:"warmcloud_rate_count"`
		WarmCloudRateWindowMinutes int `json:"warmcloud_rate_window_minutes"`
		WarmCloudRateJitterPct     int `json:"warmcloud_rate_jitter_pct"`
		// Per-server working hours override
		WarmCloudWorkingHoursEnabled bool   `json:"warmcloud_working_hours_enabled"`
		WarmCloudWorkingHours        string `json:"warmcloud_working_hours"`
	} `json:"smtp"`

	Messengers []struct {
		UUID          string `json:"uuid"`
		Enabled       bool   `json:"enabled"`
		Name          string `json:"name"`
		RootURL       string `json:"root_url"`
		Username      string `json:"username"`
		Password      string `json:"password,omitempty"`
		MaxConns      int    `json:"max_conns"`
		Timeout       string `json:"timeout"`
		MaxMsgRetries int    `json:"max_msg_retries"`
	} `json:"messengers"`

	BounceEnabled        bool `json:"bounce.enabled"`
	BounceEnableWebhooks bool `json:"bounce.webhooks_enabled"`
	BounceActions        map[string]struct {
		Count  int    `json:"count"`
		Action string `json:"action"`
	} `json:"bounce.actions"`
	SESEnabled      bool   `json:"bounce.ses_enabled"`
	SendgridEnabled bool   `json:"bounce.sendgrid_enabled"`
	SendgridKey     string `json:"bounce.sendgrid_key"`
	BouncePostmark  struct {
		Enabled  bool   `json:"enabled"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"bounce.postmark"`
	BounceForwardEmail struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key"`
	} `json:"bounce.forwardemail"`

	BouncePowermta struct {
		Enabled bool   `json:"enabled"`
		Key     string `json:"key"`
	} `json:"bounce.powermta"`
	BounceBoxes []struct {
		UUID          string `json:"uuid"`
		Enabled       bool   `json:"enabled"`
		Type          string `json:"type"`
		Host          string `json:"host"`
		Port          int    `json:"port"`
		AuthProtocol  string `json:"auth_protocol"`
		ReturnPath    string `json:"return_path"`
		Username      string `json:"username"`
		Password      string `json:"password,omitempty"`
		TLSEnabled    bool   `json:"tls_enabled"`
		TLSSkipVerify bool   `json:"tls_skip_verify"`
		ScanInterval  string `json:"scan_interval"`
	} `json:"bounce.mailboxes"`

	AdminCustomCSS  string `json:"appearance.admin.custom_css"`
	AdminCustomJS   string `json:"appearance.admin.custom_js"`
	PublicCustomCSS string `json:"appearance.public.custom_css"`
	PublicCustomJS  string `json:"appearance.public.custom_js"`

	// Global WarmCloud settings
	WarmCloudGlobalEnabled           bool   `json:"warmcloud.global_enabled"`
	WarmCloudGlobalToken             string `json:"warmcloud.global_token,omitempty"`
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

	// Personas (aka identities) grouped by domain.
	Personas []Persona `json:"personas"`
}

// Persona represents a domain-scoped identity with sender accounts and a
// persona-level suppression list.
type Persona struct {
	// Domain that defines the persona, eg: example.com
	Domain string `json:"domain"`
	// Optional display name/label for the persona.
	Label string `json:"label"`
	// SMTP servers (settings.smtp[i].name) this persona can use.
	SMTP []string `json:"smtp"`
	// Accounts (local parts) with optional IMAP tracking configuration.
	Accounts PersonaAccounts `json:"accounts"`
	// Suppressions are full email addresses to never send to when using this persona/domain.
	Suppressions []string `json:"suppressions"`
}

// PersonaAccount represents a single sender local-part and optional IMAP tracking config.
type PersonaAccount struct {
	// Local part before @domain (eg: noreply)
	Name string `json:"name"`

	// IMAP tracking configuration
	IMAPEnabled       bool   `json:"imap_enabled"`
	IMAPHost          string `json:"imap_host"`
	IMAPPort          int    `json:"imap_port"`
	IMAPUsername      string `json:"imap_username"`
	IMAPPassword      string `json:"imap_password,omitempty"`
	IMAPTLS           bool   `json:"imap_tls"`
	IMAPFolder        string `json:"imap_folder"`
	IMAPCheckInterval string `json:"imap_check_interval"`

	// POP3 reply tracking configuration (receive replies on sender mailbox)
	POPEnabled       bool   `json:"pop_enabled"`
	POPHost          string `json:"pop_host"`
	POPPort          int    `json:"pop_port"`
	POPUsername      string `json:"pop_username"`
	POPPassword      string `json:"pop_password,omitempty"`
	POPTLSEnabled    bool   `json:"pop_tls_enabled"`
	POPTLSSkipVerify bool   `json:"pop_tls_skip_verify"`
	POPCheckInterval string `json:"pop_check_interval"`
}

// PersonaAccounts is a helper type that accepts either an array of strings
// (legacy format) or an array of PersonaAccount objects.
type PersonaAccounts []PersonaAccount

func (pa *PersonaAccounts) UnmarshalJSON(b []byte) error {
	// Try the new format first: []PersonaAccount
	var objs []PersonaAccount
	if err := json.Unmarshal(b, &objs); err == nil {
		*pa = objs
		return nil
	}

	// Fallback to legacy []string
	var strs []string
	if err := json.Unmarshal(b, &strs); err == nil {
		out := make([]PersonaAccount, 0, len(strs))
		for _, s := range strs {
			name := strings.ToLower(strings.TrimSpace(s))
			if name == "" {
				continue
			}
			out = append(out, PersonaAccount{Name: name})
		}
		*pa = out
		return nil
	}

	// Single string?
	var single string
	if err := json.Unmarshal(b, &single); err == nil {
		name := strings.ToLower(strings.TrimSpace(single))
		if name == "" {
			*pa = []PersonaAccount{}
		} else {
			*pa = []PersonaAccount{{Name: name}}
		}
		return nil
	}

	return fmt.Errorf("invalid persona accounts format")
}

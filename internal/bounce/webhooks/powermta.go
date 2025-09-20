package webhooks

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "strings"
    "time"

    "github.com/knadh/listmonk/models"
)

// Powermta handles webhook notifications (mainly bounce notifications) from PowerMTA
// or any shipper that emits a compatible JSON.
type Powermta struct {
    hmacKey []byte
}

func NewPowermta(key []byte) *Powermta { return &Powermta{hmacKey: key} }

// powermtaNotif is a flexible structure to unmarshal common PowerMTA accounting
// webhook payloads. Field names vary by setups; we accept several aliases.
type powermtaNotif struct {
    // Common identifiers
    Recipient string            `json:"recipient"`
    Rcpt      string            `json:"rcpt"`
    Email     string            `json:"email"`

    // Event typing
    Event         string `json:"event"`           // eg: "bounce", "fbl", "reject"
    BounceCat     string `json:"bounceCat"`       // eg: "hard", "soft", vendor specific
    BounceCategory string `json:"bounce_category"`
    Type          string `json:"type"`            // optional

    // Timestamps
    Time        any    `json:"time"`         // unix int or RFC3339 string
    Timestamp   any    `json:"timestamp"`    // alias
    BouncedAt   any    `json:"bounced_at"`   // alias

    // Headers captured by PMTA accounting
    Headers map[string]string `json:"headers"`

    // Misc
    Reason   string `json:"reason"`
    Response string `json:"response"`
    Diag     string `json:"diag"`
}

// AccountingEvent represents a normalized delivery accounting event (non-bounce)
// eg: sent, queued/deferred. These are derived from PMTA accounting events.
type AccountingEvent struct {
    Email   string
    Subject string
    Status  string // sent|queued
}

// ProcessAccounting parses generic PowerMTA accounting webhook payloads and returns
// two outputs: bounces (as models.Bounce) and non-bounce accounting events (sent/queued).
// Optional HMAC signature is validated if configured.
func (p *Powermta) ProcessAccounting(sigHex string, token string, body []byte) ([]models.Bounce, []AccountingEvent, error) {
    // If a key is configured, require either a valid HMAC signature or a matching static token.
    if len(p.hmacKey) > 0 {
        // Prefer HMAC if provided.
        if sigHex != "" {
            sig, err := hex.DecodeString(sigHex)
            if err != nil {
                return nil, nil, fmt.Errorf("invalid signature encoding: %v", err)
            }
            mac := hmac.New(sha256.New, p.hmacKey)
            mac.Write(body)
            if !hmac.Equal(mac.Sum(nil), sig) {
                return nil, nil, errors.New("invalid signature")
            }
        } else if token != "" {
            // Static token auth: token must match the configured key exactly.
            if token != string(p.hmacKey) {
                return nil, nil, errors.New("invalid token")
            }
        } else {
            return nil, nil, errors.New("missing signature")
        }
    }

    // Parse array or single
    var arr []powermtaNotif
    if err := json.Unmarshal(body, &arr); err != nil {
        var single powermtaNotif
        if err2 := json.Unmarshal(body, &single); err2 != nil {
            return nil, nil, fmt.Errorf("invalid payload")
        }
        arr = []powermtaNotif{single}
    }

    // Build case-insensitive header getter
    getHeader := func(h map[string]string, key string) string {
        if len(h) == 0 { return "" }
        lk := strings.ToLower(key)
        for k, v := range h {
            if strings.ToLower(k) == lk { return v }
        }
        return ""
    }

    bcs := []models.Bounce{}
    evs := []AccountingEvent{}
    for _, n := range arr {
        email := firstNonEmpty(strings.ToLower(n.Email), strings.ToLower(n.Recipient), strings.ToLower(n.Rcpt))
        if email == "" { continue }

        event := strings.ToLower(n.Event)
        cat := strings.ToLower(firstNonEmpty(n.BounceCategory, n.BounceCat, n.Type))

        // Heuristics: treat complaints/bounce as bounce, delivered/sent as sent, deferred/queued as queued.
        isComplaint := event == "fbl" || event == "feedback" || strings.Contains(cat, "complaint")
        isBounce := strings.Contains(event, "bounce") || strings.Contains(cat, "bounce")
        isDelivered := event == "delivered" || event == "sent" || strings.Contains(cat, "delivered")
        isQueued := strings.Contains(event, "defer") || strings.Contains(event, "queue") || strings.Contains(cat, "defer") || strings.Contains(cat, "queue")

        if isComplaint || isBounce {
            // Reuse bounce conversion
            bs, err := p.toBounces([]powermtaNotif{n}, body)
            if err != nil { return nil, nil, err }
            bcs = append(bcs, bs...)
            continue
        }

        status := ""
        switch {
        case isDelivered:
            status = "sent"
        case isQueued:
            status = "queued"
        default:
            // Unknown; ignore silently
            continue
        }
        subj := getHeader(n.Headers, "Subject")
        evs = append(evs, AccountingEvent{Email: email, Subject: subj, Status: status})
    }
    return bcs, evs, nil
}

// ProcessBounce validates optional HMAC signature and converts one-or-more
// incoming notifications to listmonk Bounce objects.
func (p *Powermta) ProcessBounce(sigHex string, token string, body []byte) ([]models.Bounce, error) {
    // If a key is configured, require either a valid HMAC signature or a matching static token.
    if len(p.hmacKey) > 0 {
        if sigHex != "" {
            sig, err := hex.DecodeString(sigHex)
            if err != nil {
                return nil, fmt.Errorf("invalid signature encoding: %v", err)
            }
            mac := hmac.New(sha256.New, p.hmacKey)
            mac.Write(body)
            if !hmac.Equal(mac.Sum(nil), sig) {
                return nil, errors.New("invalid signature")
            }
        } else if token != "" {
            if token != string(p.hmacKey) {
                return nil, errors.New("invalid token")
            }
        } else {
            return nil, errors.New("missing signature")
        }
    }

    // Try to unmarshal as an array first.
    var arr []powermtaNotif
    if err := json.Unmarshal(body, &arr); err == nil {
        return p.toBounces(arr, body)
    }

    // Try as a single object.
    var single powermtaNotif
    if err := json.Unmarshal(body, &single); err == nil {
        return p.toBounces([]powermtaNotif{single}, body)
    }

    return nil, fmt.Errorf("invalid payload")
}

func (p *Powermta) toBounces(in []powermtaNotif, raw []byte) ([]models.Bounce, error) {
    out := make([]models.Bounce, 0, len(in))

    for _, n := range in {
        email := firstNonEmpty(strings.ToLower(n.Email), strings.ToLower(n.Recipient), strings.ToLower(n.Rcpt))
        if email == "" {
            // Skip if we can't determine recipient.
            continue
        }

        typ := models.BounceTypeHard
        event := strings.ToLower(n.Event)
        cat := strings.ToLower(firstNonEmpty(n.BounceCategory, n.BounceCat, n.Type))

        switch {
        case event == "fbl" || event == "feedback" || strings.Contains(cat, "complaint"):
            typ = models.BounceTypeComplaint
        case strings.Contains(cat, "soft") || strings.Contains(cat, "temp") || strings.Contains(cat, "defer"):
            typ = models.BounceTypeSoft
        default:
            // hard by default
        }

        // Extract X-Listmonk headers if present.
        var campUUID, subUUID string
        if len(n.Headers) > 0 {
            // Make a case-insensitive lookup
            m := map[string]string{}
            for k, v := range n.Headers {
                m[strings.ToLower(k)] = v
            }
            if v, ok := m[strings.ToLower(models.EmailHeaderCampaignUUID)]; ok {
                campUUID = v
            }
            if v, ok := m[strings.ToLower(models.EmailHeaderSubscriberUUID)]; ok {
                subUUID = v
            }
        }

        t := time.Now()
        if tt, ok := toTime(n.Time); ok {
            t = tt
        } else if tt, ok := toTime(n.Timestamp); ok {
            t = tt
        } else if tt, ok := toTime(n.BouncedAt); ok {
            t = tt
        }

        out = append(out, models.Bounce{
            Email:          email,
            CampaignUUID:   campUUID,
            SubscriberUUID: subUUID,
            Type:           typ,
            Source:         "powermta",
            Meta:           json.RawMessage(raw),
            CreatedAt:      t,
        })
    }

    return out, nil
}

func firstNonEmpty(vals ...string) string {
    for _, v := range vals {
        if strings.TrimSpace(v) != "" {
            return v
        }
    }
    return ""
}

func toTime(v any) (time.Time, bool) {
    switch t := v.(type) {
    case nil:
        return time.Time{}, false
    case float64:
        // JSON numbers are float64
        if t == 0 {
            return time.Time{}, false
        }
        return time.Unix(int64(t), 0), true
    case int64:
        return time.Unix(t, 0), true
    case int:
        return time.Unix(int64(t), 0), true
    case string:
        if ts, err := time.Parse(time.RFC3339, t); err == nil {
            return ts, true
        }
        // Try unix seconds as string
        if sec, err := time.ParseDuration(t + "s"); err == nil {
            return time.Unix(int64(sec.Seconds()), 0), true
        }
        return time.Time{}, false
    default:
        return time.Time{}, false
    }
}

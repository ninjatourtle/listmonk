package email

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    neturl "net/url"
    "strings"
    "time"
)

// WarmCloudClient represents a client for WarmCloud API
type WarmCloudClient struct {
    BaseURL string
    Token   string
    UserID  int
    RefID   string
    // optional HTTP client override (for timeouts/testing)
    http    *http.Client
}

// WarmCloudResponse represents a generic response from WarmCloud API
type WarmCloudResponse struct {
    Status  string      `json:"status"`
    Balance float64     `json:"balance,omitempty"`
    Emails  []WarmCloudGetEmail `json:"emails,omitempty"`
    Error   string      `json:"error,omitempty"`
    Detail  string      `json:"detail,omitempty"`
    Message string      `json:"message,omitempty"`
    // HTTPStatus is not from JSON; filled by client.request for status checks (eg: 403 handling).
    HTTPStatus int      `json:"-"`
}

// WarmCloudEmail represents an email for submission
type WarmCloudEmail struct {
    Email   string `json:"email"`
    Subject string `json:"subject"`
    SubmitToken string `json:"submit_token"`
}

// NewWarmCloudClient creates a new WarmCloud client
func NewWarmCloudClient(token string, userID int, refID string) *WarmCloudClient {
    return &WarmCloudClient{
        // Default WarmCloud API endpoint. Some deployments expose HTTPS on 443 and HTTP on 8000.
        BaseURL: "https://api.warmcloud.org:8000",
        Token:   token,
        UserID:  userID,
        RefID:   refID,
    }
}

// WithHTTPClient sets a custom HTTP client.
func (c *WarmCloudClient) WithHTTPClient(hc *http.Client) *WarmCloudClient {
    c.http = hc
    return c
}

// GetBalance gets the current balance
func (c *WarmCloudClient) GetBalance() (float64, error) {
    reqBody := map[string]interface{}{
        "token":   c.Token,
        "user_id": c.UserID,
    }

    resp, _, err := c.request("POST", "/get_balance", reqBody, 3)
    if err != nil {
        return 0, err
    }

    if resp.Status != "good" {
        msg := resp.Error
        if msg == "" { msg = resp.Detail }
        if msg == "" { msg = resp.Message }
        if msg == "" { msg = "unknown error" }
        return 0, fmt.Errorf("API error: %s", msg)
    }

	return resp.Balance, nil
}

// WarmCloudGetEmail represents an email returned by /get
type WarmCloudGetEmail struct {
    Email       string `json:"email"`
    SubmitToken string `json:"submit_token"`
}

// GetEmails gets email addresses for processing. 'big' is required ('m' or 'g').
func (c *WarmCloudClient) GetEmails(count int, big string) ([]WarmCloudGetEmail, error) {
    reqBody := map[string]interface{}{
        "token":   c.Token,
        "user_id": c.UserID,
        "ref_id":  c.RefID,
        "count":   count,
        "big":     big,
    }

    resp, code, err := c.request("POST", "/get", reqBody, 3)
    if err != nil {
        return nil, err
    }

    if resp.Status != "good" {
        msg := resp.Error
        if msg == "" { msg = resp.Detail }
        if msg == "" { msg = resp.Message }
        if msg == "" { msg = "unknown error" }
        // If 403 and rotation exhausted, advise caller to clear rotation.
        if code == http.StatusForbidden && strings.Contains(strings.ToLower(msg), "not enough unique emails") {
            return nil, fmt.Errorf("rotation_exhausted: %s", msg)
        }
        return nil, fmt.Errorf("API error: %s", msg)
    }

    return resp.Emails, nil
}

// SubmitEmails submits processed emails
func (c *WarmCloudClient) SubmitEmails(emails []WarmCloudEmail) error {
    return c.SubmitEmailsWithBig(emails, "")
}

// SubmitEmailsWithBig submits processed emails and optionally includes 'big' if provided.
func (c *WarmCloudClient) SubmitEmailsWithBig(emails []WarmCloudEmail, big string) error {
    reqBody := map[string]interface{}{
        "token":   c.Token,
        "user_id": c.UserID,
        "emails":  emails,
    }
    if strings.TrimSpace(big) != "" {
        reqBody["big"] = big
    }

    // Debug log (no secrets)
    if len(emails) > 0 {
        subjLen := len(emails[0].Subject)
        tokLen := len(emails[0].SubmitToken)
        elogf("WarmCloud: submit payload emails=%d first.subject_len=%d first.token_len=%d", len(emails), subjLen, tokLen)
    } else {
        elogf("WarmCloud: submit payload emails=0")
    }

    resp, _, err := c.request("POST", "/submit", reqBody, 3)
    if err != nil {
        return err
    }

    if resp.Status != "good" {
        msg := resp.Error
        if msg == "" { msg = resp.Detail }
        if msg == "" { msg = resp.Message }
        if msg == "" { msg = "unknown error" }
        if resp.HTTPStatus != 0 {
            return fmt.Errorf("API error (%d): %s", resp.HTTPStatus, msg)
        }
        return fmt.Errorf("API error: %s", msg)
    }

    return nil
}

// ClearRotation clears the individual email rotation list (use after 403 on /get)
func (c *WarmCloudClient) ClearRotation() error {
    reqBody := map[string]interface{}{
        "token":   c.Token,
        "user_id": c.UserID,
    }
    resp, _, err := c.request("POST", "/clear_rotation", reqBody, 3)
    if err != nil {
        return err
    }
    if resp.Status != "good" {
        msg := resp.Error
        if msg == "" { msg = resp.Detail }
        if msg == "" { msg = resp.Message }
        if msg == "" { msg = "unknown error" }
        return fmt.Errorf("API error: %s", msg)
    }
    return nil
}

// HealthCheck verifies API connectivity/credentials quickly.
func (c *WarmCloudClient) HealthCheck() error {
    _, err := c.GetBalance()
    return err
}

// request makes an HTTP request to WarmCloud API with simple retries on transient failures.
func (c *WarmCloudClient) request(method, endpoint string, body interface{}, retries int) (*WarmCloudResponse, int, error) {
    jsonBody, err := json.Marshal(body)
    if err != nil {
        return nil, 0, err
    }

    var lastErr error
    backoff := 500 * time.Millisecond
    if c.http == nil {
        c.http = &http.Client{Timeout: 30 * time.Second}
    }
    // Build a list of base URL candidates to try in order.
    baseCandidates := []string{c.BaseURL}
    if u, err := neturl.Parse(c.BaseURL); err == nil {
        host := u.Host
        // If host contains :8000 and scheme is https, add http://host as a fallback.
        if u.Scheme == "https" && strings.HasSuffix(host, ":8000") {
            baseCandidates = append(baseCandidates, "http://"+host)
            // Also try HTTPS on default 443 without the :8000 suffix.
            hostNoPort := strings.TrimSuffix(host, ":8000")
            baseCandidates = append(baseCandidates, "https://"+hostNoPort)
        }
    }

    // Deduplicate candidates while preserving order.
    seen := map[string]struct{}{}
    uniq := make([]string, 0, len(baseCandidates))
    for _, b := range baseCandidates {
        if _, ok := seen[b]; ok { continue }
        seen[b] = struct{}{}
        uniq = append(uniq, b)
    }

    for _, base := range uniq {
        for attempt := 0; attempt < retries; attempt++ {
            req, err := http.NewRequest(method, base+endpoint, bytes.NewBuffer(jsonBody))
            if err != nil {
                return nil, 0, err
            }
            req.Header.Set("Content-Type", "application/json")

            resp, err := c.http.Do(req)
            if err != nil {
                lastErr = err
            } else {
                defer resp.Body.Close()
                var warmCloudResp WarmCloudResponse
                if err := json.NewDecoder(resp.Body).Decode(&warmCloudResp); err != nil {
                    lastErr = err
                } else {
                    // treat 5xx or explicit error fields as retryable once; otherwise return
                    if resp.StatusCode >= 500 {
                        lastErr = fmt.Errorf("warmcloud http %d", resp.StatusCode)
                    } else {
                        // Successful response: update client's BaseURL to the working one
                        c.BaseURL = base
                        warmCloudResp.HTTPStatus = resp.StatusCode
                        return &warmCloudResp, resp.StatusCode, nil
                    }
                }
            }
            time.Sleep(backoff)
            backoff *= 2
            if backoff > 5*time.Second { backoff = 5 * time.Second }
        }
        // Reset backoff for next base candidate
        backoff = 500 * time.Millisecond
    }
    return nil, 0, lastErr
}

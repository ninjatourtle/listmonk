package mailbox

import (
    "encoding/json"
    "io"
    "regexp"
    "strings"
    "time"

    "github.com/emersion/go-message"
    _ "github.com/emersion/go-message/charset"
    "github.com/knadh/go-pop3"
    "github.com/knadh/listmonk/models"
)

// POP represents a POP mailbox for replies.
type POP struct {
    opt    Opt
    client *pop3.Client
}

// Opt represents POP3 options.
type Opt struct {
    Host          string
    Port          int
    Username      string
    Password      string
    TLSEnabled    bool
    TLSSkipVerify bool
    ScanInterval  time.Duration
}

func NewPOP(opt Opt) *POP {
    return &POP{
        opt: opt,
        client: pop3.New(pop3.Opt{
            Host:          opt.Host,
            Port:          opt.Port,
            TLSEnabled:    opt.TLSEnabled,
            TLSSkipVerify: opt.TLSSkipVerify,
        }),
    }
}

var (
    reMIDAngle = regexp.MustCompile(`^<([^>]+)>$`)
)

// Scan scans the mailbox and pushes parsed reply events into ch.
func (p *POP) Scan(limit int, ch chan models.Reply) error {
    c, err := p.client.NewConn()
    if err != nil {
        return err
    }
    defer c.Quit()

    if err := c.Auth(p.opt.Username, p.opt.Password); err != nil {
        return err
    }

    count, _, err := c.Stat()
    if err != nil {
        return err
    }
    if count == 0 {
        return nil
    }
    if limit > 0 && count > limit {
        count = limit
    }

    for id := 1; id <= count; id++ {
        // Retrieve raw message
        b, err := c.RetrRaw(id)
        if err != nil {
            return err
        }

        m, err := message.Read(b)
        if err != nil {
            return err
        }

        // Extract headers
        from := strings.TrimSpace(m.Header.Get("From"))
        subj := strings.TrimSpace(m.Header.Get("Subject"))
        inReplyTo := strings.TrimSpace(m.Header.Get("In-Reply-To"))
        if x := reMIDAngle.FindStringSubmatch(inReplyTo); len(x) == 2 {
            inReplyTo = x[1]
        }
        if inReplyTo == "" {
            refs := strings.TrimSpace(m.Header.Get("References"))
            if refs != "" {
                parts := strings.Fields(refs)
                if n := len(parts); n > 0 {
                    cand := parts[n-1]
                    if x := reMIDAngle.FindStringSubmatch(cand); len(x) == 2 {
                        inReplyTo = x[1]
                    } else {
                        inReplyTo = cand
                    }
                }
            }
        }
        msgID := strings.TrimSpace(m.Header.Get("Message-Id"))
        if x := reMIDAngle.FindStringSubmatch(msgID); len(x) == 2 {
            msgID = x[1]
        }

        // Snippet: first text/plain part or whole body if single-part
        snippet := ""
        if mr := m.MultipartReader(); mr != nil {
            for {
                part, err := mr.NextPart()
                if err == io.EOF {
                    break
                } else if err != nil {
                    break
                }
                ct := strings.ToLower(part.Header.Get("Content-Type"))
                if strings.HasPrefix(ct, "text/plain") {
                    bb, _ := io.ReadAll(part.Body)
                    snippet = makeSnippet(string(bb))
                    break
                }
            }
        } else {
            bb, _ := io.ReadAll(m.Body)
            snippet = makeSnippet(string(bb))
        }

        // Marshal raw headers map
        hdrJSON, _ := json.Marshal(m.Header.Map())

        select {
        case ch <- models.Reply{
            Email:     from,
            Subject:   subj,
            Snippet:   snippet,
            ReplyToken: "",
            MessageID: msgID,
            InReplyTo: inReplyTo,
            Headers:   hdrJSON,
            CreatedAt: time.Now(),
        }:
        default:
        }

        // Delete after processing to avoid duplicates
        _ = c.Dele(id)
    }
    return nil
}

func makeSnippet(s string) string {
    s = strings.TrimSpace(s)
    if len(s) > 300 {
        s = s[:300]
    }
    return s
}



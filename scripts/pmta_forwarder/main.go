package main

import (
    "bufio"
    "bytes"
    "crypto/tls"
    "encoding/json"
    "encoding/csv"
    "flag"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
)

// A minimal tailer that can follow a file and handle simple log rotations.
type tailer struct {
    path   string
    f      *os.File
    r      *bufio.Reader
    offset int64
}

func newTailer(path string, fromBeginning bool) (*tailer, error) {
    t := &tailer{path: path}
    if err := t.open(fromBeginning); err != nil {
        return nil, err
    }
    return t, nil
}

func (t *tailer) open(fromBeginning bool) error {
    if t.f != nil {
        t.f.Close()
    }
    f, err := os.Open(t.path)
    if err != nil {
        return err
    }
    t.f = f
    if !fromBeginning {
        if off, err := f.Seek(0, io.SeekEnd); err == nil {
            t.offset = off
        }
    } else {
        t.offset = 0
    }
    t.r = bufio.NewReader(f)
    return nil
}

func (t *tailer) readLine() (string, error) {
    for {
        line, err := t.r.ReadString('\n')
        if err == nil {
            t.offset += int64(len(line))
            return strings.TrimRight(line, "\r\n"), nil
        }
        if err == io.EOF {
            // Check for rotation: if current file size < offset, reopen from beginning.
            if fi, statErr := t.f.Stat(); statErr == nil {
                if fi.Size() < t.offset {
                    // rotated
                    _ = t.open(true)
                }
            }
            time.Sleep(500 * time.Millisecond)
            continue
        }
        return "", err
    }
}

// normalize a PMTA accounting JSON into listmonk's expected keys.
func normalize(m map[string]any) map[string]any {
    out := map[string]any{}

    // recipient
    if v, ok := firstString(m, "recipient", "rcpt", "rcpt-to", "email"); ok {
        out["rcpt"] = strings.ToLower(v)
    }

    // headers: expect object under "headers" or "msg-headers"
    if hv, ok := m["headers"].(map[string]any); ok {
        out["headers"] = toStringMap(hv)
    } else if hv, ok := m["msg-headers"].(map[string]any); ok {
        out["headers"] = toStringMap(hv)
    }

    // subject convenience if headers exist
    if h, ok := out["headers"].(map[string]string); ok {
        if _, ok := h["Subject"]; !ok {
            if s, ok := firstString(m, "subject", "msg-subject"); ok {
                h["Subject"] = s
                out["headers"] = h
            }
        }
    }

    // timestamp
    if v, ok := firstAny(m, "timestamp", "time", "ts"); ok {
        out["timestamp"] = v
    }

    // response / diag
    if v, ok := firstString(m, "response", "diag", "dsn", "smtp-response"); ok {
        out["response"] = v
    }

    // Map PMTA event codes to generic events.
    // d = delivered, t = deferred, r/rb = hard bounce, f = complaint
    if ev, ok := firstString(m, "event", "type", "evt"); ok {
        evl := strings.ToLower(ev)
        switch evl {
        case "d", "deliver", "delivered", "sent":
            out["event"] = "delivered"
        case "t", "defer", "deferred", "queued":
            out["event"] = "queued"
        case "r", "rb", "bounce", "hard-bounce":
            out["event"] = "bounce"
            out["bounceCat"] = "hard"
        case "f", "complaint", "fbl":
            out["event"] = "fbl"
        default:
            // Unknown; pass through.
            out["event"] = evl
        }
    }

    // Some logs may explicitly indicate temp/soft status.
    if bc, ok := firstString(m, "bounceCat", "bounce_category", "category"); ok {
        out["bounceCat"] = bc
    } else if ev, _ := out["event"].(string); ev == "queued" {
        out["bounceCat"] = "soft"
    }

    return out
}

func firstString(m map[string]any, keys ...string) (string, bool) {
    for _, k := range keys {
        if v, ok := m[k]; ok {
            switch t := v.(type) {
            case string:
                if strings.TrimSpace(t) != "" {
                    return t, true
                }
            }
        }
    }
    return "", false
}

func firstAny(m map[string]any, keys ...string) (any, bool) {
    for _, k := range keys {
        if v, ok := m[k]; ok {
            return v, true
        }
    }
    return nil, false
}

func toStringMap(in map[string]any) map[string]string {
    out := map[string]string{}
    for k, v := range in {
        switch t := v.(type) {
        case string:
            out[k] = t
        case []any:
            // Join multi-value headers.
            var parts []string
            for _, p := range t {
                if s, ok := p.(string); ok {
                    parts = append(parts, s)
                }
            }
            out[k] = strings.Join(parts, "; ")
        default:
            b, _ := json.Marshal(t)
            out[k] = string(b)
        }
    }
    return out
}

func main() {
    var (
        file     = flag.String("file", "/var/log/pmta/accounting.json", "Path to PMTA accounting JSON file")
        url      = flag.String("url", "", "Listmonk webhook URL")
        token    = flag.String("token", "", "Static token to send as X-PMTA-Token header")
        insecure = flag.Bool("insecure", false, "Skip TLS verification")
        batch    = flag.Int("batch", 100, "Batch size")
        flush    = flag.Duration("flush", 2*time.Second, "Flush interval")
        fromBeg  = flag.Bool("from-begin", false, "Start reading from beginning of file")
        format   = flag.String("format", "json", "Input format: json or csv")
        csvDelim = flag.String("csv-delim", ",", "CSV delimiter (single character)")
        csvHeader = flag.String("csv-header", "", "Explicit CSV header (comma separated). If empty, first line is used as header.")
    )
    flag.Parse()

    if *url == "" {
        fmt.Fprintln(os.Stderr, "-url is required")
        os.Exit(2)
    }

    t, err := newTailer(*file, *fromBeg)
    if err != nil {
        fmt.Fprintf(os.Stderr, "error opening file: %v\n", err)
        os.Exit(1)
    }

    tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure}}
    cli := &http.Client{Transport: tr, Timeout: 15 * time.Second}

    var (
        mu    sync.Mutex
        buf   []map[string]any
        tick  = time.NewTicker(*flush)
        flushFn = func() {
            mu.Lock()
            b := buf
            buf = nil
            mu.Unlock()
            if len(b) == 0 {
                return
            }
            payload, _ := json.Marshal(b)
            req, _ := http.NewRequest("POST", *url, bytes.NewReader(payload))
            req.Header.Set("Content-Type", "application/json")
            if *token != "" {
                req.Header.Set("X-PMTA-Token", *token)
            }
            resp, err := cli.Do(req)
            if err != nil {
                fmt.Fprintf(os.Stderr, "post error: %v\n", err)
                return
            }
            io.Copy(io.Discard, resp.Body)
            resp.Body.Close()
            if resp.StatusCode >= 300 {
                fmt.Fprintf(os.Stderr, "post error: http %d\n", resp.StatusCode)
            }
        }
    )

    go func() {
        for range tick.C {
            flushFn()
        }
    }()

    // CSV header handling
    var hdr []string
    if *format == "csv" && *csvHeader != "" {
        hdr = splitTrim(*csvHeader)
    }

    for {
        line, err := t.readLine()
        if err != nil {
            fmt.Fprintf(os.Stderr, "read error: %v\n", err)
            time.Sleep(time.Second)
            continue
        }
        if strings.TrimSpace(line) == "" {
            continue
        }
        if *format == "json" {
            // Parse JSON line; if the line is a JSON object or array.
            dec := json.NewDecoder(strings.NewReader(line))
            dec.UseNumber()
            var v any
            if err := dec.Decode(&v); err != nil {
                // Not JSON; skip.
                continue
            }
            mu.Lock()
            switch val := v.(type) {
            case map[string]any:
                n := normalize(val)
                if len(n) > 0 {
                    buf = append(buf, n)
                }
            case []any:
                for _, it := range val {
                    if m, ok := it.(map[string]any); ok {
                        n := normalize(m)
                        if len(n) > 0 {
                            buf = append(buf, n)
                        }
                    }
                }
            }
        } else {
            // CSV line parsing. Header detected/defined once, then normalize rows.
            if len(*csvDelim) == 0 {
                *csvDelim = ","
            }
            cr := csv.NewReader(strings.NewReader(line))
            cr.Comma = rune((*csvDelim)[0])
            cr.TrimLeadingSpace = true
            rec, err := cr.Read()
            if err != nil {
                continue
            }
            if hdr == nil || len(hdr) == 0 {
                // First line becomes header if not provided.
                hdr = make([]string, len(rec))
                copy(hdr, rec)
                continue
            }
            // If the row looks like header (first cell equals first header), skip.
            if len(rec) == len(hdr) && strings.EqualFold(strings.TrimSpace(rec[0]), strings.TrimSpace(hdr[0])) {
                continue
            }
            // Map header -> value
            m := map[string]any{}
            for i := 0; i < len(rec) && i < len(hdr); i++ {
                m[strings.TrimSpace(hdr[i])] = strings.TrimSpace(rec[i])
            }
            n := normalize(m)
            if len(n) == 0 {
                continue
            }
            mu.Lock()
            buf = append(buf, n)
        }
        // Flush if batch full
        if len(buf) >= *batch {
            b := buf
            buf = nil
            mu.Unlock()
            // immediate flush
            payload, _ := json.Marshal(b)
            req, _ := http.NewRequest("POST", *url, bytes.NewReader(payload))
            req.Header.Set("Content-Type", "application/json")
            if *token != "" {
                req.Header.Set("X-PMTA-Token", *token)
            }
            resp, err := cli.Do(req)
            if err == nil {
                io.Copy(io.Discard, resp.Body)
                resp.Body.Close()
            }
        } else {
            mu.Unlock()
        }
    }
}

func splitTrim(s string) []string {
    if s == "" { return nil }
    parts := strings.Split(s, ",")
    for i := range parts {
        parts[i] = strings.TrimSpace(parts[i])
    }
    return parts
}

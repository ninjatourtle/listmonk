package replies

import (
    "log"
    "time"

    "github.com/jmoiron/sqlx"
    "github.com/knadh/listmonk/models"
)

// Mailbox represents a POP mailbox client that can scan messages and pass
// them to a given channel.
type Mailbox interface {
    Scan(limit int, ch chan models.Reply) error
}

// Opt represents replies processing options.
type Opt struct {
    MailboxEnabled bool        `json:"mailbox_enabled"`
    Mailbox        MailboxOpt  `json:"mailbox"`

    RecordReplyCB  func(models.Reply) error
}

// MailboxOpt is a minimal POP3 options struct.
type MailboxOpt struct {
    Host          string
    Port          int
    Username      string
    Password      string
    TLSEnabled    bool
    TLSSkipVerify bool
    Folder        string
    ScanInterval  time.Duration
}

// Queries contains prepared statements used by the manager (reserved for future use).
type Queries struct {
    DB *sqlx.DB
}

// Manager handles e-mail replies ingestion.
type Manager struct {
    queue   chan models.Reply
    mailbox Mailbox
    opt     Opt
    log     *log.Logger
}

// New returns a new instance of the replies manager.
func New(opt Opt, q *Queries, lo *log.Logger) (*Manager, error) {
    m := &Manager{
        opt:   opt,
        queue: make(chan models.Reply, 1000),
        log:   lo,
    }

    if opt.MailboxEnabled {
        // mailbox will be injected by caller (constructed per-account)
    }

    return m, nil
}

// Run listens for reply events and executes the callback for DB persistence.
func (m *Manager) Run() {
    if m.mailbox != nil {
        go m.runMailboxScanner()
    }

    for r := range m.queue {
        if r.CreatedAt.IsZero() {
            r.CreatedAt = time.Now()
        }
        if err := m.opt.RecordReplyCB(r); err != nil {
            continue
        }
    }
}

func (m *Manager) runMailboxScanner() {
    for {
        if err := m.mailbox.Scan(1000, m.queue); err != nil {
            m.log.Printf("error scanning reply mailbox: %v", err)
        }
        time.Sleep(m.opt.Mailbox.ScanInterval)
    }
}

// Record enqueues a reply event.
func (m *Manager) Record(r models.Reply) error {
    m.queue <- r
    return nil
}



package email

import (
    "sync"
    "time"
)

// WarmingEvent represents a single warmcloud send/submit record for UI.
type WarmingEvent struct {
    CreatedAt time.Time `json:"created_at"`
    Server    string    `json:"server"`
    Email     string    `json:"email"`
    Subject   string    `json:"subject"`
    Submitted bool      `json:"submitted"`
    Error     string    `json:"error,omitempty"`
    SubmitToken string  `json:"-"`
}

var (
    warmEvents     = make([]WarmingEvent, 0, 1000)
    warmEventsLock sync.RWMutex
    warmEventsCap  = 1000
)

// AddWarmingEvent appends an event to the in-memory ring buffer.
func AddWarmingEvent(ev WarmingEvent) {
    warmEventsLock.Lock()
    defer warmEventsLock.Unlock()
    if len(warmEvents) >= warmEventsCap {
        // drop oldest 10% to make room
        drop := warmEventsCap / 10
        if drop < 1 { drop = 1 }
        warmEvents = append([]WarmingEvent{}, warmEvents[drop:]...)
    }
    warmEvents = append(warmEvents, ev)
}

// GetWarmingEvents returns the latest N events in reverse chronological order.
func GetWarmingEvents(limit int) []WarmingEvent {
    warmEventsLock.RLock()
    defer warmEventsLock.RUnlock()
    if limit <= 0 || limit > len(warmEvents) {
        limit = len(warmEvents)
    }
    out := make([]WarmingEvent, 0, limit)
    for i := len(warmEvents) - 1; i >= 0 && len(out) < limit; i-- {
        out = append(out, warmEvents[i])
    }
    return out
}

// UpdateWarmingEventByToken marks an event as submitted or updates error by submit token.
func UpdateWarmingEventByToken(token string, submitted bool, errStr string) {
    warmEventsLock.Lock()
    defer warmEventsLock.Unlock()
    for i := len(warmEvents) - 1; i >= 0; i-- {
        if warmEvents[i].SubmitToken == token {
            if submitted {
                warmEvents[i].Submitted = true
                warmEvents[i].Error = ""
            }
            if errStr != "" {
                warmEvents[i].Error = errStr
            }
            return
        }
    }
    // Not found: append a minimal record to reflect status anyway.
    warmEvents = append(warmEvents, WarmingEvent{
        CreatedAt: time.Now(),
        Submitted: submitted,
        Error:     errStr,
        SubmitToken: token,
    })
}

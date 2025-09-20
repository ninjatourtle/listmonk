package manager

import (
    "math/rand"
    "time"
)

// slotTimePlan returns an ordered slice of due times for distributing
// total slots across the provided window using an even spacing with jitter.
// start is the first possible due time, window is the campaign rate window
// (eg: 24h), perWindow is the maximum number of slots within a window, and
// now is the current timestamp used to avoid generating due times in the past.
func slotTimePlan(start time.Time, window time.Duration, total, perWindow int, now time.Time) []time.Time {
    if total <= 0 {
        return nil
    }
    if perWindow <= 0 {
        perWindow = 1
    }
    if window <= 0 {
        window = time.Hour
    }

    out := make([]time.Time, 0, total)
    windows := (total + perWindow - 1) / perWindow

    idx := 0
    for w := 0; w < windows; w++ {
        windowStart := start.Add(time.Duration(w) * window)

        remaining := total - idx
        emailsInWindow := perWindow
        if emailsInWindow > remaining {
            emailsInWindow = remaining
        }
        if emailsInWindow <= 0 {
            break
        }

        bin := window / time.Duration(emailsInWindow)
        if bin <= 0 {
            bin = time.Second
        }

        maxJitter := bin / 4
        if maxJitter > 15*time.Minute {
            maxJitter = 15 * time.Minute
        }
        if maxJitter > bin {
            maxJitter = bin
        }
        if maxJitter < 0 {
            maxJitter = 0
        }

        for j := 0; j < emailsInWindow; j++ {
            due := windowStart.Add(time.Duration(j) * bin)
            if !(w == 0 && j == 0) && maxJitter > 0 {
                jitter := time.Duration(rand.Int63n(maxJitter.Nanoseconds()))
                due = due.Add(jitter)
            }
            if due.Before(now) {
                due = now
            }

            out = append(out, due)
            idx++
        }
    }

    return out
}

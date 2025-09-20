package email

import (
    "log"
)

// pkgLogger is an optional logger injected by the app so that logs
// are captured in the in-memory buffer for the UI.
var pkgLogger *log.Logger

// SetLogger wires an external logger (eg: app logger) into the email package.
func SetLogger(l *log.Logger) {
    pkgLogger = l
}

func elogf(format string, args ...interface{}) {
    if pkgLogger != nil {
        pkgLogger.Printf(format, args...)
        return
    }
    log.Printf(format, args...)
}


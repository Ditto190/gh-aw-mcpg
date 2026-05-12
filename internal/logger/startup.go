package logger

import "log"

// StartupInfo logs a startup informational message to stderr (via log.Printf)
// and to the startup markdown/file log sink (via LogInfoToMarkdown with "startup" category).
// This eliminates the need to call log.Printf and LogInfoToMarkdown separately for the same message.
func StartupInfo(format string, args ...interface{}) {
	log.Printf(format, args...)
	LogInfoToMarkdown("startup", format, args...)
}

// StartupWarn logs a startup warning message to stderr (via log.Printf with
// "Warning: " prefix) and to the startup warning log sink (via LogWarn with
// "startup" category).
// This eliminates the need to call log.Printf and LogWarn separately for the same message.
func StartupWarn(format string, args ...interface{}) {
	log.Printf("Warning: "+format, args...)
	LogWarn("startup", format, args...)
}

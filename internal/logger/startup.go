package logger

import "log"

// StartupInfo logs a startup informational message to both stderr (via log.Printf)
// and the structured loggers (via LogInfoMd with "startup" category).
// This eliminates the need to call log.Printf and LogInfoMd separately for the same message.
func StartupInfo(format string, args ...interface{}) {
	log.Printf(format, args...)
	LogInfoMd("startup", format, args...)
}

// StartupWarn logs a startup warning message to both stderr (via log.Printf with
// "Warning: " prefix) and the structured loggers (via LogWarn with "startup" category).
// This eliminates the need to call log.Printf and LogWarn separately for the same message.
func StartupWarn(format string, args ...interface{}) {
	log.Printf("Warning: "+format, args...)
	LogWarn("startup", format, args...)
}

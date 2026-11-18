// Package clierr defines the typed errors of the CLI and their exit codes.
package clierr

import "fmt"

// Error is an error with a stable machine-readable code.
type Error struct {
	Code    string
	Message string
	Details map[string]any
}

// New builds an Error with a formatted message.
func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

func (e *Error) Error() string { return e.Message }

// ExitCode maps the error code to the process exit code. Unknown codes give 3.
func (e *Error) ExitCode() int {
	switch e.Code {
	case "usage", "file_not_found":
		return 1
	case "adb_not_found", "no_device", "multiple_devices", "device_unauthorized", "device_locked", "adb_failed":
		return 2
	case "app_not_installed", "multiple_packages", "app_outdated", "cli_outdated", "protocol_error", "unknown_method":
		return 3
	case "pro_required":
		return 4
	case "setup_incomplete", "setup_failed":
		return 5
	case "invalid_gpx", "invalid_speed":
		return 6
	case "start_failed", "mock_failed", "stopped":
		return 7
	case "timeout":
		return 8
	}
	return 3
}

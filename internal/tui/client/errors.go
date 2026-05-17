package client

import (
	"errors"
	"fmt"
)

// Sentinel errors for the client package.
var (
	ErrDaemonUnavailable = errors.New("daemon unavailable")
	ErrDaemonRequired    = errors.New("daemon required but not running")
)

// DaemonError wraps daemon-specific errors with status context.
type DaemonError struct {
	Status DaemonStatus
	Err    error
}

func (e *DaemonError) Error() string {
	return fmt.Sprintf("daemon %s: %v", e.Status, e.Err)
}

func (e *DaemonError) Unwrap() error {
	return e.Err
}

// LocalError wraps local operation errors.
type LocalError struct {
	Op  string
	Err error
}

func (e *LocalError) Error() string {
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *LocalError) Unwrap() error {
	return e.Err
}

// IsDaemonUnavailable reports whether err indicates the daemon is not running.
func IsDaemonUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDaemonUnavailable) {
		return true
	}
	var de *DaemonError
	if errors.As(err, &de) {
		return de.Status == DaemonUnavailable || de.Status == DaemonRequiredMissing
	}
	return false
}

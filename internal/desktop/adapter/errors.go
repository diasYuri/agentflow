package adapter

import (
	"errors"
	"fmt"
)

const (
	ErrCodeInvalidPath      = "invalid_path"
	ErrCodeWorkflowNotFound = "workflow_not_found"
	ErrCodeValidationFailed = "validation_failed"
	ErrCodeYAMLError        = "yaml_error"
	ErrCodeInternalError    = "internal_error"
	ErrCodeInvalidInput     = "invalid_input"
	ErrCodeFileSystem       = "file_system"
)

// DesktopError representa um erro normalizado para consumo pela UI.
type DesktopError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Context string `json:"context,omitempty"`
}

func (e DesktopError) Error() string {
	if e.Context != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Message, e.Context)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

var (
	ErrWorkflowNotFound = errors.New("workflow not found")
	ErrInvalidPath      = errors.New("invalid path")
)

func normalizeError(err error) DesktopError {
	if err == nil {
		return DesktopError{}
	}
	var de DesktopError
	if errors.As(err, &de) {
		return de
	}
	return DesktopError{Message: err.Error(), Code: ErrCodeInternalError}
}

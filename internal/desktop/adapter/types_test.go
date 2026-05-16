package adapter

import (
	"errors"
	"testing"
)

func TestDesktopError_Error(t *testing.T) {
	de := DesktopError{Message: "something failed", Code: ErrCodeInternalError}
	want := "[internal_error] something failed"
	if got := de.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	deWithContext := DesktopError{Message: "something failed", Code: ErrCodeInternalError, Context: "/tmp/test.yaml"}
	want = "[internal_error] something failed: /tmp/test.yaml"
	if got := deWithContext.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestNormalizeError(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got := normalizeError(nil)
		if got.Message != "" || got.Code != "" {
			t.Errorf("expected empty error, got %+v", got)
		}
	})

	t.Run("desktop error passthrough", func(t *testing.T) {
		original := DesktopError{Message: "not found", Code: ErrCodeWorkflowNotFound}
		got := normalizeError(original)
		if got != original {
			t.Errorf("expected passthrough, got %+v", got)
		}
	})

	t.Run("wrapped desktop error", func(t *testing.T) {
		original := DesktopError{Message: "bad", Code: ErrCodeInvalidPath}
		wrapped := errors.Join(original, errors.New("other"))
		got := normalizeError(wrapped)
		if got.Code != ErrCodeInvalidPath {
			t.Errorf("expected code %s, got %s", ErrCodeInvalidPath, got.Code)
		}
	})

	t.Run("plain error", func(t *testing.T) {
		got := normalizeError(errors.New("plain error"))
		if got.Code != ErrCodeInternalError {
			t.Errorf("expected code %s, got %s", ErrCodeInternalError, got.Code)
		}
		if got.Message != "plain error" {
			t.Errorf("expected message 'plain error', got %s", got.Message)
		}
	})
}

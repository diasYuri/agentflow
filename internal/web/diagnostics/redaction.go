// Package diagnostics records structured diagnostic events, applies
// redaction rules, and exports debug bundles for support tickets.
package diagnostics

import (
	"fmt"
	"regexp"
	"strings"
)

// RedactionPolicy decides how a value should be masked before it
// crosses a trust boundary (storage, SSE, debug bundle).
type RedactionPolicy struct {
	// MaxValueBytes is the longest a string can be before it is
	// truncated to MaxValueBytes characters with a marker appended.
	MaxValueBytes int
	// SecretKeySubstrings causes any map key whose name contains one of
	// these substrings (case-insensitive) to be replaced with
	// "[redacted]".
	SecretKeySubstrings []string
	// SecretValuePatterns matches whole values to redact. Useful for
	// well known token shapes (bearer/sk-/ghp_/etc.).
	SecretValuePatterns []*regexp.Regexp
}

// DefaultPolicy is a reasonable starting policy that catches obvious
// secrets without being so aggressive that it harms debuggability.
func DefaultPolicy() RedactionPolicy {
	return RedactionPolicy{
		MaxValueBytes: 4 * 1024,
		SecretKeySubstrings: []string{
			"token", "secret", "password", "passwd", "api_key",
			"apikey", "authorization", "auth", "session", "cookie",
			"private_key", "credential",
		},
		SecretValuePatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]{6,}`),
			regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
			regexp.MustCompile(`ghp_[A-Za-z0-9]{20,}`),
			regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{8,}`),
		},
	}
}

const redactedMarker = "[redacted]"

// Redact returns a copy of value with secret-shaped data masked and
// long strings truncated.
func (p RedactionPolicy) Redact(value any) any {
	if value == nil {
		return nil
	}
	return p.redact("", value)
}

// RedactMap is a convenience wrapper that always returns a map.
func (p RedactionPolicy) RedactMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return value
	}
	out := make(map[string]any, len(value))
	for k, v := range value {
		if p.isSecretKey(k) {
			out[k] = redactedMarker
			continue
		}
		out[k] = p.redact(k, v)
	}
	return out
}

func (p RedactionPolicy) redact(key string, value any) any {
	switch v := value.(type) {
	case map[string]any:
		return p.RedactMap(v)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = p.redact(key, item)
		}
		return out
	case string:
		return p.redactString(key, v)
	default:
		return value
	}
}

func (p RedactionPolicy) redactString(key, value string) string {
	if p.isSecretKey(key) {
		return redactedMarker
	}
	for _, pattern := range p.SecretValuePatterns {
		if pattern.MatchString(value) {
			return pattern.ReplaceAllString(value, redactedMarker)
		}
	}
	if p.MaxValueBytes > 0 && len(value) > p.MaxValueBytes {
		truncated := value[:p.MaxValueBytes]
		return fmt.Sprintf("%s... [truncated %d bytes]", truncated, len(value)-p.MaxValueBytes)
	}
	return value
}

func (p RedactionPolicy) isSecretKey(key string) bool {
	if key == "" {
		return false
	}
	lower := strings.ToLower(key)
	for _, sub := range p.SecretKeySubstrings {
		if sub == "" {
			continue
		}
		if strings.Contains(lower, strings.ToLower(sub)) {
			return true
		}
	}
	return false
}

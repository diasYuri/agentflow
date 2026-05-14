package run

import (
	"sort"
	"strings"
)

const MaskReplacement = "[REDACTED]"

type SecretMasker struct {
	values []string
}

func NewSecretMasker(secrets map[string]any) SecretMasker {
	values := make([]string, 0, len(secrets))
	seen := map[string]struct{}{}
	for _, value := range secrets {
		secret, ok := value.(string)
		if !ok || secret == "" {
			continue
		}
		if _, exists := seen[secret]; exists {
			continue
		}
		seen[secret] = struct{}{}
		values = append(values, secret)
	}
	sort.Slice(values, func(i, j int) bool {
		return len(values[i]) > len(values[j])
	})
	return SecretMasker{values: values}
}

func (m SecretMasker) Empty() bool {
	return len(m.values) == 0
}

func (m SecretMasker) MaskString(value string) string {
	for _, secret := range m.values {
		value = strings.ReplaceAll(value, secret, MaskReplacement)
	}
	return value
}

func (m SecretMasker) MaskValue(value any) any {
	if m.Empty() || value == nil {
		return value
	}
	switch typed := value.(type) {
	case string:
		return m.MaskString(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = m.MaskValue(item)
		}
		return out
	case []string:
		out := make([]string, len(typed))
		for i, item := range typed {
			out[i] = m.MaskString(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = m.MaskValue(item)
		}
		return out
	case map[string]string:
		out := make(map[string]string, len(typed))
		for key, item := range typed {
			out[key] = m.MaskString(item)
		}
		return out
	default:
		return value
	}
}

func (m SecretMasker) MaskEvent(event Event) Event {
	if m.Empty() {
		return event
	}
	event.Data = maskMap(m, event.Data)
	return event
}

func (m SecretMasker) MaskNodeResult(result NodeResult) NodeResult {
	if m.Empty() {
		return result
	}
	result.Output = m.MaskValue(result.Output)
	result.Outputs = maskSlice(m, result.Outputs)
	result.Stdout = m.MaskString(result.Stdout)
	result.Stderr = m.MaskString(result.Stderr)
	result.Error = m.MaskString(result.Error)
	return result
}

func (m SecretMasker) MaskSummary(summary Summary) Summary {
	if m.Empty() {
		return summary
	}
	nodes := make(map[string]NodeResult, len(summary.Nodes))
	for id, result := range summary.Nodes {
		nodes[id] = m.MaskNodeResult(result)
	}
	summary.Nodes = nodes
	return summary
}

func maskMap(masker SecretMasker, value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	out := make(map[string]any, len(value))
	for key, item := range value {
		out[key] = masker.MaskValue(item)
	}
	return out
}

func maskSlice(masker SecretMasker, value []any) []any {
	if value == nil {
		return nil
	}
	out := make([]any, len(value))
	for i, item := range value {
		out[i] = masker.MaskValue(item)
	}
	return out
}

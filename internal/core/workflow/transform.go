package workflow

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

func ApplyTransform(operation string, input any, with map[string]any) (any, error) {
	switch operation {
	case "chunk":
		return transformChunk(input, with)
	case "merge":
		return transformMerge(input)
	case "flat_map":
		return transformFlatMap(input, with)
	case "json_parse":
		text, ok := input.(string)
		if !ok {
			return nil, fmt.Errorf("json_parse input must be string, got %T", input)
		}
		var out any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			return nil, err
		}
		return out, nil
	case "json_stringify":
		data, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		return string(data), nil
	case "pick":
		path, _ := with["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("pick requires with.path")
		}
		return pick(input, path)
	default:
		return nil, fmt.Errorf("unknown transform operation %q", operation)
	}
}

func transformChunk(input any, with map[string]any) (any, error) {
	chunks := intFromAny(with["chunks"])
	if chunks <= 0 {
		return nil, fmt.Errorf("chunk requires with.chunks > 0")
	}
	items, err := ToAnySlice(input)
	if err != nil {
		if text, ok := input.(string); ok {
			return chunkString(text, chunks), nil
		}
		return nil, err
	}
	out := make([]any, 0, chunks)
	size := (len(items) + chunks - 1) / chunks
	if size == 0 {
		return []any{}, nil
	}
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		out = append(out, items[start:end])
	}
	return out, nil
}

func transformMerge(input any) (any, error) {
	items, err := ToAnySlice(input)
	if err != nil {
		return []any{input}, nil
	}
	var out []any
	for _, item := range items {
		if nested, err := ToAnySlice(item); err == nil {
			out = append(out, nested...)
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

func transformFlatMap(input any, with map[string]any) (any, error) {
	items, err := ToAnySlice(input)
	if err != nil {
		return nil, err
	}
	path, _ := with["path"].(string)
	var out []any
	for _, item := range items {
		value := item
		if path != "" {
			value, err = pick(item, path)
			if err != nil {
				return nil, err
			}
		}
		nested, err := ToAnySlice(value)
		if err != nil {
			out = append(out, value)
			continue
		}
		out = append(out, nested...)
	}
	return out, nil
}

func chunkString(text string, chunks int) []any {
	runes := []rune(text)
	size := (len(runes) + chunks - 1) / chunks
	if size == 0 {
		return []any{}
	}
	out := []any{}
	for start := 0; start < len(runes); start += size {
		end := start + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[start:end]))
	}
	return out
}

func pick(input any, path string) (any, error) {
	current := input
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			continue
		}
		switch typed := current.(type) {
		case map[string]any:
			current = typed[part]
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("invalid array index %q", part)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("cannot pick %q from %T", part, current)
		}
	}
	return current, nil
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		i, _ := strconv.Atoi(typed)
		return i
	default:
		rv := reflect.ValueOf(value)
		if rv.IsValid() {
			switch rv.Kind() {
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				return int(rv.Int())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				return int(rv.Uint())
			}
		}
		return 0
	}
}

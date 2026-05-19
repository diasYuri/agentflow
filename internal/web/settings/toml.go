package settings

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// applyTOMLFile reads the settings file at path and applies recognised
// values onto cfg. A missing file is not an error — settings.toml is
// optional and defaults are sufficient for first runs.
func applyTOMLFile(cfg *Settings, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	values, err := parseTOML(string(data))
	if err != nil {
		return err
	}
	return applyTOML(cfg, values)
}

func applyTOML(cfg *Settings, values map[string]map[string]string) error {
	if web, ok := values["web"]; ok {
		if v, ok := web["host"]; ok {
			cfg.Server.Host = v
		}
		if v, ok := web["port"]; ok {
			port, err := strconv.Atoi(v)
			if err != nil {
				return fmt.Errorf("web.port %q: %w", v, err)
			}
			cfg.Server.Port = port
		}
		if v, ok := web["open_browser"]; ok {
			b, err := parseBool(v)
			if err != nil {
				return fmt.Errorf("web.open_browser %q: %w", v, err)
			}
			cfg.Server.OpenBrowser = b
		}
		if v, ok := web["dev_assets"]; ok {
			cfg.Server.DevAssets = v
		}
		if v, ok := web["daemon"]; ok {
			cfg.Server.Daemon = DaemonRequirement(v)
		}
	}
	if auth, ok := values["web.auth"]; ok {
		if v, ok := auth["token_override"]; ok {
			cfg.Auth.TokenOverride = v
		}
	}
	if paths, ok := values["paths"]; ok {
		if v, ok := paths["root"]; ok && strings.TrimSpace(v) != "" {
			cfg.Paths.Root = v
		}
		if v, ok := paths["daemon_socket"]; ok {
			cfg.Paths.DaemonSocket = v
		}
	}
	return nil
}

// parseTOML implements the small slice of TOML the web settings need:
//   - comments starting with `#`
//   - blank lines
//   - section headers `[name]` and `[parent.child]`
//   - `key = value` pairs at the top level (mapped to the empty section)
//     or under the most recently declared section
//   - string values in double quotes with `\\`, `\n`, `\t`, `\"` escapes
//   - bare booleans (true/false), integers and decimal numbers
//
// Anything outside this subset returns an error so users see a clear
// problem instead of silently losing configuration.
func parseTOML(input string) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	current := ""
	out[current] = map[string]string{}
	reader := strings.NewReader(input)
	scanner := newLineScanner(reader)
	lineNo := 0
	for {
		line, err := scanner.next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		lineNo++
		trimmed := stripComment(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			if name == "" {
				return nil, fmt.Errorf("line %d: empty section header", lineNo)
			}
			current = name
			if _, ok := out[current]; !ok {
				out[current] = map[string]string{}
			}
			continue
		}
		eq := strings.Index(trimmed, "=")
		if eq < 0 {
			return nil, fmt.Errorf("line %d: expected key = value", lineNo)
		}
		key := strings.TrimSpace(trimmed[:eq])
		raw := strings.TrimSpace(trimmed[eq+1:])
		value, err := parseTOMLValue(raw)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
		out[current][key] = value
	}
	return out, nil
}

func stripComment(line string) string {
	inString := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' && (i == 0 || line[i-1] != '\\') {
			inString = !inString
			continue
		}
		if c == '#' && !inString {
			return strings.TrimSpace(line[:i])
		}
	}
	return line
}

func parseTOMLValue(raw string) (string, error) {
	if raw == "" {
		return "", fmt.Errorf("missing value")
	}
	if strings.HasPrefix(raw, "\"") {
		if !strings.HasSuffix(raw, "\"") || len(raw) < 2 {
			return "", fmt.Errorf("unterminated string %q", raw)
		}
		return unquoteTOMLString(raw[1 : len(raw)-1])
	}
	switch raw {
	case "true", "false":
		return raw, nil
	}
	if _, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return raw, nil
	}
	if _, err := strconv.ParseFloat(raw, 64); err == nil {
		return raw, nil
	}
	return "", fmt.Errorf("unsupported value %q", raw)
}

func unquoteTOMLString(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '\\' {
			b.WriteByte(c)
			continue
		}
		if i+1 >= len(s) {
			return "", fmt.Errorf("dangling escape in %q", s)
		}
		i++
		switch s[i] {
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default:
			return "", fmt.Errorf("unsupported escape \\%c", s[i])
		}
	}
	return b.String(), nil
}

func parseBool(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		return true, nil
	case "false", "0", "no", "off":
		return false, nil
	}
	return false, fmt.Errorf("not a boolean")
}

// lineScanner reads a string line by line without pulling in bufio so
// that small inputs (settings.toml is short) avoid the buffer allocation.
type lineScanner struct {
	r io.RuneReader
}

func newLineScanner(r io.RuneReader) *lineScanner { return &lineScanner{r: r} }

func (s *lineScanner) next() (string, error) {
	var b strings.Builder
	hasContent := false
	for {
		ch, _, err := s.r.ReadRune()
		if err != nil {
			if errors.Is(err, io.EOF) && hasContent {
				return b.String(), nil
			}
			return "", err
		}
		hasContent = true
		if ch == '\n' {
			return b.String(), nil
		}
		if ch == '\r' {
			continue
		}
		b.WriteRune(ch)
	}
}

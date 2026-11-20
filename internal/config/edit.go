package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Set writes one setting to config.toml, changing only its line (or adding
// one) so the rest of the file, comments included, stays as it is. section
// is the [table] it lives in, "" for the top level.
func (c *Config) Set(section, key string, value any) error {
	if c.ConfigDir == "" {
		return errors.New("no config folder")
	}
	return SetKey(filepath.Join(c.ConfigDir, "config.toml"), section, key, value)
}

// SetKey is Set on the file at path.
func SetKey(path, section, key string, value any) error {
	val, err := tomlValue(value)
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	out := setInText(string(b), section, key, val)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(out), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// setInText returns text with key = val set in section.
func setInText(text, section, key, val string) string {
	lines := strings.Split(text, "\n")
	cur := ""
	sectionAt := -1          // the [section] header line
	firstTable := len(lines) // where the top level ends
	lastInSection := -1      // last key line of the section
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if name, ok := tableName(t); ok {
			if firstTable == len(lines) {
				firstTable = i
			}
			cur = name
			if cur == section {
				sectionAt = i
			}
			continue
		}
		if cur != section || t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if k, rest, ok := splitKey(t); ok && k == key {
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			lines[i] = indent + key + " = " + val + trailingComment(rest)
			return strings.Join(lines, "\n")
		}
		lastInSection = i
	}

	newLine := key + " = " + val
	switch {
	case section == "":
		at := firstTable
		// Before the blank lines that separate the top level from the tables.
		for at > 0 && strings.TrimSpace(lines[at-1]) == "" && at <= len(lines)-1 {
			at--
		}
		return strings.Join(insert(lines, at, newLine), "\n")
	case sectionAt >= 0:
		at := sectionAt + 1
		if lastInSection > sectionAt {
			at = lastInSection + 1
		}
		return strings.Join(insert(lines, at, newLine), "\n")
	}
	text = strings.TrimRight(text, "\n")
	if text != "" {
		text += "\n\n"
	}
	return text + "[" + section + "]\n" + newLine + "\n"
}

func insert(lines []string, at int, line string) []string {
	lines = append(lines, "")
	copy(lines[at+1:], lines[at:])
	lines[at] = line
	return lines
}

// tableName reads a [table] header; arrays of tables are not used here.
func tableName(t string) (string, bool) {
	if !strings.HasPrefix(t, "[") || strings.HasPrefix(t, "[[") {
		return "", false
	}
	end := strings.Index(t, "]")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(t[1:end]), true
}

// splitKey splits `key = value # comment` into the key and the rest after =.
func splitKey(t string) (string, string, bool) {
	eq := strings.Index(t, "=")
	if eq <= 0 {
		return "", "", false
	}
	k := strings.Trim(strings.TrimSpace(t[:eq]), `"`)
	return k, t[eq+1:], true
}

// trailingComment keeps a comment after the old value, if there was one.
func trailingComment(rest string) string {
	inStr := false
	var quote byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		switch {
		case inStr && c == '\\' && quote == '"':
			i++
		case inStr && c == quote:
			inStr = false
		case !inStr && (c == '"' || c == '\''):
			inStr, quote = true, c
		case !inStr && c == '#':
			return "  " + strings.TrimSpace(rest[i:])
		}
	}
	return ""
}

// tomlValue formats a value as TOML.
func tomlValue(v any) (string, error) {
	switch x := v.(type) {
	case string:
		return tomlString(x), nil
	case bool:
		return strconv.FormatBool(x), nil
	case int:
		return strconv.Itoa(x), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case uint32:
		return strconv.FormatUint(uint64(x), 10), nil
	}
	return "", fmt.Errorf("cannot write %T to config.toml", v)
}

func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

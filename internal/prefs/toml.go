package prefs

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

// TOML edits the flat key = value files the boot daemon reads
// (audio-settings.toml, tau-settings.toml). The desktop ships these with
// every option present but commented out; setting a key uncomments its line
// in place, unsetting comments it back out, so the file stays readable and
// the daemon's own parser (tomlrb) sees ordinary TOML.
type TOML struct {
	path  string
	lines []string
}

var (
	tomlActive    = regexp.MustCompile(`^\s*([A-Za-z0-9_]+)\s*=\s*(.*?)\s*$`)
	tomlCommented = regexp.MustCompile(`^\s*#\s*([A-Za-z0-9_]+)\s*=\s*(.*?)\s*$`)
)

// LoadTOML reads path, seeding a missing file from template when given.
func LoadTOML(path, template string) (*TOML, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) && template != "" {
		b, err = os.ReadFile(template)
	}
	t := &TOML{path: path}
	if err != nil {
		if os.IsNotExist(err) {
			return t, nil
		}
		return nil, err
	}
	s := strings.TrimSuffix(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	if s != "" {
		t.lines = strings.Split(s, "\n")
	}
	return t, nil
}

// Path is the file this was loaded from.
func (t *TOML) Path() string { return t.path }

func stripComment(v string) string {
	if strings.HasPrefix(v, `"`) || strings.HasPrefix(v, `'`) {
		q := v[0]
		for i := 1; i < len(v); i++ {
			if v[i] == '\\' && q == '"' {
				i++
				continue
			}
			if v[i] == q {
				return v[:i+1]
			}
		}
		return v
	}
	if i := strings.Index(v, "#"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v)
}

func decodeTOMLValue(v string) string {
	v = stripComment(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if s, err := strconv.Unquote(v); err == nil {
			return s
		}
		return v[1 : len(v)-1]
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	return v
}

// EncodeTOMLValue writes v as a TOML value. Kind is "string", "int",
// "float" or "bool"; an empty kind infers numbers and booleans.
func EncodeTOMLValue(v, kind string) string {
	switch kind {
	case "string":
		return strconv.Quote(v)
	case "int", "float", "bool":
		return v
	}
	if v == "true" || v == "false" {
		return v
	}
	if _, err := strconv.ParseFloat(v, 64); err == nil {
		return v
	}
	return strconv.Quote(v)
}

// Get returns the active value of key.
func (t *TOML) Get(key string) (string, bool) {
	for _, l := range t.lines {
		if m := tomlActive.FindStringSubmatch(l); m != nil && m[1] == key {
			return decodeTOMLValue(m[2]), true
		}
	}
	return "", false
}

// Set gives key an encoded value (see EncodeTOMLValue).
func (t *TOML) Set(key, encoded string) {
	line := key + " = " + encoded
	for i, l := range t.lines {
		if m := tomlActive.FindStringSubmatch(l); m != nil && m[1] == key {
			t.lines[i] = line
			return
		}
	}
	for i, l := range t.lines {
		if m := tomlCommented.FindStringSubmatch(l); m != nil && m[1] == key {
			t.lines[i] = line
			return
		}
	}
	t.lines = append(t.lines, line)
}

// Unset comments key out. It reports whether the key was active.
func (t *TOML) Unset(key string) bool {
	for i, l := range t.lines {
		if m := tomlActive.FindStringSubmatch(l); m != nil && m[1] == key {
			t.lines[i] = "# " + key + " = " + m[2]
			return true
		}
	}
	return false
}

// Active lists keys with a value, in file order.
func (t *TOML) Active() []string {
	var keys []string
	for _, l := range t.lines {
		if m := tomlActive.FindStringSubmatch(l); m != nil {
			keys = append(keys, m[1])
		}
	}
	return keys
}

// Save writes the file back atomically.
func (t *TOML) Save() error {
	if err := os.MkdirAll(dirOf(t.path), 0o755); err != nil {
		return err
	}
	return writeAtomic(t.path, []byte(strings.Join(t.lines, "\n")+"\n"))
}

func dirOf(p string) string {
	i := strings.LastIndexAny(p, `/\`)
	if i < 0 {
		return "."
	}
	return p[:i]
}

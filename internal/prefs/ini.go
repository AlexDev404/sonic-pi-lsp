package prefs

import (
	"os"
	"path/filepath"
	"strings"
)

// INI reads and edits a file in Qt's QSettings IniFormat, which is how the
// desktop client stores gui-settings.ini. Lines the CLI does not touch
// (window geometry, @ByteArray blobs, comments) are written back unchanged.
//
// QSettings maps a key such as "prefs/scope/show-scopes" to the section
// [prefs] and the entry "scope\show-scopes". Keys with no group live in
// [General].
type INI struct {
	path  string
	lines []string
}

// LoadINI reads path. A missing file is an empty settings file.
func LoadINI(path string) (*INI, error) {
	ini := &INI{path: path}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ini, nil
		}
		return nil, err
	}
	s := strings.ReplaceAll(string(b), "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s != "" {
		ini.lines = strings.Split(s, "\n")
	}
	return ini, nil
}

// Path is the file this was loaded from.
func (f *INI) Path() string { return f.path }

func splitKey(key string) (section, name string) {
	key = strings.TrimPrefix(key, "/")
	i := strings.Index(key, "/")
	if i < 0 {
		return "General", key
	}
	return key[:i], strings.ReplaceAll(key[i+1:], "/", `\`)
}

func joinKey(section, name string) string {
	name = strings.ReplaceAll(name, `\`, "/")
	if section == "General" {
		return name
	}
	return section + "/" + name
}

func sectionOf(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
		return t[1 : len(t)-1], true
	}
	return "", false
}

func entryOf(line string) (string, string, bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "#") {
		return "", "", false
	}
	i := strings.Index(t, "=")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(t[:i]), strings.TrimSpace(t[i+1:]), true
}

// find returns the line index of key, and the index after the last line of
// its section (or -1 if the section is absent).
func (f *INI) find(key string) (at, sectionEnd int) {
	section, name := splitKey(key)
	at, sectionEnd = -1, -1
	cur := ""
	for i, l := range f.lines {
		if s, ok := sectionOf(l); ok {
			cur = s
			if s == section {
				sectionEnd = i + 1
			}
			continue
		}
		if cur != section {
			continue
		}
		if strings.TrimSpace(l) != "" {
			sectionEnd = i + 1
		}
		if k, _, ok := entryOf(l); ok && k == name {
			at = i
		}
	}
	return at, sectionEnd
}

// Get returns the decoded value of key.
func (f *INI) Get(key string) (string, bool) {
	at, _ := f.find(key)
	if at < 0 {
		return "", false
	}
	_, v, _ := entryOf(f.lines[at])
	return unquote(v), true
}

// Set stores value under key, adding the section if needed.
func (f *INI) Set(key, value string) {
	section, name := splitKey(key)
	line := name + "=" + quote(value)
	at, end := f.find(key)
	switch {
	case at >= 0:
		f.lines[at] = line
	case end >= 0:
		f.lines = append(f.lines[:end], append([]string{line}, f.lines[end:]...)...)
	default:
		if len(f.lines) > 0 && strings.TrimSpace(f.lines[len(f.lines)-1]) != "" {
			f.lines = append(f.lines, "")
		}
		f.lines = append(f.lines, "["+section+"]", line)
	}
}

// Unset removes key. It reports whether the key was present.
func (f *INI) Unset(key string) bool {
	at, _ := f.find(key)
	if at < 0 {
		return false
	}
	f.lines = append(f.lines[:at], f.lines[at+1:]...)
	return true
}

// Keys lists every key in the file, as QSettings names them.
func (f *INI) Keys() []string {
	var keys []string
	cur := "General"
	for _, l := range f.lines {
		if s, ok := sectionOf(l); ok {
			cur = s
			continue
		}
		if k, _, ok := entryOf(l); ok {
			keys = append(keys, joinKey(cur, k))
		}
	}
	return keys
}

// Save writes the file back atomically.
func (f *INI) Save() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o755); err != nil {
		return err
	}
	out := strings.Join(f.lines, "\n") + "\n"
	return writeAtomic(f.path, []byte(out))
}

func writeAtomic(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if fi, err := os.Stat(path); err == nil {
		os.Chmod(tmp.Name(), fi.Mode().Perm())
	}
	return os.Rename(tmp.Name(), path)
}

// quote encodes a value the way QSettings does: plain when it is safe,
// otherwise in double quotes with backslash escapes.
func quote(v string) string {
	if v == "" {
		return ""
	}
	needs := strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") || strings.HasPrefix(v, "@")
	for _, r := range v {
		switch r {
		case ',', ';', '=', '"', '\\', '\n', '\r', '\t', '#':
			needs = true
		}
	}
	if !needs {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`)
	return `"` + r.Replace(v) + `"`
}

func unquote(v string) string {
	if len(v) >= 2 && strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) {
		v = v[1 : len(v)-1]
		var b strings.Builder
		for i := 0; i < len(v); i++ {
			if v[i] == '\\' && i+1 < len(v) {
				i++
				switch v[i] {
				case 'n':
					b.WriteByte('\n')
				case 'r':
					b.WriteByte('\r')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(v[i])
				}
				continue
			}
			b.WriteByte(v[i])
		}
		return b.String()
	}
	return v
}

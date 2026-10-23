// Package winpath adds the install directory to, and removes it from, the
// Windows system PATH. It edits only its own entry: the value is read in
// full first, nothing is written if it cannot be read, and the previous
// value is saved before every change.
package winpath

import (
	"strings"
)

func same(a, b string) bool {
	norm := func(s string) string {
		return strings.ToLower(strings.TrimRight(strings.TrimSpace(s), `\/`))
	}
	return norm(a) == norm(b)
}

// With returns path with dir appended, and whether that changed it.
func With(path, dir string) (string, bool) {
	for _, p := range strings.Split(path, ";") {
		if same(p, dir) {
			return path, false
		}
	}
	if path == "" {
		return dir, true
	}
	if !strings.HasSuffix(path, ";") {
		path += ";"
	}
	return path + dir, true
}

// Without returns path with every entry equal to dir removed, and whether
// that changed it. Other entries, including empty ones, are kept as they are.
func Without(path, dir string) (string, bool) {
	parts := strings.Split(path, ";")
	kept := parts[:0]
	changed := false
	for _, p := range parts {
		if same(p, dir) {
			changed = true
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, ";"), changed
}

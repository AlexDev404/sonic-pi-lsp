//go:build !windows

package winpath

import "errors"

// ErrUnsupported is returned off Windows, where there is no system PATH
// in the registry to edit.
var ErrUnsupported = errors.New("editing the system PATH is only for Windows")

// Add is Windows only.
func Add(string) error { return ErrUnsupported }

// Remove is Windows only.
func Remove(string) error { return ErrUnsupported }

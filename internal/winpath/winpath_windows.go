//go:build windows

package winpath

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	envKey    = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
	backupKey = `Software\SonicPiCLI\PathBackups`
)

// Add appends dir to the system PATH unless it is already there.
func Add(dir string) error { return edit(dir, With) }

// Remove takes dir out of the system PATH.
func Remove(dir string) error { return edit(dir, Without) }

func edit(dir string, change func(path, dir string) (string, bool)) error {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, envKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening the system environment (run as administrator): %w", err)
	}
	defer k.Close()
	old, typ, err := k.GetStringValue("Path")
	if err != nil {
		// Never write a PATH we could not read: that is how one gets lost.
		return fmt.Errorf("reading the system PATH, so leaving it alone: %w", err)
	}
	updated, changed := change(old, dir)
	if !changed {
		return nil
	}
	if err := backup(old, typ); err != nil {
		return fmt.Errorf("saving a backup of the system PATH, so leaving it alone: %w", err)
	}
	if typ == registry.SZ {
		err = k.SetStringValue("Path", updated)
	} else {
		err = k.SetExpandStringValue("Path", updated)
	}
	if err != nil {
		return err
	}
	broadcast()
	return nil
}

// backup keeps the PATH as it was before each change, named by time.
func backup(path string, typ uint32) error {
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, backupKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	name := time.Now().UTC().Format("2006-01-02T15-04-05Z")
	if typ == registry.SZ {
		return k.SetStringValue(name, path)
	}
	return k.SetExpandStringValue(name, path)
}

// broadcast tells running programs (Explorer, new terminals) the
// environment changed.
func broadcast() {
	user32 := windows.NewLazySystemDLL("user32.dll")
	send := user32.NewProc("SendMessageTimeoutW")
	env, _ := syscall.UTF16PtrFromString("Environment")
	const hwndBroadcast, wmSettingChange, smtoAbortIfHung = 0xffff, 0x001A, 0x0002
	var result uintptr
	send.Call(hwndBroadcast, wmSettingChange, 0, uintptr(unsafe.Pointer(env)), smtoAbortIfHung, 5000, uintptr(unsafe.Pointer(&result)))
}

// ErrUnsupported is never returned on Windows.
var ErrUnsupported = errors.New("only on Windows")

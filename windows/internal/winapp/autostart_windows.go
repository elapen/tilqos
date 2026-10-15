//go:build windows && (amd64 || arm64)

package winapp

// Launch at sign-in through HKCU\Software\Microsoft\Windows\CurrentVersion\Run — per user,
// no elevation, visible in Task Manager's Startup tab where users expect to find it.

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"tilqos/internal/win32"
)

const (
	runKeyPath   = `Software\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "Tilqos"
)

func executablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path
}

func openRunKey(access uint32) (syscall.Handle, error) {
	var key syscall.Handle
	err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, win32.UTF16Ptr(runKeyPath), 0, access, &key)
	return key, err
}

// autostartEnabled reports whether the Run entry points at this executable.
func autostartEnabled() bool {
	key, err := openRunKey(syscall.KEY_QUERY_VALUE)
	if err != nil {
		return false
	}
	defer syscall.RegCloseKey(key)
	var buf [1024]uint16
	size := uint32(len(buf) * 2)
	var kind uint32
	if syscall.RegQueryValueEx(key, win32.UTF16Ptr(runValueName), nil, &kind, (*byte)(unsafe.Pointer(&buf[0])), &size) != nil {
		return false
	}
	value := strings.Trim(syscall.UTF16ToString(buf[:size/2]), `" `)
	return strings.EqualFold(filepath.Clean(value), filepath.Clean(executablePath()))
}

// setAutostart writes or removes the Run entry.
func setAutostart(on bool) error {
	key, err := openRunKey(syscall.KEY_SET_VALUE)
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(key)
	if !on {
		if err := win32.RegDeleteValue(key, runValueName); err != nil && err != syscall.ERROR_FILE_NOT_FOUND {
			return err
		}
		return nil
	}
	return win32.RegSetString(key, runValueName, `"`+executablePath()+`"`)
}

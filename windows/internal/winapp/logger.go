package winapp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Logger appends timestamped lines to a file, rotating it to <name>.old past a size limit.
//
// It is OFF by default: a new logger writes nothing and creates no file until SetEnabled(true),
// which the app does only when the user has turned diagnostics on. The file lives in
// %LOCALAPPDATA%\Tilqos (not the roaming profile), is at most max bytes plus one rotated copy,
// and Clear removes both.
//
// Privacy rule, same as the macOS app: callers log shapes and counts — lengths, languages,
// decisions — and never characters, clipboard content or window titles, and a value that
// comes from the user's machine and is not a number (the application in front) is logged as
// engine.LogPrivate. Nothing here can enforce that; the call sites do, and privacy_test.go
// reads every call to keep it so.
type Logger struct {
	enabled atomic.Bool

	mu   sync.Mutex
	path string
	file *os.File
	size int64
	max  int64
}

// NewLogger returns a logger for path that is switched off. maxBytes limits the file; 0 means
// no limit.
func NewLogger(path string, maxBytes int64) *Logger {
	return &Logger{path: path, max: maxBytes}
}

// Path is the log file.
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Enabled reports whether lines are being written.
func (l *Logger) Enabled() bool { return l != nil && l.enabled.Load() }

// SetEnabled switches the log on or off. Switching it off closes the file but leaves it on
// disk for the user to read or clear.
func (l *Logger) SetEnabled(on bool) {
	if l == nil {
		return
	}
	l.enabled.Store(on)
	if !on {
		l.mu.Lock()
		l.closeLocked()
		l.mu.Unlock()
	}
}

func (l *Logger) open() {
	if l.path == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return
	}
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	l.file = file
	if info, err := file.Stat(); err == nil {
		l.size = info.Size()
	}
}

func (l *Logger) closeLocked() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.size = 0
}

// Printf writes one line, if the log is on.
func (l *Logger) Printf(format string, args ...any) {
	if l == nil || !l.enabled.Load() {
		return
	}
	line := time.Now().Format("2006-01-02 15:04:05.000 ") + strings.TrimRight(fmt.Sprintf(format, args...), "\n") + "\n"
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled.Load() {
		return
	}
	if l.file == nil {
		l.open()
		if l.file == nil {
			return
		}
	}
	if l.max > 0 && l.size+int64(len(line)) > l.max {
		l.closeLocked()
		os.Remove(l.path + ".old")
		os.Rename(l.path, l.path+".old")
		l.open()
		if l.file == nil {
			return
		}
	}
	n, _ := l.file.WriteString(line)
	l.size += int64(n)
}

// Clear deletes the log and its rotated copy. The logger stays on or off as it was.
func (l *Logger) Clear() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeLocked()
	var first error
	for _, name := range []string{l.path, l.path + ".old"} {
		if name == "" {
			continue
		}
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	return first
}

// Close flushes and closes the file.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeLocked()
}

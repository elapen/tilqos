package config

// Where the files live. Tilqos keeps its settings, its learned-word store and (when the user
// turns diagnostics on) its log in %LOCALAPPDATA%\Tilqos: local to this machine. An earlier
// layout used %APPDATA%\Tilqos, the roaming profile, which follows the user to other
// computers on a domain; MigrateData moves what is there and removes the rest, the log
// included.

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DataDir is %LOCALAPPDATA%\Tilqos. getenv, cacheDir and tempDir are os.Getenv,
// os.UserCacheDir (which is %LocalAppData% on Windows) and os.TempDir; they are parameters so
// the choice can be tested on any OS.
func DataDir(getenv func(string) string, cacheDir func() (string, error), tempDir func() string) string {
	if local := getenv("LOCALAPPDATA"); local != "" {
		return filepath.Join(local, "Tilqos")
	}
	if base, err := cacheDir(); err == nil && base != "" {
		return filepath.Join(base, "Tilqos")
	}
	return filepath.Join(tempDir(), "Tilqos")
}

// RoamingDir is the earlier location, %APPDATA%\Tilqos; "" when it cannot be determined.
func RoamingDir(configDir func() (string, error)) string {
	if base, err := configDir(); err == nil && base != "" {
		return filepath.Join(base, "Tilqos")
	}
	return ""
}

// MigrateData moves the files of oldDir into newDir and removes oldDir. The log (and its
// rotated copy) and half-written temporary files are deleted instead of moved. A file that
// already exists in newDir wins: the old copy is deleted, not merged. Directories other than
// oldDir itself are left alone.
func MigrateData(oldDir, newDir string) error {
	if oldDir == "" || filepath.Clean(oldDir) == filepath.Clean(newDir) {
		return nil
	}
	entries, err := os.ReadDir(oldDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(newDir, 0o700); err != nil {
		return err
	}
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		from := filepath.Join(oldDir, name)
		switch {
		case name == "tilqos.log" || name == "tilqos.log.old",
			strings.HasPrefix(name, ".config-") && strings.HasSuffix(name, ".tmp"):
			keep(os.Remove(from))
			continue
		}
		to := filepath.Join(newDir, name)
		if _, err := os.Stat(to); err == nil {
			keep(os.Remove(from))
			continue
		}
		if err := os.Rename(from, to); err != nil {
			// Another volume, say: copy, then delete.
			if copyErr := copyFile(from, to); copyErr != nil {
				keep(copyErr)
				continue
			}
			keep(os.Remove(from))
		}
	}
	// Only succeeds when empty.
	_ = os.Remove(oldDir)
	return first
}

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(to)
		return err
	}
	return out.Close()
}

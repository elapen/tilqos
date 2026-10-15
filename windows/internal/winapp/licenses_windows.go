//go:build windows && (amd64 || arm64)

package winapp

// About -> Licenses: the license texts are embedded in the executable (internal/licenses), so
// a lone .exe passed around without its zip still carries them. They are written next to the
// other temporary files as .txt (which every Windows opens in Notepad; a .md has no default
// program) and opened from there.

import (
	"os"
	"path/filepath"
	"strings"

	"tilqos/internal/i18n"
	"tilqos/internal/licenses"
	"tilqos/internal/win32"
)

func (a *App) showLicenses() {
	files, err := licenses.Files()
	if err != nil {
		a.log.Printf("licenses: %v", err)
		messageBox(i18n.T(i18n.LicensesMissing), win32.MB_ICONINFORMATION)
		return
	}
	dir := filepath.Join(os.TempDir(), "Tilqos-licenses")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		messageBox(i18n.F(i18n.LicensesFailed, err.Error()), win32.MB_ICONWARNING)
		return
	}
	for _, name := range licenses.Names {
		target := filepath.Join(dir, strings.TrimSuffix(name, ".md")+".txt")
		if err := os.WriteFile(target, files[name], 0o644); err != nil {
			messageBox(i18n.F(i18n.LicensesFailed, err.Error()), win32.MB_ICONWARNING)
			return
		}
		win32.ShellOpen(target)
	}
}

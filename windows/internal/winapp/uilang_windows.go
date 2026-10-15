//go:build windows && (amd64 || arm64)

package winapp

// The interface language: Windows' own choice by default (GetUserDefaultUILanguage), or the
// one pinned in config.json ("uiLanguage"), which the tray menu sets.

import (
	"syscall"

	"tilqos/internal/config"
	"tilqos/internal/i18n"
)

// kernel32 is a KnownDLL, so loading it by name cannot be hijacked (see win32.SecureDLLSearch).
var procGetUserDefaultUILanguage = syscall.NewLazyDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// systemUILangID is the LANGID of the user's Windows display language (0 if it cannot be read).
func systemUILangID() uint16 {
	if procGetUserDefaultUILanguage.Find() != nil {
		return 0
	}
	id, _, _ := procGetUserDefaultUILanguage.Call()
	return uint16(id)
}

// applyUILanguage makes the texts follow the settings. Safe from any goroutine.
func applyUILanguage(c config.Config) i18n.Lang {
	lang := i18n.Resolve(i18n.Setting(c.UILanguage), systemUILangID())
	i18n.Set(lang)
	return lang
}

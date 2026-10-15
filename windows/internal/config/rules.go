package config

// Port of Core/AppRules.swift, keyed by lowercased executable name instead of bundle ID.

import "strings"

// AlwaysDisabled apps are never touched and cannot be enabled: typing into a password
// manager or a credential prompt is exactly the case where a helpful correction is a
// security problem.
var AlwaysDisabled = setOf(
	"keepass.exe", "keepassxc.exe", "1password.exe", "bitwarden.exe", "dashlane.exe",
	"enpass.exe", "lastpass.exe", "nordpass.exe", "keeper.exe", "roboform.exe",
	"proton pass.exe", "passwordsafe.exe", "credentialuibroker.exe", "consent.exe",
	"logonui.exe", "lockapp.exe",
)

// DefaultOff are remote-desktop and virtual-machine clients: the keys go to another
// machine whose layout we cannot see, so our reading of them would be a guess.
var DefaultOff = setOf(
	"mstsc.exe", "msrdc.exe", "vmconnect.exe", "virtualboxvm.exe", "vmware.exe",
	"vmplayer.exe", "anydesk.exe", "teamviewer.exe", "rustdesk.exe", "parsecd.exe",
)

// DefaultSuggestOnly are terminals, where automatic rewriting is more nuisance than help.
var DefaultSuggestOnly = setOf(
	"windowsterminal.exe", "cmd.exe", "powershell.exe", "pwsh.exe", "conhost.exe",
	"openconsole.exe", "wsl.exe", "bash.exe", "mintty.exe", "putty.exe", "kitty.exe",
	"alacritty.exe", "wezterm-gui.exe", "conemu.exe", "conemu64.exe", "mobaxterm.exe",
	"tabby.exe", "hyper.exe",
)

// TabCritical apps use Tab for completion, indentation or cell navigation; the accept key
// falls back to Insert there.
var TabCritical = setOf(
	"windowsterminal.exe", "cmd.exe", "powershell.exe", "pwsh.exe", "conhost.exe",
	"openconsole.exe", "wsl.exe", "bash.exe", "mintty.exe", "putty.exe", "kitty.exe",
	"alacritty.exe", "wezterm-gui.exe", "conemu.exe", "conemu64.exe", "mobaxterm.exe",
	"tabby.exe", "hyper.exe",
	"code.exe", "code - insiders.exe", "vscodium.exe", "cursor.exe", "windsurf.exe",
	"zed.exe", "devenv.exe", "idea64.exe", "pycharm64.exe", "webstorm64.exe", "clion64.exe",
	"rider64.exe", "goland64.exe", "phpstorm64.exe", "datagrip64.exe", "rustrover64.exe",
	"studio64.exe", "sublime_text.exe", "notepad++.exe", "excel.exe", "scalc.exe",
)

func setOf(items ...string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, item := range items {
		out[item] = true
	}
	return out
}

// NormalizeApp lowercases an executable name ("Chrome.EXE" → "chrome.exe").
func NormalizeApp(app string) string { return strings.ToLower(strings.TrimSpace(app)) }

// ModeFor is what the app may do in app. An unknown app ("") gets the default.
func (c Config) ModeFor(app string) FixMode {
	app = NormalizeApp(app)
	if app == "" {
		return c.DefaultFixMode
	}
	if AlwaysDisabled[app] {
		return ModeOff
	}
	if mode, ok := c.AppRules[app]; ok {
		return mode
	}
	if DefaultOff[app] {
		return ModeOff
	}
	if DefaultSuggestOnly[app] {
		return ModeSuggest
	}
	return c.DefaultFixMode
}

// HasRule reports whether the user set a mode for app explicitly.
func (c Config) HasRule(app string) (FixMode, bool) {
	mode, ok := c.AppRules[NormalizeApp(app)]
	return mode, ok
}

// SetMode sets (or with ok=false clears) the rule for an app. Locked apps are ignored.
func (c *Config) SetMode(app string, mode FixMode, ok bool) {
	app = NormalizeApp(app)
	if app == "" || AlwaysDisabled[app] {
		return
	}
	if !ok {
		delete(c.AppRules, app)
		return
	}
	if c.AppRules == nil {
		c.AppRules = map[string]FixMode{}
	}
	c.AppRules[app] = mode
}

// IsLocked reports an app that can never be enabled.
func IsLocked(app string) bool { return AlwaysDisabled[NormalizeApp(app)] }

// IsTerminal reports a terminal (the suggest-only list): Ctrl+C there is an interrupt, not
// a copy, so selection-based actions must not use it.
func IsTerminal(app string) bool { return DefaultSuggestOnly[NormalizeApp(app)] }

// IsTabCritical reports an app where Tab must not be taken.
func IsTabCritical(app string) bool { return TabCritical[NormalizeApp(app)] }

// RememberCyrillic records the Cyrillic layout last used in an app. Returns false when
// nothing changed, so callers can skip saving.
//
// This is the whole answer to "Russian or Kazakh?" when the letters cannot tell them
// apart: keep using whichever one this app was last written in.
func (c *Config) RememberCyrillic(layoutID, app string) bool {
	app = NormalizeApp(app)
	if !c.RememberLayoutPerApp || app == "" || layoutID == "" {
		return false
	}
	if c.AppCyrillicMemory[app] == layoutID {
		return false
	}
	if c.AppCyrillicMemory == nil {
		c.AppCyrillicMemory = map[string]string{}
	}
	c.AppCyrillicMemory[app] = layoutID
	return true
}

// RememberedCyrillic is the Cyrillic layout last used in app, or "".
func (c Config) RememberedCyrillic(app string) string {
	if !c.RememberLayoutPerApp {
		return ""
	}
	return c.AppCyrillicMemory[NormalizeApp(app)]
}

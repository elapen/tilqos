//go:build windows && (amd64 || arm64)

package winapp

// The tray menu: the counterpart of AppController.rebuildMenu(). Built fresh every time it
// opens, from the settings, the engine's status snapshot and the installed layouts.

import (
	"fmt"
	"strings"
	"time"

	"tilqos/internal/config"
	"tilqos/internal/core"
	"tilqos/internal/engine"
	"tilqos/internal/i18n"
	"tilqos/internal/translit"
	"tilqos/internal/win32"
)

const (
	cmdToggleEnabled = 100 + iota
	cmdToggleSmart
	cmdModeAuto
	cmdModeSuggest
	cmdLangEN
	cmdLangRU
	cmdLangKK
	cmdAppOff
	cmdAppSuggest
	cmdFixLastWord
	cmdFixSelection
	cmdUndo
	cmdConvertClipboard
	cmdTranslitAuto
	cmdTranslitToCyrillic
	cmdTranslitToLatin
	cmdSchemeOfficial
	cmdSchemePractical
	cmdSwitchEN
	cmdSwitchRU
	cmdSwitchKK
	cmdAutostart
	cmdOpenFolder
	cmdHelp
	cmdQuit
	cmdLearning
	cmdForgetLearned
	cmdClearExceptions
	cmdPrivacy
	cmdLicenses
	cmdLogToggle
	cmdLogClear
	// cmdSensitivity + index into config.SensitivityPresets.
	cmdSensitivity = 200
	// cmdCyrillic + index into config.AllCyrillicPreferences.
	cmdCyrillic = 220
	// cmdUILanguage + index into i18n.Settings.
	cmdUILanguage = 240
)

// sensitivityHints are the presets' explanations, short enough for a menu row.
func sensitivityHint(preset config.Sensitivity) string {
	switch preset {
	case config.SensitivityCautious:
		return i18n.T(i18n.SensitivityHintCautious)
	case config.SensitivityBalanced:
		return i18n.T(i18n.SensitivityHintBalanced)
	}
	return i18n.T(i18n.SensitivityHintBold)
}

type menuBuilder struct{ handle uintptr }

func newMenu() menuBuilder { return menuBuilder{win32.CreatePopupMenu()} }

func (m menuBuilder) item(id uintptr, text string, checked, enabled bool) {
	flags := uint32(win32.MF_STRING)
	if checked {
		flags |= win32.MF_CHECKED
	}
	if !enabled {
		flags |= win32.MF_GRAYED
	}
	win32.AppendMenu(m.handle, flags, id, text)
}

func (m menuBuilder) header(text string) { m.item(0, text, false, false) }

func (m menuBuilder) separator() { win32.AppendMenu(m.handle, win32.MF_SEPARATOR, 0, "") }

func (m menuBuilder) submenu(text string, sub menuBuilder, enabled bool) {
	flags := uint32(win32.MF_POPUP)
	if !enabled {
		flags |= win32.MF_GRAYED
	}
	win32.AppendMenu(m.handle, flags, sub.handle, text)
}

// withHotkey adds the binding on the right of a menu row, so it can be learned.
func withHotkey(text string, cfg config.Config, action config.Action) string {
	if spec := cfg.Hotkeys[string(action)]; spec != "" {
		return text + "\t" + spec
	}
	return text
}

func (a *App) showMenu() {
	cfg := a.store.Get()
	status := a.engine.Status()
	target := a.lastUserWindow.Load()
	app := ""
	if target != 0 {
		app = a.AppName(target)
	}

	menu := newMenu()
	defer win32.DestroyMenu(menu.handle) // destroys the attached submenus too

	layoutName := "—"
	if table := a.layouts.Table(LayoutID(currentLayout(target))); table != nil {
		layoutName = table.Name
	}
	menu.header(i18n.F(i18n.MenuLayout, layoutName))
	if a.hooks.Running() {
		menu.header(i18n.T(i18n.MenuHooksRunning))
	} else {
		menu.header(i18n.T(i18n.MenuHooksStopped))
	}
	menu.separator()

	menu.item(cmdToggleEnabled, withHotkey(i18n.T(i18n.MenuEnabled), cfg, config.ActionToggleEnabled), cfg.Enabled, true)
	menu.item(cmdToggleSmart, i18n.T(i18n.MenuSmartMode), cfg.SmartMode, cfg.Enabled)
	modes := newMenu()
	modes.item(cmdModeAuto, config.ModeAuto.DisplayName(), cfg.DefaultFixMode == config.ModeAuto, true)
	modes.item(cmdModeSuggest, config.ModeSuggest.DisplayName(), cfg.DefaultFixMode == config.ModeSuggest, true)
	menu.submenu(i18n.T(i18n.MenuDefaultMode), modes, cfg.Enabled)

	// How eagerly to correct, in words instead of two thresholds (the Settings presets of
	// the macOS app).
	sensitivity := newMenu()
	current := cfg.Sensitivity()
	for index, preset := range config.SensitivityPresets {
		sensitivity.item(cmdSensitivity+uintptr(index), preset.DisplayName()+" — "+sensitivityHint(preset), current == preset, true)
	}
	if current == config.SensitivityCustom {
		sensitivity.separator()
		sensitivity.header(i18n.F(i18n.MenuSensitivityCustom, cfg.AutoThreshold, cfg.SuggestThreshold,
			config.SensitivityCustom.Explanation()))
	}
	menu.submenu(i18n.F(i18n.MenuSensitivity, current.DisplayName()), sensitivity, cfg.Enabled)

	languages := newMenu()
	enabled := cfg.Languages()
	for _, entry := range []struct {
		id   uintptr
		lang core.Lang
	}{{cmdLangEN, core.LangEN}, {cmdLangRU, core.LangRU}, {cmdLangKK, core.LangKK}} {
		text := entry.lang.DisplayName()
		if a.layouts.FirstFor(entry.lang) == nil {
			text = i18n.F(i18n.MenuLayoutMissing, text)
		}
		languages.item(entry.id, text, enabled[entry.lang], true)
	}
	// All 33 Russian letters sit on the same keys in both Cyrillic layouts, so Russian-looking
	// text cannot say which of the two to switch to.
	languages.separator()
	languages.header(i18n.T(i18n.MenuCyrillicHeader))
	for index, preference := range config.AllCyrillicPreferences {
		languages.item(cmdCyrillic+uintptr(index), preference.DisplayName(), cfg.CyrillicPreference == preference, true)
	}
	menu.submenu(i18n.T(i18n.MenuLanguages), languages, true)

	// What was learned from the user. Only counts can be shown: the words are stored as
	// salted hashes, never as text.
	learner := a.engine.Learner()
	counts := learner.Counts()
	learning := newMenu()
	learning.item(cmdLearning, i18n.T(i18n.MenuLearningToggle), learner.Enabled(), true)
	learning.separator()
	learning.header(i18n.F(i18n.MenuLearningCounts1, counts.NeverConvert, counts.AlwaysConvert))
	learning.header(i18n.F(i18n.MenuLearningCounts2, counts.UserWords, counts.Pending))
	learning.separator()
	learning.item(cmdForgetLearned, i18n.T(i18n.MenuLearningReset), false, counts.Total() > 0)
	// The exception words are plain text in config.json, so they can be wiped from here too.
	learning.item(cmdClearExceptions, i18n.T(i18n.MenuClearExceptions), false, len(cfg.WordExceptions) > 0)
	menu.submenu(i18n.T(i18n.MenuLearning), learning, true)
	menu.separator()

	if app != "" {
		if config.IsLocked(app) {
			menu.header(i18n.F(i18n.MenuAppAlwaysOff, app))
		} else {
			rule, hasRule := cfg.HasRule(app)
			menu.item(cmdAppOff, i18n.F(i18n.MenuAppOff, app), hasRule && rule == config.ModeOff, true)
			menu.item(cmdAppSuggest, i18n.F(i18n.MenuAppSuggest, app), hasRule && rule == config.ModeSuggest, true)
		}
		menu.separator()
	}

	menu.item(cmdFixLastWord, withHotkey(i18n.T(i18n.ActFixLastWord), cfg, config.ActionFixLastWord), false, cfg.Enabled)
	menu.item(cmdFixSelection, withHotkey(i18n.T(i18n.ActFixSelection), cfg, config.ActionFixSelection), false, cfg.Enabled)
	menu.item(cmdUndo, withHotkey(i18n.T(i18n.ActUndoLastFix), cfg, config.ActionUndoLastFix), false, status.CanUndo)
	menu.item(cmdConvertClipboard, withHotkey(i18n.T(i18n.ActConvertClipboard), cfg, config.ActionConvertClipboard), false, cfg.Enabled)

	transliteration := newMenu()
	transliteration.item(cmdTranslitAuto, withHotkey(i18n.T(i18n.MenuTranslitAuto), cfg, config.ActionTransliterate), false, true)
	transliteration.item(cmdTranslitToCyrillic, i18n.T(i18n.MenuTranslitToCyrillic), false, true)
	transliteration.item(cmdTranslitToLatin, i18n.T(i18n.MenuTranslitToLatin), false, true)
	transliteration.separator()
	transliteration.header(i18n.T(i18n.MenuTranslitScheme))
	transliteration.item(cmdSchemeOfficial, translit.Official2021.DisplayName(), cfg.TranslitScheme == string(translit.Official2021), true)
	transliteration.item(cmdSchemePractical, translit.Practical.DisplayName(), cfg.TranslitScheme == string(translit.Practical), true)
	menu.submenu(i18n.T(i18n.MenuTransliteration), transliteration, cfg.Enabled)

	switcher := newMenu()
	currentLang := status.Language
	for _, entry := range []struct {
		id     uintptr
		lang   core.Lang
		action config.Action
	}{{cmdSwitchEN, core.LangEN, config.ActionForceEnglish}, {cmdSwitchRU, core.LangRU, config.ActionForceRussian}, {cmdSwitchKK, core.LangKK, config.ActionForceKazakh}} {
		text := withHotkey(fmt.Sprintf("%s   %s", entry.lang.DisplayName(), entry.lang.Badge()), cfg, entry.action)
		switcher.item(entry.id, text, currentLang == entry.lang, a.layouts.FirstFor(entry.lang) != nil)
	}
	menu.submenu(i18n.T(i18n.MenuSwitchLayout), switcher, true)
	menu.separator()

	menu.item(cmdAutostart, i18n.T(i18n.MenuAutostart), autostartEnabled(), true)
	menu.item(cmdOpenFolder, i18n.T(i18n.MenuOpenFolder), false, true)

	// The log is off unless asked for; what it holds is lengths, languages and decisions.
	logMenu := newMenu()
	logMenu.item(cmdLogToggle, i18n.T(i18n.MenuLogEnabled), cfg.Diagnostics, true)
	logMenu.item(cmdLogClear, i18n.T(i18n.MenuLogClear), false, true)
	menu.submenu(i18n.T(i18n.MenuLog), logMenu, true)
	menu.item(cmdHelp, i18n.T(i18n.MenuHotkeys), false, true)

	// Each language is named in itself, so it can be found whatever is showing now.
	interfaceLanguage := newMenu()
	for index, setting := range i18n.Settings {
		text := i18n.T(i18n.LanguageSystem)
		if lang, pinned := setting.Pinned(); pinned {
			text = core.LangFromCode(string(lang)).DisplayName()
		}
		interfaceLanguage.item(cmdUILanguage+uintptr(index), text, i18n.Setting(cfg.UILanguage) == setting, true)
	}
	menu.submenu(i18n.T(i18n.MenuInterfaceLanguage), interfaceLanguage, true)

	about := newMenu()
	about.header(i18n.F(i18n.MenuAboutVersion, Version))
	about.item(cmdPrivacy, i18n.T(i18n.MenuPrivacy), false, true)
	about.item(cmdLicenses, i18n.T(i18n.MenuLicenses), false, true)
	menu.submenu(i18n.T(i18n.MenuAbout), about, true)
	menu.separator()
	menu.item(cmdQuit, i18n.T(i18n.MenuQuit), false, true)

	// The classic tray-menu dance: the owner must be foreground for the menu to close when
	// the user clicks elsewhere, and a posted WM_NULL afterwards fixes a second-click glitch.
	win32.SetForegroundWindow(a.ui.hwnd)
	cursor, _ := win32.GetCursorPos()
	command := win32.TrackPopupMenu(menu.handle,
		win32.TPM_RETURNCMD|win32.TPM_RIGHTBUTTON|win32.TPM_NONOTIFY|win32.TPM_BOTTOMALIGN,
		cursor.X, cursor.Y, a.ui.hwnd)
	win32.PostMessage(a.ui.hwnd, win32.WM_NULL, 0, 0)
	if command != 0 {
		a.runCommand(command, target, app)
	}
}

func (a *App) update(fn func(*config.Config)) {
	if err := a.store.Update(fn); err != nil {
		a.log.Printf("saving settings failed: %v", err)
	}
}

func toggleLanguage(c *config.Config, lang core.Lang) {
	var kept []string
	found := false
	for _, code := range c.EnabledLanguages {
		if code == string(lang) {
			found = true
			continue
		}
		kept = append(kept, code)
	}
	if !found {
		kept = append(kept, string(lang))
	}
	// At least two languages are needed to have anything to switch between; refusing to
	// drop below one keeps the choice meaningful (an empty list would mean "all").
	if len(kept) == 0 {
		return
	}
	c.EnabledLanguages = kept
}

func (a *App) runCommand(command uint32, target uintptr, app string) {
	switch command {
	case cmdToggleEnabled:
		a.update(func(c *config.Config) { c.Enabled = !c.Enabled })
	case cmdToggleSmart:
		a.update(func(c *config.Config) { c.SmartMode = !c.SmartMode })
	case cmdModeAuto:
		a.update(func(c *config.Config) { c.DefaultFixMode = config.ModeAuto })
	case cmdModeSuggest:
		a.update(func(c *config.Config) { c.DefaultFixMode = config.ModeSuggest })
	case cmdLangEN:
		a.update(func(c *config.Config) { toggleLanguage(c, core.LangEN) })
	case cmdLangRU:
		a.update(func(c *config.Config) { toggleLanguage(c, core.LangRU) })
	case cmdLangKK:
		a.update(func(c *config.Config) { toggleLanguage(c, core.LangKK) })
	case cmdAppOff, cmdAppSuggest:
		wanted := config.ModeOff
		if command == cmdAppSuggest {
			wanted = config.ModeSuggest
		}
		a.update(func(c *config.Config) {
			if rule, ok := c.HasRule(app); ok && rule == wanted {
				c.SetMode(app, "", false)
			} else {
				c.SetMode(app, wanted, true)
			}
		})
	case cmdFixLastWord:
		a.onTarget(target, func(e *engine.Engine) { e.Perform(config.ActionFixLastWord, 0) })
	case cmdFixSelection:
		a.onTarget(target, func(e *engine.Engine) { e.FixSelection() })
	case cmdUndo:
		a.onTarget(target, func(e *engine.Engine) { e.UndoLastFix(0) })
	case cmdLearning:
		learner := a.engine.Learner()
		learner.SetEnabled(!learner.Enabled())
	case cmdForgetLearned:
		if confirm(i18n.T(i18n.ConfirmResetLearned)) {
			a.engine.Learner().ResetLearnedVocabulary()
		}
	case cmdClearExceptions:
		if confirm(i18n.T(i18n.ConfirmClearExceptions)) {
			if err := a.store.ClearExceptionWords(); err != nil {
				a.log.Printf("saving settings failed: %v", err)
			}
		}
	case cmdLogToggle:
		// The settings listener switches the logger on or off.
		a.update(func(c *config.Config) { c.Diagnostics = !c.Diagnostics })
	case cmdLogClear:
		if err := a.log.Clear(); err != nil {
			messageBox(i18n.F(i18n.MsgLogClearFailed, err.Error()), win32.MB_ICONWARNING)
		}
	case cmdPrivacy:
		messageBox(i18n.F(i18n.PrivacyText, a.dir), win32.MB_ICONINFORMATION)
	case cmdLicenses:
		a.showLicenses()
	case cmdConvertClipboard:
		a.onTarget(target, func(e *engine.Engine) { e.ConvertClipboard() })
	case cmdTranslitAuto:
		a.onTarget(target, func(e *engine.Engine) { e.Transliterate(nil, 0) })
	case cmdTranslitToCyrillic:
		direction := translit.ToCyrillic
		a.onTarget(target, func(e *engine.Engine) { e.Transliterate(&direction, 0) })
	case cmdTranslitToLatin:
		direction := translit.ToLatin
		a.onTarget(target, func(e *engine.Engine) { e.Transliterate(&direction, 0) })
	case cmdSchemeOfficial:
		a.update(func(c *config.Config) { c.TranslitScheme = string(translit.Official2021) })
	case cmdSchemePractical:
		a.update(func(c *config.Config) { c.TranslitScheme = string(translit.Practical) })
	case cmdSwitchEN:
		a.onTarget(target, func(e *engine.Engine) { e.Perform(config.ActionForceEnglish, 0) })
	case cmdSwitchRU:
		a.onTarget(target, func(e *engine.Engine) { e.Perform(config.ActionForceRussian, 0) })
	case cmdSwitchKK:
		a.onTarget(target, func(e *engine.Engine) { e.Perform(config.ActionForceKazakh, 0) })
	case cmdAutostart:
		if err := setAutostart(!autostartEnabled()); err != nil {
			messageBox(i18n.F(i18n.MsgAutostartFailed, err.Error()), win32.MB_ICONWARNING)
		}
	case cmdOpenFolder:
		win32.ShellOpen(a.dir)
	case cmdHelp:
		messageBox(a.helpText(), win32.MB_ICONINFORMATION)
	case cmdQuit:
		a.ui.Quit()
	default:
		switch {
		case command >= cmdSensitivity && command < cmdSensitivity+uint32(len(config.SensitivityPresets)):
			preset := config.SensitivityPresets[command-cmdSensitivity]
			a.update(func(c *config.Config) { c.SetSensitivity(preset) })
		case command >= cmdCyrillic && command < cmdCyrillic+uint32(len(config.AllCyrillicPreferences)):
			preference := config.AllCyrillicPreferences[command-cmdCyrillic]
			a.update(func(c *config.Config) { c.CyrillicPreference = preference })
		case command >= cmdUILanguage && command < cmdUILanguage+uint32(len(i18n.Settings)):
			setting := i18n.Settings[command-cmdUILanguage]
			// Applied by the settings listener at once: the next menu is already in the new language.
			a.update(func(c *config.Config) { c.UILanguage = string(setting) })
		}
	}
}

// onTarget runs an action against the application the user was in before opening the
// menu: the menu took the foreground, so it is handed back first.
func (a *App) onTarget(target uintptr, fn func(*engine.Engine)) {
	if target != 0 {
		win32.SetForegroundWindow(target)
	}
	a.send(func(e *engine.Engine) {
		time.Sleep(150 * time.Millisecond) // let the window activate before sending it keys
		if target != 0 {
			e.Foreground(target)
		}
		fn(e)
	})
}

func (a *App) helpText() string {
	cfg := a.store.Get()
	var b strings.Builder
	b.WriteString(i18n.T(i18n.HelpTitle) + "\n\n")
	for _, action := range config.AllActions {
		spec := cfg.Hotkeys[string(action)]
		if spec == "" {
			spec = "—"
		}
		fmt.Fprintf(&b, "%-16s  %s\n", spec, action.DisplayName())
	}
	accept := "Tab"
	if cfg.AcceptKey == config.AcceptInsert {
		accept = "Insert"
	}
	b.WriteString("\n" + i18n.F(i18n.HelpAccept, accept) + "\n")
	if fixKey := cfg.Hotkeys[string(config.ActionFixLastWord)]; fixKey != "" {
		// FixLastWordOrUndo: pressed again right after a fix, the same key takes it back.
		b.WriteString(i18n.F(i18n.HelpUndoAgain, fixKey) + "\n")
	}
	b.WriteString("\n" + i18n.F(i18n.HelpFooter, a.store.Path()))
	return b.String()
}

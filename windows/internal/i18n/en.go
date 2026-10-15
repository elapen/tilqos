package i18n

// English: the source language and the fallback.
var english = [numKeys]string{

	// Tray menu: header, state, modes
	MenuLayout:       "Layout: %s",
	MenuHooksRunning: "Keyboard monitoring: running",
	MenuHooksStopped: "⚠ Keyboard monitoring: stopped",
	MenuEnabled:      "On",
	MenuSmartMode:    "Smart mode (detect automatically)",
	MenuDefaultMode:  "Default mode",
	ModeAuto:         "Automatic correction",
	ModeSuggest:      "Suggestions only",
	ModeOff:          "Off",

	// Sensitivity
	MenuSensitivity:            "How boldly to correct: %s",
	MenuSensitivityCustom:      "Currently custom (%.2f / %.2f) — %s",
	SensitivityCautious:        "Cautious",
	SensitivityBalanced:        "Balanced",
	SensitivityBold:            "Bold",
	SensitivityCustom:          "Custom",
	SensitivityHintCautious:    "corrects only obvious mistakes by itself, offers the rest",
	SensitivityHintBalanced:    "corrects obvious mistakes by itself, offers the doubtful ones",
	SensitivityHintBold:        "corrects almost anything that looks like a mistake",
	SensitivityExplainCautious: "Corrects only obvious mistakes by itself and offers the rest. The fewest surprise replacements.",
	SensitivityExplainBalanced: "Corrects obvious mistakes by itself and offers the doubtful ones. Right for most people.",
	SensitivityExplainBold:     "Corrects almost anything that looks like a mistake by itself. More help, but now and then an unwanted replacement.",
	SensitivityExplainCustom:   "The thresholds are set by hand in config.json (autoThreshold, suggestThreshold).",

	// Languages
	MenuLanguages:      "Languages",
	MenuLayoutMissing:  "%s — layout not added in Windows",
	MenuCyrillicHeader: "Russian and Kazakh — when the letters can’t tell them apart:",
	CyrillicAutomatic:  "Auto + remember per app",
	CyrillicKazakh:     "Prefer Kazakh",
	CyrillicRussian:    "Prefer Russian",

	// Learning and stored words
	MenuLearning:           "Learning",
	MenuLearningToggle:     "Learn from my actions",
	MenuLearningCounts1:    "Stop correcting: %d · Correct without asking: %d",
	MenuLearningCounts2:    "Your words: %d · Watching: %d",
	MenuLearningReset:      "Reset learned words…",
	MenuClearExceptions:    "Clear all exception words…",
	ConfirmResetLearned:    "Reset everything Tilqos has learned from your actions?\n\nThe exception words stay.",
	ConfirmClearExceptions: "Clear all exception words?\n\nTilqos will correct these words again. The learned words stay as they are.",

	// Per-app items
	MenuAppAlwaysOff: "“%s” — always off",
	MenuAppOff:       "Don’t correct in “%s”",
	MenuAppSuggest:   "Suggestions only in “%s”",

	// Actions (menu rows and the hotkeys list)
	ActFixLastWord:      "Fix the last word",
	ActFixSelection:     "Fix the selection",
	ActUndoLastFix:      "Undo the last correction",
	ActSwitchTo:         "Switch to %s",
	ActConvertClipboard: "Convert the clipboard",
	ActTransliterate:    "Transliteration (Latin ⇄ Cyrillic)",
	ActToggleEnabled:    "Turn on / off",

	// Transliteration submenu
	MenuTransliteration:    "Transliteration",
	MenuTranslitAuto:       "Detect the direction",
	MenuTranslitToCyrillic: "Latin → Cyrillic",
	MenuTranslitToLatin:    "Cyrillic → Latin",
	MenuTranslitScheme:     "Latin scheme",
	SchemeOfficial:         "Official Latin 2021",
	SchemePractical:        "Practical (ASCII)",
	SchemeOfficialDetail:   "31 letters with diacritics: ä ğ ñ ö ū ü ş. Reversible one to one.",
	SchemePracticalDetail:  "No diacritics, the way people write in chats. Going back uses a language model.",

	// Bottom of the menu
	MenuSwitchLayout:      "Switch layout",
	MenuAutostart:         "Start with Windows",
	MenuOpenFolder:        "Open the settings and log folder",
	MenuHotkeys:           "Keyboard shortcuts…",
	MenuLog:               "Log",
	MenuLogEnabled:        "Write a log (off by default)",
	MenuLogClear:          "Delete the log file",
	MsgLogClearFailed:     "Couldn’t delete the log file: %s",
	MenuInterfaceLanguage: "Interface language",
	LanguageSystem:        "Same as system",
	MenuAbout:             "About",
	MenuAboutVersion:      "Tilqos %s",
	MenuPrivacy:           "Privacy…",
	MenuLicenses:          "Licenses…",
	MenuQuit:              "Exit",
	TrayEnabled:           "Tilqos — on",
	TrayDisabled:          "Tilqos — off",

	// Dialogs and notifications
	MsgAlreadyRunning:          "Tilqos is already running — its icon is in the notification area.",
	MsgModelsMissing:           "The language models are not embedded in this build:\n\n%s\n\nBuild the program with build.ps1 or build.sh.",
	MsgWindowFailed:            "Couldn’t create the window: %s",
	MsgAutostartFailed:         "Couldn’t change the startup setting: %s",
	BalloonHookTitle:           "Keyboard monitoring isn’t working",
	BalloonHookText:            "Couldn’t install the keyboard hook: %s",
	BalloonSettingsReset:       "Settings were reset",
	BalloonWelcomeTitle:        "Tilqos is running",
	BalloonWelcomeText:         "It corrects text typed in the wrong layout. %s — fix the word, %s — fix the selection, %s — undo. The menu is on the tray icon.",
	NotifyUndoUnavailableTitle: "Undo isn’t available",
	NotifyUndoUnavailableText:  "The text has changed since the correction, so undoing it blindly isn’t safe.",

	// Hotkeys window
	HelpTitle:     "Tilqos hotkeys",
	HelpAccept:    "%s — accept a suggestion (in terminals and editors: Insert).",
	HelpUndoAgain: "%s right after a correction takes it back.",
	HelpFooter:    "Keys are changed in %s\n(section \"hotkeys\", for example \"Ctrl+Alt+Z\").",

	// Privacy and licenses
	PrivacyText:     "Tilqos watches the keys you press so that it can notice a wrong layout. This is what it does with them:\n\n• No network access, no telemetry, no update checks.\n• What you type is kept in memory only. It is cleared after at most 5 seconds without typing, when you switch apps or windows, at Enter, Tab, Esc and similar keys, in password fields, and when the PC locks or goes to sleep.\n• “Undo” keeps the latest corrections in memory for at most 2 minutes, and wipes them when you switch apps or windows, lock or sleep the PC, or quit.\n• The clipboard is used only by actions that need it, and its previous contents are restored. Concealed or temporary content, such as from password managers, is never read: the action silently does nothing. Text over 10,000 characters is ignored.\n• Typed text, clipboard contents and window titles are never written to a log. The log is off by default; when it is on it holds lengths, languages and decisions, in the folder below.\n• Stored in %s: your settings, per-app rules, the exception words you add (plain text) and learned words (salted 64-bit hashes; this is not encryption, short common words could be guessed by someone who can read the file).\n• Windows only recognizes standard password boxes. Password fields in browsers and in custom-drawn controls may not be detected.",
	LicensesMissing: "The license texts are not part of this build. They are in the project’s source repository.",
	LicensesFailed:  "Couldn’t open the license texts: %s",
}

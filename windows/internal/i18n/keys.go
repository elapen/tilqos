// Keys of the user-visible texts. To add one: a constant here, then a text in en.go, ru.go and kk.go.

package i18n

// Key names one user-visible text. Keys are typed so that a call with a misspelt name does not compile;
// TestEveryKeyHasATextInEveryLanguage fails when a language lacks a text for any of them.
type Key int

const (
	keyInvalid Key = iota

	// Tray menu: header, state, modes
	MenuLayout
	MenuHooksRunning
	MenuHooksStopped
	MenuEnabled
	MenuSmartMode
	MenuDefaultMode
	ModeAuto
	ModeSuggest
	ModeOff

	// Sensitivity
	MenuSensitivity
	MenuSensitivityCustom
	SensitivityCautious
	SensitivityBalanced
	SensitivityBold
	SensitivityCustom
	SensitivityHintCautious
	SensitivityHintBalanced
	SensitivityHintBold
	SensitivityExplainCautious
	SensitivityExplainBalanced
	SensitivityExplainBold
	SensitivityExplainCustom

	// Languages
	MenuLanguages
	MenuLayoutMissing
	MenuCyrillicHeader
	CyrillicAutomatic
	CyrillicKazakh
	CyrillicRussian

	// Learning and stored words
	MenuLearning
	MenuLearningToggle
	MenuLearningCounts1
	MenuLearningCounts2
	MenuLearningReset
	MenuClearExceptions
	ConfirmResetLearned
	ConfirmClearExceptions

	// Per-app items
	MenuAppAlwaysOff
	MenuAppOff
	MenuAppSuggest

	// Actions (menu rows and the hotkeys list)
	ActFixLastWord
	ActFixSelection
	ActUndoLastFix
	ActSwitchTo
	ActConvertClipboard
	ActTransliterate
	ActToggleEnabled

	// Transliteration submenu
	MenuTransliteration
	MenuTranslitAuto
	MenuTranslitToCyrillic
	MenuTranslitToLatin
	MenuTranslitScheme
	SchemeOfficial
	SchemePractical
	SchemeOfficialDetail
	SchemePracticalDetail

	// Bottom of the menu
	MenuSwitchLayout
	MenuAutostart
	MenuOpenFolder
	MenuHotkeys
	MenuLog
	MenuLogEnabled
	MenuLogClear
	MsgLogClearFailed
	MenuInterfaceLanguage
	LanguageSystem
	MenuAbout
	MenuAboutVersion
	MenuPrivacy
	MenuLicenses
	MenuQuit
	TrayEnabled
	TrayDisabled

	// Dialogs and notifications
	MsgAlreadyRunning
	MsgModelsMissing
	MsgWindowFailed
	MsgAutostartFailed
	BalloonHookTitle
	BalloonHookText
	BalloonSettingsReset
	BalloonWelcomeTitle
	BalloonWelcomeText
	NotifyUndoUnavailableTitle
	NotifyUndoUnavailableText

	// Hotkeys window
	HelpTitle
	HelpAccept
	HelpUndoAgain
	HelpFooter

	// Privacy and licenses
	PrivacyText
	LicensesMissing
	LicensesFailed

	numKeys
)

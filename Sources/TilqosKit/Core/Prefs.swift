import Foundation

/// What the app is allowed to do in a given context.
public enum FixMode: String, CaseIterable, Codable, Sendable {
    /// Watch nothing, change nothing.
    case off
    /// Only offer a correction the user accepts explicitly.
    case suggest
    /// Apply confident corrections immediately, offer the rest.
    case auto

    public var displayName: String {
        switch self {
        case .off: return L10n.tr("mode.off")
        case .suggest: return L10n.tr("mode.suggest")
        case .auto: return L10n.tr("mode.auto")
        }
    }
}

/// Key used to accept a pending suggestion.
public enum AcceptKey: String, CaseIterable, Codable, Sendable {
    case tab
    case optionSpace

    public var displayName: String {
        switch self {
        case .tab: return "Tab"
        case .optionSpace: return "⌥Space"
        }
    }
}

/// Which Cyrillic layout to prefer when the letters alone cannot tell Russian from
/// Kazakh apart.
public enum CyrillicPreference: String, CaseIterable, Codable, Sendable {
    /// Do not switch without a positive signal; remember the last Cyrillic layout per app.
    case automatic
    case preferKazakh
    case preferRussian

    public var displayName: String {
        switch self {
        case .automatic: return L10n.tr("cyrillic.automatic")
        case .preferKazakh: return L10n.tr("cyrillic.kazakh")
        case .preferRussian: return L10n.tr("cyrillic.russian")
        }
    }
}

/// User-visible settings, backed by `UserDefaults`.
///
/// Reads happen on the event path, so every accessor is a plain `UserDefaults`
/// lookup with no formatting or allocation.
public final class Prefs {
    public static let shared = Prefs()

    /// Posted whenever any setting changes, so the status item and the tap can react.
    public static let didChange = Notification.Name("TilqosPrefsDidChange")

    private let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
        defaults.register(defaults: [
            Key.enabled.rawValue: true,
            Key.smartMode.rawValue: true,
            Key.defaultFixMode.rawValue: FixMode.auto.rawValue,
            Key.autoThreshold.rawValue: 0.80,
            Key.suggestThreshold.rawValue: 0.45,
            Key.showLayoutBadge.rawValue: true,
            Key.playSound.rawValue: false,
            Key.showHUD.rawValue: false,
            Key.acceptKey.rawValue: AcceptKey.tab.rawValue,
            Key.cyrillicPreference.rawValue: CyrillicPreference.automatic.rawValue,
            Key.minWordLength.rawValue: 3,
            Key.rememberLayoutPerApp.rawValue: true,
            Key.suggestionTimeout.rawValue: 4.0,
            Key.bufferIdleReset.rawValue: 5.0,
            Key.enabledLanguages.rawValue: Lang.allCases.map(\.rawValue),
            Key.diagnosticsEnabled.rawValue: false,
        ])
    }

    enum Key: String {
        case enabled
        case smartMode
        case defaultFixMode
        case autoThreshold
        case suggestThreshold
        case showLayoutBadge
        case playSound
        case showHUD
        case acceptKey
        case cyrillicPreference
        case minWordLength
        case rememberLayoutPerApp
        case suggestionTimeout
        case bufferIdleReset
        case enabledLanguages
        case diagnosticsEnabled
        case wordExceptions
        case appRules
        case appCyrillicMemory
        case didCompleteOnboarding
        case didCompleteTutorial
        case hotkeys
        case translitScheme
        case translitOverrides
        case translitNeverConvert
        case suggestionTipsShown
        case uiLanguage
    }

    // MARK: - Transliteration

    public var translitScheme: TranslitScheme {
        get {
            TranslitScheme(rawValue: defaults.string(forKey: Key.translitScheme.rawValue) ?? "")
                ?? .practical
        }
        set { set(newValue.rawValue, .translitScheme) }
    }

    /// User corrections for words the decoder reads wrongly, keyed by the lowercased Latin
    /// spelling. Some Kazakh endings are genuinely ambiguous — `jatqanyn` is both жатқанын and
    /// жатқаның — so there has to be a way to pin them.
    public var translitOverrides: [String: String] {
        get {
            defaults.dictionary(forKey: Key.translitOverrides.rawValue) as? [String: String] ?? [:]
        }
        set { set(newValue, .translitOverrides) }
    }

    public func addTranslitOverride(latin: String, cyrillic: String) {
        var current = translitOverrides
        current[latin.lowercased()] = cyrillic
        translitOverrides = current
    }

    /// Words to leave in Latin, learned from the user undoing a transliteration.
    ///
    /// Brand names cannot be recognised by any model — measured — so the app watches what the
    /// user rejects instead of guessing.
    public var translitNeverConvert: Set<String> {
        get { Set(defaults.stringArray(forKey: Key.translitNeverConvert.rawValue) ?? []) }
        set { set(Array(newValue), .translitNeverConvert) }
    }

    public func addNeverConvert(_ words: [String]) {
        guard !words.isEmpty else { return }
        var current = translitNeverConvert
        for word in words { current.insert(word.lowercased()) }
        translitNeverConvert = current
    }

    /// Deletes the list of Latin words to leave alone (plain text on disk; see `clearExceptionWords`).
    public func clearNeverConvertWords() {
        set(nil, .translitNeverConvert)
    }

    /// Deletes the pinned transliteration spellings (plain text on disk).
    public func clearTranslitOverrides() {
        set(nil, .translitOverrides)
    }

    // MARK: - Hotkeys

    /// Bindings, falling back to each action's default when unset.
    public var hotkeys: [HotkeyAction: Hotkey] {
        get {
            var result: [HotkeyAction: Hotkey] = [:]
            for action in HotkeyAction.allCases { result[action] = action.defaultHotkey }
            guard let data = defaults.data(forKey: Key.hotkeys.rawValue),
                let stored = try? JSONDecoder().decode([String: Hotkey].self, from: data)
            else { return result }
            for (raw, hotkey) in stored {
                guard let action = HotkeyAction(rawValue: raw), hotkey.isValid else { continue }
                result[action] = hotkey
            }
            return result
        }
        set {
            var stored: [String: Hotkey] = [:]
            for (action, hotkey) in newValue where hotkey.isValid {
                stored[action.rawValue] = hotkey
            }
            guard let data = try? JSONEncoder().encode(stored) else { return }
            set(data, .hotkeys)
        }
    }

    public func setHotkey(_ hotkey: Hotkey?, for action: HotkeyAction) {
        var current = hotkeys
        current[action] = hotkey ?? action.defaultHotkey
        hotkeys = current
    }

    private func set(_ value: Any?, _ key: Key) {
        defaults.set(value, forKey: key.rawValue)
        NotificationCenter.default.post(name: Self.didChange, object: nil)
    }

    // MARK: - Master switches

    /// Master switch. When false the tap stays installed but drops every event, which
    /// keeps the permission state warm and makes re-enabling instant.
    public var enabled: Bool {
        get { defaults.bool(forKey: Key.enabled.rawValue) }
        set { set(newValue, .enabled) }
    }

    /// When false, only the manual hotkeys work — no automatic detection at all.
    public var smartMode: Bool {
        get { defaults.bool(forKey: Key.smartMode.rawValue) }
        set { set(newValue, .smartMode) }
    }

    public var defaultFixMode: FixMode {
        get {
            FixMode(rawValue: defaults.string(forKey: Key.defaultFixMode.rawValue) ?? "") ?? .auto
        }
        set { set(newValue.rawValue, .defaultFixMode) }
    }

    // MARK: - Detector tuning

    /// Confidence at or above which a correction is applied without asking.
    public var autoThreshold: Double {
        get { defaults.double(forKey: Key.autoThreshold.rawValue) }
        set { set(newValue, .autoThreshold) }
    }

    /// Confidence at or above which a correction is offered as a suggestion.
    public var suggestThreshold: Double {
        get { defaults.double(forKey: Key.suggestThreshold.rawValue) }
        set { set(newValue, .suggestThreshold) }
    }

    /// Words shorter than this are never touched — too little signal, too much risk.
    public var minWordLength: Int {
        get { max(2, defaults.integer(forKey: Key.minWordLength.rawValue)) }
        set { set(newValue, .minWordLength) }
    }

    public var enabledLanguages: Set<Lang> {
        get {
            let raw = defaults.stringArray(forKey: Key.enabledLanguages.rawValue) ?? []
            let langs = raw.compactMap(Lang.init(rawValue:))
            return langs.isEmpty ? Set(Lang.allCases) : Set(langs)
        }
        set { set(newValue.map(\.rawValue), .enabledLanguages) }
    }

    public var cyrillicPreference: CyrillicPreference {
        get {
            CyrillicPreference(
                rawValue: defaults.string(forKey: Key.cyrillicPreference.rawValue) ?? "")
                ?? .automatic
        }
        set { set(newValue.rawValue, .cyrillicPreference) }
    }

    // MARK: - Interaction

    public var acceptKey: AcceptKey {
        get { AcceptKey(rawValue: defaults.string(forKey: Key.acceptKey.rawValue) ?? "") ?? .tab }
        set { set(newValue.rawValue, .acceptKey) }
    }

    /// How long a suggestion stays on screen before it gives up.
    public var suggestionTimeout: TimeInterval {
        get { defaults.double(forKey: Key.suggestionTimeout.rawValue) }
        set { set(newValue, .suggestionTimeout) }
    }

    /// How many suggestions have been shown with the "Tab — replace" explanation. The hint
    /// explains its keys the first few times only, then stays compact.
    public var suggestionTipsShown: Int {
        get { defaults.integer(forKey: Key.suggestionTipsShown.rawValue) }
        set { set(newValue, .suggestionTipsShown) }
    }

    /// Idle time after which the keystroke buffer is discarded. Always between
    /// `StrokeBuffer.minimumIdleReset` and `StrokeBuffer.maximumIdleReset` (five seconds):
    /// a value outside that range, from a hand-edited plist say, is pulled in so the promise made
    /// to the user holds.
    public var bufferIdleReset: TimeInterval {
        get { StrokeBuffer.clamped(defaults.double(forKey: Key.bufferIdleReset.rawValue)) }
        set { set(newValue, .bufferIdleReset) }
    }

    // MARK: - Presentation

    /// Language of the app's own text; `.system` follows macOS (see `L10n`).
    public var uiLanguage: UILanguageSetting {
        get { UILanguageSetting(rawValue: defaults.string(forKey: Key.uiLanguage.rawValue) ?? "") ?? .system }
        set { set(newValue.rawValue, .uiLanguage) }
    }

    public var showLayoutBadge: Bool {
        get { defaults.bool(forKey: Key.showLayoutBadge.rawValue) }
        set { set(newValue, .showLayoutBadge) }
    }

    public var playSound: Bool {
        get { defaults.bool(forKey: Key.playSound.rawValue) }
        set { set(newValue, .playSound) }
    }

    public var showHUD: Bool {
        get { defaults.bool(forKey: Key.showHUD.rawValue) }
        set { set(newValue, .showHUD) }
    }

    // MARK: - Behaviour

    public var rememberLayoutPerApp: Bool {
        get { defaults.bool(forKey: Key.rememberLayoutPerApp.rawValue) }
        set { set(newValue, .rememberLayoutPerApp) }
    }

    /// Whether the app writes its decision trail (lengths, languages, confidences, routes; never
    /// text) to the unified log. Off by default: with it off only warnings and errors are logged.
    /// `Pipeline` applies the value to `Log.diagnosticsEnabled`.
    public var diagnosticsEnabled: Bool {
        get { defaults.bool(forKey: Key.diagnosticsEnabled.rawValue) }
        set {
            set(newValue, .diagnosticsEnabled)
            Log.diagnosticsEnabled = newValue
        }
    }

    public var didCompleteOnboarding: Bool {
        get { defaults.bool(forKey: Key.didCompleteOnboarding.rawValue) }
        set { set(newValue, .didCompleteOnboarding) }
    }

    public var didCompleteTutorial: Bool {
        get { defaults.bool(forKey: Key.didCompleteTutorial.rawValue) }
        set { set(newValue, .didCompleteTutorial) }
    }

    // MARK: - Word exceptions

    /// Words the user has told us to leave alone. Stored lowercased.
    public var wordExceptions: Set<String> {
        get { Set(defaults.stringArray(forKey: Key.wordExceptions.rawValue) ?? []) }
        set { set(Array(newValue), .wordExceptions) }
    }

    public func addWordException(_ word: String) {
        var current = wordExceptions
        current.insert(word.lowercased())
        wordExceptions = current
    }

    /// Deletes every word the user added to the exceptions, including the ones the app added
    /// when a correction was undone. They are stored as plain text, so this removes the key
    /// from the preferences domain rather than writing an empty list.
    public func clearExceptionWords() {
        set(nil, .wordExceptions)
    }

    // MARK: - Per-app storage

    /// bundle ID → `FixMode`.
    public var appRules: [String: String] {
        get { defaults.dictionary(forKey: Key.appRules.rawValue) as? [String: String] ?? [:] }
        set { set(newValue, .appRules) }
    }

    /// bundle ID → input source ID of the Cyrillic layout last used there.
    public var appCyrillicMemory: [String: String] {
        get {
            defaults.dictionary(forKey: Key.appCyrillicMemory.rawValue) as? [String: String] ?? [:]
        }
        set { set(newValue, .appCyrillicMemory) }
    }

    /// Deletes the per-app modes the user chose (the apps Tilqos leaves alone, and so on).
    public func clearAppRules() {
        set(nil, .appRules)
    }

    /// Deletes the memory of which Cyrillic layout was last used in which app.
    public func clearRememberedLayouts() {
        set(nil, .appCyrillicMemory)
    }
}

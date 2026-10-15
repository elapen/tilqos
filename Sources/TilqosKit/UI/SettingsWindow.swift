import AppKit
import ServiceManagement
import SwiftUI

/// The sections of the Settings window, in sidebar order.
enum SettingsPane: String, CaseIterable, Identifiable {
    case general
    case correction
    case languages
    case transliteration
    case apps
    case exceptions
    case hotkeys
    case diagnostics
    case about

    var id: String { rawValue }

    var title: String {
        switch self {
        case .general: return L10n.tr("pane.general")
        case .correction: return L10n.tr("pane.correction")
        case .languages: return L10n.tr("pane.languages")
        case .transliteration: return L10n.tr("pane.transliteration")
        case .apps: return L10n.tr("pane.apps")
        case .exceptions: return L10n.tr("pane.exceptions")
        case .hotkeys: return L10n.tr("pane.hotkeys")
        case .diagnostics: return L10n.tr("pane.diagnostics")
        case .about: return L10n.tr("pane.about")
        }
    }

    var symbol: String {
        switch self {
        case .general: return "gearshape.fill"
        case .correction: return "wand.and.stars"
        case .languages: return "globe"
        case .transliteration: return "character.textbox"
        case .apps: return "square.grid.2x2.fill"
        case .exceptions: return "nosign"
        case .hotkeys: return "command"
        case .diagnostics: return "stethoscope"
        case .about: return "info.circle.fill"
        }
    }

    var color: Color {
        switch self {
        case .general: return .gray
        case .correction: return .indigo
        case .languages: return .blue
        case .transliteration: return .teal
        case .apps: return .orange
        case .exceptions: return .red
        case .hotkeys: return .purple
        case .diagnostics: return .green
        case .about: return .mint
        }
    }
}

/// Observable wrapper so SwiftUI controls can bind straight to `Prefs`.
@MainActor
final class SettingsModel: ObservableObject {
    let prefs: Prefs
    let rules: AppRules
    let inputSources: InputSourceManager
    let pipeline: Pipeline?

    @Published var pane: SettingsPane = .general
    /// Why the last launch-at-login change failed, shown under the switch.
    @Published var launchAtLoginProblem: String?

    private var observers: [NSObjectProtocol] = []

    init(prefs: Prefs, inputSources: InputSourceManager, pipeline: Pipeline?) {
        self.prefs = prefs
        self.rules = AppRules(prefs: prefs)
        self.inputSources = inputSources
        self.pipeline = pipeline

        // Settings changed elsewhere — the menu switch, a hotkey — must show up here too, and
        // permission changes are noticed when the user comes back from System Settings.
        let center = NotificationCenter.default
        for name in [
            Prefs.didChange, Learner.didChange, NSApplication.didBecomeActiveNotification,
            L10n.didChange,
        ] {
            observers.append(
                center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                    MainActor.assumeIsolated { self?.objectWillChange.send() }
                })
        }
    }

    deinit {
        for observer in observers { NotificationCenter.default.removeObserver(observer) }
    }

    func binding<Value>(
        _ get: @escaping (Prefs) -> Value, _ set: @escaping (Prefs, Value) -> Void
    ) -> Binding<Value> {
        Binding(
            get: { get(self.prefs) },
            set: { value in
                set(self.prefs, value)
                self.objectWillChange.send()
            })
    }

    func refresh() {
        objectWillChange.send()
    }

    /// The interface language. Applied at once: the whole window is built from `L10n` on every
    /// redraw, so nothing needs to be restarted.
    var uiLanguage: Binding<UILanguageSetting> {
        Binding(
            get: { self.prefs.uiLanguage },
            set: { value in
                self.prefs.uiLanguage = value
                L10n.setting = value
                self.objectWillChange.send()
            })
    }

    // MARK: - State

    /// Where live state is read from. Replaced in UI snapshots, which run without a tap or
    /// permissions of their own.
    var readPermissions: () -> Permissions.Status = { Permissions.status }
    var readTapRunning: (() -> Bool)?

    var permissions: Permissions.Status { readPermissions() }
    var tapRunning: Bool { readTapRunning?() ?? pipeline?.isTapRunning ?? false }
    var health: AppHealth {
        AppHealth.evaluate(
            enabled: prefs.enabled, smartMode: prefs.smartMode, permissions: permissions,
            tapRunning: tapRunning)
    }
    var modelLanguages: Set<Lang> { ModelStore.shared.availableLanguages }
    var learnedStrategies: [String: Injector.Strategy] { pipeline?.learnedStrategies ?? [:] }
    /// What the engine has learned from the user. The pipeline's own instance when there is
    /// one, so the window and the engine can never look at different stores. Replaced in UI
    /// snapshots, which must not read or write the real store.
    lazy var learner: Learner = pipeline?.learner ?? .shared

    /// Replaced in UI snapshots, where the host process is the test runner.
    var readVersion: () -> String = {
        Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "—"
    }
    var version: String { readVersion() }

    func restartTap() {
        guard let pipeline else { return }
        pipeline.tap.restart()
        if !pipeline.isTapRunning { pipeline.start() }
        objectWillChange.send()
    }

    // MARK: - Launch at login

    var launchAtLogin: Bool { SMAppService.mainApp.status == .enabled }

    /// The system can hold the item back for approval; that is not a failure, but the user
    /// has to finish it in System Settings.
    var launchAtLoginNeedsApproval: Bool { SMAppService.mainApp.status == .requiresApproval }

    func setLaunchAtLogin(_ on: Bool) {
        do {
            on ? try SMAppService.mainApp.register() : try SMAppService.mainApp.unregister()
            launchAtLoginProblem = nil
        } catch {
            Log.error(.ui, "launch at login: \(error.localizedDescription)")
            launchAtLoginProblem = L10n.tr("general.launch.failure", error.localizedDescription)
        }
        objectWillChange.send()
    }

    // MARK: - Sensitivity

    var sensitivity: Sensitivity {
        get { Sensitivity.matching(auto: prefs.autoThreshold, suggest: prefs.suggestThreshold) }
        set {
            guard let values = newValue.thresholds else { return }
            prefs.autoThreshold = values.auto
            prefs.suggestThreshold = values.suggest
            objectWillChange.send()
        }
    }

    /// The two thresholds stay ordered: offering a fix needs less confidence than making it.
    var autoThreshold: Binding<Double> {
        Binding(
            get: { self.prefs.autoThreshold },
            set: { value in
                self.prefs.autoThreshold = value
                if self.prefs.suggestThreshold > value { self.prefs.suggestThreshold = value }
                self.objectWillChange.send()
            })
    }

    var suggestThreshold: Binding<Double> {
        Binding(
            get: { self.prefs.suggestThreshold },
            set: { value in
                self.prefs.suggestThreshold = value
                if self.prefs.autoThreshold < value { self.prefs.autoThreshold = value }
                self.objectWillChange.send()
            })
    }

    // MARK: - Words

    var exceptions: [String] {
        prefs.wordExceptions.sorted { $0.localizedStandardCompare($1) == .orderedAscending }
    }

    func addException(_ word: String) {
        prefs.addWordException(word)
        objectWillChange.send()
    }

    func removeException(_ word: String) {
        var current = prefs.wordExceptions
        current.remove(word)
        prefs.wordExceptions = current
        objectWillChange.send()
    }

    /// Deletes every exception word (they are stored as plain text).
    func clearExceptionWords() {
        prefs.clearExceptionWords()
        objectWillChange.send()
    }

    /// Wipes the learned vocabulary (the salted hashes) and replaces the salt.
    func resetLearnedVocabulary() {
        learner.resetLearnedVocabulary()
        objectWillChange.send()
    }

    // MARK: - Transliteration

    var translitScheme: TranslitScheme { prefs.translitScheme }

    /// Live preview of the chosen scheme, so the difference is visible rather than described.
    func translitSample(_ scheme: TranslitScheme) -> String {
        Transliterator(models: ModelStore.shared)
            .toLatin("сәлеметсіз бе, қалайсыз?", scheme: scheme)
    }

    var translitOverrides: [(latin: String, cyrillic: String)] {
        prefs.translitOverrides.sorted { $0.key < $1.key }
            .map { (latin: $0.key, cyrillic: $0.value) }
    }

    func addTranslitOverride(latin: String, cyrillic: String) {
        prefs.addTranslitOverride(latin: latin, cyrillic: cyrillic)
        objectWillChange.send()
    }

    func removeTranslitOverride(_ latin: String) {
        var current = prefs.translitOverrides
        current.removeValue(forKey: latin)
        prefs.translitOverrides = current
        objectWillChange.send()
    }

    var neverConvert: [String] {
        prefs.translitNeverConvert.sorted { $0.localizedStandardCompare($1) == .orderedAscending }
    }

    func addNeverConvert(_ word: String) {
        prefs.addNeverConvert([word])
        objectWillChange.send()
    }

    /// Deletes the "keep in Latin" list (plain text on disk).
    func clearNeverConvertWords() {
        prefs.clearNeverConvertWords()
        objectWillChange.send()
    }

    func removeNeverConvert(_ word: String) {
        var current = prefs.translitNeverConvert
        current.remove(word)
        prefs.translitNeverConvert = current
        objectWillChange.send()
    }

    // MARK: - Languages

    /// Toggling a language off is refused when it is the last one left — with a single
    /// language there is nothing to choose between.
    func languageBinding(_ lang: Lang) -> Binding<Bool> {
        Binding(
            get: { self.prefs.enabledLanguages.contains(lang) },
            set: { isOn in
                var set = self.prefs.enabledLanguages
                if isOn {
                    set.insert(lang)
                } else {
                    set.remove(lang)
                }
                guard !set.isEmpty else { return }
                self.prefs.enabledLanguages = set
                self.objectWillChange.send()
            })
    }

    func layouts(for lang: Lang) -> [KeyMapTable] {
        inputSources.tables.filter { $0.lang == lang }
    }

    // MARK: - Apps

    /// Apps with their own behaviour: explicit rules, plus the shipped suggestions-only
    /// defaults for terminals that are actually installed.
    var appRules: [AppRuleRow] {
        var ids = Set(prefs.appRules.keys).subtracting(AppRules.alwaysDisabled)
        ids.formUnion(AppRules.defaultSuggestOnly.filter { AppInfo.isInstalled($0) })
        return ids.map { id in
            AppRuleRow(
                bundleID: id, name: AppInfo.name(for: id), mode: rules.mode(for: id),
                isExplicit: prefs.appRules[id] != nil,
                isShippedDefault: AppRules.defaultSuggestOnly.contains(id))
        }
        .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    /// Password managers present on this Mac. The rest of the built-in list is noise to
    /// someone who does not have those apps, and the system's own login and authorisation
    /// agents are not apps anyone would recognise — the footer covers them.
    var lockedApps: [AppRuleRow] {
        let systemAgents: Set<String> = ["com.apple.loginwindow", "com.apple.SecurityAgent"]
        return AppRules.alwaysDisabled.subtracting(systemAgents).filter { AppInfo.isInstalled($0) }
            .map {
                AppRuleRow(
                    bundleID: $0, name: AppInfo.name(for: $0), mode: .off, isExplicit: false,
                    isShippedDefault: true)
            }
            .sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    func setMode(_ mode: FixMode?, for bundleID: String) {
        rules.setMode(mode, for: bundleID)
        objectWillChange.send()
    }

    /// Running apps that could be added, most recently used first as the system lists them.
    var addableRunningApps: [NSRunningApplication] {
        let listed = Set(appRules.map(\.bundleID)).union(AppRules.alwaysDisabled)
        var seen = Set<String>()
        return NSWorkspace.shared.runningApplications.filter { app in
            guard app.activationPolicy == .regular, let id = app.bundleIdentifier,
                id != Bundle.main.bundleIdentifier, !listed.contains(id), !seen.contains(id)
            else { return false }
            seen.insert(id)
            return true
        }
        .sorted {
            ($0.localizedName ?? "").localizedStandardCompare($1.localizedName ?? "")
                == .orderedAscending
        }
    }

    /// A new rule starts as "don't correct here": wanting an app to behave differently from
    /// the default almost always means wanting it left alone.
    func addApp(_ bundleID: String) {
        rules.setMode(.off, for: bundleID)
        objectWillChange.send()
    }

    func chooseApp() {
        let panel = NSOpenPanel()
        panel.title = L10n.tr("apps.panel.title")
        panel.prompt = L10n.tr("apps.panel.prompt")
        panel.allowedContentTypes = [.application]
        panel.directoryURL = URL(fileURLWithPath: "/Applications")
        panel.allowsMultipleSelection = true
        guard panel.runModal() == .OK else { return }
        for url in panel.urls {
            if let id = Bundle(url: url)?.bundleIdentifier { addApp(id) }
        }
    }

    // MARK: - Hotkeys

    func hotkey(_ action: HotkeyAction) -> Hotkey {
        prefs.hotkeys[action] ?? action.defaultHotkey
    }

    /// Returns why the shortcut was refused, or nil when it was taken.
    func setHotkey(_ hotkey: Hotkey, for action: HotkeyAction) -> String? {
        if let other = HotkeyAction.allCases.first(where: {
            $0 != action && self.hotkey($0) == hotkey
        }) {
            return L10n.tr("hotkey.problem.taken", HotkeysPane.title(for: other))
        }
        prefs.setHotkey(hotkey, for: action)
        objectWillChange.send()
        return nil
    }

    func resetHotkey(_ action: HotkeyAction) {
        prefs.setHotkey(nil, for: action)
        objectWillChange.send()
    }

    func resetHotkeys() {
        prefs.hotkeys = [:]
        objectWillChange.send()
    }

    /// The one shortcut being recorded, if any. A single slot, so starting a second
    /// recorder stops the first instead of both listening for the same keys.
    @Published var recordingHotkey: HotkeyAction? {
        didSet { setHotkeysSuspended(recordingHotkey != nil) }
    }

    private var suspendedHotkeyHandler: ((KeyStroke, CGEventFlags) -> Bool)?

    /// While a shortcut is being recorded, the running app's own hotkeys stand aside —
    /// otherwise pressing an existing combination would run it instead of reaching the
    /// recorder.
    private func setHotkeysSuspended(_ suspended: Bool) {
        guard let tap = pipeline?.tap else { return }
        if suspended {
            guard suspendedHotkeyHandler == nil else { return }
            suspendedHotkeyHandler = tap.onHotkey
            tap.onHotkey = nil
        } else if let handler = suspendedHotkeyHandler {
            tap.onHotkey = handler
            suspendedHotkeyHandler = nil
        }
    }
}

/// `List` rows must be `Identifiable`, which a tuple cannot be.
struct AppRuleRow: Identifiable {
    let bundleID: String
    let name: String
    let mode: FixMode
    /// The user set this; otherwise the row shows a shipped default.
    let isExplicit: Bool
    let isShippedDefault: Bool
    var id: String { bundleID }
}

/// Names and icons of apps by bundle ID.
///
/// Cached: SwiftUI asks again on every redraw, and each answer is a Launch Services lookup.
/// Main-thread only, like the windows that use it.
enum AppInfo {
    private static var urls: [String: URL?] = [:]
    private static var icons: [String: NSImage] = [:]

    static func url(for bundleID: String) -> URL? {
        if let cached = urls[bundleID] { return cached }
        let url = NSWorkspace.shared.urlForApplication(withBundleIdentifier: bundleID)
        urls[bundleID] = url
        return url
    }

    static func isInstalled(_ bundleID: String) -> Bool {
        url(for: bundleID) != nil
    }

    /// Friendly name if the app is installed, bundle ID otherwise.
    static func name(for bundleID: String) -> String {
        guard let url = url(for: bundleID) else { return bundleID }
        let name = FileManager.default.displayName(atPath: url.path)
        return name.hasSuffix(".app") ? String(name.dropLast(4)) : name
    }

    static func icon(for bundleID: String) -> NSImage {
        if let cached = icons[bundleID] { return cached }
        let icon =
            url(for: bundleID).map { NSWorkspace.shared.icon(forFile: $0.path) }
            ?? NSWorkspace.shared.icon(for: .application)
        icons[bundleID] = icon
        return icon
    }
}

/// How eagerly to correct, in words instead of two confidence sliders.
enum Sensitivity: String, CaseIterable, Identifiable {
    case cautious
    case balanced
    case bold
    case custom

    var id: String { rawValue }

    static let presets: [Sensitivity] = [.cautious, .balanced, .bold]

    var title: String {
        switch self {
        case .cautious: return L10n.tr("sensitivity.cautious")
        case .balanced: return L10n.tr("sensitivity.balanced")
        case .bold: return L10n.tr("sensitivity.bold")
        case .custom: return L10n.tr("sensitivity.custom")
        }
    }

    /// The detector reports confidence in tiers — 0.95 for a certain fix, 0.9 for a confident
    /// one, 0.6 for "only worth offering" (see `Detector.certain` and neighbours) — so each
    /// preset sits *between* tiers rather than on one. Balanced is the shipped default;
    /// cautious lets only certain fixes happen by themselves; bold also applies what would
    /// otherwise only be offered. The values are the ones `CorpusTests.testPresets` measures.
    var thresholds: (auto: Double, suggest: Double)? {
        switch self {
        case .cautious: return (0.93, 0.55)
        case .balanced: return (0.80, 0.45)
        case .bold: return (0.55, 0.30)
        case .custom: return nil
        }
    }

    var explanation: String {
        switch self {
        case .cautious: return L10n.tr("sensitivity.cautious.detail")
        case .balanced: return L10n.tr("sensitivity.balanced.detail")
        case .bold: return L10n.tr("sensitivity.bold.detail")
        case .custom: return L10n.tr("sensitivity.custom.detail")
        }
    }

    static func matching(auto: Double, suggest: Double) -> Sensitivity {
        presets.first {
            guard let values = $0.thresholds else { return false }
            return abs(values.auto - auto) < 0.005 && abs(values.suggest - suggest) < 0.005
        } ?? .custom
    }
}

// MARK: - Root

struct SettingsView: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        NavigationSplitView {
            // The toolbar modifier has to come first: applied after the column width, it
            // hides the width from the split view and the sidebar falls back to ~140 pt,
            // truncating every label.
            SettingsSidebar(model: model)
                .toolbar(removing: .sidebarToggle)
                .navigationSplitViewColumnWidth(min: 200, ideal: 220, max: 260)
        } detail: {
            SettingsDetail(model: model)
                .frame(minWidth: 460, maxWidth: .infinity, maxHeight: .infinity)
                .navigationTitle(model.pane.title)
        }
        .frame(minWidth: 680, minHeight: 460)
    }
}

struct SettingsSidebar: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        List(selection: Binding(get: { model.pane }, set: { if let pane = $0 { model.pane = pane } })) {
            ForEach(SettingsPane.allCases) { pane in
                Label {
                    Text(pane.title)
                } icon: {
                    SettingsIcon(symbol: pane.symbol, color: pane.color, size: 20)
                }
                .badge(badge(for: pane))
                .tag(pane)
            }
        }
        .listStyle(.sidebar)
    }

    /// A problem is flagged where it can be fixed, not only on the pane that happens to be open.
    private func badge(for pane: SettingsPane) -> Text? {
        guard pane == .diagnostics, model.health.needsAttention else { return nil }
        return Text("!").foregroundStyle(.orange)
    }
}

struct SettingsDetail: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        switch model.pane {
        case .general: GeneralPane(model: model)
        case .correction: CorrectionPane(model: model)
        case .languages: LanguagesPane(model: model)
        case .transliteration: TransliterationPane(model: model)
        case .apps: AppsPane(model: model)
        case .exceptions: ExceptionsPane(model: model)
        case .hotkeys: HotkeysPane(model: model)
        case .diagnostics: DiagnosticsPane(model: model)
        case .about: AboutPane(model: model)
        }
    }
}

// MARK: - Window host

/// Plain `NSWindowController` around the SwiftUI view: a menu bar app has no scene, so
/// `Settings` from SwiftUI would never appear.
public final class SettingsWindowController: NSWindowController, NSWindowDelegate {
    private let model: SettingsModel
    private var languageObserver: NSObjectProtocol?

    @MainActor
    public init(prefs: Prefs, inputSources: InputSourceManager, pipeline: Pipeline?) {
        let model = SettingsModel(prefs: prefs, inputSources: inputSources, pipeline: pipeline)
        self.model = model

        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 760, height: 560),
            styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
            backing: .buffered,
            defer: false)
        window.title = L10n.tr("settings.title")
        window.toolbarStyle = .unified
        window.contentViewController = NSHostingController(rootView: SettingsView(model: model))
        window.setContentSize(NSSize(width: 760, height: 560))
        window.contentMinSize = NSSize(width: 680, height: 460)
        window.isReleasedWhenClosed = false
        // Reopens where the user left it; centred only the first time.
        if !window.setFrameUsingName("TilqosSettings") { window.center() }
        window.setFrameAutosaveName("TilqosSettings")

        super.init(window: window)
        window.delegate = self
        // The panes redraw themselves through the model; only the window title is ours to update.
        languageObserver = NotificationCenter.default.addObserver(
            forName: L10n.didChange, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.window?.title = L10n.tr("settings.title") }
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not used")
    }

    deinit {
        if let languageObserver { NotificationCenter.default.removeObserver(languageObserver) }
    }

    @MainActor
    public func present() {
        model.refresh()
        guard let window else { return }
        AppWindows.present(window)
    }

    public func windowWillClose(_ notification: Notification) {
        MainActor.assumeIsolated {
            model.recordingHotkey = nil
            if let window { AppWindows.windowWillClose(window) }
        }
    }
}

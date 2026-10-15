import AppKit

/// Owns the menu bar presence and the app's start-up order.
///
/// Main-actor by construction: every callback it serves — the app delegate, menu actions,
/// the tap's deferred work — already arrives on the main thread.
@MainActor
public final class AppController: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private var statusItem: NSStatusItem?
    private let inputSources = InputSourceManager.shared
    private let models = ModelStore.shared
    private let prefs = Prefs.shared
    private lazy var rules = AppRules(prefs: prefs)
    private lazy var pipeline = Pipeline(
        prefs: prefs, inputSources: inputSources, models: models)
    private let fixHUD = FixHUD()

    private var settingsWindow: SettingsWindowController?
    private var onboardingWindow: OnboardingWindowController?
    private var tutorialWindow: TutorialWindowController?
    private var permissionRetry: Timer?

    /// The menu currently on screen, so a change made from inside it — the header switch —
    /// shows up without closing it.
    private weak var openMenu: NSMenu?

    public override init() {
        super.init()
    }

    // MARK: - Lifecycle

    public func applicationDidFinishLaunching(_ notification: Notification) {
        // First of all, so that nothing is ever drawn in the wrong language.
        L10n.setting = prefs.uiLanguage
        NotificationCenter.default.addObserver(
            self, selector: #selector(languageChanged), name: L10n.didChange, object: nil)

        Log.diagnosticsEnabled = prefs.diagnosticsEnabled
        let status = Permissions.status
        Log.info(
            .app,
            "launched: bundle=\(Bundle.main.bundleIdentifier ?? "none") "
                + "inputMonitoring=\(status.inputMonitoring) "
                + "accessibility=\(status.accessibility) "
                + "onboarded=\(prefs.didCompleteOnboarding)")

        // The status item first, so the app is visibly up in a few milliseconds. Reading
        // layouts and decoding language models both happen after it appears.
        installStatusItem()

        inputSources.onSelectionChange = { [weak self] _ in self?.layoutChanged() }
        inputSources.onLayoutSetChange = { [weak self] in self?.layoutChanged() }
        inputSources.start()
        refreshStatusItem()

        NotificationCenter.default.addObserver(
            self, selector: #selector(prefsChanged), name: Prefs.didChange, object: nil)

        pipeline.statusItemAnchor = { [weak self] in
            self?.statusItem?.button?.window?.frame
        }
        pipeline.onStateChange = { [weak self] in self?.refreshStatusItem() }
        pipeline.onFixApplied = { [weak self] fix in self?.fixApplied(fix) }

        models.loadInBackground { [weak self] languages in
            Log.info(.app, "ready with \(languages.count) language models")
            self?.refreshStatusItem()
        }

        startOrOnboard()
    }

    public func applicationWillTerminate(_ notification: Notification) {
        pipeline.stop()
        inputSources.stop()
    }

    /// First run, or permissions revoked since last time: explain before doing anything.
    private func startOrOnboard() {
        let status = Permissions.status
        if status.canDetect {
            // Whatever the onboarding flag says, if we can work then work.
            let started = startPipeline()
            if !started || (!prefs.didCompleteOnboarding && !status.canApply) {
                presentOnboarding()
            } else {
                presentTutorialIfNew()
            }
            return
        }
        presentOnboarding()
        startPermissionRetry()
    }

    /// Safe to call repeatedly — the onboarding window and the retry timer can both notice
    /// the same grant — because `Pipeline.start()` itself registers its observers anew on
    /// every call.
    @discardableResult
    private func startPipeline() -> Bool {
        if pipeline.isTapRunning {
            refreshStatusItem()
            return true
        }
        guard pipeline.start() else {
            Log.warn(.app, "pipeline could not start; permissions missing")
            startPermissionRetry()
            refreshStatusItem()
            return false
        }
        permissionRetry?.invalidate()
        permissionRetry = nil
        Log.info(.app, "pipeline running")
        refreshStatusItem()
        return true
    }

    /// The tutorial has typing practice driven by the real engine, so it is only worth
    /// showing once the engine is actually running and able to replace text.
    private func presentTutorialIfNew() {
        guard !prefs.didCompleteTutorial, pipeline.isTapRunning, Permissions.status.canApply,
            onboardingWindow == nil
        else { return }
        presentTutorial()
    }

    /// Keeps trying to start once a permission appears.
    ///
    /// macOS sends no notification when a TCC grant is made in System Settings, and the tap
    /// cannot be created before then, so the only way to come up by itself is to check back.
    /// Stops as soon as the tap is running.
    private func startPermissionRetry() {
        guard permissionRetry == nil else { return }
        let timer = Timer(timeInterval: 3.0, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.retryStart() }
        }
        timer.tolerance = 1.0
        RunLoop.main.add(timer, forMode: .common)
        permissionRetry = timer
    }

    private func retryStart() {
        guard !pipeline.isTapRunning else {
            permissionRetry?.invalidate()
            permissionRetry = nil
            refreshStatusItem()
            return
        }
        guard Permissions.status.canDetect else { return }
        if pipeline.start() {
            permissionRetry?.invalidate()
            permissionRetry = nil
            Log.info(.app, "pipeline started after permission was granted")
            refreshStatusItem()
            presentTutorialIfNew()
        }
    }

    @objc private func prefsChanged() {
        // The language is a preference like the rest; the Settings picker sets it too.
        L10n.setting = prefs.uiLanguage
        refreshStatusItem()
    }

    /// The menu is rebuilt each time it opens; what is drawn already (the tooltip, the
    /// accessibility label) is not.
    @objc private func languageChanged() {
        refreshStatusItem()
    }

    private func layoutChanged() {
        refreshStatusItem()
        tutorialWindow?.layoutDidChange()
    }

    // MARK: - Status item

    private func installStatusItem() {
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        item.button?.imagePosition = .imageOnly
        let menu = NSMenu()
        menu.delegate = self
        item.menu = menu
        statusItem = item
    }

    private var health: AppHealth {
        AppHealth.evaluate(
            enabled: prefs.enabled, smartMode: prefs.smartMode, permissions: Permissions.status,
            tapRunning: pipeline.isTapRunning)
    }

    /// Updates the icon, and the menu too if it is open. The menu is otherwise built only
    /// when it opens, so layout switches and settings changes cost nothing here.
    private func refreshStatusItem() {
        guard let button = statusItem?.button else { return }
        let health = self.health
        let table = inputSources.current
        let badge = prefs.showLayoutBadge ? table?.lang?.badge : nil

        button.title = ""
        button.image = StatusIcon.image(badge: badge, health: health)
        button.appearsDisabled = health == .disabled

        let state = health.title.lowercased(with: L10n.locale)
        var description = L10n.tr("status.tooltip", state)
        if let name = table?.localizedName {
            description = L10n.tr("status.tooltip.layout", state, name)
        }
        button.toolTip = description
        button.setAccessibilityLabel(description)

        // Deferred: this can run inside the action of a control in that very menu.
        if openMenu != nil {
            DispatchQueue.main.async { [weak self] in
                guard let self, let menu = self.openMenu else { return }
                self.populate(menu)
            }
        } else if let menu = statusItem?.menu, menu.numberOfItems > 0 {
            // A closed menu keeps no text: its undo row names the last correction in plain
            // text, and that must not outlive the undo list (two minutes, or the lock, the
            // sleep, the next app). It is rebuilt when it opens (`menuNeedsUpdate`). Deferred,
            // like the branch above: this can run inside the action of one of its items.
            DispatchQueue.main.async { [weak self] in
                guard let self, self.openMenu == nil, let menu = self.statusItem?.menu else { return }
                menu.removeAllItems()
            }
        }
    }

    public func menuNeedsUpdate(_ menu: NSMenu) {
        populate(menu)
    }

    public func menuWillOpen(_ menu: NSMenu) {
        openMenu = menu
    }

    public func menuDidClose(_ menu: NSMenu) {
        openMenu = nil
    }

    private func populate(_ menu: NSMenu) {
        StatusMenu.populate(menu, state: menuState(), actions: menuActions())
    }

    private func menuState() -> StatusMenuState {
        let table = inputSources.current
        return StatusMenuState(
            health: health,
            enabled: prefs.enabled,
            layoutName: table?.localizedName,
            currentLang: table?.lang,
            installedLangs: inputSources.availableLanguages,
            lastFix: pipeline.lastFix,
            frontApp: frontApp(),
            hotkeys: prefs.hotkeys,
            translitScheme: prefs.translitScheme)
    }

    /// The app the user was typing in. A status menu does not activate us, so this is
    /// still theirs — unless one of our own windows is in front, when there is nothing to
    /// offer.
    private func frontApp() -> FrontApp? {
        guard let app = NSWorkspace.shared.frontmostApplication,
            let bundleID = app.bundleIdentifier, bundleID != Bundle.main.bundleIdentifier,
            app.activationPolicy == .regular
        else { return nil }
        return FrontApp(
            bundleID: bundleID,
            name: app.localizedName ?? bundleID,
            icon: app.icon,
            mode: rules.mode(for: bundleID),
            locked: rules.isLocked(bundleID))
    }

    private func menuActions() -> StatusMenuActions {
        StatusMenuActions(
            setEnabled: { [weak self] in self?.prefs.enabled = $0 },
            fixPermissions: { [weak self] in self?.presentOnboarding() },
            restartTap: { [weak self] in self?.restartTap() },
            setAppExcluded: { [weak self] bundleID, excluded in
                // Back to "no rule" rather than to a mode, so the app returns to whatever
                // applies by default — including suggestions-only for terminals.
                self?.rules.setMode(excluded ? .off : nil, for: bundleID)
            },
            undo: { [weak self] in self?.pipeline.undoLastFix() },
            perform: { [weak self] in self?.pipeline.perform($0) },
            transliterate: { [weak self] in self?.pipeline.transliterate(direction: $0) },
            setScheme: { [weak self] in self?.prefs.translitScheme = $0 },
            openSettings: { [weak self] in self?.openSettings() },
            openTutorial: { [weak self] in self?.presentTutorial() },
            quit: { NSApp.terminate(nil) })
    }

    private func restartTap() {
        pipeline.tap.restart()
        if !pipeline.isTapRunning { startPipeline() }
        refreshStatusItem()
    }

    // MARK: - Feedback

    private func fixApplied(_ fix: AppliedFix) {
        refreshStatusItem()
        guard prefs.showHUD else { return }
        // Next turn of the run loop: the pipeline is still mid-apply, and the caret query is
        // a round trip to the app being typed into.
        DispatchQueue.main.async { [weak self] in
            guard let self else { return }
            self.fixHUD.show(
                fix, undo: self.prefs.hotkeys[.undoLastFix],
                caret: CaretLocator.caretRect(),
                fallback: self.statusItem?.button?.window?.frame)
        }
    }

    // MARK: - Windows

    private func presentTutorial() {
        if tutorialWindow == nil {
            tutorialWindow = TutorialWindowController(
                inputSources: inputSources, prefs: prefs
            ) { [weak self] in
                self?.tutorialWindow = nil
                self?.refreshStatusItem()
            }
        }
        tutorialWindow?.present()
    }

    private func openSettings() {
        if settingsWindow == nil {
            settingsWindow = SettingsWindowController(
                prefs: prefs, inputSources: inputSources, pipeline: pipeline)
        }
        settingsWindow?.present()
    }

    private func presentOnboarding() {
        if onboardingWindow == nil {
            onboardingWindow = OnboardingWindowController(
                onGranted: { [weak self] in
                    // Start working the moment the grant lands, even before the user
                    // clicks anything in the window.
                    self?.startPipeline()
                },
                onClose: { [weak self] in
                    guard let self else { return }
                    self.onboardingWindow = nil
                    if Permissions.status.canDetect, !self.pipeline.isTapRunning {
                        self.startPipeline()
                    }
                    self.refreshStatusItem()
                    self.presentTutorialIfNew()
                })
        }
        onboardingWindow?.present()
    }
}

import AppKit
import Carbon.HIToolbox

/// What the pipeline needs from the on-screen offer. `SuggestionPanel` is the real one; the
/// privacy tests use a stand-in so no window is ever created.
protocol SuggestionPresenting: AnyObject {
    var onDismiss: (() -> Void)? { get set }
    func show(
        before: String, corrected: String, acceptKey: AcceptKey, anchor: CGRect?,
        timeout: TimeInterval, statusItemAnchor: CGRect?)
    func hide()
}

extension SuggestionPanel: SuggestionPresenting {}

/// Connects the tap to the detector to the injector, and owns the decision of when to act.
///
/// The decisions themselves live in `TypingEngine`, which the tests drive directly; this class
/// adds the side effects — the panel, the injector, the layout switch, the undo stack — and the
/// learning signals only it can see: which offers were taken and which went by.
public final class Pipeline {
    private let prefs: Prefs
    private let rules: AppRules
    private let inputSources: InputSourceManager
    private let models: ModelStore
    /// Internal, not private: the privacy tests feed it strokes and look at its buffer.
    let engine: TypingEngine
    private let injector: Injector
    private let panel: SuggestionPresenting
    let undo: UndoStack
    private let board: ClipboardBoard
    private let clipboard: SelectionClipboard
    private let scheduler: TimerScheduler
    public let tap: EventTap
    /// What the app has learned from the user. See `Learner` for the API the settings use.
    public let learner: Learner

    /// Where to put the suggestion when the focused app will not say where its caret is.
    public var statusItemAnchor: (() -> CGRect?)?
    /// Fires for every fix that actually reached the screen — automatic, accepted with Tab, or
    /// asked for with ⌥⇧F.
    public var onFixApplied: ((AppliedFix) -> Void)?
    public var onStateChange: (() -> Void)?

    /// A correction offered and awaiting the accept key.
    private var pending: Detector.Outcome?
    private var pendingBundleID: String?

    /// Settings read on the keystroke path, cached so no keypress ever pays for a
    /// `UserDefaults` lookup chain or a JSON decode.
    private struct Snapshot {
        var enabled = true
        var smartMode = true
        var acceptKey = AcceptKey.tab
        var minWordLength = 3
        var exceptions: Set<String> = []
        var autoThreshold = 0.80
        var suggestThreshold = 0.45
        var suggestionTimeout = 4.0
        var enabledLanguages = Set(Lang.allCases)
        var hotkeys: [HotkeyAction: Hotkey] = [:]
        var cyrillicPreference = CyrillicPreference.automatic
    }
    private var snapshot = Snapshot()

    /// Frontmost app, refreshed from workspace notifications rather than asked for on every
    /// keystroke — `NSWorkspace.frontmostApplication` is far too heavy for that.
    private var currentBundleID: String?
    private var currentMode: FixMode = .auto
    private var started = false

    /// Text in front of the buffer, read once per buffer session. See `precedingText()`.
    private var cachedContext: (session: Int, text: String?)?
    private var session = 0

    public convenience init(
        prefs: Prefs = .shared,
        inputSources: InputSourceManager = .shared,
        models: ModelStore = .shared,
        learner: Learner = .shared
    ) {
        self.init(
            prefs: prefs, inputSources: inputSources, models: models, learner: learner,
            board: NSPasteboardBoard(), scheduler: MainQueueScheduler.shared,
            panel: SuggestionPanel(prefs: prefs))
    }

    /// The designated initialiser: the clipboard and the clock are injectable so the privacy
    /// behaviour (idle reset, undo expiry, clipboard rules) can be tested without the real ones.
    init(
        prefs: Prefs, inputSources: InputSourceManager, models: ModelStore, learner: Learner,
        board: ClipboardBoard, scheduler: TimerScheduler, panel: SuggestionPresenting,
        now: @escaping () -> Date = Date.init
    ) {
        self.prefs = prefs
        self.rules = AppRules(prefs: prefs)
        self.inputSources = inputSources
        self.models = models
        self.learner = learner
        self.board = board
        self.scheduler = scheduler
        self.clipboard = SelectionClipboard(board: board, scheduler: scheduler)
        let detector = Detector(models: models)
        self.engine = TypingEngine(
            detector: detector, buffer: StrokeBuffer(idleReset: prefs.bufferIdleReset))
        self.tap = EventTap(prefs: prefs)
        self.injector = Injector(tap: tap)
        self.panel = panel
        self.undo = UndoStack(scheduler: scheduler, now: now)

        engine.learner = learner
        engine.isDictionaryWord = { detector.isDictionaryWord($0, lang: $1) }
        engine.resolve = { [inputSources] in inputSources.table(forSourceID: $0) }
        // Five seconds without a key empties the buffer by itself, not at the next key.
        engine.scheduler = scheduler
        engine.onIdleReset = { Log.debug(.tap, "buffer cleared: idle") }
        // Whatever empties the buffer also drops the text read from the field in front of it.
        engine.onReset = { [weak self] in
            self?.session += 1
            self?.cachedContext = nil
        }
        undo.onExpire = { [weak self] in self?.onStateChange?() }
    }

    // MARK: - Lifecycle

    /// Installs the tap. Safe to call again — after a permission appears, say: the observers
    /// are only registered once.
    @discardableResult
    public func start() -> Bool {
        refreshSnapshot()
        refreshFrontmostApp()

        if !started {
            started = true
            tap.onHotkey = { [weak self] stroke, flags in
                self?.handleHotkey(stroke, flags) ?? false
            }
            tap.onStroke = { [weak self] stroke in
                self?.handle(stroke) ?? .pass
            }
            tap.onReset = { [weak self] reason in
                self?.reset(reason)
            }
            tap.onBackspace = { [weak self] in
                self?.backspace()
            }
            panel.onDismiss = { [weak self] in
                // Timed out on screen. Not a "no": the user may simply not have looked.
                self?.dismissPending()
            }

            NotificationCenter.default.addObserver(
                self, selector: #selector(prefsChanged), name: Prefs.didChange, object: nil)
            NSWorkspace.shared.notificationCenter.addObserver(
                self, selector: #selector(appActivated),
                name: NSWorkspace.didActivateApplicationNotification, object: nil)
            // Lock, sleep and fast user switching: whatever was typed or corrected before is
            // wiped from memory, not kept for whoever is at the keyboard next.
            for name in [
                NSWorkspace.willSleepNotification, NSWorkspace.screensDidSleepNotification,
                NSWorkspace.sessionDidResignActiveNotification,
            ] {
                NSWorkspace.shared.notificationCenter.addObserver(
                    self, selector: #selector(lockedOrAsleep), name: name, object: nil)
            }
            for name in ["com.apple.screenIsLocked", "com.apple.screensaver.didstart"] {
                DistributedNotificationCenter.default().addObserver(
                    self, selector: #selector(lockedOrAsleep), name: Notification.Name(name),
                    object: nil, suspensionBehavior: .deliverImmediately)
            }
        }

        return tap.start()
    }

    public func stop() {
        panel.hide()
        tap.stop()
        learner.flush()
        NotificationCenter.default.removeObserver(self)
        NSWorkspace.shared.notificationCenter.removeObserver(self)
        DistributedNotificationCenter.default().removeObserver(self)
        started = false
        // Quitting: nothing typed or corrected stays in memory a moment longer.
        wipeSessionMemory()
    }

    public var isTapRunning: Bool { tap.isRunning }
    public var undoCount: Int { undo.count }
    /// The newest fix that `undoLastFix()` would take back, for the menu's undo row.
    public var lastFix: AppliedFix? { undo.last }
    public var learnedStrategies: [String: Injector.Strategy] { injector.knownStrategies() }

    @objc private func prefsChanged() {
        refreshSnapshot()
        refreshMode()
    }

    @objc func appActivated() {
        refreshFrontmostApp()
        // The caret is now somewhere else entirely, and so is the text a correction was made
        // in: nothing in the undo list can be put back from here.
        reset(.appSwitch)
        undo.clear()
        onStateChange?()
    }

    @objc func lockedOrAsleep() {
        wipeSessionMemory()
    }

    /// Everything the app holds about what was typed or corrected, gone: the keystroke
    /// buffer and the words derived from it, the text read from the field, the offer on
    /// screen, the undo list. Called on lock, sleep, fast user switching and quit.
    func wipeSessionMemory() {
        engine.reset()
        session += 1
        cachedContext = nil
        dismissPending()
        undo.clear()
        onStateChange?()
    }


    private func refreshSnapshot() {
        Log.diagnosticsEnabled = prefs.diagnosticsEnabled
        snapshot = Snapshot(
            enabled: prefs.enabled,
            smartMode: prefs.smartMode,
            acceptKey: prefs.acceptKey,
            minWordLength: prefs.minWordLength,
            exceptions: prefs.wordExceptions,
            autoThreshold: prefs.autoThreshold,
            suggestThreshold: prefs.suggestThreshold,
            suggestionTimeout: prefs.suggestionTimeout,
            enabledLanguages: prefs.enabledLanguages,
            hotkeys: prefs.hotkeys,
            cyrillicPreference: prefs.cyrillicPreference
        )
    }

    private func refreshFrontmostApp() {
        currentBundleID = NSWorkspace.shared.frontmostApplication?.bundleIdentifier
        refreshMode()
    }

    private func refreshMode() {
        currentMode = rules.mode(for: currentBundleID)
    }

    private var candidateLayouts: [KeyMapTable] {
        inputSources.candidateTables(limitedTo: snapshot.enabledLanguages)
    }

    // MARK: - Keystroke path

    func handle(_ stroke: KeyStroke) -> EventTap.Verdict {
        // A pending suggestion owns the accept key, and only while it is on screen.
        if pending != nil {
            if isAcceptKey(stroke) {
                Log.debug(.tap, "accept key: swallowed, accepting")
                DispatchQueue.main.async { [weak self] in self?.acceptPending() }
                return .swallow
            }
            // Typing on past an offer leaves the word as it is this time, and says nothing more:
            // people type on because they did not see the offer at least as often as because
            // they meant the word. Counting that as a refusal once taught the app to stop
            // fixing ordinary words («написал» typed on ABC) for good.
            dismissPending()
        }

        // Tab inserts a tab or moves the focus; either way the buffer no longer matches the
        // text in front of the caret. (After the accept check above: Tab is also the accept key.)
        if stroke.keyCode == UInt16(kVK_Tab) {
            reset(.navigationKey)
            return .pass
        }

        guard snapshot.enabled, currentMode != .off else {
            engine.reset()
            return .pass
        }
        // Read the layout live. Cached state goes stale the moment the user switches, and a
        // stroke attributed to the wrong layout is worse than useless: it reads as correct text.
        guard let layout = inputSources.liveCurrentTable() else { return .pass }

        let wordEnd = engine.record(
            stroke, layout: layout, at: ProcessInfo.processInfo.systemUptime,
            candidates: candidateLayouts)

        // Note which Cyrillic layout this app is written in; it is the tie-breaker when
        // the letters cannot distinguish Russian from Kazakh.
        if layout.lang?.script == .cyrillic {
            rules.rememberCyrillic(layoutID: layout.sourceID, for: currentBundleID)
        }

        guard let wordEnd, snapshot.smartMode, models.isLoaded else { return .pass }

        // Deferred deliberately. This callback runs *before* the separator reaches the
        // app, so injecting here would put the correction in front of the character that
        // triggered it. One turn of the run loop puts everything back in order, and the
        // injector waits for the app to catch up before it touches anything.
        DispatchQueue.main.async { [weak self] in
            self?.evaluate(wordEnd)
        }
        return .pass
    }

    private func backspace() {
        // The text under an offer just changed, so the offer no longer describes it — accepting
        // it now would replace characters that are not there. Not counted as ignoring it: the
        // user may be fixing that very word by hand.
        dismissPending()
        engine.backspace()
    }

    private func reset(_ reason: EventTap.ResetReason) {
        engine.reset()
        session += 1
        cachedContext = nil
        // Escape is the one deliberate "no" to an offer on screen. Arrows, Return, a click or
        // an app switch only mean the user went elsewhere.
        dismissPending(ignored: reason == .escape)
        // Not for secure input: when a password field was in front is not written down.
        if reason == .disabled {
            Log.info(.tap, "buffer cleared: \(reason.rawValue)")
        }
    }

    // MARK: - Deciding

    private func settings(mayApply: Bool) -> TypingEngine.Settings {
        TypingEngine.Settings(
            candidateLayouts: candidateLayouts,
            resolve: { [inputSources] in inputSources.table(forSourceID: $0) },
            minWordLength: snapshot.minWordLength,
            exceptions: snapshot.exceptions,
            autoThreshold: snapshot.autoThreshold,
            suggestThreshold: snapshot.suggestThreshold,
            mayApply: mayApply,
            learned: learner.isEnabled ? learner : nil)
    }

    func evaluate(_ end: TypingEngine.WordEnd) {
        guard !engine.buffer.isEmpty else { return }
        // This runs a turn of the run loop after the key. If a password field came to the front
        // in between, nothing is read from it and what was buffered goes.
        guard !tap.isSecureInputActive else {
            engine.reset()
            return
        }
        let context = precedingText()
        let decision = engine.decide(
            at: end, settings: settings(mayApply: currentMode == .auto),
            precedingText: context)
        Log.debug(.detect, "word end: \(Self.describe(decision)), strokes=\(engine.buffer.count), "
            + "mode=\(currentMode.rawValue), context=\(context.map { "\($0.split(separator: " ").count) words" } ?? "none"), "
            + "layouts=\(candidateLayouts.compactMap { $0.lang?.rawValue }.joined(separator: ","))")
        switch decision {
        case .apply(let outcome):
            apply(outcome, automatic: true)
        case .offer(let outcome):
            offer(outcome)
        case .none:
            break
        }
    }

    /// A decision in shapes only — lengths, language, confidence — for the debug trail.
    private static func describe(_ decision: TypingEngine.Decision) -> String {
        func shape(_ outcome: Detector.Outcome) -> String {
            "\(outcome.before.count)→\(outcome.after.count) chars, lang=\(outcome.targetLang.rawValue), "
                + "confidence=\(String(format: "%.2f", outcome.confidence))"
        }
        switch decision {
        case .none: return "none"
        case .apply(let outcome): return "apply (\(shape(outcome)))"
        case .offer(let outcome): return "offer (\(shape(outcome)))"
        }
    }

    /// The last few words in front of the buffer, from the text field itself, so the first
    /// word after a click still has context. Read at most once per buffer session, only when
    /// the buffer holds too little context of its own, and reduced at once to three words. It
    /// never leaves memory and is never logged.
    private func precedingText() -> String? {
        if let cached = cachedContext, cached.session == session { return cached.text }
        var text: String?
        defer { cachedContext = (session, text) }

        // Plenty of context in the buffer already: do not touch the app.
        guard engine.buffer.count < 24, AX.isTrusted, !tap.isSecureInputActive,
            !(currentBundleID.map(Injector.alwaysSynthetic.contains) ?? false),
            let element = AX.focusedElement(),
            let caret = AX.selectedRange(element), caret.length == 0
        else { return nil }
        let typed = engine.screenText(resolve: { [inputSources] in inputSources.table(forSourceID: $0) })
        let end = caret.location - typed.utf16.count
        guard end > 0 else { return nil }
        let start = max(0, end - 160)
        let field = AXTextField(element)
        guard let before = field.string(in: NSRange(location: start, length: end - start)) else {
            return nil
        }
        text = before.split(whereSeparator: \.isWhitespace).suffix(3).joined(separator: " ")
        return text
    }

    // MARK: - Offering

    private func offer(_ outcome: Detector.Outcome) {
        pending = outcome
        pendingBundleID = currentBundleID
        panel.show(
            before: outcome.before,
            corrected: outcome.after,
            acceptKey: effectiveAcceptKey(),
            anchor: CaretLocator.caretRect(),
            timeout: snapshot.suggestionTimeout,
            statusItemAnchor: statusItemAnchor?()
        )
        onStateChange?()
    }

    /// Takes the offer down. `ignored` when the user let it go by — typed on, pressed Escape, or
    /// let it time out — which counts against offering the same word again.
    private func dismissPending(ignored: Bool = false) {
        guard let outcome = pending else { return }
        Log.debug(.ui, "offer dismissed, ignored=\(ignored)")
        pending = nil
        pendingBundleID = nil
        panel.hide()
        if ignored {
            for change in outcome.changes { learner.ignored(typed: change.typed) }
        }
    }

    private func acceptPending() {
        guard let outcome = pending else { return }
        // The user may have moved on while the panel was up.
        guard pendingBundleID == currentBundleID else {
            dismissPending()
            return
        }
        dismissPending()
        if apply(outcome, automatic: false) {
            // Accepted: next time, without asking.
            for change in outcome.changes {
                learner.confirmed(typed: change.typed, to: change.targetLang)
            }
        }
    }

    /// Tab is load-bearing in terminals and editors, so the secondary binding takes over
    /// there rather than stealing a completion keystroke.
    private func effectiveAcceptKey() -> AcceptKey {
        if snapshot.acceptKey == .tab, rules.isTabCritical(currentBundleID) {
            return .optionSpace
        }
        return snapshot.acceptKey
    }

    private func isAcceptKey(_ stroke: KeyStroke) -> Bool {
        switch effectiveAcceptKey() {
        case .tab:
            return stroke.keyCode == UInt16(kVK_Tab) && !stroke.mods.contains(.option)
        case .optionSpace:
            return stroke.keyCode == UInt16(kVK_Space) && stroke.mods.contains(.option)
        }
    }

    // MARK: - Applying

    /// Returns whether the fix reached the screen.
    @discardableResult
    private func apply(_ outcome: Detector.Outcome, automatic: Bool) -> Bool {
        let previousLayoutID = inputSources.currentSourceID

        switch injector.replace(outcome.replacement, bundleID: currentBundleID) {
        case .applied(let strategy, let inserted):
            engine.didApply(
                outcome, automatic: automatic, at: ProcessInfo.processInfo.systemUptime)
            switchLayout(for: outcome)

            let fix = AppliedFix(
                replacement: Replacement(
                    deletedText: outcome.replacement.deletedText, insertText: inserted),
                bundleID: currentBundleID,
                originalWord: outcome.before,
                correctedWord: outcome.after,
                previousLayoutID: previousLayoutID,
                strategy: strategy,
                typedWords: outcome.changes.map(\.typed)
            )
            undo.push(fix)
            onFixApplied?(fix)

            if prefs.playSound { NSSound(named: "Tink")?.play() }
            Log.info(
                .inject,
                "fixed \(outcome.before.count)→\(outcome.after.count) chars "
                    + "via \(strategy.rawValue), lang=\(outcome.targetLang.rawValue), "
                    + "confidence=\(String(format: "%.2f", outcome.confidence)), "
                    + (automatic ? "automatic" : "accepted"))
            return true

        case .mismatch, .failed:
            // Whatever happened, the buffer can no longer be trusted to describe the screen.
            engine.reset()
            session += 1
            return false
        }
    }

    /// Switches the keyboard so what the user types next lands in the right language.
    ///
    /// When the corrected text carries no Kazakh-specific letter, Russian and Kazakh are
    /// indistinguishable from the text alone. Then the user's preference decides — a fixed
    /// language if they chose one, otherwise the Cyrillic layout this app was last written in
    /// — over whatever the models marginally preferred.
    private func switchLayout(for outcome: Detector.Outcome) {
        var targetID = outcome.targetLayoutID
        if outcome.targetLang.script == .cyrillic, !Alphabet.containsKazakhSpecific(outcome.after) {
            switch snapshot.cyrillicPreference {
            case .preferKazakh:
                if let kazakh = inputSources.table(for: .kk) { targetID = kazakh.sourceID }
            case .preferRussian:
                if let russian = inputSources.table(for: .ru) { targetID = russian.sourceID }
            case .automatic:
                if let remembered = rules.rememberedCyrillic(for: currentBundleID),
                    inputSources.table(forSourceID: remembered)?.lang?.script == .cyrillic
                {
                    targetID = remembered
                }
            }
        }
        guard targetID != inputSources.currentSourceID else { return }
        inputSources.select(sourceID: targetID)
        if inputSources.table(forSourceID: targetID)?.lang?.script == .cyrillic {
            rules.rememberCyrillic(layoutID: targetID, for: currentBundleID)
        }
    }

    // MARK: - Hotkeys

    private func handleHotkey(_ stroke: KeyStroke, _ flags: CGEventFlags) -> Bool {
        // Deliberately live even when the app is switched off, so the toggle hotkey can
        // switch it back on.
        for (action, hotkey) in snapshot.hotkeys
        where hotkey.matches(keyCode: stroke.keyCode, flags: flags) {
            // Off the callback: hotkey work touches AX and the pasteboard, neither of
            // which belongs inside the tap's time budget.
            DispatchQueue.main.async { [weak self] in self?.perform(action) }
            return true
        }
        return false
    }

    public func perform(_ action: HotkeyAction) {
        switch action {
        case .toggleEnabled:
            prefs.enabled.toggle()
        case .undoLastFix:
            undoLastFix()
        case .fixLastWord:
            fixLastWord()
        case .fixSelection:
            fixSelection()
        case .convertClipboard:
            convertClipboard()
        case .transliterate:
            transliterate(direction: nil)
        case .forceEnglish:
            inputSources.select(lang: .en)
        case .forceRussian:
            inputSources.select(lang: .ru)
        case .forceKazakh:
            inputSources.select(lang: .kk)
        }
    }

    /// Manual correction: acts even when confidence is below the automatic threshold, and even
    /// on a real word, because the user asked for it explicitly. That makes it a lesson too.
    public func fixLastWord() {
        guard !blockedBySecureInput() else { return }
        guard !engine.buffer.isEmpty, models.isLoaded else { return }
        dismissPending()
        guard let outcome = engine.manualFix(settings: settings(mayApply: true)) else { return }
        if apply(outcome, automatic: false) {
            for change in outcome.changes {
                learner.confirmed(typed: change.typed, to: change.targetLang)
            }
        }
    }

    public func undoLastFix() {
        guard !blockedBySecureInput() else { return }
        // Nothing to take back, or the two minutes are up: a quiet no-op.
        guard let fix = undo.popLast() else { return }
        let wasRecent = Date().timeIntervalSince(fix.time) <= 8

        if let clipboardBefore = fix.clipboardBefore {
            // The change was made to the clipboard, so put the clipboard back.
            board.replaceWithText(clipboardBefore, transient: false)
        } else {
            let result = injector.replace(fix.replacement.inverse, bundleID: fix.bundleID)
            guard result.strategy != nil else {
                // The fixed text is no longer where we left it; undoing blindly would damage
                // whatever is there now.
                Log.info(.inject, "undo skipped: the corrected text is not at the caret")
                engine.reset()
                return
            }
        }
        if let previous = fix.previousLayoutID {
            inputSources.select(sourceID: previous)
        }

        // Undoing straight away means we got it wrong, and what to remember depends on what
        // was undone.
        if wasRecent {
            switch fix.kind {
            case .layoutFix:
                // Each changed word goes on the visible exceptions list, where the user can see
                // it and take it off again.
                let words = fix.typedWords.isEmpty ? [fix.originalWord] : fix.typedWords
                for word in words where !word.isEmpty {
                    prefs.addWordException(word)
                    learner.forget(word: word)
                }
                Log.info(.detect, "learned \(words.count) word exception(s) from undo")
            case .transliteration:
                // Only the words that actually changed are the rejected ones — the rest of the
                // passage was converted correctly and must keep working.
                let rejected = makeTransliterator().changedWords(
                    from: fix.originalWord, to: fix.correctedWord)
                prefs.addNeverConvert(rejected)
                Log.info(.detect, "learned \(rejected.count) never-convert word(s) from undo")
            }
        }
        engine.reset()
        session += 1
        onStateChange?()
    }

    /// Converts the current selection in place. Used for text pasted from someone else,
    /// where there are no keystrokes to reason about.
    public func fixSelection() {
        guard !blockedBySecureInput() else { return }
        guard let source = inputSources.current, models.isLoaded else { return }
        let layouts = inputSources.candidateTables(limitedTo: snapshot.enabledLanguages)
        let convert = { [models] (text: String) in
            TextConverter.bestConversion(text, from: source, layouts: layouts, models: models)
        }

        if let element = AX.focusedElement(), let selected = AX.selectedText(element),
            !selected.isEmpty
        {
            guard let best = convert(selected) else { return nothing("selection needs no fix") }
            tap.isInjecting = true
            defer { tap.isInjecting = false }
            if AX.setSelectedText(element, best.text) {
                inputSources.select(best.layout)
                engine.reset()
                return
            }
            Log.warn(.inject, "selection replacement rejected by the app; copying instead")
        }

        guard mayCopySelection else { return nothing("no selection to convert") }
        clipboard.copySelection { [weak self] selected, saved in
            guard let self else { return }
            guard let selected else { return self.nothing("no selection to convert") }
            guard let best = convert(selected) else {
                self.clipboard.restore(saved)
                return self.nothing("selection needs no fix")
            }
            self.clipboard.paste(best.text, restoring: saved)
            self.inputSources.select(best.layout)
            self.engine.reset()
        }
    }

    /// Whether ⌘C may stand in for reading the selection here. Not in terminals and code
    /// editors: with nothing selected, VS Code copies the whole line, and pasting it back would
    /// duplicate it.
    private var mayCopySelection: Bool {
        !(currentBundleID.map(Injector.alwaysSynthetic.contains) ?? false)
    }

    /// Nothing could be done — say so with a beep rather than silently doing nothing.
    private func nothing(_ reason: String) {
        Log.info(.inject, reason)
        beep()
    }

    /// The beep of "nothing to do"; replaced in tests.
    var beep: () -> Void = { NSSound.beep() }

    /// True while a password field is in front: the commands below that read text or the
    /// clipboard then do nothing at all. (From the keyboard they never get here, since the tap
    /// steps aside entirely; this covers the status menu and the settings window.)
    private func blockedBySecureInput() -> Bool {
        guard tap.isSecureInputActive else { return false }
        nothing("secure input is on; nothing was read")
        return true
    }

    // MARK: - Transliteration

    /// Rewrites the selection between Latin and Cyrillic Kazakh, falling back to the clipboard
    /// when nothing is selected. `direction` nil means "work it out from the text".
    private func makeTransliterator() -> Transliterator {
        Transliterator(
            models: models, overrides: prefs.translitOverrides,
            neverConvert: prefs.translitNeverConvert)
    }

    public func transliterate(direction: Transliterator.Direction?) {
        guard !blockedBySecureInput() else { return }
        let translit = makeTransliterator()
        let scheme = prefs.translitScheme

        // Selection first: that is what the user is looking at.
        if let element = AX.focusedElement(), let selected = AX.selectedText(element),
            !selected.isEmpty
        {
            let resolved = direction ?? Transliterator.direction(for: selected)
            let converted = translit.convert(selected, scheme: scheme, direction: resolved)
            guard converted != selected else { return }

            tap.isInjecting = true
            defer { tap.isInjecting = false }
            if AX.setSelectedText(element, converted) {
                engine.reset()
                // Follow the text: after writing Cyrillic, the next keystroke should be Cyrillic.
                switchLayoutForScript(resolved == .toCyrillic ? .cyrillic : .latin)
                // Recorded on the same undo stack as layout fixes. After the write the caret
                // sits past the new text, so the inverse replacement undoes it exactly the way
                // a layout fix is undone.
                undo.push(
                    AppliedFix(
                        kind: .transliteration,
                        replacement: Replacement(deletedText: selected, insertText: converted),
                        bundleID: currentBundleID,
                        originalWord: selected,
                        correctedWord: converted,
                        previousLayoutID: nil,
                        strategy: .accessibility))
                onStateChange?()
                return
            }
            Log.warn(.inject, "selection rejected the transliteration; copying instead")
        }

        // Nothing selected that we can see, and a line just typed: that line. Nobody selects
        // what they have just written before asking for it in the other alphabet.
        if transliterateTyped(direction: direction) { return }

        // A selection the app will not show us: copy it, convert it, paste it back.
        guard mayCopySelection else { return transliterateClipboard(direction: direction) }
        clipboard.copySelection { [weak self] selected, saved in
            guard let self else { return }
            guard let selected else {
                // Nothing selected at all: the documented fallback, the clipboard itself.
                return self.transliterateClipboard(direction: direction)
            }
            let resolved = direction ?? Transliterator.direction(for: selected)
            let converted = translit.convert(selected, scheme: scheme, direction: resolved)
            guard converted != selected else {
                self.clipboard.restore(saved)
                return self.nothing("nothing to transliterate in the selection")
            }
            self.clipboard.paste(converted, restoring: saved)
            self.engine.reset()
            self.switchLayoutForScript(resolved == .toCyrillic ? .cyrillic : .latin)
            self.undo.push(
                AppliedFix(
                    kind: .transliteration,
                    replacement: Replacement(deletedText: selected, insertText: converted),
                    bundleID: self.currentBundleID,
                    originalWord: selected,
                    correctedWord: converted,
                    previousLayoutID: nil,
                    strategy: .synthetic))
            self.onStateChange?()
        }
    }

    /// Converts the text typed since the buffer last started, where it stands. False when there
    /// is none, or it could not be replaced — the caller then tries the selection.
    private func transliterateTyped(direction: Transliterator.Direction?) -> Bool {
        let keyed = engine.screenText(resolve: { [inputSources] in
            inputSources.table(forSourceID: $0)
        })
        guard keyed.contains(where: \.isLetter) else { return false }
        // The direction comes from what the user typed, never from the rest of the line.
        let resolved = direction ?? Transliterator.direction(for: keyed)
        // What is on screen, where the app will say: the host app may have autocorrected the
        // Latin words (`qalaisyz` → `Qalaisyn` in TextEdit), and the letters on screen are the
        // ones to convert. Otherwise the keystrokes are all there is.
        let typed = screenTail(words: Self.wordCount(keyed), maxLength: keyed.utf16.count * 2 + 32) ?? keyed
        let converted = makeTransliterator().convert(
            typed, scheme: prefs.translitScheme, direction: resolved)
        guard converted != typed else { return false }

        // Only the part that changes is retyped; the caret stays at the end.
        let shared = typed.commonPrefix(with: converted).count
        let plan = Replacement(
            deletedText: String(typed.dropFirst(shared)),
            insertText: String(converted.dropFirst(shared)))

        switch injector.replace(plan, bundleID: currentBundleID) {
        case .applied(let strategy, let inserted):
            engine.reset()
            session += 1
            switchLayoutForScript(resolved == .toCyrillic ? .cyrillic : .latin)
            undo.push(
                AppliedFix(
                    kind: .transliteration,
                    replacement: Replacement(deletedText: plan.deletedText, insertText: inserted),
                    bundleID: currentBundleID,
                    originalWord: typed,
                    correctedWord: converted,
                    previousLayoutID: nil,
                    strategy: strategy))
            onStateChange?()
            return true
        case .mismatch, .failed:
            engine.reset()
            session += 1
            return false
        }
    }

    private static func wordCount(_ text: String) -> Int {
        text.split(whereSeparator: \.isWhitespace).count
    }

    /// The last `words` words before the caret with whatever follows them, read from the
    /// focused field — or nil where the field cannot be read. Stops at a line break: the words
    /// typed since the buffer started are all on the current line.
    private func screenTail(words: Int, maxLength: Int) -> String? {
        guard words > 0, AX.isTrusted, !tap.isSecureInputActive,
            !(currentBundleID.map(Injector.alwaysSynthetic.contains) ?? false),
            let element = AX.focusedElement(),
            let caret = AX.selectedRange(element), caret.length == 0, caret.location > 0
        else { return nil }
        let start = max(0, caret.location - maxLength)
        guard
            let before = AXTextField(element).string(
                in: NSRange(location: start, length: caret.location - start))
        else { return nil }

        var seen = 0
        var inWord = false
        var cut = before.endIndex
        for index in before.indices.reversed() {
            let character = before[index]
            if character.isNewline { break }
            if character.isWhitespace {
                if inWord {
                    inWord = false
                    if seen == words { break }
                }
            } else if !inWord {
                inWord = true
                seen += 1
            }
            cut = index
        }
        guard seen == words else { return nil }
        let tail = String(before[cut...])
        return tail.isEmpty ? nil : tail
    }

    public func transliterateClipboard(direction: Transliterator.Direction?) {
        guard !blockedBySecureInput() else { return }
        let translit = makeTransliterator()
        guard let text = usableClipboardText(whenEmpty: "nothing to transliterate") else { return }
        let resolved = direction ?? Transliterator.direction(for: text)
        let converted = translit.convert(text, scheme: prefs.translitScheme, direction: resolved)
        guard converted != text else { return }
        board.replaceWithText(converted, transient: false)

        undo.push(
            AppliedFix(
                kind: .transliteration,
                clipboardBefore: text,
                replacement: Replacement(deletedText: text, insertText: converted),
                bundleID: currentBundleID,
                originalWord: text,
                correctedWord: converted,
                previousLayoutID: nil,
                strategy: .accessibility))
        onStateChange?()
    }

    /// The clipboard's text if the privacy rules let it be used: not flagged concealed or
    /// transient, not longer than 10,000 characters. Otherwise nil, after one beep with the
    /// reason (`whenEmpty` for a clipboard without text; nil to stay silent about that). A
    /// refused clipboard is neither read further, converted, nor kept.
    private func usableClipboardText(whenEmpty emptyReason: String?) -> String? {
        switch ClipboardPolicy.readText(board) {
        case .text(let text):
            return text
        case .empty:
            if let emptyReason { nothing(emptyReason) }
            return nil
        case .refusedSensitive:
            nothing("clipboard is flagged concealed or transient; left alone")
            return nil
        case .refusedTooLong:
            nothing("clipboard text is too long; left alone")
            return nil
        }
    }

    /// Moves to a layout of the given script, preferring the one this app was last written in.
    private func switchLayoutForScript(_ script: Script) {
        if script == .cyrillic, let remembered = rules.rememberedCyrillic(for: currentBundleID),
            inputSources.table(forSourceID: remembered) != nil
        {
            inputSources.select(sourceID: remembered)
            return
        }
        guard
            let table = inputSources.tables.first(where: { $0.lang?.script == script })
        else { return }
        inputSources.select(table)
    }

    public func convertClipboard() {
        guard !blockedBySecureInput() else { return }
        // The privacy check first: a clipboard that is off limits is not even looked at.
        guard let text = usableClipboardText(whenEmpty: nil) else { return }
        guard let source = inputSources.current, models.isLoaded else { return }
        let layouts = inputSources.candidateTables(limitedTo: snapshot.enabledLanguages)
        guard
            let best = TextConverter.bestConversion(
                text, from: source, layouts: layouts, models: models)
        else { return }
        board.replaceWithText(best.text, transient: false)
    }
}

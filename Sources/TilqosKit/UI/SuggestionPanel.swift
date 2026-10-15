import AppKit

/// The floating hint that offers a correction without interrupting typing.
///
/// It must never take focus. A panel that activates would steal the caret from the text
/// field the user is typing into, which would break the very thing it is offering to fix.
public final class SuggestionPanel {
    private let host = CaretPanel()
    private let prefs: Prefs

    public var isVisible: Bool { host.isVisible }

    /// Fired when the panel hides on its own, so the pipeline can forget the pending fix.
    public var onDismiss: (() -> Void)?

    /// How many times the panel spells out what the keys do before trusting the user to
    /// know. Enough to learn it, few enough not to become noise.
    static let tipCount = 3

    public init(prefs: Prefs = .shared) {
        self.prefs = prefs
        host.onTimeout = { [weak self] in self?.onDismiss?() }
    }

    // MARK: - Presentation

    /// Shows the corrected text and which key applies it.
    ///
    /// `anchor` is the caret rectangle when the app exposes one. Everything else is a
    /// fallback chain ending at the menu bar, so the hint is always somewhere sensible.
    public func show(
        before: String,
        corrected: String,
        acceptKey: AcceptKey,
        anchor: CGRect?,
        timeout: TimeInterval,
        statusItemAnchor: CGRect? = nil
    ) {
        var tip: String?
        let shown = prefs.suggestionTipsShown
        if shown < Self.tipCount {
            tip = Self.tip(for: acceptKey)
            prefs.suggestionTipsShown = shown + 1
        }
        host.hint.configure(
            .init(
                before: before, after: corrected,
                style: .suggestion(acceptKey: acceptKey.keycapTitle), tip: tip))
        host.show(caret: anchor, fallback: statusItemAnchor, timeout: timeout, countdown: true)
    }

    static func tip(for acceptKey: AcceptKey) -> String {
        L10n.tr("hint.tip", acceptKey.keycapTitle)
    }

    public func hide() {
        host.hide()
    }
}

extension AcceptKey {
    /// Glyph shown in the hint.
    public var symbol: String {
        switch self {
        case .tab: return "⇥"
        case .optionSpace: return "⌥␣"
        }
    }

    /// What the keycap says. Words rather than glyphs: `⇥` is not something most people
    /// recognise, `Tab` is printed on the key.
    public var keycapTitle: String {
        switch self {
        case .tab: return "Tab"
        case .optionSpace: return "⌥ Space"
        }
    }
}

// MARK: - Panel host

/// A borderless, non-activating panel that sits at the caret and fades in and out.
///
/// Shared by the suggestion and the "corrected" confirmation so both appear in the same
/// place, move the same way and never steal focus.
final class CaretPanel {
    let hint = HintView()
    private let panel: NSPanel

    private(set) var isVisible = false
    /// Called when the panel hides because its time ran out, not when it is dismissed.
    var onTimeout: (() -> Void)?

    private var dismissTimer: Timer?
    /// Bumped on every show and hide, so a fade-out that finishes after the panel was
    /// shown again does not order it out.
    private var generation = 0

    init() {
        panel = NSPanel(
            contentRect: NSRect(x: 0, y: 0, width: 260, height: 36),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: true
        )
        panel.isFloatingPanel = true
        panel.level = .statusBar
        panel.hidesOnDeactivate = false
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        // Never accept a click or a key: this is a hint, not a control. The accept key is
        // taken by the event tap, which does not require focus here.
        panel.ignoresMouseEvents = true
        panel.becomesKeyOnlyIfNeeded = true
        panel.animationBehavior = .none
        // Stay put when the user switches Spaces or goes full screen.
        panel.collectionBehavior = [
            .canJoinAllSpaces, .fullScreenAuxiliary, .stationary, .ignoresCycle,
        ]
        panel.contentView = hint
    }

    func show(caret: CGRect?, fallback: CGRect?, timeout: TimeInterval, countdown: Bool) {
        generation += 1
        let size = hint.fittingSize
        let target = Self.frame(for: size, caret: caret, fallback: fallback)
        let wasVisible = isVisible && panel.isVisible
        isVisible = true

        if wasVisible {
            // Already up: update in place. Sliding it again would draw the eye for nothing.
            panel.setFrame(target, display: true)
        } else {
            let reduceMotion = NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
            // Starts a few points towards the text, so it reads as coming from the caret.
            let start = reduceMotion ? target : target.offsetBy(dx: 0, dy: 4)
            panel.setFrame(start, display: true)
            panel.alphaValue = 0
            panel.orderFrontRegardless()
            NSAnimationContext.runAnimationGroup { context in
                context.duration = 0.14
                context.timingFunction = CAMediaTimingFunction(name: .easeOut)
                panel.animator().alphaValue = 1
                if !reduceMotion { panel.animator().setFrame(target, display: true) }
            }
        }
        panel.invalidateShadow()

        if countdown { hint.startCountdown(timeout) } else { hint.stopCountdown() }
        announce()

        dismissTimer?.invalidate()
        let timer = Timer(timeInterval: timeout, repeats: false) { [weak self] _ in
            guard let self, self.isVisible else { return }
            self.hide()
            self.onTimeout?()
        }
        RunLoop.main.add(timer, forMode: .common)
        dismissTimer = timer
    }

    func hide() {
        dismissTimer?.invalidate()
        dismissTimer = nil
        guard isVisible else { return }
        isVisible = false
        generation += 1
        let expected = generation
        hint.stopCountdown()
        NSAnimationContext.runAnimationGroup { context in
            context.duration = 0.12
            panel.animator().alphaValue = 0
        } completionHandler: { [weak self] in
            self?.finishHide(expected: expected)
        }
    }

    /// After the fade-out: take the panel off screen and forget the words it showed, so that
    /// nothing typed lingers in a window nobody can see. A panel shown again in the meantime
    /// (a different generation) is left alone.
    func finishHide(expected: Int) {
        guard generation == expected else { return }
        panel.orderOut(nil)
        hint.reset()
    }

    /// VoiceOver users cannot see a hint that never takes focus, so it is read out instead.
    private func announce() {
        guard NSWorkspace.shared.isVoiceOverEnabled else { return }
        NSAccessibility.post(
            element: panel,
            notification: .announcementRequested,
            userInfo: [
                .announcement: HintView.spokenDescription(hint.content),
                .priority: NSAccessibilityPriorityLevel.high.rawValue,
            ])
    }

    // MARK: - Placement

    /// Just below the line being typed, starting at the caret, so the card covers the
    /// empty space the text is about to grow into rather than the word it is about.
    ///
    /// Screens are picked by where the caret is, not by where the panel would land, so a
    /// caret near the bottom edge flips the panel above the line instead of throwing it
    /// onto another display.
    static func frame(for size: NSSize, caret: CGRect?, fallback: CGRect?) -> NSRect {
        let gap: CGFloat = 6
        let margin: CGFloat = 6

        if let caret = plausible(caret), let screen = screen(for: caret) {
            let bounds = screen.visibleFrame
            var origin = NSPoint(x: caret.minX - 12, y: caret.minY - gap - size.height)
            if origin.y < bounds.minY + margin {
                origin.y = caret.maxY + gap
            }
            origin.x = min(max(origin.x, bounds.minX + margin), bounds.maxX - size.width - margin)
            origin.y = min(max(origin.y, bounds.minY + margin), bounds.maxY - size.height - margin)
            return NSRect(origin: origin, size: size)
        }

        // No caret: hang it from the menu bar icon, where it is at least always the same place.
        if let fallback, let screen = screen(for: fallback) ?? NSScreen.main {
            let bounds = screen.visibleFrame
            var origin = NSPoint(x: fallback.midX - size.width / 2, y: fallback.minY - gap - size.height)
            origin.x = min(max(origin.x, bounds.minX + margin), bounds.maxX - size.width - margin)
            origin.y = min(origin.y, bounds.maxY - size.height - margin)
            return NSRect(origin: origin, size: size)
        }

        let bounds = NSScreen.main?.visibleFrame ?? NSRect(x: 0, y: 0, width: 1440, height: 900)
        return NSRect(
            x: bounds.midX - size.width / 2, y: bounds.maxY - size.height - gap,
            width: size.width, height: size.height)
    }

    /// Some apps answer the caret query with a zero rect at the screen origin, or with
    /// coordinates for a window that is not on any display. Neither is somewhere to point at.
    private static func plausible(_ rect: CGRect?) -> CGRect? {
        guard let rect, rect.origin.x.isFinite, rect.origin.y.isFinite, rect.height > 0,
            rect.height < 400
        else { return nil }
        guard screen(for: rect) != nil else { return nil }
        return rect
    }

    private static func screen(for rect: CGRect) -> NSScreen? {
        let point = NSPoint(x: rect.midX, y: rect.midY)
        return NSScreen.screens.first { $0.frame.contains(point) }
            ?? NSScreen.screens.first { $0.frame.intersects(rect) }
    }
}

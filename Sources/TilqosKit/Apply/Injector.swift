import ApplicationServices
import Carbon.HIToolbox
import CoreGraphics
import Foundation

/// Applies a `Replacement` to whatever text field has focus.
///
/// Two routes, because no single one works everywhere. The accessibility API is precise and
/// leaves the host app's undo stack intact, but apps that draw their own text — VS Code, most
/// Electron editors, terminals — do not expose a settable caret. For those, synthetic Delete
/// presses and Unicode key events. Which route, and how each is checked, is `TextReplacer`'s
/// business; this class supplies the real text field and keyboard, and remembers per app what
/// worked.
public final class Injector {
    public enum Strategy: String, Sendable {
        case accessibility
        case synthetic
    }

    /// What happened to one replacement.
    public enum Result: Equatable, Sendable {
        /// Applied. `inserted` is the text that went in, which differs from the plan only by a
        /// capital the host app had given the word.
        case applied(Strategy, inserted: String)
        /// The text before the caret is not what the buffer says it is — autocorrect, a moved
        /// caret, a paste. Nothing was touched; the buffer must be thrown away.
        case mismatch
        /// Nothing could be applied, or the result could not be confirmed.
        case failed(String)

        public var strategy: Strategy? {
            if case .applied(let strategy, _) = self { return strategy }
            return nil
        }
    }

    /// Apps known to draw their own text. Probing them wastes a round trip per correction and
    /// occasionally moves the caret, so skip straight to synthetic — and do not try to read the
    /// text either: a terminal's accessibility value is its scrollback, not the line being
    /// typed, so a check against it would fail every time.
    static let alwaysSynthetic: Set<String> = [
        "com.microsoft.VSCode",
        "com.microsoft.VSCodeInsiders",
        "com.visualstudio.code.oss",
        "com.todesktop.230313mzl4w4u92",  // Cursor
        "com.apple.Terminal",
        "com.googlecode.iterm2",
        "com.mitchellh.ghostty",
        "net.kovidgoyal.kitty",
        "dev.warp.Warp-Stable",
        "org.alacritty",
        "com.jetbrains.intellij",
        "com.sublimetext.4",
    ]

    private weak var tap: EventTap?
    private var strategyCache: [String: Strategy] = [:]

    public init(tap: EventTap?) {
        self.tap = tap
    }

    // MARK: - Applying

    /// Returns the strategy that worked, or nil if the edit was not made. Use `replace` to find
    /// out why.
    @discardableResult
    public func apply(_ replacement: Replacement, bundleID: String? = nil) -> Strategy? {
        replace(replacement, bundleID: bundleID).strategy
    }

    public func replace(_ replacement: Replacement, bundleID: String? = nil) -> Result {
        guard !replacement.isEmpty else { return .failed("empty replacement") }
        let bundleID = bundleID ?? AX.frontmostBundleID
        let trusted = AX.isTrusted
        let drawsOwnText = bundleID.map(Self.alwaysSynthetic.contains) ?? false

        // Everything we post comes back through our own tap; this flag plus the source marker
        // is what stops the app correcting its own output.
        tap?.isInjecting = true
        defer { tap?.isInjecting = false }

        var replacer = TextReplacer(
            field: trusted && !drawsOwnText ? AX.focusedElement().map(AXTextField.init) : nil,
            keys: SyntheticKeys(),
            wait: { usleep(useconds_t($0 * 1_000_000)) },
            preferAccessibility: preferredStrategy(for: bundleID) == .accessibility,
            verify: !drawsOwnText)
        replacer.trace = { Log.debug(.inject, $0) }
        let outcome = replacer.apply(replacement)

        // Remember an app whose accessibility support misbehaved, so the next correction there
        // goes straight to keystrokes. A text mismatch is not the app's fault and teaches
        // nothing about it.
        if replacer.accessibilityUnreliable, let bundleID {
            strategyCache[bundleID] = .synthetic
            // Which app is the user's business: it goes in as a private value.
            Log.info(.inject, "accessibility edit unreliable, using keystrokes for app", private: bundleID)
        }

        switch outcome {
        case .applied(let strategy, let inserted):
            return .applied(strategy, inserted: inserted)
        case .mismatch:
            Log.info(
                .inject,
                "screen does not match the buffer (\(replacement.deleteCount) chars); nothing done")
            return .mismatch
        case .failed(let reason):
            Log.warn(.inject, "replacement not applied: \(reason)")
            return .failed(reason)
        }
    }

    private func preferredStrategy(for bundleID: String?) -> Strategy {
        guard AX.isTrusted else { return .synthetic }
        guard let bundleID else { return .accessibility }
        if Self.alwaysSynthetic.contains(bundleID) { return .synthetic }
        return strategyCache[bundleID] ?? .accessibility
    }

    /// Forgets what was learned about apps, so a fresh probe happens next time.
    public func resetStrategyCache() {
        strategyCache.removeAll()
    }

    public func knownStrategies() -> [String: Strategy] { strategyCache }
}

// MARK: - The real text field

/// `TextFieldAccess` over an `AXUIElement`.
final class AXTextField: TextFieldAccess {
    let element: AXUIElement

    /// Longest value read whole when the app cannot give us a range. Past this, the round trip
    /// costs more than the check is worth.
    static let maximumValueLength = 20_000

    init(_ element: AXUIElement) {
        self.element = element
    }

    func selectedRange() -> NSRange? {
        AX.selectedRange(element).map { NSRange(location: $0.location, length: $0.length) }
    }

    func setSelectedRange(_ range: NSRange) -> Bool {
        AX.setSelectedRange(element, CFRange(location: range.location, length: range.length))
    }

    func string(in range: NSRange) -> String? {
        // Just the characters we need, where the app supports it.
        if let text = AX.string(element, for: CFRange(location: range.location, length: range.length)) {
            return text
        }
        // Otherwise a slice of the whole value — for fields small enough to read.
        if let count = AX.numberOfCharacters(element), count > Self.maximumValueLength { return nil }
        guard let value = AX.string(element, kAXValueAttribute as String) else { return nil }
        let text = value as NSString
        guard range.location >= 0, NSMaxRange(range) <= text.length else { return nil }
        return text.substring(with: range)
    }

    func selectedText() -> String? {
        AX.selectedText(element)
    }

    func setSelectedText(_ text: String) -> Bool {
        AX.setSelectedText(element, text)
    }

    var canSetSelectedRange: Bool { AX.isSettable(element, kAXSelectedTextRangeAttribute) }
    var canSetSelectedText: Bool { AX.isSettable(element, kAXSelectedTextAttribute) }
}

// MARK: - The real keyboard

/// Posts synthetic keystrokes stamped with our marker.
struct SyntheticKeys: KeystrokeSender {
    /// Spacing between synthesised keystrokes, so each carries its own timestamp. Ordering does
    /// not depend on it — the events queue behind our own tap until this call returns — but
    /// some apps coalesce events that arrive with identical timestamps.
    private let interval: useconds_t = 200

    func sendBackspaces(_ count: Int) -> Bool {
        guard let source = EventMarker.makeSource() else {
            Log.error(.inject, "could not create event source")
            return false
        }
        for _ in 0..<count {
            guard post(keyCode: UInt16(kVK_Delete), text: nil, source: source) else { return false }
            usleep(interval)
        }
        return true
    }

    /// The text rides on Unicode key events of at most 20 UTF-16 units each — the documented
    /// limit of `keyboardSetUnicodeString`.
    ///
    /// The events carry virtual key 0, the conventional choice: apps read the attached string.
    /// A few that insist on reading the key code itself (some games, remote-desktop clients)
    /// would see the A key; they are not places where layout correction is useful anyway.
    func sendText(_ text: String) -> Bool {
        guard let source = EventMarker.makeSource() else {
            Log.error(.inject, "could not create event source")
            return false
        }
        for chunk in TextReplacer.chunks(text) {
            guard post(keyCode: 0, text: chunk, source: source) else { return false }
            usleep(interval)
        }
        return true
    }

    private func post(keyCode: UInt16, text: String?, source: CGEventSource) -> Bool {
        guard
            let down = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: true),
            let up = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: false)
        else { return false }
        // Whatever the user is holding must not colour our synthetic events.
        down.flags = []
        up.flags = []
        if let text {
            let units = Array(text.utf16)
            down.keyboardSetUnicodeString(stringLength: units.count, unicodeString: units)
            up.keyboardSetUnicodeString(stringLength: units.count, unicodeString: units)
        }
        down.post(tap: .cghidEventTap)
        up.post(tap: .cghidEventTap)
        return true
    }
}

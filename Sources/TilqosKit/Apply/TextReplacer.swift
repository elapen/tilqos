import Foundation

/// The focused text field, as far as the accessibility API lets us see and touch it.
///
/// A protocol so the replacement logic can be tested against simulated apps — including the
/// ones that misbehave, which is the whole point. The production implementation is a thin
/// wrapper over `AXUIElement` (`AXTextField`).
protocol TextFieldAccess: AnyObject {
    /// The selection, in UTF-16 units. A zero-length range is the caret.
    func selectedRange() -> NSRange?
    @discardableResult func setSelectedRange(_ range: NSRange) -> Bool
    /// The text in a range, or nil when the app will not say.
    func string(in range: NSRange) -> String?
    func selectedText() -> String?
    @discardableResult func setSelectedText(_ text: String) -> Bool
    var canSetSelectedRange: Bool { get }
    var canSetSelectedText: Bool { get }
}

/// Synthetic key presses.
protocol KeystrokeSender {
    /// Posts `count` presses of Delete. False when the events could not be created.
    func sendBackspaces(_ count: Int) -> Bool
    /// Types `text` as Unicode key events.
    func sendText(_ text: String) -> Bool
}

/// Replaces the text right before the caret, and refuses to guess.
///
/// The failure this exists to prevent is the one users reported: the correction appearing
/// *next to* the wrong text instead of replacing it. That happens when an edit is made without
/// confirming what it acts on — an app accepts "select these characters" but ignores it, or
/// cannot report the selection, and then "replace the selection with this" simply inserts. So
/// every step is checked wherever the app lets us check it:
///
/// 1. Before anything is touched, the text before the caret must be the text about to be
///    deleted. If the app lets us read it and it is not, nothing is done at all — the host app
///    autocorrected, or the caret moved, and the buffer no longer describes the screen.
/// 2. The accessibility route writes only once the selection is confirmed to cover exactly that
///    text, and reads the result back. If the app inserted without deleting, the damage is
///    visible and bounded, and it is repaired with keystrokes.
/// 3. Otherwise keystrokes: Delete presses, then the text in chunks small enough that no app
///    truncates them.
struct TextReplacer {
    enum Outcome: Equatable {
        /// Done. `inserted` is what went in — the planned text, with the capital the host app
        /// gave the word carried over.
        case applied(Injector.Strategy, inserted: String)
        /// The screen does not show what we meant to replace. Nothing was touched.
        case mismatch
        /// The app refused, or left the text in a state we cannot vouch for. Nothing more is
        /// done: another attempt on text we cannot see would be a guess.
        case failed(String)

        var strategy: Injector.Strategy? {
            if case .applied(let strategy, _) = self { return strategy }
            return nil
        }
    }

    /// Nil when there is no focused element or no accessibility permission.
    let field: TextFieldAccess?
    let keys: KeystrokeSender
    /// Sleeps the calling thread. Injected so tests run instantly.
    let wait: (TimeInterval) -> Void
    /// Try the accessibility route first. False for apps known to draw their own text, and for
    /// apps where it has already failed once.
    let preferAccessibility: Bool
    /// Read the text to confirm it before deleting. False for terminals, whose accessibility
    /// value is the scrollback rather than the line being edited.
    let verify: Bool

    /// How long to wait for the app to catch up: with the keystroke that triggered the fix — it
    /// may not have inserted the separator yet when we first look — and with accessibility
    /// changes that some apps apply asynchronously. Short on purpose: the main thread, and with
    /// it the event tap, waits meanwhile.
    static let settleAttempts = 5
    static let settleInterval: TimeInterval = 0.008

    /// Set when the accessibility route misbehaved in a way that says to stop using it here.
    private(set) var accessibilityUnreliable = false

    /// Receives each step taken, in shapes only, for the debug log.
    var trace: (String) -> Void = { _ in }

    init(
        field: TextFieldAccess?, keys: KeystrokeSender, wait: @escaping (TimeInterval) -> Void,
        preferAccessibility: Bool, verify: Bool
    ) {
        self.field = field
        self.keys = keys
        self.wait = wait
        self.preferAccessibility = preferAccessibility
        self.verify = verify
    }

    mutating func apply(_ replacement: Replacement) -> Outcome {
        guard !replacement.isEmpty else { return .failed("empty replacement") }
        var replacement = replacement

        // 1. What is actually in front of the caret.
        var caret: NSRange?
        var confirmed = false
        if verify, let field {
            let confirmation = confirmText(replacement.deletedText, in: field)
            trace("confirm \(replacement.deleteCount) chars: \(confirmation.summary)")
            switch confirmation {
            case .matches(let at, let onScreen):
                caret = at
                confirmed = true
                replacement = Self.carryingCapital(from: onScreen, into: replacement)
            case .differs:
                return .mismatch
            case .selection:
                // Delete presses would remove the selection instead of our characters.
                return .failed("text is selected")
            case .unreadable(let at):
                caret = at
            }
        }

        // 2. Accessibility: only on confirmed text, only when the app lets us set both.
        if confirmed, preferAccessibility, let field, let caret,
            field.canSetSelectedRange, field.canSetSelectedText
        {
            let result = replaceViaAccessibility(replacement, caret: caret, in: field)
            trace("accessibility route: \(result)")
            switch result {
            case .done:
                return .applied(.accessibility, inserted: replacement.insertText)
            case .untouched:
                // The app would not take the edit and the text is as it was, so keystrokes
                // can still do the job.
                accessibilityUnreliable = true
            case .repaired:
                accessibilityUnreliable = true
                return .applied(.synthetic, inserted: replacement.insertText)
            case .unknown(let reason):
                accessibilityUnreliable = true
                return .failed(reason)
            }
        }

        // 3. Keystrokes.
        trace(
            "keystrokes: \(replacement.deleteCount) deletes, \(replacement.insertText.count) chars"
                + (field == nil ? ", no field" : confirmed ? ", confirmed" : ", unconfirmed"))
        return sendKeys(deleting: replacement.deleteCount, inserting: replacement.insertText)
            ? .applied(.synthetic, inserted: replacement.insertText)
            : .failed("could not post key events")
    }

    // MARK: - Confirming the text

    private enum Confirmation {
        case matches(NSRange, onScreen: String)
        case differs
        case selection
        /// The app will not say. The range is the caret if it would at least report that.
        case unreadable(NSRange?)

        /// For the log: the verdict without the text it was reached on.
        var summary: String {
            switch self {
            case .matches(_, let onScreen): return "matches" + (onScreen.first?.isUppercase == true ? ", capitalised" : "")
            case .differs: return "differs"
            case .selection: return "text selected"
            case .unreadable(let caret): return caret == nil ? "unreadable" : "unreadable, caret known"
            }
        }
    }

    private func confirmText(_ expected: String, in field: TextFieldAccess) -> Confirmation {
        let length = expected.utf16.count
        var sawText = false
        var sawSelection = false
        var caret: NSRange?
        for attempt in 0..<Self.settleAttempts {
            if attempt > 0 { wait(Self.settleInterval) }
            guard let selection = field.selectedRange() else { return .unreadable(caret) }
            caret = selection
            // A selection can be the app still catching up; give it a moment.
            sawSelection = selection.length > 0
            guard !sawSelection else { continue }
            guard selection.location >= length else {
                sawText = true
                continue
            }
            let range = NSRange(location: selection.location - length, length: length)
            guard let before = field.string(in: range) else { return .unreadable(selection) }
            sawText = true
            // The host app may have capitalised the word (sentence start in Notes, Mail); the
            // letters are still ours.
            if before == expected || before.lowercased() == expected.lowercased() {
                return .matches(selection, onScreen: before)
            }
        }
        if sawSelection { return .selection }
        return sawText ? .differs : .unreadable(caret)
    }

    /// When the app capitalised the first letter of what we are replacing, the replacement gets
    /// a capital too: `Ghbdtn` becomes `Привет`, not `привет`.
    static func carryingCapital(from onScreen: String, into replacement: Replacement)
        -> Replacement
    {
        guard onScreen != replacement.deletedText,
            let shown = onScreen.first, let planned = replacement.deletedText.first,
            shown.isUppercase, !planned.isUppercase,
            let first = replacement.insertText.first, first.isLetter
        else { return Replacement(deletedText: onScreen, insertText: replacement.insertText) }
        return Replacement(
            deletedText: onScreen,
            insertText: first.uppercased() + replacement.insertText.dropFirst())
    }

    // MARK: - Accessibility route

    private enum AccessibilityResult {
        case done
        /// Nothing changed; safe to fall back.
        case untouched
        /// The app inserted without deleting; fixed with keystrokes.
        case repaired
        case unknown(String)
    }

    private func replaceViaAccessibility(
        _ replacement: Replacement, caret: NSRange, in field: TextFieldAccess
    ) -> AccessibilityResult {
        let length = replacement.deleteUTF16Count
        let start = caret.location - length
        let target = NSRange(location: start, length: length)
        let restore = NSRange(location: caret.location, length: 0)

        // Select exactly the characters to replace, and see that the app really did.
        field.setSelectedRange(target)
        guard settle({ selection(of: field, is: target, holding: replacement.deletedText) }) else {
            // Put the caret back, and make sure it is back before anything else happens: an app
            // applying the selection late would otherwise turn the first Delete into "delete the
            // whole selection".
            field.setSelectedRange(restore)
            return settle({ field.selectedRange().map { $0 == restore } })
                ? .untouched : .unknown("selection did not settle")
        }

        // The return value is not trusted either way: some apps report an error for a write they
        // performed (a timeout), others report success for one they ignored. The text decides.
        field.setSelectedText(replacement.insertText)

        let insertLength = replacement.insertText.utf16.count
        let written = NSRange(location: start, length: insertLength)
        let expectedCaret = NSRange(location: start + insertLength, length: 0)
        if settle({
            guard let now = field.selectedRange() else { return nil }
            return now == expectedCaret && field.string(in: written) == replacement.insertText
        }) {
            return .done
        }

        // Not what we asked for. Work out from the screen what happened.
        if field.selectedRange() == target, field.string(in: target) == replacement.deletedText {
            // The write was ignored and our selection is still up. Take it down again, and the
            // text is exactly as it was.
            field.setSelectedRange(restore)
            return settle({ field.selectedRange().map { $0 == restore } })
                ? .untouched : .unknown("selection did not clear")
        }
        guard let now = field.selectedRange(), now.length == 0, now.location >= start,
            let region = field.string(in: NSRange(location: start, length: now.location - start))
        else { return .unknown("result unreadable") }

        if region == replacement.deletedText {
            // The write was ignored outright: the text is untouched and the caret where it
            // started, so keystrokes can do the whole job.
            return now.location == caret.location ? .untouched : .unknown("caret moved")
        }
        // Inserted without deleting — the reported bug — or any other arrangement of our own
        // two strings. Everything from `start` to the caret is ours: remove it and type the
        // replacement.
        if region == replacement.deletedText + replacement.insertText
            || region == replacement.insertText + replacement.deletedText
        {
            return sendKeys(deleting: region.count, inserting: replacement.insertText)
                ? .repaired : .unknown("repair failed")
        }
        return .unknown("unexpected result")
    }

    /// Nil when the app will not answer, which ends the wait.
    private func selection(of field: TextFieldAccess, is range: NSRange, holding text: String)
        -> Bool?
    {
        guard let selection = field.selectedRange() else { return nil }
        guard selection == range else { return false }
        // Where the app reports the selected text it must agree too; otherwise the range
        // read-back plus the text confirmed a moment ago will do.
        if let selected = field.selectedText() { return selected == text }
        return field.string(in: range).map { $0 == text } ?? true
    }

    /// Polls `condition` for as long as an app may take to apply a change asynchronously. A nil
    /// answer means the app is not answering at all, and waiting longer will not change that.
    private func settle(_ condition: () -> Bool?) -> Bool {
        for attempt in 0..<Self.settleAttempts {
            if attempt > 0 { wait(Self.settleInterval) }
            switch condition() {
            case true?: return true
            case false?: continue
            case nil: return false
            }
        }
        return false
    }

    // MARK: - Keystrokes

    private func sendKeys(deleting count: Int, inserting text: String) -> Bool {
        if count > 0, !keys.sendBackspaces(count) { return false }
        if text.isEmpty { return true }
        return keys.sendText(text)
    }

    /// Splits text into pieces of at most `limit` UTF-16 units without breaking a character.
    ///
    /// `CGEventKeyboardSetUnicodeString` carries at most 20 units per event, and a multi-word
    /// correction sent as one event came out truncated.
    static func chunks(_ text: String, limit: Int = 20) -> [String] {
        var result: [String] = []
        var current = ""
        var units = 0
        for character in text {
            let size = character.utf16.count
            if units + size > limit, !current.isEmpty {
                result.append(current)
                current = ""
                units = 0
            }
            current.append(character)
            units += size
        }
        if !current.isEmpty { result.append(current) }
        return result
    }
}

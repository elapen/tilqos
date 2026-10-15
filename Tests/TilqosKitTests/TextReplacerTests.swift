import XCTest

@testable import TilqosKit

/// The replacement logic against simulated apps, including the ones that misbehave.
///
/// The reported bug — Tab puts the correction *next to* the wrong text — is an app accepting an
/// accessibility edit it does not really perform. It cannot be reproduced against a live app in
/// a test, but every way an app can mislead us can be simulated, and for each one the text must
/// end up right or be left untouched. Never both old and new.
final class TextReplacerTests: XCTestCase {
    // MARK: - A simulated app

    /// A text field with a caret, and the quirks real apps have.
    final class FakeField: TextFieldAccess {
        struct Quirks {
            /// Accepts a new selection but keeps the old one (and says it succeeded).
            var ignoresSelection = false
            /// Applies a new selection only after this many reads of it — asynchronous apps.
            var selectionDelay = 0
            /// "Replace the selection" inserts at the end of it and deletes nothing.
            var insertsWithoutDeleting = false
            /// "Replace the selection" does nothing (and says it succeeded).
            var ignoresWrites = false
            /// "Replace the selection" works but reports an error.
            var writeReportsFailure = false
            /// Cannot report the text at all.
            var unreadable = false
            /// Cannot report the selection at all.
            var noSelection = false
            /// The last keystroke has not reached the field yet for this many reads.
            var lagReads = 0
            var laggingText = ""
        }

        var text: String
        var selection: NSRange
        var quirks: Quirks
        private var pendingSelection: NSRange?
        private var pendingReads = 0
        private var lagReadsLeft: Int

        init(_ text: String, quirks: Quirks = Quirks()) {
            self.text = text
            self.selection = NSRange(location: text.utf16.count, length: 0)
            self.quirks = quirks
            self.lagReadsLeft = quirks.lagReads
        }

        private var ns: NSString { text as NSString }

        func selectedRange() -> NSRange? {
            if quirks.noSelection { return nil }
            if let pending = pendingSelection {
                pendingReads -= 1
                if pendingReads < 0 {
                    selection = pending
                    pendingSelection = nil
                }
            }
            if lagReadsLeft > 0 {
                lagReadsLeft -= 1
                if lagReadsLeft == 0 {
                    // The keystroke arrives: text and caret move together.
                    text += quirks.laggingText
                    selection = NSRange(location: text.utf16.count, length: 0)
                }
                return selection
            }
            return selection
        }

        func setSelectedRange(_ range: NSRange) -> Bool {
            if quirks.ignoresSelection { return true }
            if quirks.selectionDelay > 0 {
                pendingSelection = range
                pendingReads = quirks.selectionDelay
                return true
            }
            selection = range
            return true
        }

        func string(in range: NSRange) -> String? {
            if quirks.unreadable { return nil }
            guard range.location >= 0, NSMaxRange(range) <= ns.length else { return nil }
            return ns.substring(with: range)
        }

        func selectedText() -> String? {
            if quirks.unreadable { return nil }
            return ns.substring(with: selection)
        }

        func setSelectedText(_ replacement: String) -> Bool {
            if quirks.ignoresWrites { return true }
            if quirks.insertsWithoutDeleting {
                let at = NSMaxRange(selection)
                text = ns.replacingCharacters(in: NSRange(location: at, length: 0), with: replacement)
                selection = NSRange(location: at + replacement.utf16.count, length: 0)
                return true
            }
            text = ns.replacingCharacters(in: selection, with: replacement)
            selection = NSRange(location: selection.location + replacement.utf16.count, length: 0)
            return !quirks.writeReportsFailure
        }

        var canSetSelectedRange: Bool { true }
        var canSetSelectedText: Bool { true }

        /// What a keystroke does to it.
        func backspace() {
            if selection.length > 0 {
                text = ns.replacingCharacters(in: selection, with: "")
                selection.length = 0
                return
            }
            guard selection.location > 0 else { return }
            let range = ns.rangeOfComposedCharacterSequence(at: selection.location - 1)
            text = ns.replacingCharacters(in: range, with: "")
            selection = NSRange(location: range.location, length: 0)
        }

        func type(_ typed: String) {
            text = ns.replacingCharacters(in: selection, with: typed)
            selection = NSRange(location: selection.location + typed.utf16.count, length: 0)
        }
    }

    /// Keystrokes that land in the fake field, and a record of them.
    final class FakeKeys: KeystrokeSender {
        weak var field: FakeField?
        var backspaces = 0
        var chunks: [String] = []

        func sendBackspaces(_ count: Int) -> Bool {
            backspaces += count
            for _ in 0..<count { field?.backspace() }
            return true
        }

        func sendText(_ text: String) -> Bool {
            for chunk in TextReplacer.chunks(text) {
                chunks.append(chunk)
                field?.type(chunk)
            }
            return true
        }
    }

    private let fix = Replacement(deletedText: "ghbdtn ", insertText: "привет ")

    private func run(
        _ field: FakeField?, _ replacement: Replacement? = nil, preferAccessibility: Bool = true,
        verify: Bool = true
    ) -> (TextReplacer.Outcome, FakeKeys, TextReplacer) {
        let keys = FakeKeys()
        keys.field = field
        var replacer = TextReplacer(
            field: field, keys: keys, wait: { _ in }, preferAccessibility: preferAccessibility,
            verify: verify)
        let outcome = replacer.apply(replacement ?? fix)
        return (outcome, keys, replacer)
    }

    // MARK: - Well-behaved apps

    func testWellBehavedAppIsEditedThroughAccessibility() {
        let field = FakeField("я сказал ghbdtn ")
        let (outcome, keys, _) = run(field)
        XCTAssertEqual(outcome, .applied(.accessibility, inserted: "привет "))
        XCTAssertEqual(field.text, "я сказал привет ")
        XCTAssertEqual(field.selection, NSRange(location: field.text.utf16.count, length: 0))
        XCTAssertEqual(keys.backspaces, 0)
    }

    func testAsynchronousSelectionIsWaitedFor() {
        var quirks = FakeField.Quirks()
        quirks.selectionDelay = 2
        let field = FakeField("ghbdtn ", quirks: quirks)
        let (outcome, _, _) = run(field)
        XCTAssertEqual(outcome.strategy, .accessibility)
        XCTAssertEqual(field.text, "привет ")
    }

    func testWriteThatReportsFailureButWorksIsAccepted() {
        var quirks = FakeField.Quirks()
        quirks.writeReportsFailure = true
        let field = FakeField("ghbdtn ", quirks: quirks)
        let (outcome, keys, _) = run(field)
        XCTAssertEqual(outcome.strategy, .accessibility)
        XCTAssertEqual(field.text, "привет ")
        XCTAssertEqual(keys.backspaces, 0, "a second, synthetic attempt would have doubled it")
    }

    // MARK: - The reported bug: text lands next to the old text

    /// The app says it selected the range but did not. The old code then "replaced the
    /// selection" — an empty one — and the correction appeared next to the garble.
    func testAppIgnoringTheSelectionNeverGetsTextNextToTheOldOne() {
        var quirks = FakeField.Quirks()
        quirks.ignoresSelection = true
        let field = FakeField("ok ghbdtn ", quirks: quirks)
        let (outcome, keys, replacer) = run(field)
        XCTAssertEqual(field.text, "ok привет ")
        XCTAssertEqual(outcome.strategy, .synthetic)
        XCTAssertEqual(keys.backspaces, 7)
        XCTAssertTrue(replacer.accessibilityUnreliable, "the app must be remembered as unreliable")
    }

    /// The app selects the range, then inserts at its end and deletes nothing.
    func testAppInsertingWithoutDeletingIsRepaired() {
        var quirks = FakeField.Quirks()
        quirks.insertsWithoutDeleting = true
        let field = FakeField("ok ghbdtn ", quirks: quirks)
        let (outcome, _, replacer) = run(field)
        XCTAssertEqual(field.text, "ok привет ", "the stray old text must be gone")
        XCTAssertEqual(outcome.strategy, .synthetic)
        XCTAssertTrue(replacer.accessibilityUnreliable)
    }

    func testAppIgnoringTheWriteFallsBackCleanly() {
        var quirks = FakeField.Quirks()
        quirks.ignoresWrites = true
        let field = FakeField("ok ghbdtn ", quirks: quirks)
        let (outcome, _, replacer) = run(field)
        XCTAssertEqual(field.text, "ok привет ")
        XCTAssertEqual(outcome.strategy, .synthetic)
        XCTAssertTrue(replacer.accessibilityUnreliable)
    }

    // MARK: - The screen disagrees with the buffer

    /// The host app autocorrected the word after we saw it. Deleting seven characters blindly
    /// would eat into text we never saw; nothing must happen instead.
    func testMismatchTouchesNothing() {
        let field = FakeField("ok ghbdtm ")
        let (outcome, keys, replacer) = run(field)
        XCTAssertEqual(outcome, .mismatch)
        XCTAssertEqual(field.text, "ok ghbdtm ")
        XCTAssertEqual(keys.backspaces, 0)
        XCTAssertTrue(keys.chunks.isEmpty)
        XCTAssertFalse(replacer.accessibilityUnreliable, "a mismatch is not the app's fault")
    }

    func testMismatchAlsoStopsTheKeystrokeRoute() {
        let field = FakeField("ok ghbdtm ")
        let (outcome, keys, _) = run(field, preferAccessibility: false)
        XCTAssertEqual(outcome, .mismatch)
        XCTAssertEqual(keys.backspaces, 0)
    }

    /// The separator that triggered the fix has not reached the app yet.
    func testLaggingKeystrokeIsWaitedFor() {
        var quirks = FakeField.Quirks()
        quirks.lagReads = 2
        quirks.laggingText = " "
        let field = FakeField("ghbdtn", quirks: quirks)
        let (outcome, _, _) = run(field)
        XCTAssertEqual(outcome.strategy, .accessibility)
        XCTAssertEqual(field.text, "привет ")
    }

    /// Notes and Mail capitalise the first word of a sentence behind our back.
    func testHostCapitalisationIsCarriedOver() {
        let field = FakeField("Ghbdtn ")
        let (outcome, _, _) = run(field)
        XCTAssertEqual(outcome, .applied(.accessibility, inserted: "Привет "))
        XCTAssertEqual(field.text, "Привет ")
    }

    func testSelectedTextIsNeverDeleted() {
        let field = FakeField("ok ghbdtn ")
        field.selection = NSRange(location: 0, length: 2)
        let (outcome, keys, _) = run(field)
        XCTAssertEqual(outcome, .failed("text is selected"))
        XCTAssertEqual(field.text, "ok ghbdtn ")
        XCTAssertEqual(keys.backspaces, 0)
    }

    // MARK: - Apps we cannot read

    /// With nothing to read, keystrokes are the only way, exactly as before.
    func testUnreadableFieldUsesKeystrokes() {
        var quirks = FakeField.Quirks()
        quirks.unreadable = true
        let field = FakeField("ghbdtn ", quirks: quirks)
        let (outcome, keys, _) = run(field)
        XCTAssertEqual(outcome.strategy, .synthetic)
        XCTAssertEqual(keys.backspaces, 7)
        XCTAssertEqual(field.text, "привет ")
    }

    func testNoFocusedElementUsesKeystrokes() {
        let keys = FakeKeys()
        var replacer = TextReplacer(
            field: nil, keys: keys, wait: { _ in }, preferAccessibility: true, verify: true)
        XCTAssertEqual(replacer.apply(fix), .applied(.synthetic, inserted: "привет "))
        XCTAssertEqual(keys.backspaces, 7)
        XCTAssertEqual(keys.chunks, ["привет "])
    }

    func testTerminalsSkipTheCheck() {
        // A terminal's value is its scrollback; it would never match.
        let field = FakeField("$ ls\nsomething else entirely")
        let (outcome, keys, _) = run(field, preferAccessibility: false, verify: false)
        XCTAssertEqual(outcome.strategy, .synthetic)
        XCTAssertEqual(keys.backspaces, 7)
    }

    // MARK: - Long text

    /// A multi-word correction used to ride on one event and got truncated past 20 units.
    func testLongInsertIsChunked() {
        let long = Replacement(
            deletedText: "ghbdtn rfr ltkf e nt,z ",
            insertText: "привет как дела у тебя, всё хорошо? ")
        var quirks = FakeField.Quirks()
        quirks.unreadable = true
        let field = FakeField(long.deletedText, quirks: quirks)
        let (_, keys, _) = run(field, long)
        XCTAssertEqual(field.text, long.insertText)
        XCTAssertGreaterThan(keys.chunks.count, 1)
        XCTAssertTrue(keys.chunks.allSatisfy { $0.utf16.count <= 20 })
    }

    func testChunksNeverSplitACharacter() {
        let text = "👨‍👩‍👧‍👦 сәлем 🇰🇿 hello"
        let chunks = TextReplacer.chunks(text, limit: 8)
        XCTAssertEqual(chunks.joined(), text)
        for chunk in chunks where chunk.count > 1 {
            XCTAssertLessThanOrEqual(chunk.utf16.count, 8)
        }
    }
}

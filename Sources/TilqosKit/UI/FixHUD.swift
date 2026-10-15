import AppKit

/// Brief "corrected" card after an automatic fix, with the key that takes it back.
///
/// Off by default (`Prefs.showHUD`). An automatic fix already happens exactly where the user
/// is looking — the word changes under the caret and the layout indicator flips — so a card
/// on top of that, dozens of times a day, is more distraction than information. It is there
/// for people who want the confirmation, or who are still learning that ⌥⇧Z exists; the
/// menu shows the last fix with a one-click undo either way.
final class FixHUD {
    private let host = CaretPanel()

    /// Long enough to read four words, short enough to be gone before the next sentence.
    static let duration: TimeInterval = 2.2

    func show(_ fix: AppliedFix, undo: Hotkey?, caret: CGRect?, fallback: CGRect?) {
        host.hint.configure(
            .init(
                before: fix.originalWord, after: fix.correctedWord,
                style: .applied(undoKey: undo.map(KeyNames.display))))
        host.show(caret: caret, fallback: fallback, timeout: Self.duration, countdown: false)
    }

    func hide() {
        host.hide()
    }
}

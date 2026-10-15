import AppKit
import Carbon.HIToolbox
import CoreGraphics

/// Reads and replaces the selection through the clipboard, for apps that will not show it to
/// the accessibility API — Chrome, Electron apps, Telegram.
///
/// ⌘C, read, ⌘V, then the user's clipboard put back as it was. Asynchronous by necessity: the
/// keystrokes we post pass through our own event tap, which runs on the main thread, so the app
/// cannot see ⌘C — let alone answer it — until this thread has returned to its run loop.
///
/// The privacy rules are `ClipboardPolicy`'s: a clipboard that is flagged concealed or
/// transient is left alone from the start, a copied selection that is flagged so is dropped
/// unread, text over 10,000 characters is ignored, and the previous contents come back in full.
final class SelectionClipboard {
    /// What was on the clipboard before we borrowed it.
    struct Saved {
        fileprivate let snapshot: ClipboardSnapshot?

        /// Nothing was borrowed, so there is nothing to give back.
        static let untouched = Saved(snapshot: nil)
    }

    /// How long an app gets to answer ⌘C. Generous: Electron apps copy asynchronously.
    static let copyTimeout: TimeInterval = 0.5
    /// How long the app gets to read the clipboard after ⌘V before it is put back.
    static let restoreDelay: TimeInterval = 0.5
    private static let pollInterval: TimeInterval = 0.02
    /// The most that is held in memory to restore the clipboard afterwards. A clipboard bigger
    /// than this is not borrowed at all: it could not be put back faithfully.
    static let saveLimit = 32 << 20

    private let board: ClipboardBoard
    private let scheduler: TimerScheduler
    private let sendShortcut: (UInt16) -> Bool

    init(
        board: ClipboardBoard = NSPasteboardBoard(),
        scheduler: TimerScheduler = MainQueueScheduler.shared,
        sendShortcut: @escaping (UInt16) -> Bool = SelectionClipboard.postShortcut
    ) {
        self.board = board
        self.scheduler = scheduler
        self.sendShortcut = sendShortcut
    }

    /// Copies the selection. Calls back on the main thread with the text and the clipboard as
    /// it was before. The text is nil when nothing usable was copied; the clipboard then holds
    /// what it held before (or was never touched), and `restore` need not be called.
    func copySelection(_ completion: @escaping (_ text: String?, _ saved: Saved) -> Void) {
        // Whatever is on the clipboard now is somebody's secret or scratch copy: ⌘C would
        // overwrite it, and putting it back would mean holding it. Do neither.
        guard !ClipboardPolicy.isSensitive(board) else {
            Log.debug(.inject, "clipboard is flagged concealed or transient; selection not copied")
            completion(nil, .untouched)
            return
        }
        guard let snapshot = board.snapshot(byteLimit: Self.saveLimit) else {
            Log.debug(.inject, "clipboard too large to put back; selection not copied")
            completion(nil, .untouched)
            return
        }
        let saved = Saved(snapshot: snapshot)
        let before = board.changeCount
        guard sendShortcut(UInt16(kVK_ANSI_C)) else {
            completion(nil, .untouched)
            return
        }
        let deadline = scheduler.now + Self.copyTimeout
        func poll() {
            if board.changeCount != before {
                switch ClipboardPolicy.readText(board) {
                case .text(let text):
                    completion(text, saved)
                case .refusedSensitive, .refusedTooLong, .empty:
                    // Not ours to read or convert. The user's clipboard goes back at once.
                    restore(saved)
                    completion(nil, .untouched)
                }
                return
            }
            guard scheduler.now < deadline else {
                completion(nil, .untouched)
                return
            }
            scheduler.schedule(after: Self.pollInterval, poll)
        }
        scheduler.schedule(after: Self.pollInterval, poll)
    }

    /// Pastes `text` over the selection, then puts `saved` back once the app has read it —
    /// unless something else has written to the clipboard in the meantime.
    func paste(_ text: String, restoring saved: Saved) {
        board.replaceWithText(text, transient: true)
        let ours = board.changeCount
        guard sendShortcut(UInt16(kVK_ANSI_V)) else {
            restore(saved)
            return
        }
        scheduler.schedule(after: Self.restoreDelay) { [weak self] in
            guard let self, self.board.changeCount == ours else { return }
            self.restore(saved)
        }
    }

    /// Puts the clipboard back without pasting anything.
    func restore(_ saved: Saved) {
        guard let snapshot = saved.snapshot else { return }
        board.restore(snapshot)
    }

    /// ⌘ plus a key, stamped as ours so our own tap lets it through untouched.
    static func postShortcut(_ keyCode: UInt16) -> Bool {
        guard let source = EventMarker.makeSource(),
            let down = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: true),
            let up = CGEvent(keyboardEventSource: source, virtualKey: keyCode, keyDown: false)
        else {
            Log.error(.inject, "could not create shortcut events")
            return false
        }
        down.flags = .maskCommand
        up.flags = .maskCommand
        down.post(tap: .cghidEventTap)
        up.post(tap: .cghidEventTap)
        return true
    }
}

import AppKit
import ApplicationServices

/// Finds where on screen the text caret is, so the suggestion appears next to what the
/// user is typing rather than in a corner.
public enum CaretLocator {
    /// Caret rectangle in Cocoa screen coordinates, or nil when the focused app does not
    /// expose it — Electron editors and terminals usually do not.
    public static func caretRect() -> CGRect? {
        guard AX.isTrusted, let element = AX.focusedElement() else { return nil }
        guard let selection = AX.selectedRange(element) else { return nil }

        // A zero-length range gives an empty rect in some apps, so try one character wide
        // as a fallback, then the character just before the caret.
        let attempts = [
            CFRange(location: selection.location, length: 0),
            CFRange(location: selection.location, length: 1),
            CFRange(location: max(0, selection.location - 1), length: 1),
        ]
        for range in attempts {
            guard let rect = AX.boundsForRange(element, range) else { continue }
            guard rect.width.isFinite, rect.height.isFinite, rect.height > 0 else { continue }
            return flipToCocoa(rect)
        }
        return nil
    }

    /// Accessibility reports rectangles with the origin at the top-left of the primary
    /// display; Cocoa windows are placed from the bottom-left.
    private static func flipToCocoa(_ rect: CGRect) -> CGRect? {
        guard let primary = NSScreen.screens.first else { return nil }
        let flippedY = primary.frame.maxY - rect.origin.y - rect.height
        return CGRect(x: rect.origin.x, y: flippedY, width: rect.width, height: rect.height)
    }
}

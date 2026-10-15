import Foundation

/// An edit to apply at the caret: remove `deletedText` from behind it, put `insertText`
/// there instead.
///
/// Holding the removed text rather than just a count pays off three times: backspaces
/// need a grapheme count, accessibility ranges need a UTF-16 count, and undo needs the
/// original characters back.
public struct Replacement: Equatable, Sendable {
    /// What is on screen now and is about to go away.
    public var deletedText: String
    public var insertText: String

    public init(deletedText: String, insertText: String) {
        self.deletedText = deletedText
        self.insertText = insertText
    }

    /// Backspace presses required — one per grapheme cluster.
    public var deleteCount: Int { deletedText.count }
    /// Accessibility text ranges are measured in UTF-16 code units.
    public var deleteUTF16Count: Int { deletedText.utf16.count }

    public var isEmpty: Bool { deletedText.isEmpty && insertText.isEmpty }

    /// The edit that puts things back the way they were.
    public var inverse: Replacement {
        Replacement(deletedText: insertText, insertText: deletedText)
    }
}

/// A word the detector wants re-rendered under a different layout.
public struct WordCorrection: Sendable {
    /// Indices into the recorded strokes.
    public var range: Range<Int>
    /// Input source ID of the layout the word should be rendered under.
    public var targetLayoutID: String

    public init(range: Range<Int>, targetLayoutID: String) {
        self.range = range
        self.targetLayoutID = targetLayoutID
    }
}

/// Turns a set of word corrections into a single minimal edit.
///
/// Two rules matter here. Separators are never re-rendered — a period the user typed is
/// a period they meant, even though the same key yields `ю` on the Russian layout. And
/// the common prefix is preserved, so correcting `с2леметс3з` to `сәлеметсіз` deletes
/// nine characters rather than ten.
public enum ReplacementPlanner {
    public static func plan(
        recorded: [RecordedStroke],
        corrections: [WordCorrection],
        layout: (String) -> KeyMapTable?
    ) -> Replacement? {
        guard let start = corrections.map(\.range.lowerBound).min(), start < recorded.count
        else { return nil }

        var displayed = ""
        var corrected = ""

        for index in start..<recorded.count {
            let entry = recorded[index]
            guard let source = layout(entry.layoutID) else { return nil }
            let asTyped = source.text(for: entry.stroke) ?? ""
            displayed += asTyped

            // Only strokes inside a correction change layout; everything between them —
            // spaces, commas, digits the user meant — stays exactly as typed.
            if let correction = corrections.first(where: { $0.range.contains(index) }),
                let target = layout(correction.targetLayoutID)
            {
                corrected += target.text(for: entry.stroke) ?? asTyped
            } else {
                corrected += asTyped
            }
        }

        guard displayed != corrected else { return nil }

        let shared = displayed.commonPrefix(with: corrected).count
        let replacement = Replacement(
            deletedText: String(displayed.dropFirst(shared)),
            insertText: String(corrected.dropFirst(shared))
        )
        return replacement.isEmpty ? nil : replacement
    }

    /// The strokes that replace the tail once a correction is applied, so the buffer can
    /// keep matching the screen without waiting to see its own output come back.
    public static func rewrittenTail(
        recorded: [RecordedStroke],
        corrections: [WordCorrection],
        from start: Int
    ) -> [RecordedStroke] {
        recorded[start...].enumerated().map { offset, entry in
            var entry = entry
            if let correction = corrections.first(where: { $0.range.contains(start + offset) }) {
                entry.layoutID = correction.targetLayoutID
            }
            return entry
        }
    }
}

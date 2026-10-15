import Foundation

/// The recent keystrokes, in memory only.
///
/// This is the most privacy-sensitive object in the app, so it is deliberately small,
/// never written to disk, and thrown away at the first sign that it no longer describes
/// what is on screen.
public final class StrokeBuffer {
    public private(set) var recorded: [RecordedStroke] = []

    /// Long enough to hold a sentence or two, which is the span the mixed-language
    /// segmenter reasons over; short enough that it is never a transcript.
    public let capacity: Int
    /// Seconds without a key after which the buffer is dropped. Never more than
    /// `maximumIdleReset`: the app promises that typed text does not outlive five seconds of
    /// silence, so a hand-edited preference cannot stretch it.
    public let idleReset: TimeInterval

    /// The longest idle period the buffer will honour.
    public static let maximumIdleReset: TimeInterval = 5
    /// The shortest: below this the buffer would be wiped between two keys of one word.
    public static let minimumIdleReset: TimeInterval = 1

    /// Pulls a period into `minimumIdleReset...maximumIdleReset`; zero or less (unset) means the
    /// default, five seconds.
    public static func clamped(_ seconds: TimeInterval) -> TimeInterval {
        seconds <= 0 ? maximumIdleReset : min(max(seconds, minimumIdleReset), maximumIdleReset)
    }

    public init(capacity: Int = 256, idleReset: TimeInterval = 5) {
        self.capacity = capacity
        self.idleReset = Self.clamped(idleReset)
        recorded.reserveCapacity(capacity)
    }

    public var isEmpty: Bool { recorded.isEmpty }
    public var count: Int { recorded.count }
    public var lastActivity: TimeInterval { recorded.last?.time ?? 0 }

    /// Appends a stroke, first discarding the buffer if the user has been idle long
    /// enough that the old text is no longer in front of them.
    /// Returns true when an idle reset happened.
    ///
    /// This check is only the second line of defence: `TypingEngine` also runs a real timer that
    /// empties the buffer after `idleReset` seconds, without waiting for the next key.
    @discardableResult
    public func append(_ stroke: KeyStroke, at time: TimeInterval, layoutID: String) -> Bool {
        var didReset = false
        if let last = recorded.last, time - last.time > idleReset {
            recorded.removeAll(keepingCapacity: true)
            didReset = true
        }
        recorded.append(RecordedStroke(stroke: stroke, time: time, layoutID: layoutID))
        if recorded.count > capacity {
            recorded.removeFirst(recorded.count - capacity)
        }
        return didReset
    }

    /// Mirrors a Delete keypress so the buffer keeps tracking the visible text.
    public func removeLast() {
        guard !recorded.isEmpty else { return }
        recorded.removeLast()
    }

    public func reset() {
        recorded.removeAll(keepingCapacity: true)
    }

    /// Drops the trailing strokes covered by a replacement we just applied, and
    /// substitutes the strokes the corrected text now stands for, so the buffer keeps
    /// matching the screen without a round trip through the tap.
    public func replaceTail(from index: Int, with strokes: [RecordedStroke]) {
        guard index <= recorded.count else { return }
        recorded.replaceSubrange(index..<recorded.count, with: strokes)
    }
}

/// Splits a keystroke run into words and separators.
///
/// Word characters are letters, digits and the apostrophe. Digits count on purpose:
/// a Kazakh word typed on the Russian layout comes out as `с2леметс3з`, and that has to
/// stay one token for the detector to see the mistake.
public enum Tokenizer {
    public struct Token {
        public var isWord: Bool
        /// Indices into the recorded strokes.
        public var range: Range<Int>
        /// The text as it currently appears on screen.
        public var displayed: String

        public init(isWord: Bool, range: Range<Int>, displayed: String) {
            self.isWord = isWord
            self.range = range
            self.displayed = displayed
        }
    }

    public static func isWordCharacter(_ character: Character) -> Bool {
        character.isLetter || character.isNumber || character == "'" || character == "\u{2019}"
    }

    /// `layout` resolves an input source ID to its table; strokes whose layout is gone
    /// are treated as separators, since we can no longer say what they produced.
    public static func tokenize(
        _ recorded: [RecordedStroke], layout: (String) -> KeyMapTable?
    ) -> [Token] {
        var tokens: [Token] = []
        var index = 0

        while index < recorded.count {
            let startIndex = index
            let startIsWord = classify(recorded[index], layout: layout).isWord
            var text = ""

            while index < recorded.count {
                let (isWord, produced) = classify(recorded[index], layout: layout)
                if isWord != startIsWord { break }
                text += produced
                index += 1
            }

            tokens.append(
                Token(isWord: startIsWord, range: startIndex..<index, displayed: text))
        }
        return tokens
    }

    private static func classify(
        _ recorded: RecordedStroke, layout: (String) -> KeyMapTable?
    ) -> (isWord: Bool, produced: String) {
        guard let table = layout(recorded.layoutID),
            let text = table.text(for: recorded.stroke)
        else {
            return (false, "")
        }
        let isWord = !text.isEmpty && text.allSatisfy(isWordCharacter)
        return (isWord, text)
    }

    /// The word the caret sits at the end of, if the last thing typed was part of a word.
    public static func wordInProgress(_ tokens: [Token]) -> Token? {
        guard let last = tokens.last, last.isWord else { return nil }
        return last
    }

    /// The most recent finished word — the one a just-typed space or comma closed.
    public static func lastCompletedWord(_ tokens: [Token]) -> Token? {
        guard let last = tokens.last else { return nil }
        if last.isWord { return nil }
        return tokens.dropLast().last(where: \.isWord)
    }
}

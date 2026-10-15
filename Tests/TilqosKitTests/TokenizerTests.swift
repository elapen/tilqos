import XCTest

@testable import TilqosKit

final class TokenizerTests: XCTestCase {
    private static let manager: InputSourceManager = {
        let manager = InputSourceManager()
        manager.reloadTables()
        return manager
    }()

    private func table(_ sourceID: String) throws -> KeyMapTable {
        guard let table = Self.manager.table(forSourceID: sourceID) else {
            throw XCTSkip("Layout \(sourceID) is not enabled on this machine")
        }
        return table
    }

    /// Resolves layout IDs the way the running app does.
    private func resolver() -> (String) -> KeyMapTable? {
        { [tables = Self.manager.tables] id in tables.first { $0.sourceID == id } }
    }

    /// Builds the stroke record that typing `text` on `table` would have produced.
    private func record(_ text: String, on table: KeyMapTable) throws -> [RecordedStroke] {
        let strokes = try XCTUnwrap(table.strokes(for: text), "\(table.sourceID) cannot type \(text)")
        return strokes.enumerated().map {
            RecordedStroke(stroke: $1, time: Double($0) * 0.05, layoutID: table.sourceID)
        }
    }

    // MARK: - Segmentation

    func testWordsAndSeparatorsAlternate() throws {
        let russian = try table(Self.russianID)
        let tokens = Tokenizer.tokenize(try record("привет мир", on: russian), layout: resolver())

        XCTAssertEqual(tokens.map(\.isWord), [true, false, true])
        XCTAssertEqual(tokens.map(\.displayed), ["привет", " ", "мир"])
    }

    /// The digits a Kazakh word turns into on the Russian layout must not split the word,
    /// or the detector never gets to see the mistake.
    func testDigitsStayInsideAWord() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        // Typed on the Kazakh layout, but displayed through Russian — the mistake case.
        let strokes = try XCTUnwrap(kazakh.strokes(for: "сәлеметсіз"))
        let recorded = strokes.enumerated().map {
            RecordedStroke(stroke: $1, time: Double($0) * 0.05, layoutID: russian.sourceID)
        }

        let tokens = Tokenizer.tokenize(recorded, layout: resolver())
        XCTAssertEqual(tokens.count, 1)
        XCTAssertEqual(tokens[0].displayed, "с2леметс3з")
        XCTAssertTrue(tokens[0].isWord)
    }

    func testPunctuationBreaksWords() throws {
        let abc = try table(Self.abcID)
        let tokens = Tokenizer.tokenize(try record("a, b.", on: abc), layout: resolver())

        XCTAssertEqual(tokens.map(\.displayed), ["a", ", ", "b", "."])
    }

    func testApostropheStaysInsideAWord() throws {
        let abc = try table(Self.abcID)
        let tokens = Tokenizer.tokenize(try record("don't", on: abc), layout: resolver())

        XCTAssertEqual(tokens.count, 1)
        XCTAssertEqual(tokens[0].displayed, "don't")
    }

    // MARK: - Position helpers

    func testWordInProgressAndLastCompletedWord() throws {
        let abc = try table(Self.abcID)

        let midWord = Tokenizer.tokenize(try record("hello wor", on: abc), layout: resolver())
        XCTAssertEqual(Tokenizer.wordInProgress(midWord)?.displayed, "wor")
        XCTAssertNil(Tokenizer.lastCompletedWord(midWord))

        let justClosed = Tokenizer.tokenize(try record("hello ", on: abc), layout: resolver())
        XCTAssertNil(Tokenizer.wordInProgress(justClosed))
        XCTAssertEqual(Tokenizer.lastCompletedWord(justClosed)?.displayed, "hello")
    }

    // MARK: - Mixed layouts in one buffer

    /// A user writing a mixed paragraph switches layout mid-sentence. Each stroke has to
    /// be rendered under the layout it was typed on, or the reconstruction is wrong.
    func testStrokesRenderUnderTheirOwnLayout() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        var recorded = try record("hello", on: abc)
        recorded += try record(" ", on: abc)
        recorded += try record("мир", on: russian)

        let tokens = Tokenizer.tokenize(recorded, layout: resolver())
        XCTAssertEqual(tokens.map(\.displayed), ["hello", " ", "мир"])
    }

    // MARK: - Buffer behaviour

    func testIdleResetDiscardsStaleStrokes() throws {
        let abc = try table(Self.abcID)
        let buffer = StrokeBuffer(capacity: 64, idleReset: 5)

        buffer.append(KeyStroke(keyCode: 0), at: 0, layoutID: abc.sourceID)
        XCTAssertFalse(buffer.append(KeyStroke(keyCode: 1), at: 1, layoutID: abc.sourceID))
        XCTAssertEqual(buffer.count, 2)

        // A six second gap means whatever was buffered is no longer in front of the user.
        XCTAssertTrue(buffer.append(KeyStroke(keyCode: 2), at: 7, layoutID: abc.sourceID))
        XCTAssertEqual(buffer.count, 1)
    }

    func testBufferNeverGrowsPastCapacity() throws {
        let abc = try table(Self.abcID)
        let buffer = StrokeBuffer(capacity: 8, idleReset: 5)

        for index in 0..<40 {
            buffer.append(
                KeyStroke(keyCode: UInt16(index % 30)), at: Double(index) * 0.05,
                layoutID: abc.sourceID)
        }
        XCTAssertEqual(buffer.count, 8)
    }

    func testBackspaceMirrorsIntoTheBuffer() throws {
        let abc = try table(Self.abcID)
        let buffer = StrokeBuffer()

        buffer.append(KeyStroke(keyCode: 0), at: 0, layoutID: abc.sourceID)
        buffer.append(KeyStroke(keyCode: 1), at: 0.1, layoutID: abc.sourceID)
        buffer.removeLast()
        XCTAssertEqual(buffer.count, 1)

        buffer.reset()
        XCTAssertTrue(buffer.isEmpty)
        // Deleting on an empty buffer must not trap.
        buffer.removeLast()
        XCTAssertTrue(buffer.isEmpty)
    }

    private static let abcID = "com.apple.keylayout.ABC"
    private static let russianID = "com.apple.keylayout.Russian"
    private static let kazakhID = "com.apple.keylayout.Kazakh"
}

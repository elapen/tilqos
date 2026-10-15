import XCTest

@testable import TilqosKit

final class ReplacementPlannerTests: XCTestCase {
    private static let abcID = "com.apple.keylayout.ABC"
    private static let russianID = "com.apple.keylayout.Russian"
    private static let kazakhID = "com.apple.keylayout.Kazakh"

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

    private func resolver() -> (String) -> KeyMapTable? {
        { [tables = Self.manager.tables] id in tables.first { $0.sourceID == id } }
    }

    /// Strokes for `intended` as typed on `typedOn`, but recorded as having happened
    /// while `activeLayout` was selected — i.e. the wrong-layout mistake.
    private func mistyped(
        intended: String, typedOn: KeyMapTable, activeLayout: KeyMapTable
    ) throws -> [RecordedStroke] {
        let strokes = try XCTUnwrap(typedOn.strokes(for: intended))
        return strokes.enumerated().map {
            RecordedStroke(stroke: $1, time: Double($0) * 0.05, layoutID: activeLayout.sourceID)
        }
    }

    // MARK: - Latin / Cyrillic

    func testLatinToCyrillicCorrection() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        // Wanted "привет", ABC was active, so "ghbdtn" appeared.
        let recorded = try mistyped(intended: "привет", typedOn: russian, activeLayout: abc)
        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: 0..<recorded.count, targetLayoutID: russian.sourceID)
                ],
                layout: resolver()))

        XCTAssertEqual(plan.deletedText, "ghbdtn")
        XCTAssertEqual(plan.insertText, "привет")
        XCTAssertEqual(plan.deleteCount, 6)
    }

    // MARK: - Russian / Kazakh

    /// The shared leading "с" is kept, so this deletes nine characters rather than ten.
    func testKazakhCorrectionPreservesTheCommonPrefix() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        let recorded = try mistyped(
            intended: "сәлеметсіз", typedOn: kazakh, activeLayout: russian)
        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: 0..<recorded.count, targetLayoutID: kazakh.sourceID)
                ],
                layout: resolver()))

        XCTAssertEqual(plan.deletedText, "2леметс3з")
        XCTAssertEqual(plan.insertText, "әлеметсіз")
        XCTAssertEqual(plan.deleteCount, 9)
    }

    func testKazakhLettersBackToDigits() throws {
        let abc = try table(Self.abcID)
        let kazakh = try table(Self.kazakhID)

        // Wanted "25" while the Kazakh layout was active, so "әғ" appeared.
        let recorded = try mistyped(intended: "25", typedOn: abc, activeLayout: kazakh)
        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: 0..<recorded.count, targetLayoutID: abc.sourceID)
                ],
                layout: resolver()))

        XCTAssertEqual(plan.deletedText, "әғ")
        XCTAssertEqual(plan.insertText, "25")
    }

    // MARK: - Separators

    /// A period the user typed is a period they meant, even though the same key yields
    /// `ю` on the Russian layout. Only the word may change layout.
    func testTrailingPunctuationIsNotReRendered() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        var recorded = try mistyped(intended: "привет", typedOn: russian, activeLayout: abc)
        let wordLength = recorded.count
        recorded += try mistyped(intended: ".", typedOn: abc, activeLayout: abc)

        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: 0..<wordLength, targetLayoutID: russian.sourceID)
                ],
                layout: resolver()))

        XCTAssertEqual(plan.deletedText, "ghbdtn.")
        XCTAssertEqual(plan.insertText, "привет.")
    }

    func testCorrectionSpanningASpaceLeavesTheSpaceAlone() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        var recorded = try mistyped(intended: "привет", typedOn: russian, activeLayout: abc)
        let firstWord = 0..<recorded.count
        recorded += try mistyped(intended: " ", typedOn: abc, activeLayout: abc)
        let secondStart = recorded.count
        recorded += try mistyped(intended: "мир", typedOn: russian, activeLayout: abc)
        let secondWord = secondStart..<recorded.count

        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: firstWord, targetLayoutID: russian.sourceID),
                    WordCorrection(range: secondWord, targetLayoutID: russian.sourceID),
                ],
                layout: resolver()))

        XCTAssertEqual(plan.insertText, "привет мир")
    }

    /// Mixed text: only the flagged word moves, the untouched neighbour stays put.
    func testOnlyFlaggedWordsChange() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        var recorded = try mistyped(intended: "привет", typedOn: russian, activeLayout: abc)
        let firstWord = 0..<recorded.count
        recorded += try mistyped(intended: " iPhone", typedOn: abc, activeLayout: abc)

        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: firstWord, targetLayoutID: russian.sourceID)
                ],
                layout: resolver()))

        XCTAssertEqual(plan.deletedText, "ghbdtn iPhone")
        XCTAssertEqual(plan.insertText, "привет iPhone")
    }

    // MARK: - Degenerate cases

    func testNoChangeYieldsNoPlan() throws {
        let russian = try table(Self.russianID)
        let kazakh = try table(Self.kazakhID)

        // Russian letters render identically on the Kazakh layout, so "correcting"
        // between them is a no-op and must not produce an edit.
        let recorded = try mistyped(intended: "Астана", typedOn: russian, activeLayout: russian)
        XCTAssertNil(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: 0..<recorded.count, targetLayoutID: kazakh.sourceID)
                ],
                layout: resolver()))
    }

    func testEmptyInputYieldsNoPlan() throws {
        XCTAssertNil(
            ReplacementPlanner.plan(
                recorded: [], corrections: [], layout: resolver()))
    }

    // MARK: - Undo

    func testInverseRestoresTheOriginal() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        let recorded = try mistyped(intended: "привет", typedOn: russian, activeLayout: abc)
        let plan = try XCTUnwrap(
            ReplacementPlanner.plan(
                recorded: recorded,
                corrections: [
                    WordCorrection(range: 0..<recorded.count, targetLayoutID: russian.sourceID)
                ],
                layout: resolver()))

        let undo = plan.inverse
        XCTAssertEqual(undo.deletedText, "привет")
        XCTAssertEqual(undo.insertText, "ghbdtn")
        XCTAssertEqual(undo.inverse, plan)
    }

    // MARK: - Buffer bookkeeping

    func testRewrittenTailAdoptsTheTargetLayout() throws {
        let abc = try table(Self.abcID)
        let russian = try table(Self.russianID)

        let recorded = try mistyped(intended: "привет", typedOn: russian, activeLayout: abc)
        let tail = ReplacementPlanner.rewrittenTail(
            recorded: recorded,
            corrections: [
                WordCorrection(range: 0..<recorded.count, targetLayoutID: russian.sourceID)
            ],
            from: 0)

        XCTAssertEqual(tail.count, recorded.count)
        XCTAssertTrue(tail.allSatisfy { $0.layoutID == russian.sourceID })
        // Same keys, only the layout they are read under changed.
        XCTAssertEqual(tail.map(\.stroke), recorded.map(\.stroke))
    }
}

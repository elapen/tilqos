import XCTest

@testable import TilqosKit

/// The event tap callback runs on the path of every keystroke in the system and has a hard
/// time budget — exceed it and macOS silently disables the tap. These measure the work that
/// budget has to cover.
final class PerformanceTests: XCTestCase {
    private static let manager: InputSourceManager = {
        let manager = InputSourceManager()
        manager.reloadTables()
        return manager
    }()

    private static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: TrigramModelTests.modelDirectory)
        return store
    }()

    private var russian: KeyMapTable!
    private var abc: KeyMapTable!
    private var kazakh: KeyMapTable!

    override func setUpWithError() throws {
        guard let russian = Self.manager.table(forSourceID: "com.apple.keylayout.Russian"),
            let abc = Self.manager.table(forSourceID: "com.apple.keylayout.ABC"),
            let kazakh = Self.manager.table(forSourceID: "com.apple.keylayout.Kazakh"),
            Self.store.model(for: .kk) != nil
        else { throw XCTSkip("Layouts or models unavailable") }
        self.russian = russian
        self.abc = abc
        self.kazakh = kazakh
    }

    /// A realistic buffer: a couple of sentences of mistyped Cyrillic.
    private func buffer() -> [RecordedStroke] {
        let text = "привет как дела я сегодня работаю над клавиатурой и переключателем"
        guard let strokes = russian.strokes(for: text) else { return [] }
        return strokes.enumerated().map {
            RecordedStroke(stroke: $1, time: Double($0) * 0.05, layoutID: abc.sourceID)
        }
    }

    private func elapsed(iterations: Int, _ body: () -> Void) -> Double {
        // Warm caches first so the measurement is of steady-state work.
        for _ in 0..<5 { body() }
        let start = DispatchTime.now().uptimeNanoseconds
        for _ in 0..<iterations { body() }
        let end = DispatchTime.now().uptimeNanoseconds
        return Double(end - start) / Double(iterations) / 1000.0  // microseconds
    }

    /// What actually happens inside the tap callback: append a stroke to the buffer.
    func testKeystrokeCostIsNegligible() {
        let buffer = StrokeBuffer()
        let stroke = KeyStroke(keyCode: 4)
        let layoutID = russian.sourceID

        let micros = elapsed(iterations: 20_000) {
            _ = buffer.append(stroke, at: 1, layoutID: layoutID)
        }
        print(String(format: "keystroke append: %.3f µs", micros))
        XCTAssertLessThan(micros, 20, "the tap callback must stay far inside its budget")
    }

    func testSingleWordScoringIsFastEnough() {
        let recorded = buffer()
        let detector = Detector(models: Self.store)
        let context = Detector.Context(
            recorded: recorded,
            candidateLayouts: [abc, russian, kazakh],
            resolve: { [manager = Self.manager] in manager.table(forSourceID: $0) })

        let micros = elapsed(iterations: 2_000) {
            _ = detector.evaluate(context, scope: .lastWord)
        }
        print(String(format: "last-word evaluation: %.1f µs", micros))
        XCTAssertLessThan(micros, 1_000)
    }

    /// The wide scan, run once per sentence rather than per keystroke.
    func testTailScanIsFastEnough() {
        let recorded = buffer()
        let detector = Detector(models: Self.store)
        let context = Detector.Context(
            recorded: recorded,
            candidateLayouts: [abc, russian, kazakh],
            resolve: { [manager = Self.manager] in manager.table(forSourceID: $0) })

        let micros = elapsed(iterations: 500) {
            _ = detector.evaluate(context, scope: .tail)
        }
        print(String(format: "tail scan (%d words): %.1f µs", 11, micros))
        XCTAssertLessThan(micros, 10_000)
    }

    func testBuildingAllLayoutTablesIsFastEnough() {
        let micros = elapsed(iterations: 20) {
            let manager = InputSourceManager()
            manager.reloadTables()
        }
        print(String(format: "rebuild all layout tables: %.0f µs", micros))
        // Happens at launch and when the user edits their layouts, not on the key path.
        XCTAssertLessThan(micros, 100_000)
    }
}

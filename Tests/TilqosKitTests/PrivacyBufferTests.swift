import Carbon.HIToolbox
import XCTest

@testable import TilqosKit

/// P1: the typed-word buffer lives in memory only and is emptied by a real timer five seconds
/// after the last key, and on every boundary: app switch, Return/Tab/Escape-type keys, secure
/// input, lock and sleep. Time is driven by hand; nothing here sleeps.
final class PrivacyBufferTests: XCTestCase {
    /// A made-up layout, so these tests do not depend on what the machine has enabled:
    /// key codes 0...25 type a...z, 49 types a space.
    static let layout: KeyMapTable = {
        var forward = [String?](repeating: nil, count: KeyMapTable.keyCodeCount * ModState.variantCount)
        for code in 0..<26 {
            let letter = String(UnicodeScalar(UInt8(97 + code)))
            forward[KeyStroke(keyCode: UInt16(code)).tableIndex] = letter
            forward[KeyStroke(keyCode: UInt16(code), mods: .shift).tableIndex] = letter.uppercased()
        }
        forward[KeyStroke(keyCode: 49).tableIndex] = " "
        return KeyMapTable(sourceID: "test.layout", localizedName: "Test", lang: .en, forward: forward)
    }()

    private var scheduler: ManualScheduler!
    private var scratch: ScratchDefaults!

    override func setUp() {
        scheduler = ManualScheduler()
        scratch = ScratchDefaults.make()
    }

    override func tearDown() {
        scratch?.remove()
    }

    private func makeEngine(idle: TimeInterval = 5) -> TypingEngine {
        let engine = TypingEngine(
            detector: Detector(models: ModelStore()), buffer: StrokeBuffer(idleReset: idle))
        engine.scheduler = scheduler
        return engine
    }

    @discardableResult
    private func type(_ engine: TypingEngine, _ keys: [UInt16]) -> TimeInterval {
        for code in keys {
            engine.record(
                KeyStroke(keyCode: code), layout: Self.layout, at: scheduler.now,
                candidates: [Self.layout])
        }
        return scheduler.now
    }

    // MARK: - The timer

    func testBufferIsEmptiedFiveSecondsAfterTheLastKeyWithoutAnotherKey() {
        let engine = makeEngine()
        var notified = 0
        engine.onIdleReset = { notified += 1 }
        type(engine, [6, 7, 1, 3])  // "ghbd"
        XCTAssertEqual(engine.buffer.count, 4)

        scheduler.advance(by: 4.9)
        XCTAssertEqual(engine.buffer.count, 4, "still inside the five seconds")
        XCTAssertEqual(notified, 0)

        scheduler.advance(by: 0.2)
        XCTAssertTrue(engine.buffer.isEmpty, "five seconds without typing: the buffer is gone")
        XCTAssertEqual(
            engine.screenText(resolve: { _ in Self.layout }), "",
            "nothing of the word is left to read back")
        XCTAssertEqual(notified, 1)
        XCTAssertEqual(scheduler.pendingCount, 0, "and no timer keeps running")
    }

    func testTimerMeasuresFromTheLastKeyNotTheFirst() {
        let engine = makeEngine()
        type(engine, [0])  // t = 0
        scheduler.advance(by: 3)
        type(engine, [1])  // t = 3
        scheduler.advance(by: 4.9)  // t = 7.9, which is 4.9 after the last key
        XCTAssertEqual(engine.buffer.count, 2)
        scheduler.advance(by: 0.2)  // t = 8.1
        XCTAssertTrue(engine.buffer.isEmpty)
    }

    func testOneTimerCoversAWholeBurstOfTyping() {
        let engine = makeEngine()
        for _ in 0..<50 {
            type(engine, [0])
            scheduler.advance(by: 0.1)
        }
        XCTAssertEqual(scheduler.pendingCount, 1, "the keystroke path schedules nothing per key")
    }

    func testDeleteCountsAsTyping() {
        let engine = makeEngine()
        type(engine, [0, 1, 2])
        scheduler.advance(by: 4)
        engine.backspace()
        scheduler.advance(by: 4)
        XCTAssertEqual(engine.buffer.count, 2, "the Delete restarted the five seconds")
        scheduler.advance(by: 1.1)
        XCTAssertTrue(engine.buffer.isEmpty)
    }

    func testResetCancelsTheTimer() {
        let engine = makeEngine()
        var notified = 0
        engine.onIdleReset = { notified += 1 }
        type(engine, [0, 1])
        engine.reset()
        XCTAssertEqual(scheduler.pendingCount, 0)
        scheduler.advance(by: 60)
        XCTAssertEqual(notified, 0)
    }

    func testTypingAfterTheResetStartsANewFiveSeconds() {
        let engine = makeEngine()
        type(engine, [0, 1])
        scheduler.advance(by: 6)
        XCTAssertTrue(engine.buffer.isEmpty)
        type(engine, [2])
        scheduler.advance(by: 4.9)
        XCTAssertEqual(engine.buffer.count, 1)
        scheduler.advance(by: 0.2)
        XCTAssertTrue(engine.buffer.isEmpty)
    }

    func testIdleResetCannotBeStretchedBeyondFiveSeconds() {
        XCTAssertEqual(StrokeBuffer(idleReset: 600).idleReset, 5)
        XCTAssertEqual(StrokeBuffer(idleReset: 0).idleReset, 5, "unset means the default")
        XCTAssertEqual(StrokeBuffer(idleReset: 0.2).idleReset, 1)
        let prefs = Prefs(defaults: scratch.defaults)
        scratch.defaults.set(3600.0, forKey: "bufferIdleReset")
        XCTAssertEqual(prefs.bufferIdleReset, 5, "a hand-edited preference does not break the promise")
        scratch.defaults.set(0.2, forKey: "bufferIdleReset")
        XCTAssertEqual(prefs.bufferIdleReset, 1)
        scratch.defaults.set(0.0, forKey: "bufferIdleReset")
        XCTAssertEqual(prefs.bufferIdleReset, 5)

        let engine = makeEngine(idle: 600)
        type(engine, [0])
        scheduler.advance(by: 5.1)
        XCTAssertTrue(engine.buffer.isEmpty)
    }

    func testBufferStaysBoundedAndHoldsKeyCodesNotText() {
        let engine = makeEngine()
        type(engine, [UInt16](repeating: 3, count: 300))
        XCTAssertEqual(engine.buffer.count, 256, "never a transcript")
        let mirror = String(describing: engine.buffer.recorded.first!)
        XCTAssertFalse(mirror.contains("\"d\""), "strokes are key codes with modifiers, not characters")
    }

    // MARK: - The pipeline's boundaries

    private func makePipeline() -> (Pipeline, FakePresenter, MemoryBoard) {
        let presenter = FakePresenter()
        let board = MemoryBoard()
        let pipeline = Pipeline(
            prefs: Prefs(defaults: scratch.defaults), inputSources: CorpusTests.manager,
            models: ModelStore(), learner: Learner(defaults: scratch.defaults),
            board: board, scheduler: scheduler, panel: presenter)
        pipeline.tap.secureInputProbe = { false }
        pipeline.beep = {}
        return (pipeline, presenter, board)
    }

    private func fill(_ pipeline: Pipeline) {
        for code: UInt16 in [6, 7, 1] {
            pipeline.engine.record(
                KeyStroke(keyCode: code), layout: Self.layout, at: scheduler.now,
                candidates: [Self.layout])
        }
        XCTAssertEqual(pipeline.engine.buffer.count, 3)
    }

    func testIdleTimerWorksThroughThePipeline() {
        let (pipeline, _, _) = makePipeline()
        fill(pipeline)
        scheduler.advance(by: 5.2)
        XCTAssertTrue(pipeline.engine.buffer.isEmpty)
    }

    func testTabEmptiesTheBufferLikeReturnAndEscape() {
        let (pipeline, _, _) = makePipeline()
        fill(pipeline)
        XCTAssertEqual(pipeline.handle(KeyStroke(keyCode: UInt16(kVK_Tab))), .pass)
        XCTAssertTrue(pipeline.engine.buffer.isEmpty, "Tab moves focus or inserts a tab: the text is somewhere else")
        // Return and Escape are reset by the event tap before they reach the pipeline.
        XCTAssertTrue(EventTap.navigationKeyCodes.contains(UInt16(kVK_Return)))
        XCTAssertTrue(EventTap.navigationKeyCodes.contains(UInt16(kVK_ANSI_KeypadEnter)))
        XCTAssertTrue(EventTap.navigationKeyCodes.contains(UInt16(kVK_Escape)))
    }

    func testAppSwitchEmptiesTheBuffer() {
        let (pipeline, _, _) = makePipeline()
        fill(pipeline)
        pipeline.appActivated()
        XCTAssertTrue(pipeline.engine.buffer.isEmpty)
    }

    func testLockAndSleepEmptyTheBuffer() {
        let (pipeline, _, _) = makePipeline()
        fill(pipeline)
        pipeline.lockedOrAsleep()
        XCTAssertTrue(pipeline.engine.buffer.isEmpty)
        XCTAssertEqual(scheduler.pendingCount, 0, "the idle timer went with it")
    }

    func testStoppingWipesTheSessionToo() {
        let (pipeline, _, _) = makePipeline()
        fill(pipeline)
        pipeline.undo.push(PrivacyTestSupport.fix(at: scheduler.date))
        pipeline.stop()
        XCTAssertTrue(pipeline.engine.buffer.isEmpty)
        XCTAssertEqual(pipeline.undoCount, 0)
    }

    func testSecureInputEmptiesTheBufferBeforeAnythingIsRead() throws {
        // The tap reports it once, on the first event it ignores; the pipeline drops the buffer.
        let (pipeline, _, _) = makePipeline()
        fill(pipeline)
        var reasons: [EventTap.ResetReason] = []
        pipeline.tap.onReset = { reasons.append($0) }
        pipeline.tap.onStroke = { _ in XCTFail("no key is read while secure input is on"); return .pass }
        pipeline.tap.secureInputProbe = { true }
        let event = try XCTUnwrap(CGEvent(keyboardEventSource: nil, virtualKey: 0, keyDown: true))
        withExtendedLifetime(event) {
            _ = pipeline.tap.process(.keyDown, event)
            _ = pipeline.tap.process(.keyDown, event)
        }
        XCTAssertEqual(reasons, [.secureInput])
        // and the word that was waiting to be evaluated is dropped, not looked at
        pipeline.evaluate(TypingEngine.WordEnd(sentenceEnd: false))
        XCTAssertTrue(pipeline.engine.buffer.isEmpty)
    }
}

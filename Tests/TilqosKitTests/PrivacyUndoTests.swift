import XCTest

@testable import TilqosKit

/// P2: the undo list holds the last few corrections in memory for two minutes at most. A timer
/// wipes them, lock/sleep/quit and a change of the frontmost app wipe them at once, and after
/// that "undo" quietly does nothing.
final class PrivacyUndoTests: XCTestCase {
    private var scheduler: ManualScheduler!
    private var scratch: ScratchDefaults!

    override func setUp() {
        scheduler = ManualScheduler()
        scratch = ScratchDefaults.make()
    }

    override func tearDown() {
        scratch?.remove()
    }

    private func makeStack(lifetime: TimeInterval = UndoStack.defaultLifetime) -> UndoStack {
        UndoStack(lifetime: lifetime, scheduler: scheduler, now: { [scheduler] in scheduler!.date })
    }

    func testTheLifetimeIsTwoMinutes() {
        XCTAssertEqual(UndoStack.defaultLifetime, 120)
    }

    func testEntryIsWipedByTheTimerAtTwoMinutes() {
        let stack = makeStack()
        var expired = 0
        stack.onExpire = { expired += 1 }
        stack.push(PrivacyTestSupport.fix(at: scheduler.date))

        scheduler.advance(by: 119)
        XCTAssertEqual(stack.count, 1, "one second before the end it can still be undone")
        XCTAssertEqual(expired, 0)

        scheduler.advance(by: 1.5)
        XCTAssertEqual(stack.count, 0)
        XCTAssertNil(stack.last)
        XCTAssertEqual(expired, 1, "the menu is told, so it stops offering Undo")
        XCTAssertEqual(scheduler.pendingCount, 0)
    }

    func testTimerWipesWithoutAnyoneAskingTheStack() {
        // No call to `count`, `last` or `popLast` between the push and the wipe: only the
        // timer can have removed the entry. (Reading `count` first would purge it lazily.)
        let stack = makeStack()
        var expired = 0
        stack.onExpire = { expired += 1 }
        stack.push(PrivacyTestSupport.fix(at: scheduler.date))
        scheduler.advance(by: 121)
        XCTAssertEqual(expired, 1, "the timer fired and removed the entry by itself")
    }

    func testEachEntryHasItsOwnTwoMinutes() {
        let stack = makeStack()
        stack.push(PrivacyTestSupport.fix("first", "один", at: scheduler.date))
        scheduler.advance(by: 90)
        stack.push(PrivacyTestSupport.fix("second", "два", at: scheduler.date))

        scheduler.advance(by: 31)  // t = 121: the first is gone, the second is 31 s old
        XCTAssertEqual(stack.count, 1)
        XCTAssertEqual(stack.last?.originalWord, "second")

        scheduler.advance(by: 90)  // t = 211: the second is 121 s old
        XCTAssertEqual(stack.count, 0)
        XCTAssertEqual(scheduler.pendingCount, 0)
    }

    func testAnExpiredEntryIsNeverReturnedEvenIfTheTimerIsLate() {
        // The Mac slept through the timer, say: the wall clock jumped. The entry is still refused.
        var clock = Date(timeIntervalSinceReferenceDate: 1_000)
        let stack = UndoStack(scheduler: nil, now: { clock })
        stack.push(PrivacyTestSupport.fix(at: clock))
        clock = clock.addingTimeInterval(121)
        XCTAssertNil(stack.popLast())
        XCTAssertEqual(stack.count, 0)
    }

    func testUndoAfterExpiryDoesNothingAndDoesNotCrash() {
        let presenter = FakePresenter()
        let pipeline = Pipeline(
            prefs: Prefs(defaults: scratch.defaults), inputSources: CorpusTests.manager,
            models: ModelStore(), learner: Learner(defaults: scratch.defaults),
            board: MemoryBoard(), scheduler: scheduler, panel: presenter,
            now: { [scheduler] in scheduler!.date })
        pipeline.tap.secureInputProbe = { false }
        var states = 0
        pipeline.onStateChange = { states += 1 }

        pipeline.undo.push(PrivacyTestSupport.fix(at: scheduler.date))
        XCTAssertEqual(pipeline.undoCount, 1)
        XCTAssertNotNil(pipeline.lastFix)

        scheduler.advance(by: 121)
        XCTAssertEqual(pipeline.undoCount, 0)
        XCTAssertNil(pipeline.lastFix)
        XCTAssertGreaterThan(states, 0, "the menu was told")

        pipeline.undoLastFix()  // must neither crash nor touch anything
        pipeline.perform(.undoLastFix)
        XCTAssertEqual(pipeline.undoCount, 0)
    }

    func testClearWipesEverythingAndStopsTheTimer() {
        let stack = makeStack()
        for word in ["a", "b", "c"] { stack.push(PrivacyTestSupport.fix(word, word, at: scheduler.date)) }
        stack.clear()
        XCTAssertTrue(stack.isEmpty)
        XCTAssertEqual(scheduler.pendingCount, 0)
    }

    func testStackHoldsOnlyTheMostRecentCorrections() {
        let stack = makeStack()
        for index in 0..<30 { stack.push(PrivacyTestSupport.fix("w\(index)", "x", at: scheduler.date)) }
        XCTAssertEqual(stack.count, stack.limit)
        XCTAssertEqual(stack.last?.originalWord, "w29")
    }

    func testClipboardUndoExpiresWithTheRest() {
        let stack = makeStack()
        stack.push(PrivacyTestSupport.fix("qalaisyz", "қалайсыз", at: scheduler.date, clipboardBefore: "qalaisyz"))
        scheduler.advance(by: 121)
        XCTAssertNil(stack.popLast(), "the previous clipboard text does not outlive two minutes either")
    }

    // MARK: - The pipeline's wipes

    private func makePipeline() -> Pipeline {
        let pipeline = Pipeline(
            prefs: Prefs(defaults: scratch.defaults), inputSources: CorpusTests.manager,
            models: ModelStore(), learner: Learner(defaults: scratch.defaults),
            board: MemoryBoard(), scheduler: scheduler, panel: FakePresenter(),
            now: { [scheduler] in scheduler!.date })
        pipeline.tap.secureInputProbe = { false }
        return pipeline
    }

    func testFrontmostAppChangeWipesTheUndoList() {
        let pipeline = makePipeline()
        pipeline.undo.push(PrivacyTestSupport.fix(at: scheduler.date))
        pipeline.appActivated()
        XCTAssertEqual(pipeline.undoCount, 0)
    }

    func testLockAndSleepWipeTheUndoList() {
        let pipeline = makePipeline()
        pipeline.undo.push(PrivacyTestSupport.fix(at: scheduler.date))
        pipeline.lockedOrAsleep()
        XCTAssertEqual(pipeline.undoCount, 0)
    }

    func testQuitWipesTheUndoList() {
        let pipeline = makePipeline()
        pipeline.undo.push(PrivacyTestSupport.fix(at: scheduler.date))
        pipeline.stop()
        XCTAssertEqual(pipeline.undoCount, 0)
    }
}

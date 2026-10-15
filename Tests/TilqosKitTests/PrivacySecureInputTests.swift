import Carbon.HIToolbox
import XCTest

@testable import TilqosKit

/// P7: while secure event input is on (a password field is in front) the app reads nothing and
/// logs nothing. The system switch is replaced by a probe the tests control; the real one is
/// never turned on for the machine.
final class PrivacySecureInputTests: XCTestCase {
    private var capture: LogCapture!
    private var scratch: ScratchDefaults!

    override func setUp() {
        scratch = ScratchDefaults.make()
        capture = LogCapture(diagnostics: true)
    }

    override func tearDown() {
        capture?.stop()
        scratch?.remove()
    }

    private func keyEvent(_ keyCode: Int, flags: CGEventFlags = []) throws -> CGEvent {
        let event = try XCTUnwrap(
            CGEvent(keyboardEventSource: nil, virtualKey: CGKeyCode(keyCode), keyDown: true))
        event.flags = flags
        return event
    }

    private func makeTap(secure: @escaping () -> Bool) -> EventTap {
        let tap = EventTap(prefs: Prefs(defaults: scratch.defaults))
        tap.secureInputProbe = secure
        return tap
    }

    func testNoKeyIsReadOrHandledWhileSecureInputIsOn() throws {
        let tap = makeTap { true }
        tap.onStroke = { _ in XCTFail("a stroke reached the typing path"); return .pass }
        tap.onHotkey = { _, _ in XCTFail("a hotkey was matched"); return true }
        tap.onBackspace = { XCTFail("a Delete was seen") }
        var resets: [EventTap.ResetReason] = []
        tap.onReset = { resets.append($0) }

        let events = [
            try keyEvent(kVK_ANSI_G), try keyEvent(kVK_Delete), try keyEvent(kVK_Escape),
            try keyEvent(kVK_ANSI_T, flags: [.maskAlternate, .maskShift]),
            try keyEvent(kVK_ANSI_C, flags: .maskCommand),
        ]
        for event in events {
            withExtendedLifetime(event) {
                let delivered = tap.process(.keyDown, event)?.takeUnretainedValue()
                XCTAssertTrue(delivered === event, "and every key still reaches the app untouched")
            }
        }
        XCTAssertEqual(resets, [.secureInput], "one reset, reported once, and nothing else")
    }

    func testNothingIsLoggedWhileSecureInputIsOnOrWhenItBeginsAndEnds() throws {
        var secure = false
        let tap = makeTap { secure }
        tap.onStroke = { _ in .pass }
        let event = try keyEvent(kVK_ANSI_G)

        secure = true
        for _ in 0..<5 { withExtendedLifetime(event) { _ = tap.process(.keyDown, event) } }
        secure = false
        withExtendedLifetime(event) { _ = tap.process(.keyDown, event) }

        XCTAssertEqual(
            capture.lines.filter { $0.text.lowercased().contains("secure") }.count, 0,
            "when a password field was in front is not written down")
        XCTAssertEqual(capture.lines.count, 0, "and the tap logs nothing at all on this path")
    }

    func testTypingResumesWhenSecureInputEnds() throws {
        var secure = true
        let tap = makeTap { secure }
        var strokes = 0
        tap.onStroke = { _ in strokes += 1; return .pass }
        let event = try keyEvent(kVK_ANSI_G)
        withExtendedLifetime(event) { _ = tap.process(.keyDown, event) }
        XCTAssertEqual(strokes, 0)
        secure = false
        withExtendedLifetime(event) { _ = tap.process(.keyDown, event) }
        XCTAssertEqual(strokes, 1)
    }

    func testMouseEventsAreNotActedOnEitherWhileSecureInputIsOn() throws {
        let tap = makeTap { true }
        var resets: [EventTap.ResetReason] = []
        tap.onReset = { resets.append($0) }
        let click = try XCTUnwrap(
            CGEvent(
                mouseEventSource: nil, mouseType: .leftMouseDown, mouseCursorPosition: .zero,
                mouseButton: .left))
        withExtendedLifetime(click) { _ = tap.process(.leftMouseDown, click) }
        XCTAssertEqual(resets, [.secureInput])
    }

    // MARK: - The pipeline

    private func makePipeline(board: MemoryBoard, secure: Bool) -> Pipeline {
        let pipeline = Pipeline(
            prefs: Prefs(defaults: scratch.defaults), inputSources: CorpusTests.manager,
            models: CorpusTests.store, learner: Learner(defaults: scratch.defaults),
            board: board, scheduler: ManualScheduler(), panel: FakePresenter())
        pipeline.tap.secureInputProbe = { secure }
        pipeline.beep = {}
        return pipeline
    }

    func testCommandsThatReadTextDoNothingWhileSecureInputIsOn() {
        let board = MemoryBoard()
        board.setText("qalaisyz")
        let pipeline = makePipeline(board: board, secure: true)
        let before = board.items

        pipeline.convertClipboard()
        pipeline.transliterateClipboard(direction: .toCyrillic)
        pipeline.fixLastWord()
        pipeline.undoLastFix()
        pipeline.perform(.convertClipboard)

        XCTAssertEqual(board.items, before)
        XCTAssertEqual(board.textReads, 0, "the clipboard was not read")
        XCTAssertEqual(pipeline.undoCount, 0)
    }

    func testTheSameCommandsWorkWhenSecureInputIsOff() throws {
        try XCTSkipIf(CorpusTests.store.model(for: .kk) == nil, "Models missing")
        let board = MemoryBoard()
        board.setText("qalaisyz")
        let pipeline = makePipeline(board: board, secure: false)
        pipeline.transliterateClipboard(direction: .toCyrillic)
        XCTAssertEqual(board.text, "қалайсыз", "positive control: the guard is what stopped it before")
    }

    func testTheLogLinesAboutLeavingSecureInputAreGoneFromTheSource() throws {
        for (name, text) in try PrivacyTestSupport.appSources() {
            XCTAssertFalse(text.contains("secure input active"), name)
            XCTAssertFalse(text.contains("secure input cleared"), name)
        }
    }
}

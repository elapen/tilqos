import Carbon.HIToolbox
import CoreGraphics
import XCTest

@testable import TilqosKit

/// The tap's verdicts, without a live tap.
///
/// For months a swallowed key was delivered anyway — `handleKeyDown(...) ?? passthrough` turned
/// the nil that meant "swallow" back into "pass". The Tab that accepted a suggestion reached the
/// app together with the correction, and every hotkey typed its own character. These tests pin
/// the one property the accept key and the hotkeys rest on.
final class EventTapTests: XCTestCase {
    private func makeTap(enabled: Bool = true) -> EventTap {
        let prefs = Prefs(defaults: ScratchDefaults.make().defaults)
        prefs.enabled = enabled
        return EventTap(prefs: prefs)
    }

    private func keyDown(
        _ keyCode: Int, flags: CGEventFlags = [], source: CGEventSource? = nil
    ) throws -> CGEvent {
        let event = try XCTUnwrap(
            CGEvent(keyboardEventSource: source, virtualKey: CGKeyCode(keyCode), keyDown: true))
        event.flags = flags
        return event
    }

    /// Whether the tap delivered this exact event. The tap hands back an unretained reference,
    /// so the event is kept alive here for as long as the answer is being looked at.
    private func delivers(_ tap: EventTap, _ event: CGEvent) -> Bool {
        withExtendedLifetime(event) {
            tap.process(.keyDown, event)?.takeUnretainedValue() === event
        }
    }

    override func setUpWithError() throws {
        // A focused password field anywhere on the machine makes the tap step aside entirely.
        try XCTSkipIf(IsSecureEventInputEnabled(), "secure input is active on this machine")
    }

    override func tearDown() {
        ScratchDefaults.removeAll()
    }

    func testSwallowedStrokeIsNotDelivered() throws {
        let tap = makeTap()
        tap.onStroke = { _ in .swallow }
        XCTAssertFalse(delivers(tap, try keyDown(kVK_Tab)))
    }

    func testPassedStrokeIsDelivered() throws {
        let tap = makeTap()
        tap.onStroke = { _ in .pass }
        XCTAssertTrue(delivers(tap, try keyDown(kVK_ANSI_G)))
    }

    func testHotkeyIsSwallowed() throws {
        let tap = makeTap()
        tap.onHotkey = { _, _ in true }
        tap.onStroke = { _ in XCTFail("a hotkey must not reach the typing path"); return .pass }
        XCTAssertFalse(delivers(tap, try keyDown(kVK_ANSI_T, flags: [.maskAlternate, .maskShift])))
    }

    /// The hotkey that switches the app back on has to work while it is off.
    func testHotkeyIsSwallowedWhileDisabled() throws {
        let tap = makeTap(enabled: false)
        tap.onHotkey = { _, _ in true }
        XCTAssertFalse(delivers(tap, try keyDown(kVK_ANSI_L, flags: [.maskAlternate, .maskShift])))
    }

    func testNothingIsSwallowedWhileDisabled() throws {
        let tap = makeTap(enabled: false)
        tap.onHotkey = { _, _ in false }
        tap.onStroke = { _ in .swallow }
        XCTAssertTrue(delivers(tap, try keyDown(kVK_Tab)))
    }

    /// Our own injected events pass untouched, whatever the typing path would say.
    func testOwnEventsAlwaysPass() throws {
        let tap = makeTap()
        tap.onStroke = { _ in .swallow }
        let source = try XCTUnwrap(EventMarker.makeSource())
        XCTAssertTrue(delivers(tap, try keyDown(kVK_Delete, source: source)))
    }
}

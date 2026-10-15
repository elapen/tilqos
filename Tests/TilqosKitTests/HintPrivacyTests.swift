import AppKit
import XCTest

@testable import TilqosKit

/// The suggestion card and the "corrected" card must not keep typed words after they are
/// hidden: the hidden panel is still a window in memory.
@MainActor
final class HintPrivacyTests: XCTestCase {
    func testResetEmptiesEverythingTheCardHolds() {
        let hint = HintView(background: .opaque)
        hint.configure(
            .init(
                before: "ghbdtn", after: "привет", style: .suggestion(acceptKey: "Tab"),
                tip: "Tab — replace"))
        XCTAssertEqual(hint.content.before, "ghbdtn")
        XCTAssertNotNil(hint.accessibilityLabel())

        hint.reset()

        XCTAssertEqual(hint.content.before, "")
        XCTAssertEqual(hint.content.after, "")
        XCTAssertNil(hint.content.tip)
        XCTAssertNil(hint.accessibilityLabel())
        XCTAssertFalse((hint.accessibilityLabel() ?? "").contains("ghbdtn"))
    }

    func testPanelForgetsTheWordsWhenItFinishesHiding() {
        let panel = CaretPanel()
        panel.hint.configure(
            .init(before: "ghbdtn", after: "привет", style: .applied(undoKey: "⌥⇧Z")))

        // A panel that was shown again before the fade-out ended keeps its text.
        panel.finishHide(expected: -1)
        XCTAssertEqual(panel.hint.content.before, "ghbdtn")

        panel.finishHide(expected: 0)
        XCTAssertEqual(panel.hint.content.before, "")
        XCTAssertEqual(panel.hint.content.after, "")
    }
}

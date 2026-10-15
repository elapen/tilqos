import XCTest

@testable import TilqosKit

/// The app builds its detector at launch and loads the models in the background afterwards.
///
/// The scorer used to copy the dictionaries out of the store when it was built — which in the
/// app was always before they had loaded — so the shipped app ran without them: every word
/// "unknown", no fix ever more than an offer, Kazakh morphology never applied. Every other test
/// loads the models first, which is why none of them noticed.
final class ModelLoadingTests: XCTestCase {
    func testDetectorBuiltBeforeTheModelsUsesThemOnceTheyLoad() throws {
        let manager = CorpusTests.manager
        var tables: [Lang: KeyMapTable] = [:]
        for (lang, id) in [
            (Lang.en, "com.apple.keylayout.ABC"), (.ru, "com.apple.keylayout.Russian"),
            (.kk, "com.apple.keylayout.Kazakh"),
        ] {
            guard let table = manager.table(forSourceID: id) else {
                throw XCTSkip("Layout \(id) is not enabled on this machine")
            }
            tables[lang] = table
        }

        let store = ModelStore()
        let harness = HarnessEngine(tables: tables, store: store, manager: manager)
        func type(_ text: String) {
            for character in text {
                if let stroke = tables[.ru]!.strokes(for: String(character))?.first {
                    harness.type(stroke)
                }
            }
        }

        // Typed while nothing is loaded yet: whatever is judged now must not stick.
        type("привет ")
        XCTAssertEqual(harness.events, [], "nothing can be decided without models")

        store.load(from: TrigramModelTests.modelDirectory)
        guard store.lexicon(for: .ru) != nil else { throw XCTSkip("Lexicons missing") }
        // A new line, so the word is judged on its own rather than with the first one.
        harness.engine.reset()

        type("привет ")
        XCTAssertEqual(
            harness.events, [.fixed(before: "ghbdtn", after: "привет")],
            "a dictionary word must be fixed outright, not merely offered")
    }
}

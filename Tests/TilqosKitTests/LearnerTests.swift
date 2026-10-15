import XCTest

@testable import TilqosKit

/// Learning from what the user does, end to end through the real engine where it matters.
final class LearnerTests: XCTestCase {
    private var scratch: ScratchDefaults!
    private var defaults: UserDefaults { scratch.defaults }
    private var tables: [Lang: KeyMapTable] = [:]

    override func setUpWithError() throws {
        // Never the app's own domain: tests get a throwaway store under the temp directory.
        scratch = ScratchDefaults.make()
        for (lang, id) in [
            (Lang.en, "com.apple.keylayout.ABC"), (.ru, "com.apple.keylayout.Russian"),
            (.kk, "com.apple.keylayout.Kazakh"),
        ] {
            tables[lang] = CorpusTests.manager.table(forSourceID: id)
        }
    }

    override func tearDown() {
        scratch?.remove()
    }

    private func makeLearner() -> Learner { Learner(defaults: defaults) }

    // MARK: - The store

    func testIgnoredTwiceMeansKeep() {
        let learner = makeLearner()
        learner.ignored(typed: "nen")
        XCTAssertNil(learner.decision(for: "nen"), "one ignored offer proves nothing")
        learner.ignored(typed: "nen")
        XCTAssertEqual(learner.decision(for: "nen"), .keep)
    }

    func testAcceptanceOutranksPassiveLearning() {
        let learner = makeLearner()
        learner.ignored(typed: "nen")
        learner.ignored(typed: "nen")
        learner.confirmed(typed: "nen", to: .ru)
        XCTAssertEqual(learner.decision(for: "nen"), .convert(to: .ru))
        learner.rejected(typed: "nen")
        XCTAssertEqual(learner.decision(for: "nen"), .keep)
    }

    func testVocabularyNeedsTwoSightings() {
        let learner = makeLearner()
        learner.kept(typed: "Айдар", lang: .ru)
        XCTAssertFalse(learner.isKnown("айдар", in: .ru))
        learner.kept(typed: "айдар", lang: .ru)
        XCTAssertTrue(learner.isKnown("АЙДАР", in: .ru), "case and ё do not make a new word")
        XCTAssertFalse(learner.isKnown("айдар", in: .en))
        XCTAssertEqual(learner.decision(for: "айдар"), .keep)
    }

    func testJunkIsNotLearnedAsVocabulary() {
        let learner = makeLearner()
        for _ in 0..<3 { learner.kept(typed: "ab", lang: .en) }
        for _ in 0..<3 { learner.kept(typed: "x1y2", lang: .en) }
        XCTAssertEqual(learner.counts.total, 0)
    }

    func testPersistsWithoutPlaintext() throws {
        let learner = makeLearner()
        learner.confirmed(typed: "ghbdtn", to: .ru)
        learner.kept(typed: "Айдар", lang: .ru)
        learner.kept(typed: "Айдар", lang: .ru)
        learner.flush()

        // Nothing readable in the stored data.
        let stored = try XCTUnwrap(defaults.dictionary(forKey: Learner.Key.entries))
        let dump = "\(stored)"
        XCTAssertFalse(dump.contains("ghbdtn"))
        XCTAssertFalse(dump.lowercased().contains("айдар"))

        let reloaded = makeLearner()
        XCTAssertEqual(reloaded.decision(for: "ghbdtn"), .convert(to: .ru))
        XCTAssertTrue(reloaded.isKnown("айдар", in: .ru))
        XCTAssertEqual(reloaded.counts, Learner.Counts(alwaysConvert: 1, userWords: 1))
    }

    func testForgetting() {
        let learner = makeLearner()
        learner.confirmed(typed: "ghbdtn", to: .ru)
        learner.rejected(typed: "tot")
        XCTAssertEqual(learner.items().count, 2)

        XCTAssertTrue(learner.forget(word: "GHBDTN"))
        XCTAssertNil(learner.decision(for: "ghbdtn"))
        XCTAssertFalse(learner.forget(word: "ghbdtn"))

        let id = learner.items()[0].id
        learner.forget(id: id)
        XCTAssertEqual(learner.counts.total, 0)

        learner.confirmed(typed: "ghbdtn", to: .ru)
        let hashBefore = learner.hash("ghbdtn")
        learner.forgetAll()
        XCTAssertEqual(learner.counts.total, 0)
        XCTAssertNotEqual(learner.hash("ghbdtn"), hashBefore, "a wipe also changes the salt")
    }

    func testDisabledLearnerNeitherLearnsNorAnswers() {
        let learner = makeLearner()
        learner.confirmed(typed: "ghbdtn", to: .ru)
        learner.isEnabled = false
        XCTAssertNil(learner.decision(for: "ghbdtn"))
        learner.rejected(typed: "tot")
        learner.isEnabled = true
        XCTAssertNil(learner.decision(for: "tot"), "nothing is learned while switched off")
        XCTAssertEqual(learner.decision(for: "ghbdtn"), .convert(to: .ru))
    }

    func testCapacityEvictsTheOldest() {
        var day = 0.0
        let learner = Learner(defaults: defaults, now: { Date(timeIntervalSinceReferenceDate: day) })
        learner.rejected(typed: "oldest")
        day = 86_400 * 10
        for index in 0..<Learner.capacity { learner.rejected(typed: "word\(index)") }
        XCTAssertEqual(learner.counts.total, Learner.capacity)
        XCTAssertNil(learner.decision(for: "oldest"))
    }

    // MARK: - Through the engine

    private func harness(_ learner: Learner) throws -> HarnessEngine {
        guard tables.count == 3, CorpusTests.store.lexicon(for: .ru) != nil else {
            throw XCTSkip("Layouts or models unavailable")
        }
        let harness = HarnessEngine(
            tables: tables, store: CorpusTests.store, manager: CorpusTests.manager)
        harness.learned = learner
        harness.engine.learner = learner
        harness.engine.resolve = { CorpusTests.manager.table(forSourceID: $0) }
        let detector = harness.engine.detector
        harness.engine.isDictionaryWord = { detector.isDictionaryWord($0, lang: $1) }
        return harness
    }

    private func type(_ text: String, intended: Lang, on harness: HarnessEngine) {
        for character in text {
            guard let stroke = tables[intended]?.strokes(for: String(character))?.first else {
                return XCTFail("cannot type \(character)")
            }
            harness.type(stroke)
        }
    }

    /// Fixed automatically, erased, and the original typed again: the fix was wrong, and it
    /// must not be made a second time — neither now nor later.
    func testErasingAFixAndRetypingTheOriginalIsARejection() throws {
        let learner = makeLearner()
        let harness = try harness(learner)
        harness.activeLayout = tables[.en]!
        type("привет ", intended: .ru, on: harness)
        XCTAssertEqual(harness.screen, "привет ")

        // Erase it, switch back by hand, type the original again.
        for _ in 0..<7 { harness.backspace() }
        harness.activeLayout = tables[.en]!
        type("привет ", intended: .ru, on: harness)
        XCTAssertEqual(harness.screen, "ghbdtn ", "the same fix must not be made twice")
        XCTAssertEqual(learner.decision(for: "ghbdtn"), .keep)

        // And in a later session too.
        let later = try self.harness(learner)
        later.activeLayout = tables[.en]!
        type("привет ", intended: .ru, on: later)
        XCTAssertEqual(later.screen, "ghbdtn ")
    }

    /// Erasing a fix and typing something else is not a rejection.
    func testErasingAFixAndTypingSomethingElseTeachesNothing() throws {
        let learner = makeLearner()
        let harness = try harness(learner)
        harness.activeLayout = tables[.en]!
        type("привет ", intended: .ru, on: harness)
        for _ in 0..<7 { harness.backspace() }
        type("здравствуйте ", intended: .ru, on: harness)
        XCTAssertNil(learner.decision(for: "ghbdtn"))
    }

    /// Accepted once, applied without asking from then on.
    func testAcceptedConversionIsAppliedNextTime() throws {
        let learner = makeLearner()
        let before = try harness(learner)
        before.activeLayout = tables[.en]!
        type("тут ", intended: .ru, on: before)
        XCTAssertNotEqual(before.screen, "тут ", "on its own, too short to impose")

        learner.confirmed(typed: "nen", to: .ru)
        let after = try harness(learner)
        after.activeLayout = tables[.en]!
        type("тут ", intended: .ru, on: after)
        XCTAssertEqual(after.screen, "тут ")
    }

    /// The user's own word, learned from keeping it, becomes something the detector can fix
    /// when it is typed on the wrong layout.
    func testLearnedVocabularyMakesTheUsersWordsFixable() throws {
        let learner = makeLearner()
        let unknown = "зюзяка"
        XCTAssertFalse(
            CorpusTests.store.lexicon(for: .ru)!.contains(unknown), "the test needs an unknown word")

        // Typed on the wrong layout, an unknown word can at best be offered.
        let before = try harness(learner)
        before.activeLayout = tables[.en]!
        type(unknown + " ", intended: .ru, on: before)
        XCTAssertNotEqual(before.screen, unknown + " ")

        // Kept in two sessions...
        for _ in 0..<2 {
            let session = try harness(learner)
            session.activeLayout = tables[.ru]!
            type("мой " + unknown + " друг ", intended: .ru, on: session)
        }
        XCTAssertTrue(learner.isKnown(unknown, in: .ru))

        // ...and now it is a word.
        let after = try harness(learner)
        after.activeLayout = tables[.en]!
        type(unknown + " ", intended: .ru, on: after)
        XCTAssertEqual(after.screen, unknown + " ")
    }

    /// A layout mistake that slipped through and was left standing is not the user's word.
    ///
    /// Learned as vocabulary, it would never be corrected again — this is how «я написал так но
    /// потом» typed on ABC stopped being fixed at all.
    func testAMissedLayoutMistakeIsNeverLearnedAsVocabulary() throws {
        let learner = makeLearner()
        for _ in 0..<3 {
            let session = try harness(learner)
            // Nothing is fixed, as when the fix did not happen for whatever reason.
            session.thresholds = (auto: 2, suggest: 2)
            session.activeLayout = tables[.en]!
            type("я написал так но потом всё ", intended: .ru, on: session)
            XCTAssertEqual(session.screen, #"z yfgbcfk nfr yj gjnjv dc\ "#)
        }
        XCTAssertEqual(learner.items(), [], "every one of those is a Russian word on another layout")
    }

    /// Russian typed on the Kazakh layout is correct Russian, not Kazakh vocabulary to learn —
    /// but a word no dictionary has, like the slang spelling «кароче», is the user's own.
    func testRussianOnTheKazakhLayoutIsCheckedAgainstRussian() throws {
        let learner = makeLearner()
        for _ in 0..<2 {
            let session = try harness(learner)
            session.activeLayout = tables[.kk]!
            type("ну короче вот так кароче сделаем ", intended: .kk, on: session)
        }
        XCTAssertFalse(learner.isKnown("короче", in: .kk))
        XCTAssertFalse(learner.isKnown("сделаем", in: .kk))
        XCTAssertTrue(learner.isKnown("кароче", in: .kk))
    }

    /// What the first version learned from meaningless signals is dropped on upgrade; what the
    /// user accepted explicitly stays.
    func testUpgradeKeepsOnlyExplicitAcceptances() {
        let old = makeLearner()
        old.confirmed(typed: "rfhjxt", to: .ru)
        old.rejected(typed: "yfgbcfk")
        old.ignored(typed: "ghbdtn")
        old.kept(typed: "nfrjq", lang: .en)
        old.kept(typed: "nfrjq", lang: .en)
        old.kept(typed: "gjnjv", lang: .en)
        old.flush()
        defaults.set(1, forKey: "learningSchema")

        let upgraded = makeLearner()
        XCTAssertEqual(upgraded.decision(for: "rfhjxt"), .convert(to: .ru))
        XCTAssertNil(upgraded.decision(for: "yfgbcfk"))
        XCTAssertFalse(upgraded.isKnown("nfrjq", in: .en))
        XCTAssertEqual(upgraded.counts.total, 1)

        // Once only: later entries survive the next launch.
        upgraded.rejected(typed: "tot")
        upgraded.flush()
        XCTAssertEqual(makeLearner().decision(for: "tot"), .keep)
    }

    /// Words the user keeps never turn into mistakes later, whatever the context.
    func testKeptWordIsNeverOffered() throws {
        let learner = makeLearner()
        learner.rejected(typed: "tot")
        let harness = try harness(learner)
        harness.activeLayout = tables[.ru]!
        type("мне кажется ", intended: .ru, on: harness)
        harness.activeLayout = tables[.en]!
        type("tot ", intended: .en, on: harness)
        XCTAssertEqual(harness.screen, "мне кажется tot ")
        XCTAssertTrue(harness.events.isEmpty)
    }
}

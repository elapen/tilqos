import XCTest

@testable import TilqosKit

/// The behaviour table from the plan, executed.
final class DetectorTests: XCTestCase {
    private static let abcID = "com.apple.keylayout.ABC"
    private static let russianID = "com.apple.keylayout.Russian"
    private static let kazakhID = "com.apple.keylayout.Kazakh"

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

    private var abc: KeyMapTable!
    private var russian: KeyMapTable!
    private var kazakh: KeyMapTable!
    private var detector: Detector!

    override func setUpWithError() throws {
        for id in [Self.abcID, Self.russianID, Self.kazakhID] {
            guard Self.manager.table(forSourceID: id) != nil else {
                throw XCTSkip("Layout \(id) is not enabled on this machine")
            }
        }
        guard Self.store.model(for: .kk) != nil else {
            throw XCTSkip("Models missing — run ./generate-models.sh")
        }
        abc = Self.manager.table(forSourceID: Self.abcID)
        russian = Self.manager.table(forSourceID: Self.russianID)
        kazakh = Self.manager.table(forSourceID: Self.kazakhID)
        detector = Detector(models: Self.store)
    }

    // MARK: - Typing simulation

    /// Accumulates keystrokes the way the event tap would see them.
    private struct Session {
        var recorded: [RecordedStroke] = []
        var clock: TimeInterval = 0

        /// Types `intended` — the keys someone would press to get that text on
        /// `intendedOn` — while `active` is the layout actually selected.
        mutating func type(_ intended: String, intendedOn: KeyMapTable, active: KeyMapTable) {
            guard let strokes = intendedOn.strokes(for: intended) else {
                XCTFail("\(intendedOn.sourceID) cannot type \"\(intended)\"")
                return
            }
            for stroke in strokes {
                clock += 0.05
                recorded.append(
                    RecordedStroke(stroke: stroke, time: clock, layoutID: active.sourceID))
            }
        }

        /// Text typed with the right layout selected.
        mutating func type(_ text: String, on layout: KeyMapTable) {
            type(text, intendedOn: layout, active: layout)
        }
    }

    private func resolve(_ id: String) -> KeyMapTable? {
        Self.manager.table(forSourceID: id)
    }

    private func context(
        _ session: Session, minWordLength: Int = 3, exceptions: Set<String> = []
    ) -> Detector.Context {
        Detector.Context(
            recorded: session.recorded,
            candidateLayouts: [abc, russian, kazakh],
            resolve: { [weak self] in self?.resolve($0) },
            minWordLength: minWordLength,
            exceptions: exceptions
        )
    }

    private func evaluate(
        _ session: Session, scope: Detector.Scope = .tail,
        minWordLength: Int = 3, exceptions: Set<String> = []
    ) -> Detector.Action {
        detector.evaluate(
            context(session, minWordLength: minWordLength, exceptions: exceptions), scope: scope)
    }

    // MARK: - Latin typed where Cyrillic was meant

    func testGhbdtnBecomesPrivet() throws {
        var session = Session()
        session.type("привет", intendedOn: russian, active: abc)
        session.type(" ", on: abc)

        let outcome = try XCTUnwrap(evaluate(session).outcome, "expected a correction")
        XCTAssertEqual(outcome.before, "ghbdtn")
        XCTAssertEqual(outcome.after, "привет")
        XCTAssertEqual(outcome.targetLang, .ru)
        XCTAssertEqual(outcome.replacement.insertText, "привет ")
    }

    func testCyrillicTypedWhereLatinWasMeant() throws {
        var session = Session()
        session.type("keyboard", intendedOn: abc, active: russian)
        session.type(" ", on: russian)

        let outcome = try XCTUnwrap(evaluate(session).outcome)
        XCTAssertEqual(outcome.after, "keyboard")
        XCTAssertEqual(outcome.targetLang, .en)
    }

    // MARK: - Kazakh typed on the Russian layout

    func testKazakhGreetingWithDigitsIsCorrected() throws {
        var session = Session()
        session.type("сәлеметсіз", intendedOn: kazakh, active: russian)
        session.type(" ", on: russian)

        let outcome = try XCTUnwrap(evaluate(session).outcome)
        XCTAssertEqual(outcome.before, "с2леметс3з")
        XCTAssertEqual(outcome.after, "сәлеметсіз")
        XCTAssertEqual(outcome.targetLang, .kk)
    }

    func testQazaqWithDigitsIsCorrected() throws {
        var session = Session()
        session.type("қазақ", intendedOn: kazakh, active: russian)
        session.type(" ", on: russian)

        let outcome = try XCTUnwrap(evaluate(session).outcome)
        XCTAssertEqual(outcome.before, "0аза0")
        XCTAssertEqual(outcome.after, "қазақ")
        XCTAssertEqual(outcome.targetLang, .kk)
    }

    // MARK: - Digits typed on the Kazakh layout

    /// The reverse mistake, and the reason numbers need their own cost.
    func testNumberTypedOnKazakhLayoutIsRestored() throws {
        var session = Session()
        session.type("мне ", on: russian)
        session.type("25", intendedOn: abc, active: kazakh)
        session.type(" лет ", on: russian)

        let outcome = try XCTUnwrap(evaluate(session).outcome, "expected әғ to be read as 25")
        XCTAssertEqual(outcome.before, "әғ")
        XCTAssertEqual(outcome.after, "25")
    }

    // MARK: - Cases that must be left alone

    /// Every Russian letter is on the same key in both Cyrillic layouts, so this text is
    /// already correct however it was typed. Nothing to fix, and no reason to switch.
    func testRussianWordTypedOnKazakhLayoutIsLeftAlone() {
        var session = Session()
        session.type("Астана ", intendedOn: russian, active: kazakh)

        guard case .none = evaluate(session) else {
            return XCTFail("must not touch text that is already correct")
        }
    }

    func testCorrectlyTypedTextIsLeftAlone() {
        for (text, layout) in [
            ("привет мир ", "ru"), ("hello world ", "en"), ("сәлеметсіз бе ", "kk"),
        ] {
            let table = layout == "ru" ? russian! : (layout == "en" ? abc! : kazakh!)
            var session = Session()
            session.type(text, on: table)

            guard case .none = evaluate(session) else {
                return XCTFail("\"\(text)\" (\(layout)) was typed correctly and must not change")
            }
        }
    }

    func testShortWordsAreLeftAlone() {
        for word in ["ok ", "да ", "ия "] {
            var session = Session()
            session.type(word, on: word == "ok " ? abc : russian)
            guard case .none = evaluate(session) else {
                return XCTFail("\"\(word)\" is too short to judge")
            }
        }
    }

    func testEmailIsLeftAlone() {
        var session = Session()
        session.type("user@example.com ", on: abc)

        guard case .none = evaluate(session) else {
            return XCTFail("an address must never be rewritten")
        }
    }

    func testUrlIsLeftAlone() {
        var session = Session()
        session.type("https://github.com/taem/hunspell-kk ", on: abc)

        guard case .none = evaluate(session) else {
            return XCTFail("a URL must never be rewritten")
        }
    }

    func testCodeIdentifiersAreLeftAlone() {
        for snippet in ["let user_name = 1 ", "getUserName() ", "$PATH "] {
            var session = Session()
            session.type(snippet, on: abc)
            guard case .none = evaluate(session) else {
                return XCTFail("\"\(snippet)\" is code and must not change")
            }
        }
    }

    func testWordExceptionsAreHonoured() {
        var session = Session()
        session.type("привет", intendedOn: russian, active: abc)
        session.type(" ", on: abc)

        XCTAssertNotNil(evaluate(session).outcome, "baseline: this is normally corrected")

        guard case .none = evaluate(session, exceptions: ["ghbdtn"]) else {
            return XCTFail("an excepted word must be left alone")
        }
    }

    // MARK: - Mixed languages

    /// The behaviour the whole segmenter exists for: an English brand name inside Kazakh
    /// text is deliberate and must survive.
    func testIsolatedEnglishWordInsideKazakhTextSurvives() {
        var session = Session()
        session.type("менің ", on: kazakh)
        session.type("iPhone", on: abc)
        session.type(" телефоным ", on: kazakh)

        guard case .none = evaluate(session) else {
            return XCTFail("a deliberate foreign word must not be flipped")
        }
    }

    /// A run of mistyped words is corrected as a block, while the correctly typed
    /// neighbour between them stays put.
    func testOnlyMistypedWordsInAMixedParagraphAreCorrected() throws {
        var session = Session()
        session.type("привет как дела", intendedOn: russian, active: abc)
        session.type(" iPhone ", on: abc)

        let outcome = try XCTUnwrap(evaluate(session).outcome)
        XCTAssertTrue(
            outcome.after.contains("привет как дела"),
            "the mistyped run should be corrected, got \"\(outcome.after)\"")
        XCTAssertFalse(
            outcome.after.contains("шЗрщту"),
            "iPhone must not be converted, got \"\(outcome.after)\"")
        XCTAssertEqual(outcome.targetLang, .ru)
    }

    // MARK: - Scope

    func testLastWordScopeOnlyLooksAtTheClosingWord() throws {
        var session = Session()
        session.type("hello ", on: abc)
        session.type("привет", intendedOn: russian, active: abc)
        session.type(" ", on: abc)

        let outcome = try XCTUnwrap(evaluate(session, scope: .lastWord).outcome)
        XCTAssertEqual(outcome.before, "ghbdtn")
        XCTAssertEqual(outcome.corrections.count, 1)
    }

    // MARK: - Confidence

    func testConfidenceMapping() {
        XCTAssertEqual(Detector.confidence(gap: 0), 0)
        XCTAssertEqual(Detector.confidence(gap: 0.4), 0)
        XCTAssertGreaterThan(Detector.confidence(gap: 2.6), 0.95)
        XCTAssertEqual(Detector.confidence(gap: 100), 1)
    }

    func testRealMistakesClearTheAutoThreshold() throws {
        for (intended, typedOn) in [
            ("привет", "ru"), ("клавиатура", "ru"), ("keyboard", "en"), ("сәлеметсіз", "kk"),
        ] {
            let source = typedOn == "ru" ? russian! : (typedOn == "en" ? abc! : kazakh!)
            let wrong = typedOn == "en" ? russian! : abc!
            var session = Session()
            session.type(intended, intendedOn: source, active: wrong)
            session.type(" ", on: wrong)

            let action = evaluate(session, scope: .lastWord)
            guard case .fix(let outcome) = action else {
                return XCTFail("\"\(intended)\" should be a confident fix, got \(action)")
            }
            XCTAssertEqual(outcome.after, intended)
        }
    }

    // MARK: - Real words are never rewritten

    /// The reported bug: `еще` typed correctly became `tot`, because `tot` is English too.
    func testCorrectRussianWordIsNeverRewritten() {
        for word in ["еще ", "ещё ", "где ", "тот ", "тут ", "рук "] {
            var session = Session()
            session.type(word, on: russian)
            guard case .none = evaluate(session, scope: .lastWord) else {
                return XCTFail("\"\(word)\" was typed correctly and must not change")
            }
        }
    }

    func testCorrectEnglishWordIsNeverRewritten() {
        for word in ["tot ", "her ", "the ", "afk ", "lol "] {
            var session = Session()
            session.type(word, on: abc)
            guard case .none = evaluate(session, scope: .lastWord) else {
                return XCTFail("\"\(word)\" was typed correctly and must not change")
            }
        }
    }

    // MARK: - Punctuation keys that are letters elsewhere

    /// On ABC `,` `[` `;` are punctuation, but on the Russian layout they are б, х, ж — so a
    /// Russian word typed on ABC must not be cut at them.
    func testPunctuationKeysStayInsideTheWord() throws {
        for word in ["было", "хорошо", "тебе", "тоже", "люблю", "ещё"] {
            var session = Session()
            session.type(word, intendedOn: russian, active: abc)
            session.type(" ", on: abc)
            let outcome = try XCTUnwrap(evaluate(session, scope: .lastWord).outcome, word)
            XCTAssertEqual(outcome.after, word)
        }
    }

    func testOnlySeparatorsOfEveryLayoutEndAWord() throws {
        let layouts: [KeyMapTable] = [abc, russian, kazakh]
        func stroke(_ text: String, on table: KeyMapTable) throws -> KeyStroke {
            try XCTUnwrap(table.strokes(for: text)?.first)
        }
        XCTAssertTrue(Detector.endsWord(try stroke(" ", on: abc), typedOn: abc, candidates: layouts))
        // б, х, ю on the Russian layout.
        for key in [",", "[", "."] {
            XCTAssertFalse(
                Detector.endsWord(try stroke(key, on: abc), typedOn: abc, candidates: layouts), key)
        }
        // The Russian comma is Shift+6, which nothing else reads as a letter.
        XCTAssertTrue(
            Detector.endsWord(try stroke(",", on: russian), typedOn: russian, candidates: layouts))
    }

    /// `^` is where the Russian comma lives: typed on the same wrong layout right after a
    /// garbled word, it was meant as a comma.
    func testGarbledPunctuationAfterAFixComesAlong() throws {
        var session = Session()
        session.type("привет,", intendedOn: russian, active: abc)
        let outcome = try XCTUnwrap(evaluate(session, scope: .lastWord).outcome)
        XCTAssertEqual(outcome.replacement.insertText, "привет,")
    }

    // MARK: - Neighbours and context

    /// A sure fix pulls in the short garble before it, which alone was too short to touch.
    func testConfidentFixPullsInShortNeighbours() throws {
        var session = Session()
        session.type("я тебя", intendedOn: russian, active: abc)
        session.type(" ", on: abc)
        let outcome = try XCTUnwrap(evaluate(session, scope: .lastWord).outcome)
        XCTAssertEqual(outcome.after, "я тебя")
        XCTAssertEqual(outcome.changes.map(\.typed), ["z", "nt,z"])
    }

    /// A word that is a word on both layouts is left alone without context, and only offered
    /// with it.
    func testTwoSidedWordNeedsContext() {
        var alone = Session()
        alone.type("где", intendedOn: russian, active: abc)
        alone.type(" ", on: abc)
        guard case .none = evaluate(alone, scope: .lastWord) else {
            return XCTFail("`ult` is an English word too; without context it must stay")
        }

        var context = self.context(alone)
        context.precedingText = "мы вчера говорили,"
        guard case .suggest(let outcome) = detector.evaluate(context, scope: .lastWord) else {
            return XCTFail("with Russian text before it, `где` should be offered")
        }
        XCTAssertEqual(outcome.after, "где")
    }

    func testBrandInsideRussianTextStays() {
        var session = Session()
        session.type("скачай ", on: russian)
        session.type("Telegram ", on: abc)
        guard case .none = evaluate(session, scope: .lastWord) else {
            return XCTFail("a known Latin word inside Russian text is deliberate")
        }
    }

    // MARK: - What the user taught

    private final class Taught: LearnedVocabulary {
        var decisions: [String: LearnedDecision] = [:]
        var version = 0
        func isKnown(_ word: String, in lang: Lang) -> Bool { false }
        func decision(for typed: String) -> LearnedDecision? { decisions[typed.lowercased()] }
    }

    func testLearnedDecisionsOutrankInference() {
        let taught = Taught()
        var session = Session()
        session.type("привет", intendedOn: russian, active: abc)
        session.type(" ", on: abc)
        var context = self.context(session)
        context.learned = taught

        taught.decisions["ghbdtn"] = .keep
        guard case .none = detector.evaluate(context, scope: .lastWord) else {
            return XCTFail("a word the user keeps must stay")
        }

        // Converting a real word only because the user asked for it before.
        var tot = Session()
        tot.type("tot ", on: abc)
        var totContext = self.context(tot)
        totContext.learned = taught
        taught.decisions = ["tot": .convert(to: .ru)]
        taught.version += 1
        guard case .fix(let outcome) = detector.evaluate(totContext, scope: .lastWord) else {
            return XCTFail("an accepted conversion is applied next time")
        }
        XCTAssertEqual(outcome.after, "еще")
    }

    // MARK: - Empty and degenerate input

    func testEmptyBufferProducesNothing() {
        guard case .none = evaluate(Session()) else {
            return XCTFail("nothing typed, nothing to do")
        }
    }

    func testSingleLayoutInstalledProducesNothing() {
        var session = Session()
        session.type("привет", intendedOn: russian, active: abc)

        let action = detector.evaluate(
            Detector.Context(
                recorded: session.recorded,
                candidateLayouts: [abc],
                resolve: { [weak self] in self?.resolve($0) }),
            scope: .tail)
        guard case .none = action else {
            return XCTFail("with one layout there is nothing to switch to")
        }
    }
}

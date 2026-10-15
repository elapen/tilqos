import XCTest

@testable import TilqosKit

/// Measures the detector against `Corpus`, simulating typing the way the pipeline sees it.
///
/// The simulated user presses the keys for the text they *intend*, on whatever layout happens to
/// be active, and — like the real app — the active layout switches when a fix is applied, so the
/// rest of the sentence arrives correctly. Suggestions are ignored by the simulated user; they are
/// counted separately, as a softer kind of catch (or of false alarm).
final class CorpusTests: XCTestCase {
    static let manager: InputSourceManager = {
        let manager = InputSourceManager()
        manager.reloadTables()
        return manager
    }()

    static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: TrigramModelTests.modelDirectory)
        return store
    }()

    private var tables: [Lang: KeyMapTable] = [:]

    override func setUpWithError() throws {
        for (lang, id) in [
            (Lang.en, "com.apple.keylayout.ABC"), (.ru, "com.apple.keylayout.Russian"),
            (.kk, "com.apple.keylayout.Kazakh"),
        ] {
            guard let table = Self.manager.table(forSourceID: id) else {
                throw XCTSkip("Layout \(id) is not enabled on this machine")
            }
            tables[lang] = table
        }
        guard Self.store.model(for: .kk) != nil else { throw XCTSkip("Models missing") }
    }

    // MARK: - Simulation

    enum Event: Equatable {
        case fixed(before: String, after: String)
        case suggested(before: String, after: String)
    }

    struct Run {
        var screen: String
        var events: [Event]
        var fixes: Int { events.filter { if case .fixed = $0 { return true } else { return false } }.count }
        var suggestions: Int {
            events.filter { if case .suggested = $0 { return true } else { return false } }.count
        }
    }

    /// Thresholds the simulated user has chosen; the defaults unless a test says otherwise.
    var thresholds = (auto: 0.80, suggest: 0.45)
    /// Text in the field before the buffer, as the pipeline reads it through accessibility.
    var fieldText: String?

    /// Types the segments and returns what ends up on screen.
    func simulate(_ segments: [Corpus.Segment]) -> Run {
        let engine = HarnessEngine(tables: tables, store: Self.store, manager: Self.manager)
        engine.thresholds = thresholds
        engine.precedingText = fieldText
        var active: Lang? = nil
        for segment in segments {
            // A new segment means the user (or something) changed the layout on purpose.
            active = segment.active
            engine.activeLayout = tables[active!]!
            guard let intendedTable = tables[segment.intended] else { continue }
            for character in segment.text {
                guard let stroke = intendedTable.strokes(for: String(character))?.first else {
                    XCTFail("\(segment.intended) cannot type \(character)")
                    continue
                }
                engine.type(stroke)
            }
        }
        return Run(screen: engine.screen, events: engine.events)
    }

    func simulate(_ text: String, intended: Lang, active: Lang) -> Run {
        simulate([Corpus.Segment(text: text, intended: intended, active: active)])
    }

    // MARK: - Reporting

    struct Tally: CustomStringConvertible {
        var label: String
        var total = 0
        var changed = 0
        var suggested = 0
        var examples: [String] = []

        var description: String {
            let rate = total == 0 ? 0 : Double(changed) / Double(total) * 100
            let soft = total == 0 ? 0 : Double(suggested) / Double(total) * 100
            let shown = examples.prefix(12).joined(separator: ", ")
            return String(
                format: "%@: %d/%d changed (%.1f%%), %d suggested (%.1f%%)%@",
                label, changed, total, rate, suggested, soft,
                shown.isEmpty ? "" : "  e.g. \(shown)")
        }
    }

    struct Catch: CustomStringConvertible {
        var label: String
        var total = 0
        var fixed = 0
        var suggested = 0
        var wrong = 0
        var misses: [String] = []
        var offered: [String] = []

        var description: String {
            let pct = { (n: Int) in self.total == 0 ? 0 : Double(n) / Double(self.total) * 100 }
            return String(
                format: "%@: fixed %d/%d (%.1f%%), suggested %d (%.1f%%), missed %d (%.1f%%), wrong %d%@",
                label, fixed, total, pct(fixed), suggested, pct(suggested),
                total - fixed - suggested - wrong, pct(total - fixed - suggested - wrong), wrong,
                (misses.isEmpty ? "" : "  missed e.g. \(misses.prefix(14).joined(separator: ", "))")
                    + (offered.isEmpty ? "" : "  offered: \(offered.prefix(8).joined(separator: ", "))"))
        }
    }

    /// Correct words, each on its own, followed by a space. Any change is a false positive.
    func mustStay(_ label: String, _ words: [String], lang: Lang, prefix: String = "") -> Tally {
        var tally = Tally(label: label)
        for word in words {
            tally.total += 1
            let run = simulate(prefix + word + " ", intended: lang, active: lang)
            if run.screen != prefix + word + " " {
                tally.changed += 1
                tally.examples.append("\(word)→\(run.screen.dropFirst(prefix.count).trimmingCharacters(in: .whitespaces))")
            } else if run.suggestions > 0 {
                tally.suggested += 1
                if case .suggested(_, let after)? = run.events.last {
                    tally.examples.append("\(word)?\(after)")
                }
            }
        }
        return tally
    }

    /// Words typed on the wrong layout. Anything short of the intended text is a miss.
    func mustConvert(
        _ label: String, _ words: [String], intended: Lang, active: Lang,
        context: (text: String, lang: Lang)? = nil
    ) -> Catch {
        var result = Catch(label: label)
        for word in words {
            result.total += 1
            var segments: [Corpus.Segment] = []
            if let context {
                segments.append(Corpus.Segment(text: context.text, intended: context.lang, active: context.lang))
            }
            segments.append(Corpus.Segment(text: word + " ", intended: intended, active: active))
            let run = simulate(segments)
            let expected = (context?.text ?? "") + word + " "
            if run.screen == expected {
                result.fixed += 1
            } else if run.events.contains(where: {
                if case .suggested(_, let after) = $0 { return after.hasPrefix(word) || after == word }
                return false
            }) {
                result.suggested += 1
                result.offered.append(word)
            } else if run.fixes > 0 {
                result.wrong += 1
                result.misses.append("\(word)⇒\(run.screen.trimmingCharacters(in: .whitespaces))")
            } else {
                result.misses.append(word)
            }
        }
        return result
    }

    // MARK: - The report and the quality bar

    /// Prints the whole report — the numbers quoted in the README — and holds the line on it.
    ///
    /// The bar is asymmetric on purpose. A correctly typed word must never be rewritten, so every
    /// must-stay set allows zero automatic changes. Misses are allowed, within limits, because the
    /// cases left are genuinely ambiguous (`her` is `рук`, both are words).
    func testCorpusReport() throws {
        var lines: [String] = []

        // Correctly typed words, on their own.
        let stay = [
            mustStay("ru frequent", Corpus.russianFrequent, lang: .ru),
            mustStay("en frequent", Corpus.englishFrequent, lang: .en),
            mustStay("kk frequent", Corpus.kazakhFrequent, lang: .kk),
            mustStay("ru frequent after context", Corpus.russianFrequent, lang: .ru, prefix: "это "),
            mustStay("en frequent after context", Corpus.englishFrequent, lang: .en, prefix: "this "),
            mustStay("ru unknown/slang/names", Corpus.russianUnknown, lang: .ru),
            mustStay("en unknown/slang/names", Corpus.englishUnknown, lang: .en),
            mustStay("ru typos", Corpus.typos(of: Corpus.russianFrequent, limit: 300), lang: .ru),
            mustStay("en typos", Corpus.typos(of: Corpus.englishFrequent, limit: 300), lang: .en),
            mustStay("ru collisions", Corpus.russianCollisions, lang: .ru),
            mustStay("ru collisions after context", Corpus.russianCollisions, lang: .ru, prefix: "это "),
            mustStay("en collisions", Corpus.englishCollisions, lang: .en),
            mustStay("en collisions after context", Corpus.englishCollisions, lang: .en, prefix: "this "),
            mustStay("en punctuated", Corpus.englishPunctuated, lang: .en),
            mustStay("ru punctuated", Corpus.russianPunctuated, lang: .ru),
        ]
        lines.append("=== MUST STAY (false positives) ===")
        lines += stay.map(\.description)
        for tally in stay {
            XCTAssertEqual(tally.changed, 0, "correct words were rewritten: \(tally)")
        }
        XCTAssertLessThanOrEqual(
            stay.map(\.suggested).reduce(0, +), 2, "too many offers on correct words")

        // Correct sentences.
        lines.append("=== SENTENCES TYPED CORRECTLY ===")
        for (label, sentences, lang) in [
            ("ru", Corpus.russianSentences, Lang.ru), ("en", Corpus.englishSentences, .en),
            ("kk", Corpus.kazakhSentences, .kk),
        ] {
            var broken: [String] = []
            var suggestedCount = 0
            for sentence in sentences {
                let run = simulate(sentence + " ", intended: lang, active: lang)
                if run.screen != sentence + " " { broken.append("«\(sentence)»→«\(run.screen)»") }
                suggestedCount += run.suggestions
            }
            lines.append(
                "\(label): \(broken.count)/\(sentences.count) sentences changed, "
                    + "\(suggestedCount) suggestions" + (broken.isEmpty ? "" : "\n    " + broken.prefix(8).joined(separator: "\n    ")))
            XCTAssertTrue(broken.isEmpty, "\(label) sentences typed correctly were changed")
            XCTAssertEqual(suggestedCount, 0, "\(label) sentences typed correctly drew offers")
        }

        // Wrong layout.
        let convert = [
            mustConvert("ru typed on en", Corpus.russianOnEnglish, intended: .ru, active: .en),
            mustConvert("en typed on ru", Corpus.englishOnRussian, intended: .en, active: .ru),
            mustConvert("kk typed on ru", Corpus.kazakhOnRussian, intended: .kk, active: .ru),
            mustConvert("kk typed on en", Corpus.kazakhOnEnglish, intended: .kk, active: .en),
            mustConvert(
                "ru typed on en after ru context", Corpus.russianOnEnglish, intended: .ru,
                active: .en, context: ("мне кажется ", .ru)),
            mustConvert(
                "en typed on ru after en context", Corpus.englishOnRussian, intended: .en,
                active: .ru, context: ("i think ", .en)),
            mustConvert(
                "rare ru typed on en", Corpus.russianRareOnEnglish, intended: .ru, active: .en),
            mustConvert(
                "ru collisions typed on en", Corpus.russianCollisions, intended: .ru, active: .en),
            mustConvert(
                "ru collisions typed on en after ru context", Corpus.russianCollisions,
                intended: .ru, active: .en, context: ("мне кажется ", .ru)),
        ]
        lines.append("=== MUST CONVERT (false negatives) ===")
        lines += convert.map(\.description)
        for result in convert {
            // Never half a word: a conversion is right or it does not happen.
            XCTAssertEqual(result.wrong, 0, "wrong conversions: \(result)")
        }
        for result in convert.prefix(6) {
            let caught = Double(result.fixed + result.suggested) / Double(max(1, result.total))
            XCTAssertGreaterThanOrEqual(caught, 0.95, "too many misses: \(result)")
        }

        // Whole sentences on the wrong layout.
        lines.append("=== SENTENCES TYPED ON THE WRONG LAYOUT ===")
        for (label, sentences, intended, active) in [
            ("ru on en", Corpus.russianSentences, Lang.ru, Lang.en),
            ("en on ru", Corpus.englishSentences, .en, .ru),
            ("kk on ru", Corpus.kazakhSentences, .kk, .ru),
            ("kk on en", Corpus.kazakhSentences, .kk, .en),
        ] {
            var fixedSentences = 0
            var failures: [String] = []
            for sentence in sentences {
                let run = simulate(sentence + " ", intended: intended, active: active)
                if run.screen == sentence + " " {
                    fixedSentences += 1
                } else {
                    failures.append("«\(sentence)»→«\(run.screen)»")
                }
            }
            lines.append(
                "\(label): \(fixedSentences)/\(sentences.count) sentences fully correct"
                    + (failures.isEmpty ? "" : "\n    " + failures.prefix(6).joined(separator: "\n    ")))
            XCTAssertGreaterThanOrEqual(
                Double(fixedSentences) / Double(sentences.count), 0.95, "\(label): \(failures)")
        }

        // Mixed and context cases.
        lines.append("=== MIXED / CONTEXT CASES ===")
        var passed = 0
        for testCase in Corpus.mixedCases {
            let run = simulate(testCase.segments)
            if run.screen == testCase.expected {
                passed += 1
            } else {
                lines.append("  FAIL \(testCase.name): «\(run.screen)» (expected «\(testCase.expected)»)")
            }
        }
        lines.append("mixed: \(passed)/\(Corpus.mixedCases.count) pass")
        XCTAssertEqual(passed, Corpus.mixedCases.count, "mixed and context cases")

        print("\n" + lines.joined(separator: "\n") + "\n")
    }
}

extension CorpusTests {
    // MARK: - Context from the text field

    /// The buffer starts empty after every click, Enter and pause — exactly when a wrong layout
    /// is most likely — so the pipeline reads the words in front of it. Measured here on the
    /// words that need context most: short ones and two-sided ones.
    func testContextFromTheTextField() {
        let short = Corpus.russianOnEnglish.filter { $0.count <= 4 }
        var lines = ["=== CONTEXT FROM THE TEXT FIELD ==="]

        fieldText = nil
        let alone = mustConvert("short ru typed on en, empty field", short, intended: .ru, active: .en)
        fieldText = "мы вчера долго говорили,"
        let withRussian = mustConvert(
            "short ru typed on en, Russian text before", short, intended: .ru, active: .en)
        lines += [alone.description, withRussian.description]
        XCTAssertGreaterThanOrEqual(withRussian.fixed, alone.fixed)
        XCTAssertEqual(withRussian.wrong, 0)

        // Context tilts, never overrules: English typed correctly after Russian text stays.
        let englishAfterRussian = mustStay(
            "en frequent after Russian text", Corpus.englishFrequent, lang: .en)
        let collisions = mustStay(
            "en collisions after Russian text", Corpus.englishCollisions, lang: .en)
        fieldText = "we talked for a long time yesterday,"
        let russianAfterEnglish = mustStay(
            "ru frequent after English text", Corpus.russianFrequent, lang: .ru)
        fieldText = nil
        lines += [englishAfterRussian.description, collisions.description, russianAfterEnglish.description]
        for tally in [englishAfterRussian, collisions, russianAfterEnglish] {
            XCTAssertEqual(tally.changed, 0, "\(tally)")
        }
        print("\n" + lines.joined(separator: "\n") + "\n")
    }

    // MARK: - Presets

    /// What the settings presets do, on the numbers that matter: rewrites of correct words, and
    /// what happens to real mistakes.
    func testPresets() {
        var lines = ["=== PRESETS (auto / suggest) ==="]
        for (name, preset) in [
            ("cautious", (auto: 0.93, suggest: 0.55)), ("balanced", (auto: 0.80, suggest: 0.45)),
            ("bold", (auto: 0.55, suggest: 0.30)),
        ] {
            thresholds = preset
            let stay = [
                mustStay("ru", Corpus.russianFrequent, lang: .ru),
                mustStay("en", Corpus.englishFrequent, lang: .en),
                mustStay("en collisions", Corpus.englishCollisions, lang: .en),
                mustStay("ru typos", Corpus.typos(of: Corpus.russianFrequent, limit: 300), lang: .ru),
            ]
            let convert = [
                mustConvert("ru on en", Corpus.russianOnEnglish, intended: .ru, active: .en),
                mustConvert("en on ru", Corpus.englishOnRussian, intended: .en, active: .ru),
                mustConvert("kk on ru", Corpus.kazakhOnRussian, intended: .kk, active: .ru),
            ]
            let changed = stay.map(\.changed).reduce(0, +)
            let offered = stay.map(\.suggested).reduce(0, +)
            let total = convert.map(\.total).reduce(0, +)
            let fixed = convert.map(\.fixed).reduce(0, +)
            let suggested = convert.map(\.suggested).reduce(0, +)
            lines.append(
                String(
                    format: "%@ %.2f/%.2f: correct words rewritten %d, offered %d | mistakes fixed %d/%d (%.1f%%), offered %d",
                    name, preset.auto, preset.suggest, changed, offered, fixed, total,
                    Double(fixed) / Double(total) * 100, suggested))
            if name != "bold" { XCTAssertEqual(changed, 0, name) }
        }
        thresholds = (0.80, 0.45)
        print("\n" + lines.joined(separator: "\n") + "\n")
    }
}

// MARK: - The engine under test

/// Drives `TypingEngine` exactly the way `Pipeline` does, minus the side effects: every fix is
/// applied to the buffer and switches the active layout, as the real app does.
final class HarnessEngine {
    let tables: [Lang: KeyMapTable]
    let engine: TypingEngine
    let manager: InputSourceManager
    var activeLayout: KeyMapTable
    var learned: LearnedVocabulary?
    var precedingText: String?
    var thresholds = (auto: 0.80, suggest: 0.45)
    private(set) var events: [CorpusTests.Event] = []
    private var clock: TimeInterval = 0

    init(tables: [Lang: KeyMapTable], store: ModelStore, manager: InputSourceManager) {
        self.tables = tables
        self.engine = TypingEngine(detector: Detector(models: store))
        self.manager = manager
        self.activeLayout = tables[.en]!
    }

    private var candidates: [KeyMapTable] { [tables[.en]!, tables[.ru]!, tables[.kk]!] }

    var screen: String {
        engine.screenText(resolve: { [manager] in manager.table(forSourceID: $0) })
    }

    func backspace() {
        clock += 0.08
        engine.backspace()
    }

    func type(_ stroke: KeyStroke) {
        clock += 0.08
        guard let end = engine.record(stroke, layout: activeLayout, at: clock, candidates: candidates)
        else { return }
        let settings = TypingEngine.Settings(
            candidateLayouts: candidates,
            resolve: { [manager] in manager.table(forSourceID: $0) },
            autoThreshold: thresholds.auto, suggestThreshold: thresholds.suggest,
            learned: learned)
        switch engine.decide(at: end, settings: settings, precedingText: precedingText) {
        case .apply(let outcome):
            engine.didApply(outcome, automatic: true, at: clock)
            events.append(.fixed(before: outcome.before, after: outcome.after))
            if let target = manager.table(forSourceID: outcome.targetLayoutID) { activeLayout = target }
        case .offer(let outcome):
            events.append(.suggested(before: outcome.before, after: outcome.after))
        case .none:
            break
        }
    }
}

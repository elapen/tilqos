import XCTest

@testable import TilqosKit

/// P4: nothing typed, copied or titled is ever logged, at any level, in any build; app
/// identifiers go in as private values; diagnostics are off by default.
final class PrivacyLogTests: XCTestCase {
    private var capture: LogCapture!

    override func tearDown() {
        capture?.stop()
    }

    // MARK: - Behaviour

    /// Types a Russian word on the English layout through the real engine, detector and replacer,
    /// with the debug trail switched on, and reads back everything that was logged.
    func testTypedWordNeverReachesTheLog() throws {
        let manager = CorpusTests.manager
        guard let abc = manager.table(forSourceID: "com.apple.keylayout.ABC"),
            let russian = manager.table(forSourceID: "com.apple.keylayout.Russian"),
            let kazakh = manager.table(forSourceID: "com.apple.keylayout.Kazakh")
        else { throw XCTSkip("ABC, Russian and Kazakh layouts are not all enabled on this machine") }
        try XCTSkipIf(CorpusTests.store.model(for: .kk) == nil, "Models missing")
        capture = LogCapture(diagnostics: true)

        let typedWords = ["ghbdtn", "Ghbdtn", "vbh", "qalaisyz", "hunter2"]
        // 1. The detector, word after word.
        let harness = HarnessEngine(
            tables: [.en: abc, .ru: russian, .kk: kazakh], store: CorpusTests.store, manager: manager)
        for word in typedWords {
            for character in word + " " {
                if let stroke = abc.strokes(for: String(character))?.first { harness.type(stroke) }
            }
        }
        XCTAssertFalse(harness.events.isEmpty, "the engine did decide something")

        // 2. The replacer, with its step-by-step trace wired to the log exactly as the injector does.
        let field = TextReplacerTests.FakeField("ghbdtn ")
        let keys = TextReplacerTests.FakeKeys()
        keys.field = field
        var replacer = TextReplacer(
            field: field, keys: keys, wait: { _ in }, preferAccessibility: true, verify: true)
        replacer.trace = { Log.debug(.inject, $0) }
        _ = replacer.apply(Replacement(deletedText: "ghbdtn ", insertText: "привет "))
        // and again by keystrokes, where the app will not say what it holds
        let blind = TextReplacerTests.FakeField("vbh ", quirks: .init(unreadable: true))
        keys.field = blind
        var blindReplacer = TextReplacer(
            field: blind, keys: keys, wait: { _ in }, preferAccessibility: true, verify: true)
        blindReplacer.trace = { Log.debug(.inject, $0) }
        _ = blindReplacer.apply(Replacement(deletedText: "vbh ", insertText: "мир "))

        let log = capture.all.lowercased()
        XCTAssertFalse(capture.lines.isEmpty, "the test must actually have logged something")
        XCTAssertTrue(capture.lines.contains { $0.category == .detect }, "detector trail present")
        XCTAssertTrue(capture.lines.contains { $0.category == .inject }, "replacer trail present")
        let forbidden = typedWords + ["привет", "мир", "қалайсыз", "ghbdtn "]
        for word in forbidden {
            XCTAssertFalse(log.contains(word.lowercased()), "the log contains typed or corrected text: \(word)")
        }
        // the Cyrillic letters of the corrections do not appear in any form either
        XCTAssertNil(log.range(of: "[а-яёәіңғүұқөһ]", options: .regularExpression))
    }

    func testPrivateDetailIsRedactedEverywhereWeCanSeeIt() {
        capture = LogCapture(diagnostics: true)
        Log.info(.inject, "accessibility edit unreliable, using keystrokes for app", private: "com.secret.app")
        Log.warn(.inject, "something", private: "com.secret.app")
        XCTAssertEqual(capture.lines.count, 2)
        for line in capture.lines {
            XCTAssertTrue(line.text.hasSuffix(Log.redacted))
            XCTAssertFalse(line.text.contains("com.secret.app"))
        }
    }

    func testDiagnosticsAreOffByDefaultAndOnlyFailuresAreWritten() {
        XCTAssertFalse(Prefs(defaults: ScratchDefaults.make().defaults).diagnosticsEnabled)
        capture = LogCapture(diagnostics: false)
        Log.debug(.detect, "per-word trail")
        Log.info(.app, "launched")
        Log.warn(.tap, "tap disabled")
        Log.error(.app, "model rejected")
        XCTAssertEqual(capture.lines.map(\.level), [.warn, .error])
        ScratchDefaults.removeAll()
    }

    func testDebugMessageIsNotEvenBuiltWhileDiagnosticsAreOff() {
        capture = LogCapture(diagnostics: false)
        var built = false
        Log.debug(.detect, { built = true; return "x" }())
        XCTAssertFalse(built, "the per-keystroke trail costs nothing when nobody asked for it")
    }

    func testTheDiagnosticsPreferenceSwitchesTheLog() {
        capture = LogCapture(diagnostics: false)
        let scratch = ScratchDefaults.make()
        defer { scratch.remove() }
        let prefs = Prefs(defaults: scratch.defaults)
        prefs.diagnosticsEnabled = true
        XCTAssertTrue(Log.diagnosticsEnabled)
        Log.info(.app, "now it is written")
        XCTAssertEqual(capture.lines.count, 1)
        prefs.diagnosticsEnabled = false
        XCTAssertFalse(Log.diagnosticsEnabled)
    }

    // MARK: - Audit of the source

    /// No other logging road exists: everything goes through `Log`.
    func testNoStrayLoggingCalls() throws {
        let forbidden = try NSRegularExpression(
            pattern: #"\bNSLog\s*\(|\bos_log\s*\(|(?<![\w.])print\s*\(|\bdebugPrint\s*\(|(?<![\w.])dump\s*\(|\bfputs\s*\(|\bFileHandle\.standard(Error|Output)|\bLogger\s*\(|\bOSLog\b|\bfwrite\s*\("#)
        var offenders: [String] = []
        for (name, text) in try PrivacyTestSupport.appSources() where name != "TilqosKit/Core/Log.swift" {
            for (index, line) in text.components(separatedBy: "\n").enumerated() {
                let code = line.trimmingCharacters(in: .whitespaces)
                if code.hasPrefix("//") { continue }
                if forbidden.firstMatch(in: code, range: NSRange(code.startIndex..., in: code)) != nil {
                    offenders.append("\(name):\(index + 1): \(code)")
                }
            }
        }
        XCTAssertTrue(
            offenders.isEmpty,
            "log through `Log` (Core/Log.swift) only:\n" + offenders.joined(separator: "\n"))
    }

    /// Every value interpolated into a log message has been reviewed: it is a number, a flag, a
    /// language or layout name, a fixed phrase, or an error of the app itself, never text from
    /// the user. A new interpolation fails this test until someone reviews it and adds it here.
    static let reviewedInterpolations: Set<String> = [
        "$0.lang.rawValue",  // model language code
        #"$0.split(separator: " ").count"#,  // number of words in the field context (a count)
        "$0.stemCount",
        #"Bundle.main.bundleIdentifier ?? "none""#,  // the app's own identifier
        "Self.describe(decision)",  // lengths, language, confidence (see Pipeline.describe)
        "Self.describe(world.slots[index], skip: skipReason(world.slots[index], context: world.context), judgement: found)",
        #"String(format: "%.2f", confidence)"#,
        #"String(format: "%.2f", outcome.confidence)"#,
        #"String(format: "%.2f", target.cost)"#,
        #"String(format: "%.2f", typed.cost)"#,
        "before",  // an entry count (Learner)
        #"candidateLayouts.compactMap { $0.lang?.rawValue }.joined(separator: ",")"#,
        "confirmation.summary",  // fixed words (TextReplacer.Confirmation.summary)
        #"context.map { "\($0.split(separator: " ").count) words" } ?? "none""#,
        "currentMode.rawValue",
        "engine.buffer.count",
        "entries.count",
        "error",  // a model file failing to load: path inside the app bundle
        "error.localizedDescription",  // launch-at-login registration failing
        "ignored",  // a Bool
        "lang.rawValue",
        "languages.count",
        #"layout.sourceID.split(separator: ".").last ?? """#,  // layout name, e.g. "ABC"
        #"loadedModels.keys.map(\.rawValue).sorted().joined(separator: ",")"#,
        "loadedMorphology.stemCount",
        "loadedMorphology.suffixCount",
        "outcome.after.count",  // a length
        "outcome.before.count",  // a length
        "outcome.targetLang.rawValue",
        "pattern",  // the app's own regular expressions
        "prefs.didCompleteOnboarding",
        "reason",  // fixed phrases of TextReplacer / the engine
        "reason.rawValue",
        "rejected.count",
        "replacement.deleteCount",
        "replacement.insertText.count",
        "result",  // TextReplacer.AccessibilityResult: fixed phrases
        "status.accessibility",
        "status.inputMonitoring",
        "strategy.rawValue",
        "target.isClean",
        "target.isKnown",
        "target.length",
        "target.text == typed.text",  // a Bool
        #"type == .tapDisabledByTimeout ? "timeout" : "user input""#,
        "typed.isKnown",
        "typed.length",
        "url.lastPathComponent",  // a model file name
        "words.count",
    ]

    func testEveryLoggedInterpolationHasBeenReviewed() throws {
        var unreviewed: [String] = []
        for (name, text) in try PrivacyTestSupport.appSources() where name != "TilqosKit/Core/Log.swift" {
            for expression in Self.interpolations(inCallsOf: text) where !Self.reviewedInterpolations.contains(expression) {
                unreviewed.append("\(name): \\(\(expression))")
            }
        }
        XCTAssertTrue(
            unreviewed.isEmpty,
            "new values in log messages must be reviewed: they may never be typed text, clipboard "
                + "text, or window titles. Use `private:` for app identifiers. Then add them to "
                + "reviewedInterpolations:\n" + unreviewed.joined(separator: "\n"))
    }

    func testTheScannerSeesTheCallsItIsMeantToSee() throws {
        let sample = """
            Log.info(.app, "a \\(secret) b \\(f(x)) c")
            Log.debug(.detect, "d \\(other.text)", private: bundleID)
            trace("steps \\(count)")
            Log.warn(.tap, "none")
            """
        XCTAssertEqual(Self.interpolations(inCallsOf: sample), ["secret", "f(x)", "other.text", "count"])
    }

    /// The `\(...)` expressions inside `Log.<level>(...)` and `trace(...)` calls.
    static func interpolations(inCallsOf source: String) -> [String] {
        let characters = Array(source)
        let starts = try! NSRegularExpression(pattern: #"Log\.(?:info|debug|warn|error)\(|\btrace\("#)
        var result: [String] = []
        for match in starts.matches(in: source, range: NSRange(source.startIndex..., in: source)) {
            guard let range = Range(match.range, in: source) else { continue }
            var index = source.distance(from: source.startIndex, to: range.upperBound)
            var depth = 1
            while index < characters.count, depth > 0 {
                let character = characters[index]
                if character == "\"" {
                    index += 1
                    while index < characters.count, characters[index] != "\"" {
                        if characters[index] == "\\", index + 1 < characters.count, characters[index + 1] == "(" {
                            var inner = 1
                            var end = index + 2
                            while end < characters.count, inner > 0 {
                                if characters[end] == "(" { inner += 1 }
                                if characters[end] == ")" { inner -= 1 }
                                end += 1
                            }
                            let expression = String(characters[(index + 2)..<(end - 1)])
                            result.append(expression.split(whereSeparator: \.isWhitespace).joined(separator: " "))
                            index = end
                            continue
                        }
                        if characters[index] == "\\" { index += 1 }
                        index += 1
                    }
                } else if character == "(" {
                    depth += 1
                } else if character == ")" {
                    depth -= 1
                }
                index += 1
            }
        }
        return result
    }
}

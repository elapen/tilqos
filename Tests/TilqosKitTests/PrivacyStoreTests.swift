import XCTest

@testable import TilqosKit

/// P5: what is stored on disk, and the functions that wipe each store.
///
/// All local. Plain text: the user's exception words, the Latin words to leave alone, the
/// pinned transliteration spellings. App identifiers: per-app rules and the Cyrillic layout
/// remembered per app. Learned vocabulary: salted 64-bit FNV-1a hashes, which are NOT secret
/// (the salt is stored beside them and the hash is not cryptographic).
final class PrivacyStoreTests: XCTestCase {
    private var scratch: ScratchDefaults!
    private var prefs: Prefs!
    private var defaults: UserDefaults { scratch.defaults }

    override func setUp() {
        scratch = ScratchDefaults.make()
        prefs = Prefs(defaults: scratch.defaults)
    }

    override func tearDown() {
        scratch?.remove()
    }

    private func stored(_ key: String) -> Any? { defaults.object(forKey: key) }

    /// Everything the store holds, as one string, for "is this word anywhere in it".
    private var everythingStored: String { "\(defaults.dictionaryRepresentation())" }

    // MARK: - Plain-text stores

    func testClearExceptionWordsRemovesTheWordsAndTheKey() {
        prefs.addWordException("Kaspi")
        prefs.addWordException("zhmyh")
        prefs.wordExceptions.insert("another")
        XCTAssertEqual(prefs.wordExceptions, ["kaspi", "zhmyh", "another"])
        XCTAssertTrue(everythingStored.contains("zhmyh"), "they are stored as plain text, as documented")

        var posted = 0
        let token = NotificationCenter.default.addObserver(
            forName: Prefs.didChange, object: nil, queue: nil) { _ in posted += 1 }
        defer { NotificationCenter.default.removeObserver(token) }

        prefs.clearExceptionWords()
        XCTAssertTrue(prefs.wordExceptions.isEmpty)
        XCTAssertNil(stored("wordExceptions"), "the key is gone, not just emptied")
        XCTAssertFalse(everythingStored.contains("zhmyh"))
        XCTAssertGreaterThan(posted, 0, "the pipeline hears about it and refreshes its snapshot")
    }

    func testClearNeverConvertAndOverrides() {
        prefs.addNeverConvert(["Instagram", "zoom"])
        prefs.addTranslitOverride(latin: "jatqanyn", cyrillic: "жатқанын")
        XCTAssertFalse(prefs.translitNeverConvert.isEmpty)
        XCTAssertFalse(prefs.translitOverrides.isEmpty)

        prefs.clearNeverConvertWords()
        XCTAssertTrue(prefs.translitNeverConvert.isEmpty)
        XCTAssertNil(stored("translitNeverConvert"))
        XCTAssertFalse(prefs.translitOverrides.isEmpty, "each store is wiped on its own")

        prefs.clearTranslitOverrides()
        XCTAssertTrue(prefs.translitOverrides.isEmpty)
        XCTAssertNil(stored("translitOverrides"))
        XCTAssertFalse(everythingStored.contains("instagram"))
        XCTAssertFalse(everythingStored.contains("жатқанын"))
    }

    func testClearPerAppData() {
        let rules = AppRules(prefs: prefs)
        rules.setMode(.off, for: "com.example.editor")
        rules.rememberCyrillic(layoutID: "com.apple.keylayout.Kazakh", for: "com.example.editor")
        XCTAssertEqual(prefs.appRules.count, 1)
        XCTAssertEqual(prefs.appCyrillicMemory.count, 1)

        prefs.clearRememberedLayouts()
        XCTAssertTrue(prefs.appCyrillicMemory.isEmpty)
        XCTAssertEqual(prefs.appRules.count, 1, "rules are untouched by forgetting layouts")

        prefs.clearAppRules()
        XCTAssertTrue(prefs.appRules.isEmpty)
        XCTAssertFalse(everythingStored.contains("com.example.editor"))
    }

    func testWipingOneStoreLeavesTheSettingsAlone() {
        prefs.autoThreshold = 0.7
        prefs.addWordException("word")
        prefs.clearExceptionWords()
        XCTAssertEqual(prefs.autoThreshold, 0.7)
    }

    // MARK: - Learned vocabulary

    func testResetLearnedVocabularyWipesEntriesAndRotatesTheSalt() throws {
        let learner = Learner(defaults: defaults)
        learner.kept(typed: "Aidar", lang: .en)
        learner.kept(typed: "Aidar", lang: .en)
        learner.confirmed(typed: "ghbdtn", to: .ru)
        learner.rejected(typed: "instagram")
        learner.flush()
        XCTAssertEqual(learner.counts.total, 3)
        let oldSalt = try XCTUnwrap(defaults.data(forKey: Learner.Key.salt))
        let oldHash = learner.hash("ghbdtn")

        learner.resetLearnedVocabulary()

        XCTAssertEqual(learner.counts.total, 0)
        XCTAssertNil(learner.decision(for: "ghbdtn"))
        XCTAssertFalse(learner.isKnown("Aidar", in: .en))
        XCTAssertNotEqual(try XCTUnwrap(defaults.data(forKey: Learner.Key.salt)), oldSalt, "new salt")
        XCTAssertNotEqual(learner.hash("ghbdtn"), oldHash)
        let onDisk = try XCTUnwrap(defaults.dictionary(forKey: Learner.Key.entries))
        XCTAssertTrue(onDisk.isEmpty, "written at once, not two seconds later")

        // a restart sees nothing
        XCTAssertEqual(Learner(defaults: defaults).counts.total, 0)
        // and the old name still works
        learner.confirmed(typed: "abc", to: .en)
        learner.forgetAll()
        XCTAssertEqual(learner.counts.total, 0)
    }

    func testLearnedWordsAreStoredAsSaltedHashesAndSayWhatTheyAre() throws {
        let learner = Learner(defaults: defaults)
        learner.confirmed(typed: "qalaisyz", to: .kk)
        learner.flush()
        let dump = everythingStored
        XCTAssertFalse(dump.contains("qalaisyz"))
        // 64-bit hash as 16 hex digits, salt of 8 bytes in the same store.
        let entries = try XCTUnwrap(defaults.dictionary(forKey: Learner.Key.entries))
        XCTAssertEqual(entries.keys.first?.count, 16)
        XCTAssertEqual(defaults.data(forKey: Learner.Key.salt)?.count, 8)
    }

    func testTheHashIsGuessableGivenTheSaltSoNoOneMayClaimOtherwise() throws {
        // Whoever can read the preferences file has the salt next to the hashes and can test
        // guesses. This is what the README says; the test pins the facts it rests on.
        let learner = Learner(defaults: defaults)
        learner.confirmed(typed: "hello", to: .en)
        learner.flush()
        let salt = [UInt8](try XCTUnwrap(defaults.data(forKey: Learner.Key.salt)))
        func fnv(_ word: String) -> UInt64 {
            var value: UInt64 = 0xCBF2_9CE4_8422_2325
            for byte in salt + Array(word.utf8) { value = (value ^ UInt64(byte)) &* 0x0000_0100_0000_01B3 }
            return value
        }
        let entries = try XCTUnwrap(defaults.dictionary(forKey: Learner.Key.entries))
        let guessed = ["world", "hello", "tilqos"].filter { entries[String(format: "%016llx", fnv($0))] != nil }
        XCTAssertEqual(guessed, ["hello"], "a guess with the stored salt confirms the word")
    }

    func testNoSourceFileCallsTheHashesOneWayOrIrreversible() throws {
        let words = try NSRegularExpression(
            pattern: #"one-way|irreversib|cannot be (reversed|recovered|reconstructed)|необратим"#,
            options: .caseInsensitive)
        var offenders: [String] = []
        for (name, text) in try PrivacyTestSupport.appSources() {
            for (index, line) in text.components(separatedBy: "\n").enumerated()
            where words.firstMatch(in: line, range: NSRange(line.startIndex..., in: line)) != nil {
                offenders.append("\(name):\(index + 1): \(line.trimmingCharacters(in: .whitespaces))")
            }
        }
        XCTAssertTrue(offenders.isEmpty, "FNV-1a is not a one-way function:\n" + offenders.joined(separator: "\n"))
    }

    // MARK: - The formerly dead preference

    func testDiagnosticsPreferenceIsRealNow() {
        let previous = Log.diagnosticsEnabled
        defer { Log.diagnosticsEnabled = previous }
        XCTAssertFalse(prefs.diagnosticsEnabled, "off by default")
        prefs.diagnosticsEnabled = true
        XCTAssertTrue(prefs.diagnosticsEnabled)
        XCTAssertEqual(stored("diagnosticsEnabled") as? Bool, true)
        XCTAssertTrue(Log.diagnosticsEnabled, "and it switches the log")
    }
}

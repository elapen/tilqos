import XCTest

@testable import TilqosKit

final class TransliteratorTests: XCTestCase {
    private static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: TrigramModelTests.modelDirectory)
        return store
    }()

    private var translit: Transliterator!

    override func setUpWithError() throws {
        guard Self.store.model(for: .kk) != nil else {
            throw XCTSkip("Kazakh model missing — run ./generate-models.sh")
        }
        translit = Transliterator(models: Self.store)
    }

    /// The user's own sample, paired with the Cyrillic it stands for. This is the reference
    /// the practical decoder is judged against.
    private static let sample: [(latin: String, cyrillic: String)] = [
        ("qalaisyz", "қалайсыз"),
        ("qai", "қай"),
        ("jylgysyz", "жылғысыз"),
        ("kim", "кім"),
        ("qalai", "қалай"),
        ("dese", "десе"),
        ("magan", "маған"),
        ("baribir", "бәрібір"),
        ("eken", "екен"),
        ("siz", "сіз"),
        ("bilmeisiz", "білмейсіз"),
        ("goi", "ғой"),
        ("menin", "менің"),
        ("ishimde", "ішімде"),
        ("bolyp", "болып"),
        ("jatqanyn", "жатқанын"),
        ("aitqan", "айтқан"),
        ("edi", "еді"),
        ("koz", "көз"),
        ("kore", "көре"),
        ("uirenedi", "үйренеді"),
        ("dep", "деп"),
        ("alde", "әлде"),
        ("olai", "олай"),
        ("aitpady", "айтпады"),
        ("ony", "оны"),
        ("bilmeidi", "білмейді"),
        ("ekenmin", "екенмін"),
        ("jylqy", "жылқы"),
        ("qysta", "қыста"),
        ("jazda", "жазда"),
        ("qoigan", "қойған"),
        ("apai", "апай"),
        ("sanyrau", "саңырау"),
        ("qalgan", "қалған"),
        ("qaharman", "қаһарман"),
        ("ulken", "үлкен"),
        ("tulga", "тұлға"),
        ("bolamyn", "боламын"),
        ("desenizder", "десеңіздер"),
        ("kop", "көп"),
        ("enbek", "еңбек"),
        ("etu", "ету"),
        ("kerek", "керек"),
    ]

    // MARK: - Cyrillic to practical Latin

    /// This direction is a plain substitution, so it must reproduce the user's spelling exactly.
    func testCyrillicToPracticalMatchesTheUsersSpelling() {
        var failures: [String] = []
        for entry in Self.sample {
            let produced = translit.toLatin(entry.cyrillic, scheme: .practical)
            if produced != entry.latin {
                failures.append("\(entry.cyrillic): expected \(entry.latin), got \(produced)")
            }
        }
        XCTAssertTrue(failures.isEmpty, "\n" + failures.joined(separator: "\n"))
    }

    func testCyrillicToPracticalPreservesCase() {
        XCTAssertEqual(translit.toLatin("Ұрғашы", scheme: .practical), "Urgashy")
        XCTAssertEqual(translit.toLatin("Шерхан", scheme: .practical), "Sherhan")
        XCTAssertEqual(translit.toLatin("ҚАЗАҚСТАН", scheme: .practical), "QAZAQSTAN")
    }

    // MARK: - Official 2021

    func testOfficialAlphabetRoundTrips() {
        // Every letter of the 31-letter alphabet, in order.
        let cyrillic = "аәбвгғдежзиклқлмнңоөпрстуұүфхшыі"
        let latin = translit.toLatin(cyrillic, scheme: .official2021)
        XCTAssertEqual(translit.toCyrillic(latin, scheme: .official2021), cyrillic)
    }

    func testOfficialUsesTheDecreedLetters() {
        XCTAssertEqual(translit.toLatin("әғңөұүш", scheme: .official2021), "äğñöūüş")
        XCTAssertEqual(translit.toLatin("жақсы", scheme: .official2021), "jaqsy")
        XCTAssertEqual(translit.toLatin("сәлеметсіз", scheme: .official2021), "sälemetsız")
    }

    /// The standard writes both и and й as `i`, so reading it back depends on position.
    func testOfficialResolvesIVersusYod() {
        XCTAssertEqual(translit.toCyrillic("qalaisyz", scheme: .official2021), "қалайсыз")
        XCTAssertEqual(translit.toCyrillic("kino", scheme: .official2021), "кино")
    }

    // MARK: - Practical Latin to Cyrillic

    /// The hard direction. Reported as a rate rather than all-or-nothing, because `u` alone has
    /// three readings and no decoder gets every word.
    func testPracticalDecodingAccuracy() {
        var correct = 0
        var failures: [String] = []
        for entry in Self.sample {
            let produced = translit.toCyrillic(entry.latin, scheme: .practical)
            if produced == entry.cyrillic {
                correct += 1
            } else {
                failures.append("\(entry.latin): expected \(entry.cyrillic), got \(produced)")
            }
        }

        let rate = Double(correct) / Double(Self.sample.count)
        print(
            String(
                format: "practical Latin → Cyrillic: %d/%d words (%.0f%%)",
                correct, Self.sample.count, rate * 100))
        if !failures.isEmpty { print("misses:\n  " + failures.joined(separator: "\n  ")) }

        XCTAssertGreaterThan(rate, 0.75, "decoding regressed below 75% of the sample")
    }

    /// Vowel harmony is what makes the ambiguous vowels recoverable at all, so check it
    /// directly: the same `u` has to come out differently in a front word and a back one.
    func testVowelHarmonyDisambiguatesU() {
        XCTAssertEqual(translit.toCyrillic("ulken", scheme: .practical), "үлкен")
        XCTAssertEqual(translit.toCyrillic("tulga", scheme: .practical), "тұлға")
    }

    func testVowelHarmonyDisambiguatesO() {
        XCTAssertEqual(translit.toCyrillic("koz", scheme: .practical), "көз")
        XCTAssertEqual(translit.toCyrillic("bolyp", scheme: .practical), "болып")
    }

    /// қ and ғ only occur in back-vowel words, к and г only in front-vowel ones, so `g` is
    /// decided by the company it keeps.
    func testConsonantHarmonyDisambiguatesG() {
        XCTAssertEqual(translit.toCyrillic("magan", scheme: .practical), "маған")
        XCTAssertEqual(translit.toCyrillic("tulga", scheme: .practical), "тұлға")
    }

    func testHehExceptionList() {
        XCTAssertEqual(translit.toCyrillic("qaharman", scheme: .practical), "қаһарман")
        // Everything else keeps х.
        XCTAssertEqual(translit.toCyrillic("Sherhan", scheme: .practical), "Шерхан")
    }

    // MARK: - Structure

    func testPunctuationAndSpacingSurvive() {
        let input = "assalaumagaleikum aga, qalaisyz?"
        let output = translit.toCyrillic(input, scheme: .practical)
        XCTAssertTrue(output.hasSuffix("?"))
        XCTAssertEqual(output.filter { $0 == " " }.count, 2)
        XCTAssertTrue(output.contains(","))
    }

    func testNonKazakhTextIsLeftAlone() {
        // Digits and symbols must survive untouched.
        let output = translit.toCyrillic("qalaisyz 25 + 3 = 28", scheme: .practical)
        XCTAssertTrue(output.contains("25"))
        XCTAssertTrue(output.contains("+"))
        XCTAssertTrue(output.contains("28"))
    }

    func testDirectionDetection() {
        XCTAssertEqual(Transliterator.direction(for: "qalaisyz aga"), .toCyrillic)
        XCTAssertEqual(Transliterator.direction(for: "қалайсыз аға"), .toLatin)
    }

    func testEmptyAndSymbolOnlyInput() {
        XCTAssertEqual(translit.toCyrillic("", scheme: .practical), "")
        XCTAssertEqual(translit.toLatin("", scheme: .practical), "")
        XCTAssertEqual(translit.toCyrillic("!!! ???", scheme: .practical), "!!! ???")
    }

    /// The whole passage the user supplied, end to end.
    func testFullSamplePassage() {
        let passage = """
            assalaumagaleikum aga qalaisyz?
            qai jylgysyz?
            kim qalai dese de magan baribir eken
            siz bilmeisiz goi
            menin ishimde ne bolyp jatqanyn
            Sherhan aga aitqan edi
            koz kore kore uirenedi dep
            """
        let output = translit.toCyrillic(passage, scheme: .practical)
        print("=== full passage ===\n" + output)

        // Line structure preserved.
        XCTAssertEqual(output.split(separator: "\n").count, 7)
        // Names keep their capital.
        XCTAssertTrue(output.contains("Шерхан"), "got: \(output)")
        // A few anchors that must be right.
        for expected in ["қалайсыз", "менің", "ғой", "үйренеді", "көз"] {
            XCTAssertTrue(output.contains(expected), "missing \(expected) in: \(output)")
        }
    }
}

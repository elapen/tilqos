import XCTest

@testable import TilqosKit

/// Measures how the two models score labelled words, so the language bias is set from data.
///
/// The two trigram models are not calibrated against each other: Kazakh is trained on 54,063
/// stems and Russian on 146,269 words, so their absolute costs are not directly comparable and
/// a raw comparison leans one way for every word.
final class TranslitCalibrationTests: XCTestCase {
    private static let store: ModelStore = {
        let store = ModelStore()
        store.load(from: TrigramModelTests.modelDirectory)
        return store
    }()

    private var translit: Transliterator!

    override func setUpWithError() throws {
        guard Self.store.model(for: .kk) != nil, Self.store.model(for: .ru) != nil else {
            throw XCTSkip("Models missing")
        }
        translit = Transliterator(models: Self.store)
    }

    private static let russian = [
        "zametki", "spisok", "pokupki", "nastroiki", "raspisanie", "zagruzka", "ssylka",
        "papka", "kofe", "poisk", "pochta", "vkladka", "uvedomlenie", "podpiska",
        "izbrannoe", "obnovlenie", "shablon", "okno", "rabote", "privet",
    ]
    private static let kazakh = [
        "qalaisyz", "kerek", "jaqsy", "ademi", "jasalgan", "ulken", "tulga", "koz",
        "bugin", "keshke", "jospar", "arqyly", "qosu", "aldymen", "menin", "ishimde",
        "bilmeisiz", "uirenedi", "sanyrau", "qalgan",
    ]
    private static let foreign = ["zzxq", "xkcd", "instagram", "whatsapp", "telegram"]

    /// Prints the margin between the Kazakh and Russian readings for each labelled group. A
    /// positive margin means the correct language won.
    func testPrintCalibration() {
        func report(_ label: String, _ words: [String], expect: Lang?) {
            var margins: [Double] = []
            var lines: [String] = []
            for word in words {
                let readings = translit.readings(of: word)
                guard let kk = readings.first(where: { $0.lang == .kk }),
                    let ru = readings.first(where: { $0.lang == .ru })
                else { continue }
                // Positive = Russian is cheaper, i.e. leans Russian.
                let margin = kk.cost - ru.cost
                margins.append(margin)
                lines.append(
                    String(
                        format: "  %-12@ kk %.2f (%@)  ru %.2f (%@)  margin %+.2f",
                        word as NSString, kk.cost, kk.text as NSString, ru.cost,
                        ru.text as NSString, margin))
            }
            let sorted = margins.sorted()
            print(
                String(
                    format: "%@ (expect %@): margin min %+.2f  median %+.2f  max %+.2f",
                    label, expect?.rawValue ?? "neither",
                    sorted.first ?? 0, sorted[sorted.count / 2], sorted.last ?? 0))
            print(lines.joined(separator: "\n"))
        }

        report("RUSSIAN", Self.russian, expect: .ru)
        report("KAZAKH", Self.kazakh, expect: .kk)

        print("FOREIGN (should be left in Latin) — cheapest reading cost:")
        for word in Self.foreign {
            let best = translit.readings(of: word).min { $0.cost < $1.cost }
            print(
                String(
                    format: "  %-12@ %.2f (%@)", word as NSString, best?.cost ?? 0,
                    (best?.text ?? "-") as NSString))
        }
        print("KAZAKH cheapest reading cost (must stay under the leave-alone bar):")
        for word in Self.kazakh.prefix(8) {
            let best = translit.readings(of: word).min { $0.cost < $1.cost }
            print(String(format: "  %-12@ %.2f", word as NSString, best?.cost ?? 0))
        }
    }
}

extension TranslitCalibrationTests {
    /// Can the English model act as a third competitor, to tell brand names from real words?
    ///
    /// The obstacle is that the Latin alphabet has 28 symbols against Cyrillic's 43, so costs
    /// are not comparable without an offset. This measures whether an offset exists at all.
    func testPrintEnglishModelSeparation() throws {
        guard let english = Self.store.model(for: .en) else { throw XCTSkip("no en model") }

        func report(_ label: String, _ words: [String]) {
            var costs: [Double] = []
            var lines: [String] = []
            for word in words {
                guard let score = english.score(word) else { continue }
                costs.append(score.penalizedCost)
                // Cheapest Cyrillic reading, for comparison.
                let cyrillic = translit.readings(of: word).min { $0.cost < $1.cost }
                lines.append(
                    String(
                        format: "  %-12@ en %.2f   best-cyrillic %.2f   diff %+.2f",
                        word as NSString, score.penalizedCost, cyrillic?.cost ?? 0,
                        score.penalizedCost - (cyrillic?.cost ?? 0)))
            }
            let sorted = costs.sorted()
            print(
                String(
                    format: "%@: en cost min %.2f median %.2f max %.2f",
                    label, sorted.first ?? 0, sorted[sorted.count / 2], sorted.last ?? 0))
            print(lines.joined(separator: "\n"))
        }

        report("FOREIGN/BRAND", ["zzxq", "xkcd", "instagram", "whatsapp", "telegram", "google"])
        report("RUSSIAN-IN-LATIN", ["zametki", "spisok", "pokupki", "papka", "poisk"])
        report("KAZAKH-IN-LATIN", ["qalaisyz", "kerek", "jasalgan", "ademi", "arqyly"])
    }
}

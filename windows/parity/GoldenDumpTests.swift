// Golden-data dump for the Go port in windows/.
//
// This file is NOT part of the Swift test suite. windows/parity/regen-golden.sh copies it
// into Tests/TilqosKitTests/ for one run, executes it, and deletes it again, so the
// Swift tree stays untouched. The JSON it writes (windows/testdata/golden/*.json) is what
// the Go parity tests compare against.
//
// Everything here goes through the real Swift implementation: the real layouts macOS
// ships (ABC, Russian, Kazakh), the real models and lexicons in Resources/Models, the real
// detector, TypingEngine and the CorpusTests harness (HarnessEngine, Corpus).
//
// Written against the detection rewrite (lexicons, TypingEngine, Learner, TextReplacer);
// learning.json needs the stricter vocabulary rule of the Learner.

import Foundation
import XCTest

@testable import TilqosKit

final class GoldenDumpTests: XCTestCase {
    // MARK: - Locations

    static let packageRoot = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()  // TilqosKitTests
        .deletingLastPathComponent()  // Tests
        .deletingLastPathComponent()  // package root

    static var outputDirectory: URL {
        if let custom = ProcessInfo.processInfo.environment["GOLDEN_OUT"], !custom.isEmpty {
            return URL(fileURLWithPath: custom)
        }
        return packageRoot.appendingPathComponent("windows/testdata/golden")
    }

    // MARK: - Encoding helpers

    /// Doubles travel as Swift's shortest round-trip description ("3.5", "inf", "nan"), so the
    /// Go side can parse them back bit-exactly and infinities survive JSON.
    static func d(_ value: Double) -> String { "\(value)" }

    static func compact<T: Encodable>(_ value: T) throws -> String {
        let encoder = JSONEncoder()
        encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
        return String(decoding: try encoder.encode(value), as: UTF8.self)
    }

    /// One record per line: compact, deterministic and still diffable.
    static func writeLines<T: Encodable>(_ records: [T], header: [String: String], to name: String)
        throws
    {
        var text = "{\n"
        for key in header.keys.sorted() {
            text += "\"\(key)\": \(try compact(header[key]!)),\n"
        }
        text += "\"records\": [\n"
        text += try records.map { try compact($0) }.joined(separator: ",\n")
        text += "\n]\n}\n"
        let url = outputDirectory.appendingPathComponent(name)
        try FileManager.default.createDirectory(
            at: outputDirectory, withIntermediateDirectories: true)
        try Data(text.utf8).write(to: url)
        print("golden: wrote \(records.count) records to \(url.path)")
    }

    /// Deterministic generator so the dump is reproducible run to run.
    struct LCG {
        var state: UInt64
        mutating func next() -> UInt64 {
            state = state &* 6_364_136_223_846_793_005 &+ 1_442_695_040_888_963_407
            return state >> 33
        }
        mutating func below(_ bound: Int) -> Int { Int(next() % UInt64(bound)) }
    }

    // MARK: - Fixtures

    static let abcID = "com.apple.keylayout.ABC"
    static let russianID = "com.apple.keylayout.Russian"
    static let kazakhID = "com.apple.keylayout.Kazakh"

    var manager: InputSourceManager!
    var store: ModelStore!
    var abc: KeyMapTable!
    var russian: KeyMapTable!
    var kazakh: KeyMapTable!

    func table(_ short: String) -> KeyMapTable {
        switch short {
        case "abc": return abc
        case "ru": return russian
        default: return kazakh
        }
    }

    override func setUpWithError() throws {
        manager = InputSourceManager()
        manager.reloadTables()
        store = ModelStore()
        store.load(from: Self.packageRoot.appendingPathComponent("Resources/Models"))
        guard let abc = manager.table(forSourceID: Self.abcID),
            let russian = manager.table(forSourceID: Self.russianID),
            let kazakh = manager.table(forSourceID: Self.kazakhID)
        else {
            XCTFail("ABC, Russian and Kazakh layouts must be enabled to dump golden data")
            throw XCTSkip("layouts missing")
        }
        guard store.model(for: .en) != nil, store.model(for: .ru) != nil,
            store.model(for: .kk) != nil, store.kazakhMorphology() != nil,
            store.lexicon(for: .en) != nil, store.lexicon(for: .ru) != nil,
            store.lexicon(for: .kk) != nil
        else {
            XCTFail("models missing")
            throw XCTSkip("models missing")
        }
        self.abc = abc
        self.russian = russian
        self.kazakh = kazakh
    }

    // MARK: - Corpora

    static let russianWords: [String] = """
        привет спасибо клавиатура переключатель который сегодня мир дела как работа работаю над \
        и я ты он она мы вы они это что где когда почему потому если или но да нет хорошо плохо \
        большой маленький новый старый человек время год день ночь утро вечер жизнь рука глаз \
        голова дом город страна земля вода огонь воздух солнце небо море река лес поле дорога \
        машина телефон компьютер программа система проблема вопрос ответ слово язык текст книга \
        письмо сообщение раскладка исправление пользователь приложение настройки окно кнопка меню \
        файл папка документ сеть интернет сайт страница ссылка почта адрес имя фамилия пароль \
        логин деньги цена магазин покупка заказ доставка оплата работать говорить думать знать \
        видеть слышать писать читать играть жить любить хотеть мочь делать сказать взять дать \
        идти ехать бежать стоять сидеть лежать спать есть пить ещё еще уже тоже также очень \
        всегда никогда иногда часто редко здесь там туда сюда вчера завтра сейчас потом раньше \
        позже быстро медленно красивый умный добрый злой весёлый грустный счастливый объявление \
        подъезд съёмка вьюга ёжик щука чаща шёпот жёлтый эхо юла яблоко Москва Россия Алматы \
        Астана Казахстан Иван Мария Анна Сергей ООО ГОСТ СССР РФ МГУ работаем работали говорила \
        делаешь написали прочитал сделайте пожалуйста извините здравствуйте свидания увидимся \
        конечно наверное возможно действительно обязательно например кстати вообще короче значит \
        въезд разъём отъезд обьявление щёлкнуть экран ключ шифр чемпион фильм музыка кофе чай \
        молоко хлеб сыр мясо рыба овощи фрукты Привет Спасибо ПРИВЕТ Ёлка ёлка
        """.split(separator: " ").map(String.init)

    static let kazakhWords: [String] = """
        сәлем сәлеметсіз рақмет қазақ қазақстан мүмкіндік әңгіме кітап мектеп бала балалар ана \
        әке үй су нан ет тіл ел жер көк ақ қара қызыл сары жасыл бір екі үш төрт бес алты жеті \
        сегіз тоғыз он жүз мың бүгін ертең кеше қазір жақсы жаман үлкен кіші әдемі қала ауыл \
        дала тау өзен көл теңіз күн ай жұлдыз жұмыс оқу жазу айту келу кету бару отыру тұру \
        жүру көру білу беру алу сөз сұрақ жауап дос достар менің сенің оның біздің сіздің \
        олардың қалайсыз қалай жылы суық ыстық түн таң кеш апта уақыт адам халық мемлекет \
        үкімет заң құқық білім ғылым тарих мәдениет өнер әдебиет ән күй домбыра шай қымыз \
        бауырсақ қой жылқы түйе сиыр ит мысық құс балық ағаш гүл шөп жаңбыр қар жел бұлт \
        көктем жаз күз қыс дүйсенбі сейсенбі сәрсенбі бейсенбі жұма сенбі жексенбі қаңтар \
        ақпан наурыз сәуір мамыр маусым шілде тамыз қыркүйек қазан қараша желтоқсан керек \
        болады болмайды келеді барамын айтты көрдім білемін білмеймін түсіндім түсінбедім \
        кешіріңіз рахмет иә жоқ бар мен сен ол біз сіз олар бұл сол осы анау мынау неге \
        қашан қайда кім не қандай қанша неше өйткені сондықтан бірақ және немесе егер әрине \
        мүмкін шығар екен еді дейді деп десеңіздер жатқанын айтқан үйренеді білмейсіз ішімде \
        болып қыста жазда қойған апай саңырау қалған қаһарман тұлға боламын еңбек ету Алматы \
        Астана Қарағанды Шымкент Ақтөбе Абай Әлия Айгүл Ержан ҚР ҚАЗАҚСТАН Қазақстан
        """.split(separator: " ").map(String.init)

    static let englishCommon: [String] = """
        hello world keyboard language the and you that was for are with his they this have \
        from one had word but not what all were when your can said there use each which she \
        how their will other about out many then them these some her would make like him into \
        time has look two more write see number way could people than first water been call \
        who oil its now find long down day did get come made may part don't can't it's I'm \
        you're rock'n'roll iPhone macOS GitHub email Windows Microsoft OK USA NASA HTTP API JSON \
        switch please layout correction settings window button menu file folder document \
        network internet site page link mail address name password login money price shop \
        order delivery payment work speak think know read play live love want
        """.split(separator: " ").map(String.init)

    static func englishFromDictionary() -> [String] {
        guard let raw = try? String(contentsOfFile: "/usr/share/dict/words", encoding: .utf8)
        else { return [] }
        let all = raw.split(separator: "\n").map(String.init)
        var picked: [String] = []
        var index = 17
        while index < all.count {
            picked.append(all[index])
            index += 997
        }
        return picked
    }

    static let identifiers: [String] = [
        "user_name", "getUserName", "snake_case", "camelCase", "HTTPServer", "iPhone15",
        "COVID19", "x86", "a1b2", "25", "2023", "007", "с2леметс3з", "0аза0", "әғ", "ӘҒ",
        "don't", "don’t", "O'Neil", "naïve", "café", "résumé", "e\u{301}cole", "Е\u{308}лка",
        "и\u{306}од", "İstanbul", "straße", "STRAẞE", "ǅemal", "ﬁle", "Ⅻ", "½", "٣٤٥", "ⓐⓑ",
        "ab", "a", "я", "ъь", "''", "'a'", "a'", "'a", "ё", "Ё", "йод", "ЙОД", "Щи",
        "qwerty", "йцукен", "asdf", "фыва", "zxcv", "ячсм", "qazaq", "сәлем2", "2сәлем",
        "hello123", "123hello", "при3вет", "ab'cd", "әңгіме", "ӘҢГІМЕ", "Әңгіме",
    ]

    static func randomWords() -> [String] {
        var rng = LCG(state: 0x5EED_0001)
        let latin = Array("abcdefghijklmnopqrstuvwxyz'")
        let cyrillic = Array(
            "абвгдеёжзийклмнопрстуфхцчшщъыьэюя\u{04D9}\u{0493}\u{049B}\u{04A3}\u{04E9}\u{04B1}\u{04AF}\u{04BB}\u{0456}"
        )
        let digits = Array("0123456789")
        var words: [String] = []
        for index in 0..<320 {
            let length = 1 + rng.below(12)
            var word = ""
            for _ in 0..<length {
                let roll = rng.below(100)
                let pool: [Character]
                switch index % 4 {
                case 0: pool = latin
                case 1: pool = cyrillic
                case 2: pool = roll < 80 ? cyrillic : digits
                default: pool = roll < 45 ? latin : (roll < 90 ? cyrillic : digits)
                }
                var character = pool[rng.below(pool.count)]
                if rng.below(10) == 0 { character = Character(character.uppercased()) }
                word.append(character)
            }
            words.append(word)
        }
        return words
    }

    /// Words as they come out when typed on the wrong layout.
    func garbled() -> [String] {
        var result: [String] = []
        for word in Self.russianWords {
            if let strokes = russian.strokes(for: word) {
                result.append(abc.render(strokes))
                result.append(kazakh.render(strokes))
            }
        }
        for word in Self.kazakhWords {
            if let strokes = kazakh.strokes(for: word) {
                result.append(russian.render(strokes))
                result.append(abc.render(strokes))
            }
        }
        for word in Self.englishCommon {
            if let strokes = abc.strokes(for: word) {
                result.append(russian.render(strokes))
                result.append(kazakh.render(strokes))
            }
        }
        return result
    }

    func allWords() -> [String] {
        var seen = Set<String>()
        var result: [String] = []
        for word in Self.russianWords + Self.kazakhWords + Self.englishCommon
            + Self.englishFromDictionary() + Self.identifiers + Self.randomWords() + garbled()
        where seen.insert(word).inserted {
            result.append(word)
        }
        return result
    }

    // MARK: - The dump

    func testDumpGolden() throws {
        try dumpTables()
        try dumpUnicode()
        try dumpWords()
        try dumpConfidence()
        try dumpGuards()
        try dumpLexicon()
        try dumpReadings()
        try dumpDetector()
        try dumpCorpus()
        try dumpLearner()
        try dumpLearning()
        try dumpChunks()
        try dumpTextConverter()
        try dumpTranslit()
    }

    // MARK: Tables

    struct LayoutDump: Encodable {
        let short: String
        let id: String
        let name: String
        let lang: String?
        let forward: [String?]
        /// `producesWordCharacter` for every forward entry, as a string of 0 and 1.
        let word: String
        let reverse: [String: [Int]]
    }

    func dumpTables() throws {
        var layouts: [LayoutDump] = []
        for (short, table) in [("abc", abc!), ("ru", russian!), ("kk", kazakh!)] {
            var forward: [String?] = []
            var word = ""
            var characters = Set<Character>()
            for keyCode in 0..<KeyMapTable.keyCodeCount {
                for mods in 0..<ModState.variantCount {
                    let stroke = KeyStroke(keyCode: UInt16(keyCode), mods: ModState(rawValue: UInt8(mods)))
                    let text = table.text(for: stroke)
                    forward.append(text)
                    word += table.producesWordCharacter(stroke) ? "1" : "0"
                    if let text, text.count == 1 { characters.insert(text.first!) }
                }
            }
            var reverse: [String: [Int]] = [:]
            for character in characters {
                if let stroke = table.stroke(for: character) {
                    reverse[String(character)] = [Int(stroke.keyCode), Int(stroke.mods.rawValue)]
                }
            }
            layouts.append(
                LayoutDump(
                    short: short, id: table.sourceID, name: table.localizedName,
                    lang: table.lang?.rawValue, forward: forward, word: word, reverse: reverse))
        }
        try Self.writeLines(
            layouts,
            header: [
                "platform": "macOS",
                "keyCodeCount": "\(KeyMapTable.keyCodeCount)",
                "keypad": KeyMapTable.keypadKeyCodes.sorted().map(String.init).joined(separator: ","),
            ],
            to: "tables.json")
    }

    // MARK: Unicode behaviour the port has to reproduce

    struct CharDump: Encodable {
        let c: String
        let isLetter: Bool
        let isNumber: Bool
        let isUppercase: Bool
        let isWord: Bool
        let isASCII: Bool
        let isWhitespace: Bool
        let isNewline: Bool
    }

    struct StringDump: Encodable {
        let s: String
        let count: Int
        let lower: String
        let upper: String
        let chars: [CharDump]
    }

    func dumpUnicode() throws {
        let samples: [String] =
            [
                "Привет", "ПРИВЕТ", "привет", "Ёлка", "ёлка", "Е\u{308}лка", "и\u{306}од", "ÉCOLE",
                "école", "e\u{301}cole", "İstanbul", "ıi", "straße", "STRASSE", "ǅemal", "ﬁle", "ΐ",
                "ς", "Σ", "Ⅻ", "ⅻ", "½", "٣٤٥", "①", "𝟠", "25", "2023", "iPhone15", "с2леметс3з",
                "0аза0", "әғ", "ӘҒ", "Қазақстан", "ҚАЗАҚСТАН", "don't", "don’t", "_", "user_name",
                "👍🏽", "🇰🇿🇷🇺", "👨‍👩‍👧", "a\u{200D}b", "\r\n", "tab\tx", "ⓐⒶ", "ª", "ʰ", "Ǆ",
                "ǆ", "ŉ", "ǰ", "ﬀ", "ﬃ", "և", "ẞ", "µ", "ÿ", "Ÿ", "ı", "ſ", "K", "Ω", "ﬆ", "ǈ",
                "a\u{301}\u{302}", "\u{301}a", "한국어", "각", "ᄀ\u{1161}\u{11A8}", "e\u{20DD}",
                "x\u{FE0F}", "☺\u{FE0F}", "#\u{FE0F}\u{20E3}", " ", "\u{00A0}", "\u{2003}", "-",
                "—", "…", "«»", "№", "$", "@", "%", "&", "~", "`", "\\", "/", "(", ")",
                // Whitespace and line breaks, as the transliteration of typed text walks them.
                "a\nb\rc\r\nd", "\u{0B}\u{0C}\u{85}", "x\u{2028}y\u{2029}z", "\u{1680}\u{2000}\u{200A}",
                "\u{202F}\u{205F}\u{3000}", "\u{200B}\u{FEFF}\u{180E}", " \u{301}x", "\n\u{301}",
                "qalaisyz aga", "қалайсыз\tаға",
            ] + Self.identifiers
        var records: [StringDump] = []
        for sample in samples {
            records.append(
                StringDump(
                    s: sample, count: sample.count, lower: sample.lowercased(),
                    upper: sample.uppercased(),
                    chars: sample.map {
                        CharDump(
                            c: String($0), isLetter: $0.isLetter, isNumber: $0.isNumber,
                            isUppercase: $0.isUppercase, isWord: Tokenizer.isWordCharacter($0),
                            isASCII: $0.isASCII, isWhitespace: $0.isWhitespace,
                            isNewline: $0.isNewline)
                    }))
        }
        try Self.writeLines(records, header: [:], to: "unicode.json")
    }

    // MARK: Per-word model outputs

    struct ScoreDump: Encodable {
        let mean: String
        let interior: Int
        let unknown: Int
        let length: Int
        let penalized: String
    }

    struct EncodeDump: Encodable {
        let symbols: [Int]
        let unknown: Int
        let covers: Bool
    }

    struct DecompDump: Encodable {
        let stem: String
        let suffixes: [String]
    }

    struct WordDump: Encodable {
        let w: String
        let latin: EncodeDump
        let cyrillic: EncodeDump
        let en: ScoreDump?
        let ru: ScoreDump?
        let kk: ScoreDump?
        let hash: UInt32
        let isStem: Bool
        let decomp: DecompDump?
        let decomp1: Bool
        let containsKK: Bool
        let numeric: Bool
    }

    func score(_ lang: Lang, _ word: String) -> ScoreDump? {
        guard let score = store.model(for: lang)?.score(word) else { return nil }
        return ScoreDump(
            mean: Self.d(score.meanCost), interior: score.interiorUnknownCount,
            unknown: score.unknownCount, length: score.length, penalized: Self.d(score.penalizedCost))
    }

    func dumpWords() throws {
        let morphology = store.kazakhMorphology()!
        var records: [WordDump] = []
        for word in allWords() {
            let latin = Alphabet.latin.encode(word)
            let cyrillic = Alphabet.cyrillic.encode(word)
            let decomposition = morphology.decomposition(of: word)
            records.append(
                WordDump(
                    w: word,
                    latin: EncodeDump(
                        symbols: latin.symbols, unknown: latin.unknown,
                        covers: Alphabet.latin.covers(word)),
                    cyrillic: EncodeDump(
                        symbols: cyrillic.symbols, unknown: cyrillic.unknown,
                        covers: Alphabet.cyrillic.covers(word)),
                    en: score(.en, word), ru: score(.ru, word), kk: score(.kk, word),
                    hash: Morphology.hash(word), isStem: morphology.isStem(word),
                    decomp: decomposition.map { DecompDump(stem: $0.stem, suffixes: $0.suffixes) },
                    decomp1: morphology.decomposes(word, depth: 1),
                    containsKK: Alphabet.containsKazakhSpecific(word),
                    numeric: WordScorer.isNumeric(word)))
        }
        try Self.writeLines(
            records,
            header: [
                "stemCount": "\(morphology.stemCount)", "suffixCount": "\(morphology.suffixCount)",
            ], to: "words.json")
    }

    // MARK: Confidence

    struct ConfidenceDump: Encodable {
        let gap: String
        let confidence: String
    }

    func dumpConfidence() throws {
        var gaps: [Double] = [.infinity, -.infinity, .nan, 0, 0.4, 2.6, 100, -0.0]
        var gap = -3.0
        while gap < 6.0 {
            gaps.append(gap)
            gap += 0.037
        }
        try Self.writeLines(
            gaps.map {
                ConfidenceDump(gap: Self.d($0), confidence: Self.d(Detector.confidence(gap: $0)))
            }, header: [:], to: "confidence.json")
    }

    // MARK: Typing sessions (shared by guards and detector)

    struct Seg: Encodable {
        let text: String
        let on: String
        let active: String
    }

    /// Replays the DetectorTests session helper: the keys someone would press to get `text`
    /// on `on`, recorded as if `active` had been selected. Nil when `on` cannot type it.
    func record(_ segments: [Seg]) -> [RecordedStroke]? {
        var recorded: [RecordedStroke] = []
        var clock: TimeInterval = 0
        for segment in segments {
            guard let strokes = table(segment.on).strokes(for: segment.text) else { return nil }
            for stroke in strokes {
                clock += 0.05
                recorded.append(
                    RecordedStroke(stroke: stroke, time: clock, layoutID: table(segment.active).sourceID))
            }
        }
        return recorded
    }

    func resolve(_ id: String) -> KeyMapTable? { manager.table(forSourceID: id) }

    func sentences() -> [[Seg]] {
        var result: [[Seg]] = []
        func s(_ items: [(String, String, String)]) {
            result.append(items.map { Seg(text: $0.0, on: $0.1, active: $0.2) })
        }
        // From the Swift DetectorTests.
        s([("привет", "ru", "abc"), (" ", "abc", "abc")])
        s([("keyboard", "abc", "ru"), (" ", "ru", "ru")])
        s([("сәлеметсіз", "kk", "ru"), (" ", "ru", "ru")])
        s([("қазақ", "kk", "ru"), (" ", "ru", "ru")])
        s([("мне ", "ru", "ru"), ("25", "abc", "kk"), (" лет ", "ru", "ru")])
        s([("Астана ", "ru", "kk")])
        s([("привет мир ", "ru", "ru")])
        s([("hello world ", "abc", "abc")])
        s([("сәлеметсіз бе ", "kk", "kk")])
        s([("ok ", "abc", "abc")])
        s([("да ", "ru", "ru")])
        s([("ия ", "ru", "ru")])
        s([("user@example.com ", "abc", "abc")])
        s([("https://github.com/taem/hunspell-kk ", "abc", "abc")])
        s([("let user_name = 1 ", "abc", "abc")])
        s([("getUserName() ", "abc", "abc")])
        s([("$PATH ", "abc", "abc")])
        s([("менің ", "kk", "kk"), ("iPhone", "abc", "abc"), (" телефоным ", "kk", "kk")])
        s([("привет как дела", "ru", "abc"), (" iPhone ", "abc", "abc")])
        s([("hello ", "abc", "abc"), ("привет", "ru", "abc"), (" ", "abc", "abc")])
        s([("клавиатура", "ru", "abc"), (" ", "abc", "abc")])
        s([("привет", "ru", "abc")])
        s([("привет как дела я сегодня работаю над клавиатурой и переключателем", "ru", "abc")])
        // Hand-written mixed cases.
        s([("сәлем ", "kk", "kk"), ("достар", "kk", "ru"), (", ", "ru", "ru")])
        s([("ghbdtn", "abc", "abc"), (". ", "abc", "abc")])
        s([("Привет", "ru", "abc"), ("! ", "abc", "abc")])
        s([("ПРИВЕТ ", "ru", "abc")])
        s([("НАСА ", "ru", "ru")])
        s([("NASA ", "abc", "abc")])
        s([("NASA", "abc", "ru"), (" ", "ru", "ru")])
        s([("ГОСТ", "ru", "abc"), (" ", "abc", "abc")])
        s([("don't", "abc", "ru"), (" ", "ru", "ru")])
        s([("I'm here", "abc", "ru"), (" ", "ru", "ru")])
        s([("это не так", "ru", "abc"), ("? ", "abc", "abc")])
        s([("мен ", "kk", "kk"), ("бүгін", "kk", "ru"), (" ", "ru", "ru"), ("жұмыс", "kk", "ru"), (" ", "ru", "ru")])
        s([("мой номер ", "ru", "ru"), ("2023", "abc", "kk"), (" ", "ru", "ru")])
        s([("в ", "ru", "ru"), ("1990", "abc", "kk"), (" году ", "ru", "ru")])
        s([("цена ", "ru", "ru"), ("5", "abc", "kk"), (" тысяч ", "ru", "ru")])
        s([("iPhone15 ", "abc", "abc")])
        s([("iPhone15", "abc", "ru"), (" ", "ru", "ru")])
        s([("www.example.com ", "abc", "abc")])
        s([("пишите на site.kz ", "ru", "ru")])
        s([("пишите на ", "ru", "ru"), ("site.kz", "abc", "abc"), (" ", "ru", "ru")])
        s([("C:\\Users\\user ", "abc", "abc")])
        s([("cd ~/Projects/tilqos ", "abc", "abc")])
        s([("./build.sh ", "abc", "abc")])
        s([("#hashtag ", "abc", "abc")])
        s([("@mention ", "abc", "abc")])
        s([("`code` ", "abc", "abc")])
        s([("print(value) ", "abc", "abc")])
        s([("snake_case_name ", "abc", "abc")])
        s([("x ", "abc", "abc")])
        s([("я ", "ru", "ru")])
        s([("z ", "ru", "abc")])
        s([("ә", "kk", "kk"), (" ", "kk", "kk")])
        s([("2", "abc", "kk"), (" ", "kk", "kk")])
        s([("мне ", "ru", "ru"), ("2", "abc", "kk"), (" года ", "ru", "ru")])
        s([("hello", "abc", "abc"), (",", "abc", "abc"), ("world", "abc", "ru"), (" ", "ru", "ru")])
        s([("сәлеметсіз", "kk", "abc"), (" ", "abc", "abc")])
        s([("рақмет", "kk", "ru"), (". ", "ru", "ru")])
        s([("Қазақстан", "kk", "ru"), (" ", "ru", "ru")])
        s([("қазақ тілі", "kk", "ru"), (" ", "ru", "ru")])
        s([("керек", "kk", "kk"), (" ", "kk", "kk")])
        s([("еще", "ru", "ru"), (" ", "ru", "ru")])
        s([("еще", "ru", "abc"), (" ", "abc", "abc")])
        s([("ещё", "ru", "abc"), (" ", "abc", "abc")])
        s([("the ", "abc", "abc"), ("ещё", "ru", "abc"), (" ", "abc", "abc")])
        s([("Hello, ", "abc", "abc"), ("мир", "ru", "abc"), ("!", "abc", "abc")])
        s([("t.me/durov ", "abc", "abc")])
        s([("мама мыла раму", "ru", "abc"), (".", "abc", "abc")])
        s([("мама мыла раму", "ru", "ru"), (".", "ru", "ru")])
        s([("мама", "ru", "abc"), (" мыла раму.", "ru", "ru")])
        return result
    }

    func generatedSentences() -> [[Seg]] {
        var rng = LCG(state: 0x5EED_0003)
        var result: [[Seg]] = []
        let pools: [(String, [String])] = [
            ("ru", Self.russianWords), ("kk", Self.kazakhWords), ("abc", Self.englishCommon),
        ]
        let separators = [" ", " ", " ", ", ", ". ", "! ", "? ", "; ", " - "]
        for _ in 0..<260 {
            var segments: [Seg] = []
            let count = 2 + rng.below(6)
            let (lang, words) = pools[rng.below(pools.count)]
            for _ in 0..<count {
                // Mostly one language per sentence, with the occasional foreign word.
                let (wordLang, pool) = rng.below(10) < 8 ? (lang, words) : pools[rng.below(pools.count)]
                let word = pool[rng.below(pool.count)]
                let wrong = rng.below(10) < 4
                var active = wordLang
                if wrong {
                    let others = ["abc", "ru", "kk"].filter { $0 != wordLang }
                    active = others[rng.below(others.count)]
                }
                segments.append(Seg(text: word, on: wordLang, active: active))
                let separator = separators[rng.below(separators.count)]
                // The separator is typed on whatever layout was active.
                segments.append(Seg(text: separator, on: active, active: active))
            }
            result.append(segments)
        }
        return result
    }

    func singleWordSessions() -> [[Seg]] {
        var result: [[Seg]] = []
        for word in Self.russianWords {
            result.append([Seg(text: word, on: "ru", active: "ru"), Seg(text: " ", on: "ru", active: "ru")])
            result.append([Seg(text: word, on: "ru", active: "abc"), Seg(text: " ", on: "abc", active: "abc")])
            result.append([Seg(text: word, on: "ru", active: "kk"), Seg(text: " ", on: "kk", active: "kk")])
        }
        for word in Self.kazakhWords {
            result.append([Seg(text: word, on: "kk", active: "kk"), Seg(text: " ", on: "kk", active: "kk")])
            result.append([Seg(text: word, on: "kk", active: "ru"), Seg(text: " ", on: "ru", active: "ru")])
            result.append([Seg(text: word, on: "kk", active: "abc"), Seg(text: " ", on: "abc", active: "abc")])
        }
        for word in Self.englishCommon {
            result.append([Seg(text: word, on: "abc", active: "abc"), Seg(text: " ", on: "abc", active: "abc")])
            result.append([Seg(text: word, on: "abc", active: "ru"), Seg(text: " ", on: "ru", active: "ru")])
            result.append([Seg(text: word, on: "abc", active: "kk"), Seg(text: " ", on: "kk", active: "kk")])
        }
        return result
    }

    // MARK: Guards

    struct TokenDump: Encodable {
        let isWord: Bool
        let lo: Int
        let hi: Int
        let displayed: String
    }

    struct WordReasonDump: Encodable {
        let token: Int
        let plain: String?
        let numeric: String?
        let min2: String?
        let excepted: String?
    }

    struct GuardDump: Encodable {
        let segs: [Seg]
        let tokens: [TokenDump]
        let protected: [String: String]
        let reasons: [WordReasonDump]
        let inProgress: Int?
        let lastCompleted: Int?
    }

    func dumpGuards() throws {
        var records: [GuardDump] = []
        var extra: [[Seg]] = []
        for text in [
            "a, b.", "don't", "hello wor", "hello ", "e-mail me at a.b@c.de now",
            "see http://x.y/z?q=1 and ftp://host ", "visit www.google.com, ok", "go to site.kz. ",
            "path /usr/local/bin and ~/x and ../up ", "c:\\windows\\system32 ", "a_b _c d_ e__f ",
            "camelCase CamelCase camel ABc aB ", "fn(x) f (x) ", "$HOME `ls` #tag \\n ~x @me %s &amp ",
            "100 200 3a 3 ", "USA and NASA, OK? ", "ГОСТ СССР РФ ", "  spaces  ", "",
            "user@host", "a.b.c.d ", "x.com.y ", "sh.sh ", "foo.io.bar ",
        ] {
            extra.append([Seg(text: text, on: "abc", active: "abc")])
        }
        for text in ["привет, мир. ", "ёлка и йод ", "почта ivan@mail.ru ", "сайт мой.рф ", "раз_два ", "Раз-два "] {
            extra.append([Seg(text: text, on: "ru", active: "ru")])
        }
        for segments in sentences() + extra + generatedSentences().prefix(80) {
            guard let recorded = record(segments) else { continue }
            let tokens = Tokenizer.tokenize(recorded, layout: resolve)
            let protected = Guards.protected(tokens: tokens)
            var reasons: [WordReasonDump] = []
            for (index, token) in tokens.enumerated() where token.isWord {
                reasons.append(
                    WordReasonDump(
                        token: index,
                        plain: Guards.wordLevelReason(token, minLength: 3, exceptions: [])?.rawValue,
                        numeric: Guards.wordLevelReason(
                            token, minLength: 3, exceptions: [], allowNumeric: true)?.rawValue,
                        min2: Guards.wordLevelReason(token, minLength: 2, exceptions: [])?.rawValue,
                        excepted: Guards.wordLevelReason(
                            token, minLength: 3, exceptions: ["ghbdtn", "привет", "hello"])?.rawValue))
            }
            let inProgress = Tokenizer.wordInProgress(tokens).flatMap { word in
                tokens.firstIndex { $0.range == word.range }
            }
            let lastCompleted = Tokenizer.lastCompletedWord(tokens).flatMap { word in
                tokens.firstIndex { $0.range == word.range }
            }
            records.append(
                GuardDump(
                    segs: segments,
                    tokens: tokens.map {
                        TokenDump(
                            isWord: $0.isWord, lo: $0.range.lowerBound, hi: $0.range.upperBound,
                            displayed: $0.displayed)
                    },
                    protected: Dictionary(
                        uniqueKeysWithValues: protected.map { ("\($0.key)", $0.value.rawValue) }),
                    reasons: reasons, inProgress: inProgress, lastCompleted: lastCompleted))
        }
        try Self.writeLines(records, header: [:], to: "guards.json")
    }

    // MARK: Detector

    struct CorrectionDump: Encodable {
        let lo: Int
        let hi: Int
        let layout: String
    }

    struct ChangeDump: Encodable {
        let typed: String
        let corrected: String
        let typedLang: String
        let targetLang: String
    }

    struct OutcomeDump: Encodable {
        let kind: String
        let confidence: String?
        let before: String?
        let after: String?
        let deleted: String?
        let insert: String?
        let corrections: [CorrectionDump]?
        let targetLang: String?
        let targetLayout: String?
        let changes: [ChangeDump]?
        let wordRanges: [[Int]]?
    }

    struct SessionDump: Encodable {
        let segs: [Seg]
        let layouts: [String]
        let minLen: Int
        let exceptions: [String]
        let scope: String
        let manual: Bool
        let auto: String
        let suggest: String
        let preceding: String?
        let taught: Bool
        let result: OutcomeDump
    }

    /// A fixed set of learned decisions, for the sessions that test them. Keys are lowercased,
    /// as the Go side builds the same set from the header.
    final class Taught: LearnedVocabulary {
        var decisions: [String: LearnedDecision] = [:]
        var known: [Lang: Set<String>] = [:]
        var version = 1
        func isKnown(_ word: String, in lang: Lang) -> Bool {
            known[lang]?.contains(word.lowercased()) ?? false
        }
        func decision(for typed: String) -> LearnedDecision? { decisions[typed.lowercased()] }
    }

    static let taughtDecisions: [String: String] = [
        "ghbdtn": "keep", "tot": "ru", "nen": "ru", "ult": "ru", "rfr": "keep", "yt": "ru",
        "hello": "ru", "руддщ": "en", "ещк": "keep", "ntcn": "keep", "z": "ru", "ytn": "kk",
    ]
    static let taughtKnown: [String: [String]] = [
        "ru": ["зюзяка", "приветики", "кринжатина"], "en": ["aidar", "zzyx"], "kk": ["сәлемші"],
    ]

    func makeTaught() -> Taught {
        let taught = Taught()
        for (word, decision) in Self.taughtDecisions {
            taught.decisions[word] = decision == "keep" ? .keep : .convert(to: Lang(rawValue: decision)!)
        }
        for (lang, words) in Self.taughtKnown {
            taught.known[Lang(rawValue: lang)!] = Set(words)
        }
        return taught
    }

    func outcomeDump(_ action: Detector.Action) -> OutcomeDump {
        let kind: String
        switch action {
        case .none: kind = "none"
        case .suggest: kind = "suggest"
        case .fix: kind = "fix"
        }
        guard let outcome = action.outcome else {
            return OutcomeDump(
                kind: kind, confidence: nil, before: nil, after: nil, deleted: nil, insert: nil,
                corrections: nil, targetLang: nil, targetLayout: nil, changes: nil, wordRanges: nil)
        }
        return OutcomeDump(
            kind: kind, confidence: Self.d(outcome.confidence), before: outcome.before,
            after: outcome.after, deleted: outcome.replacement.deletedText,
            insert: outcome.replacement.insertText,
            corrections: outcome.corrections.map {
                CorrectionDump(lo: $0.range.lowerBound, hi: $0.range.upperBound, layout: $0.targetLayoutID)
            },
            targetLang: outcome.targetLang.rawValue, targetLayout: outcome.targetLayoutID,
            changes: outcome.changes.map {
                ChangeDump(
                    typed: $0.typed, corrected: $0.corrected, typedLang: $0.typedLang.rawValue,
                    targetLang: $0.targetLang.rawValue)
            },
            wordRanges: outcome.wordRanges.map { [$0.lowerBound, $0.upperBound] })
    }

    /// Sessions the new DetectorTests use, plus long ones that cross the last-word window.
    func detectorSentences() -> [[Seg]] {
        var result: [[Seg]] = []
        func s(_ items: [(String, String, String)]) {
            result.append(items.map { Seg(text: $0.0, on: $0.1, active: $0.2) })
        }
        for word in ["еще ", "ещё ", "где ", "тот ", "тут ", "рук "] { s([(word, "ru", "ru")]) }
        for word in ["tot ", "her ", "the ", "afk ", "lol "] { s([(word, "abc", "abc")]) }
        for word in ["было", "хорошо", "тебе", "тоже", "люблю", "ещё"] {
            s([(word, "ru", "abc"), (" ", "abc", "abc")])
        }
        s([("привет,", "ru", "abc")])
        s([("привет,", "ru", "abc"), (" ", "abc", "abc")])
        s([("я тебя", "ru", "abc"), (" ", "abc", "abc")])
        s([("где", "ru", "abc"), (" ", "abc", "abc")])
        s([("скачай ", "ru", "ru"), ("Telegram ", "abc", "abc")])
        s([("tot ", "abc", "abc")])
        s([("мне кажется ", "ru", "ru"), ("tot ", "abc", "abc")])
        s([("не знаю", "ru", "abc"), (" ", "abc", "abc")])
        s([("зюзяка", "ru", "abc"), (" ", "abc", "abc")])
        s([("hello", "abc", "ru"), (" ", "ru", "ru")])
        s([("мы вчера долго говорили о том, как сделать хорошую программу для всех ", "ru", "ru"),
           ("привет", "ru", "abc"), (" ", "abc", "abc")])
        s([("мы вчера долго говорили о том, как сделать хорошую программу для всех ", "ru", "ru"),
           ("я тебя люблю", "ru", "abc"), (" ", "abc", "abc")])
        s([("the quick brown fox jumps over the lazy dog and runs away from the farm ", "abc", "abc"),
           ("hello", "abc", "ru"), (" ", "ru", "ru")])
        s([("the quick brown fox jumps over the lazy dog and runs away from the farm ", "abc", "abc"),
           ("где", "ru", "abc"), (" ", "abc", "abc")])
        s([("привет как дела я сегодня работаю над клавиатурой и переключателем и всё ", "ru", "abc")])
        s([("сәлеметсіз бе, қалайсыз? мен жақсымын, рақмет. бүгін ауа райы жақсы ", "kk", "kk"),
           ("жұмыс", "kk", "ru"), (" ", "ru", "ru")])
        s([("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "abc", "abc")])
        s([("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa ghbdtn ", "abc", "abc")])
        s([("мне ", "ru", "ru"), ("25", "abc", "kk"), (" лет", "ru", "ru")])
        s([("ok ", "abc", "abc"), ("спасибо ", "ru", "abc")])
        s([("привет, как дела? ", "ru", "abc")])
        s([("было хорошо, тебе тоже ", "ru", "abc")])
        s([("сәлем қалай ", "kk", "abc")])
        s([("Привет ", "ru", "abc")])
        s([("a tot of whisky ", "abc", "abc")])
        s([("это был не баг, а ", "ru", "ru"), ("feature ", "abc", "abc")])
        s([("foo.bar ", "abc", "abc")])
        s([("pfib,bcm ", "abc", "abc")])
        s([("зашибись ", "ru", "abc")])
        s([("т.е. ", "ru", "ru")])
        s([("т.е. ", "ru", "abc")])
        s([("кто-то ", "ru", "abc")])
        s([("көк ", "kk", "ru")])
        s([("к-к ", "ru", "ru")])
        s([("ГОСТ ", "ru", "abc")])
        s([("$PATH ", "abc", "abc")])
        s([("пришёл ", "ru", "abc")])
        return result
    }

    func dumpDetector() throws {
        let detector = Detector(models: store)
        let taught = makeTaught()
        var records: [SessionDump] = []

        func run(
            _ segments: [Seg], layouts: [String] = ["abc", "ru", "kk"], minLen: Int = 3,
            exceptions: [String] = [], scope: Detector.Scope, manual: Bool = false,
            thresholds: (Double, Double) = (0.80, 0.45), preceding: String? = nil,
            useTaught: Bool = false
        ) {
            guard let recorded = record(segments) else { return }
            var context = Detector.Context(
                recorded: recorded, candidateLayouts: layouts.map(table),
                resolve: { [unowned self] in self.resolve($0) }, minWordLength: minLen,
                exceptions: Set(exceptions), autoThreshold: thresholds.0,
                suggestThreshold: thresholds.1, precedingText: preceding,
                learned: useTaught ? (taught as LearnedVocabulary) : nil)
            // As TypingEngine.manualFix builds it.
            if manual {
                context.manual = true
                context.minWordLength = 1
            }
            let action = detector.evaluate(context, scope: scope)
            records.append(
                SessionDump(
                    segs: segments, layouts: layouts, minLen: context.minWordLength,
                    exceptions: exceptions, scope: scope == .lastWord ? "lastWord" : "tail",
                    manual: manual, auto: Self.d(thresholds.0), suggest: Self.d(thresholds.1),
                    preceding: preceding, taught: useTaught, result: outcomeDump(action)))
        }

        let russianText = "мы вчера долго говорили,"
        let englishText = "we talked for a long time yesterday,"
        for segments in sentences() + detectorSentences() {
            run(segments, scope: .tail)
            run(segments, scope: .lastWord)
            run(segments, scope: .lastWord, manual: true)
            run(segments, minLen: 2, exceptions: ["ghbdtn", "руддщ", "nen"], scope: .tail)
            run(segments, minLen: 4, exceptions: ["ghbdtn"], scope: .lastWord)
            run(segments, layouts: ["abc", "ru"], scope: .tail)
            run(segments, layouts: ["ru", "kk"], scope: .tail)
            run(segments, layouts: ["kk", "abc", "ru"], scope: .tail)
            run(segments, layouts: ["abc"], scope: .tail)
            run(segments, layouts: ["kk", "abc", "ru"], scope: .lastWord)
            run(segments, scope: .lastWord, preceding: russianText)
            run(segments, scope: .lastWord, preceding: englishText)
            run(segments, scope: .tail, preceding: russianText)
            run(segments, scope: .lastWord, preceding: "")
            run(segments, scope: .lastWord, thresholds: (0.55, 0.30))
            run(segments, scope: .lastWord, thresholds: (0.93, 0.55))
            run(segments, scope: .tail, thresholds: (0.55, 0.30))
            run(segments, scope: .lastWord, useTaught: true)
            run(segments, scope: .tail, useTaught: true)
            run(segments, scope: .lastWord, manual: true, useTaught: true)
            run(segments, scope: .lastWord, preceding: russianText, useTaught: true)
        }
        for segments in generatedSentences() {
            run(segments, scope: .tail)
            run(segments, scope: .lastWord)
            run(segments, scope: .lastWord, manual: true)
            run(segments, scope: .lastWord, preceding: russianText)
            run(segments, scope: .tail, thresholds: (0.55, 0.30))
        }
        for segments in singleWordSessions() {
            run(segments, scope: .lastWord)
            run(segments, scope: .lastWord, preceding: russianText)
        }
        run([], scope: .tail)
        run([], scope: .lastWord)

        var decisions = Self.taughtDecisions.map { "\($0.key)=\($0.value)" }
        decisions.sort()
        var known = Self.taughtKnown.map { "\($0.key)=\($0.value.joined(separator: " "))" }
        known.sort()
        try Self.writeLines(
            records,
            header: [
                "taughtDecisions": decisions.joined(separator: ";"),
                "taughtKnown": known.joined(separator: ";"),
                "taughtVersion": "1",
            ], to: "detector.json")
    }

    // MARK: Lexicon

    /// The hand-made dictionary of LexiconTests, where every rule kind is visible.
    static let syntheticAffixes = """
        SET UTF-8
        PFX U Y 1
        PFX U   0     un         .

        SFX S Y 3
        SFX S   y     ies        [^aeiou]y
        SFX S   0     s          [aeiou]y
        SFX S   0     s          [^y]

        SFX D N 2
        SFX D   0     d          e
        SFX D   0     ed         [^e]

        SFX L Y 2
        SFX L   ь     ю          ь
        SFX L   0     ом         [^ь]
        """

    static let syntheticDictionary = """
        6
        happy/U
        try/SD
        toy/S
        play/SDU
        боль/L
        стол/L
        """

    static let syntheticQueries: [String] = [
        "happy", "try", "toy", "play", "боль", "стол", "happ", "", "tries", "toys", "tryies",
        "toies", "tried", "played", "tryed", "болю", "столом", "столю", "больом", "happies",
        "happys", "unhappy", "unplay", "unplays", "unplayed", "untoy", "iPhone", "iphone",
        "# a comment", "UNHAPPY", "Tries", "Played", "un", "u", "unun", "unhappys", "plays",
        "playd", "trys", "toyed", "СТОЛОМ", "Болю", "еще", "ЕЩЁ", "ещё", "don\u{2019}t", "don't",
        "DON'T", "ещ", "ё", "е", "y", "s", "ies", "ю", "ом",
    ]

    struct LexiconQueryDump: Encodable {
        let w: String
        let contains: Bool
        let core: Bool
        let lookup: [Bool]
    }

    struct SyntheticLexiconDump: Encodable {
        let name: String
        let lang: String
        let data: String
        let stems: Int
        let rules: Int
        let cores: Int
        let queries: [LexiconQueryDump]
    }

    struct LexiconWordDump: Encodable {
        let w: String
        let norm: String
        let hash: UInt32
        /// `Lexicon.lookup` per language: [known, core].
        let en: [Bool]
        let ru: [Bool]
        let kk: [Bool]
        /// `WordScorer.isKnown` (learned: nil) and `WordScorer.isCore`, per language en/ru/kk.
        let known: [Bool]
        let core: [Bool]
    }

    func corpusWords() -> [String] {
        // Appended one list at a time: a long chain of `+` defeats the type checker.
        var words: [String] = []
        words += Corpus.russianFrequent
        words += Corpus.englishFrequent
        words += Corpus.kazakhFrequent
        words += Corpus.russianOnEnglish
        words += Corpus.englishOnRussian
        words += Corpus.kazakhOnRussian
        words += Corpus.kazakhOnEnglish
        words += Corpus.russianUnknown
        words += Corpus.englishUnknown
        words += Corpus.russianCollisions
        words += Corpus.englishCollisions
        words += Corpus.englishPunctuated
        words += Corpus.russianPunctuated
        words += Corpus.russianRareOnEnglish
        words += Corpus.typos(of: Corpus.russianFrequent, limit: 300)
        words += Corpus.typos(of: Corpus.englishFrequent, limit: 300)
        // The same words as the wrong layout renders them.
        var garble: [String] = []
        for word in Corpus.russianFrequent + Corpus.russianOnEnglish + Corpus.russianCollisions {
            if let strokes = russian.strokes(for: word) {
                garble.append(abc.render(strokes))
                garble.append(kazakh.render(strokes))
            }
        }
        for word in Corpus.kazakhFrequent + Corpus.kazakhOnRussian {
            if let strokes = kazakh.strokes(for: word) {
                garble.append(russian.render(strokes))
                garble.append(abc.render(strokes))
            }
        }
        for word in Corpus.englishFrequent + Corpus.englishOnRussian + Corpus.englishCollisions {
            if let strokes = abc.strokes(for: word) {
                garble.append(russian.render(strokes))
                garble.append(kazakh.render(strokes))
            }
        }
        words += garble
        words += [
            "кто-то", "из-за", "к-к", "-те", "x-ray", "well-known", "co-op", "re-enter", "по-русски",
            "ну-ка", "что-нибудь", "т.е.", "т.к.", "и.т.д.", "e.g.", "i.e.", "U.S.", "а-а", "аа-бб",
            "кто--то", "-", "--", "a", "i", "I", "A", "я", "Я", "в", "В", "е", "о", "ә", "zz",
            "мектептер", "сделала", "людей", "книгу", "пришёл", "пришел", "unhappy", "running",
            "telegram", "Telegram", "iphone", "github", "afk", "lol", "ghbdtn", "rfr", "xnj",
            "ntcn", "ltkf", "cgfcb,j", "vyt", "ytn", "руддщ", "цщкдв", "лунищфкв", "еуые", "нщг",
            "тще", "зюзяка", "Instagram", "WhatsApp", "iPhone", "API", "email", "Kaspi", "Wolt",
            "Glovo", "Zoom", "Slack", "Google", "deploy", "feature", "ok", "OK",
        ]
        return words
    }

    func dumpLexicon() throws {
        var synthetic: [SyntheticLexiconDump] = []
        let builds: [(String, Lexicon)] = [
            (
                "plain",
                Lexicon.build(
                    lang: .en, dictionary: Self.syntheticDictionary, affixes: Self.syntheticAffixes)
            ),
            (
                "extra",
                Lexicon.build(
                    lang: .en, dictionary: Self.syntheticDictionary, affixes: Self.syntheticAffixes,
                    extra: ["iphone", "# a comment", ""], core: ["try", "toy"])
            ),
            (
                "roundtrip",
                Lexicon.build(
                    lang: .en, dictionary: Self.syntheticDictionary, affixes: Self.syntheticAffixes,
                    extra: ["iphone"], core: ["try"])
            ),
            ("normalization", Lexicon.build(lang: .ru, dictionary: "2\nещё\ndon't", affixes: "SET UTF-8")),
        ]
        for (name, lexicon) in builds {
            synthetic.append(
                SyntheticLexiconDump(
                    name: name, lang: lexicon.lang.rawValue,
                    data: lexicon.encode().base64EncodedString(), stems: lexicon.stemCount,
                    rules: lexicon.ruleCount, cores: lexicon.coreCount,
                    queries: Self.syntheticQueries.map { word in
                        let lookup = lexicon.lookup(word)
                        return LexiconQueryDump(
                            w: word, contains: lexicon.contains(word), core: lexicon.isCore(word),
                            lookup: [lookup.known, lookup.core])
                    }))
        }
        try Self.writeLines(synthetic, header: [:], to: "lexicon-synthetic.json")

        let scorer = WordScorer(models: store)
        guard let en = store.lexicon(for: .en), let ru = store.lexicon(for: .ru),
            let kk = store.lexicon(for: .kk)
        else {
            XCTFail("lexicons missing")
            return
        }
        var seen = Set<String>()
        var records: [LexiconWordDump] = []
        for word in allWords() + corpusWords() where seen.insert(word).inserted {
            let normalized = Lexicon.normalize(word)
            func pair(_ lexicon: Lexicon) -> [Bool] {
                let lookup = lexicon.lookup(word)
                return [lookup.known, lookup.core]
            }
            records.append(
                LexiconWordDump(
                    w: word, norm: String(String.UnicodeScalarView(normalized)),
                    hash: Lexicon.hash(normalized), en: pair(en), ru: pair(ru), kk: pair(kk),
                    known: Lang.allCases.map { scorer.isKnown(word, lang: $0, learned: nil) },
                    core: Lang.allCases.map { scorer.isCore(word, lang: $0) }))
        }
        var header: [String: String] = [:]
        for lexicon in [en, ru, kk] {
            header["\(lexicon.lang.rawValue)"] =
                "\(lexicon.stemCount),\(lexicon.ruleCount),\(lexicon.coreCount)"
        }
        header["langs"] = Lang.allCases.map(\.rawValue).joined(separator: ",")
        try Self.writeLines(records, header: header, to: "lexicon.json")
    }

    // MARK: Readings

    struct ReadingFieldsDump: Encodable {
        let layout: String
        let lang: String
        let text: String
        let core: String
        let length: Int
        let odd: Bool
        let clean: Bool
        let numeric: Bool
        let known: Bool
        let isCore: Bool
        let cost: String
        let junk: Int
    }

    struct ReadingsDump: Encodable {
        let text: String
        let on: String
        let readings: [ReadingFieldsDump]
    }

    /// The first layout that can type the text, with its keystrokes.
    func typeable(_ text: String) -> (String, [KeyStroke])? {
        for short in ["abc", "ru", "kk"] {
            if let strokes = table(short).strokes(for: text) { return (short, strokes) }
        }
        return nil
    }

    func dumpReadings() throws {
        let scorer = WordScorer(models: store)
        var texts = allWords() + corpusWords()
        for word in ["привет", "hello", "ещё", "было", "тебе", "don't", "кто-то", "x-ray", "сәлем", "25"] {
            texts += [
                word + ",", "(" + word + ")", word + ".", word + "\\", "," + word, word + "!",
                "«" + word + "»", word + "...", "[" + word + "]", word + ";", "\"" + word + "\"",
                word + "?!", "'" + word + "'", word + ")", word + "-", "-" + word, word + ":",
            ]
        }
        var records: [ReadingsDump] = []
        var seen = Set<String>()
        for text in texts where seen.insert(text).inserted {
            guard let typed = typeable(text), !typed.1.isEmpty else { continue }
            let (short, strokes) = typed
            let id = table(short).sourceID
            let recorded = strokes.enumerated().map {
                RecordedStroke(stroke: $0.element, time: Double($0.offset) * 0.05, layoutID: id)
            }
            var readings: [ReadingFieldsDump] = []
            for (layoutShort, layout) in [("abc", abc!), ("ru", russian!), ("kk", kazakh!)] {
                let reading = scorer.reading(
                    recorded, 0..<recorded.count, layout: layout, lang: layout.lang!, learned: nil)
                readings.append(
                    ReadingFieldsDump(
                        layout: layoutShort, lang: reading.lang.rawValue, text: reading.text,
                        core: reading.core, length: reading.length, odd: reading.hasOddEdges,
                        clean: reading.isClean, numeric: reading.isNumeric, known: reading.isKnown,
                        isCore: reading.isCore, cost: Self.d(reading.cost),
                        junk: reading.interiorJunk))
            }
            records.append(ReadingsDump(text: text, on: short, readings: readings))
        }
        try Self.writeLines(records, header: [:], to: "readings.json")
    }

    // MARK: Corpus

    /// One run of the CorpusTests harness: what was typed, with which thresholds and field
    /// text, and what ended up on screen. `group` names the tally the run belongs to, in the
    /// order CorpusTests reports them; `kind` says how the tally reads it.
    struct CorpusRunDump: Encodable {
        let group: String
        let kind: String
        let word: String?
        let expected: String
        let segs: [Seg]
        let auto: String
        let suggest: String
        let field: String?
        let screen: String
        let events: [[String]]
    }

    static func short(_ lang: Lang) -> String { lang == .en ? "abc" : lang.rawValue }

    func dumpCorpus() throws {
        let tables: [Lang: KeyMapTable] = [.en: abc, .ru: russian, .kk: kazakh]
        var records: [CorpusRunDump] = []
        var thresholds = (auto: 0.80, suggest: 0.45)
        var fieldText: String?

        // CorpusTests.simulate, verbatim in behaviour.
        func simulate(_ segments: [Corpus.Segment]) -> (String, [[String]]) {
            let engine = HarnessEngine(tables: tables, store: store, manager: manager)
            engine.thresholds = thresholds
            engine.precedingText = fieldText
            for segment in segments {
                engine.activeLayout = tables[segment.active]!
                guard let intendedTable = tables[segment.intended] else { continue }
                for character in segment.text {
                    guard let stroke = intendedTable.strokes(for: String(character))?.first else {
                        XCTFail("\(segment.intended) cannot type \(character)")
                        continue
                    }
                    engine.type(stroke)
                }
            }
            let events = engine.events.map { event -> [String] in
                switch event {
                case .fixed(let before, let after): return ["fixed", before, after]
                case .suggested(let before, let after): return ["suggested", before, after]
                }
            }
            return (engine.screen, events)
        }

        func add(
            _ group: String, _ kind: String, word: String?, expected: String,
            _ segments: [Corpus.Segment]
        ) {
            let (screen, events) = simulate(segments)
            records.append(
                CorpusRunDump(
                    group: group, kind: kind, word: word, expected: expected,
                    segs: segments.map {
                        Seg(text: $0.text, on: Self.short($0.intended), active: Self.short($0.active))
                    },
                    auto: Self.d(thresholds.auto), suggest: Self.d(thresholds.suggest),
                    field: fieldText, screen: screen, events: events))
        }

        func stay(_ group: String, _ words: [String], lang: Lang, prefix: String = "") {
            for word in words {
                add(
                    group, "stay", word: word, expected: prefix + word + " ",
                    [Corpus.Segment(text: prefix + word + " ", intended: lang, active: lang)])
            }
        }

        func convert(
            _ group: String, _ words: [String], intended: Lang, active: Lang,
            context: (text: String, lang: Lang)? = nil
        ) {
            for word in words {
                var segments: [Corpus.Segment] = []
                if let context {
                    segments.append(
                        Corpus.Segment(text: context.text, intended: context.lang, active: context.lang))
                }
                segments.append(Corpus.Segment(text: word + " ", intended: intended, active: active))
                add(group, "convert", word: word, expected: (context?.text ?? "") + word + " ", segments)
            }
        }

        // testCorpusReport
        stay("report/stay/ru frequent", Corpus.russianFrequent, lang: .ru)
        stay("report/stay/en frequent", Corpus.englishFrequent, lang: .en)
        stay("report/stay/kk frequent", Corpus.kazakhFrequent, lang: .kk)
        stay("report/stay/ru frequent after context", Corpus.russianFrequent, lang: .ru, prefix: "это ")
        stay("report/stay/en frequent after context", Corpus.englishFrequent, lang: .en, prefix: "this ")
        stay("report/stay/ru unknown/slang/names", Corpus.russianUnknown, lang: .ru)
        stay("report/stay/en unknown/slang/names", Corpus.englishUnknown, lang: .en)
        stay("report/stay/ru typos", Corpus.typos(of: Corpus.russianFrequent, limit: 300), lang: .ru)
        stay("report/stay/en typos", Corpus.typos(of: Corpus.englishFrequent, limit: 300), lang: .en)
        stay("report/stay/ru collisions", Corpus.russianCollisions, lang: .ru)
        stay("report/stay/ru collisions after context", Corpus.russianCollisions, lang: .ru, prefix: "это ")
        stay("report/stay/en collisions", Corpus.englishCollisions, lang: .en)
        stay("report/stay/en collisions after context", Corpus.englishCollisions, lang: .en, prefix: "this ")
        stay("report/stay/en punctuated", Corpus.englishPunctuated, lang: .en)
        stay("report/stay/ru punctuated", Corpus.russianPunctuated, lang: .ru)

        for (label, sentences, lang) in [
            ("ru", Corpus.russianSentences, Lang.ru), ("en", Corpus.englishSentences, .en),
            ("kk", Corpus.kazakhSentences, .kk),
        ] {
            for sentence in sentences {
                add(
                    "report/sentences-correct/\(label)", "sentence-stay", word: nil,
                    expected: sentence + " ",
                    [Corpus.Segment(text: sentence + " ", intended: lang, active: lang)])
            }
        }

        convert("report/convert/ru typed on en", Corpus.russianOnEnglish, intended: .ru, active: .en)
        convert("report/convert/en typed on ru", Corpus.englishOnRussian, intended: .en, active: .ru)
        convert("report/convert/kk typed on ru", Corpus.kazakhOnRussian, intended: .kk, active: .ru)
        convert("report/convert/kk typed on en", Corpus.kazakhOnEnglish, intended: .kk, active: .en)
        convert(
            "report/convert/ru typed on en after ru context", Corpus.russianOnEnglish, intended: .ru,
            active: .en, context: ("мне кажется ", .ru))
        convert(
            "report/convert/en typed on ru after en context", Corpus.englishOnRussian, intended: .en,
            active: .ru, context: ("i think ", .en))
        convert("report/convert/rare ru typed on en", Corpus.russianRareOnEnglish, intended: .ru, active: .en)
        convert(
            "report/convert/ru collisions typed on en", Corpus.russianCollisions, intended: .ru,
            active: .en)
        convert(
            "report/convert/ru collisions typed on en after ru context", Corpus.russianCollisions,
            intended: .ru, active: .en, context: ("мне кажется ", .ru))

        for (label, sentences, intended, active) in [
            ("ru on en", Corpus.russianSentences, Lang.ru, Lang.en),
            ("en on ru", Corpus.englishSentences, .en, .ru),
            ("kk on ru", Corpus.kazakhSentences, .kk, .ru),
            ("kk on en", Corpus.kazakhSentences, .kk, .en),
        ] {
            for sentence in sentences {
                add(
                    "report/sentences-wrong/\(label)", "sentence-convert", word: nil,
                    expected: sentence + " ",
                    [Corpus.Segment(text: sentence + " ", intended: intended, active: active)])
            }
        }

        for testCase in Corpus.mixedCases {
            add("report/mixed", "mixed", word: testCase.name, expected: testCase.expected, testCase.segments)
        }

        // testContextFromTheTextField
        let short = Corpus.russianOnEnglish.filter { $0.count <= 4 }
        fieldText = nil
        convert("context/short ru typed on en, empty field", short, intended: .ru, active: .en)
        fieldText = "мы вчера долго говорили,"
        convert("context/short ru typed on en, Russian text before", short, intended: .ru, active: .en)
        stay("context/en frequent after Russian text", Corpus.englishFrequent, lang: .en)
        stay("context/en collisions after Russian text", Corpus.englishCollisions, lang: .en)
        fieldText = "we talked for a long time yesterday,"
        stay("context/ru frequent after English text", Corpus.russianFrequent, lang: .ru)
        fieldText = nil

        // testPresets
        for (name, preset) in [
            ("cautious", (auto: 0.93, suggest: 0.55)), ("balanced", (auto: 0.80, suggest: 0.45)),
            ("bold", (auto: 0.55, suggest: 0.30)),
        ] {
            thresholds = preset
            stay("presets/\(name)/stay/ru", Corpus.russianFrequent, lang: .ru)
            stay("presets/\(name)/stay/en", Corpus.englishFrequent, lang: .en)
            stay("presets/\(name)/stay/en collisions", Corpus.englishCollisions, lang: .en)
            stay(
                "presets/\(name)/stay/ru typos", Corpus.typos(of: Corpus.russianFrequent, limit: 300),
                lang: .ru)
            convert("presets/\(name)/convert/ru on en", Corpus.russianOnEnglish, intended: .ru, active: .en)
            convert("presets/\(name)/convert/en on ru", Corpus.englishOnRussian, intended: .en, active: .ru)
            convert("presets/\(name)/convert/kk on ru", Corpus.kazakhOnRussian, intended: .kk, active: .ru)
        }
        thresholds = (0.80, 0.45)

        try Self.writeLines(records, header: [:], to: "corpus.json")
    }

    // MARK: Learner

    struct LearnerHashDump: Encodable {
        let w: String
        let hash: String
    }

    func dumpLearner() throws {
        // Under the temp directory, not ~/Library/Preferences (see ScratchDefaults).
        let scratch = ScratchDefaults.make()
        defer { scratch.remove() }
        let defaults = scratch.defaults
        let salt: [UInt8] = [0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF]
        defaults.set(Data(salt), forKey: Learner.Key.salt)
        let learner = Learner(defaults: defaults)
        var words = [
            "", "a", "ghbdtn", "GHBDTN", "Ghbdtn", "привет", "ПРИВЕТ", "ещё", "еще", "ЕЩЁ",
            "don't", "don\u{2019}t", "Айдар", "айдар", "сәлем", "СӘЛЕМ", "tot", "nen", "e\u{301}",
            "é", "İstanbul", "straße", "👍", "zz-top",
        ]
        words += Corpus.russianRareOnEnglish + Corpus.englishCollisions + Corpus.russianUnknown
        try Self.writeLines(
            words.map {
                LearnerHashDump(w: $0, hash: String(format: "%016llx", learner.hash($0)))
            },
            header: ["salt": salt.map { String(format: "%02x", $0) }.joined()],
            to: "learner.json")
    }

    // MARK: Learning through the engine

    /// One learning scenario: the same text typed in `sessions` separate sessions through the
    /// CorpusTests harness with a learner attached, the way LearnerTests drive it, and what the
    /// learner holds afterwards. The salt and the day are fixed, so the entries come out the
    /// same on every run, and on the Go side.
    struct LearningDump: Encodable {
        let group: String
        let segs: [Seg]
        let sessions: Int
        let auto: String
        let suggest: String
        /// The screen after each session.
        let screens: [String]
        /// The fixes and offers of each session.
        let events: [[[String]]]
        /// `learner.items()` as `[id, kind, lang]`, lang empty when the entry has none.
        let items: [[String]]
    }

    static let learningSalt: [UInt8] = [0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF]
    /// Seconds since the reference date: the learner's clock, so `lastUsed` is one fixed day.
    static let learningDay: TimeInterval = 800_000_000

    func dumpLearning() throws {
        let tables: [Lang: KeyMapTable] = [.en: abc, .ru: russian, .kk: kazakh]
        let manager = self.manager!
        let store = self.store!
        var records: [LearningDump] = []

        func run(
            _ group: String, _ segments: [Corpus.Segment], sessions: Int = 2,
            thresholds: (auto: Double, suggest: Double) = (0.80, 0.45)
        ) {
            // Only what can be typed: a character missing from a layout would end the word early.
            var strokes: [[KeyStroke]] = []
            for segment in segments {
                var keys: [KeyStroke] = []
                for character in segment.text {
                    guard let stroke = tables[segment.intended]?.strokes(for: String(character))?.first
                    else { return }
                    keys.append(stroke)
                }
                strokes.append(keys)
            }

            let scratch = ScratchDefaults.make()
            defer { scratch.remove() }
            let defaults = scratch.defaults
            defaults.set(Data(Self.learningSalt), forKey: Learner.Key.salt)
            let learner = Learner(
                defaults: defaults, now: { Date(timeIntervalSinceReferenceDate: Self.learningDay) })

            var screens: [String] = []
            var events: [[[String]]] = []
            for _ in 0..<sessions {
                let harness = HarnessEngine(tables: tables, store: store, manager: manager)
                harness.thresholds = thresholds
                harness.learned = learner
                harness.engine.learner = learner
                harness.engine.resolve = { manager.table(forSourceID: $0) }
                let detector = harness.engine.detector
                harness.engine.isDictionaryWord = { detector.isDictionaryWord($0, lang: $1) }
                for (segment, keys) in zip(segments, strokes) {
                    harness.activeLayout = tables[segment.active]!
                    for stroke in keys { harness.type(stroke) }
                }
                screens.append(harness.screen)
                events.append(
                    harness.events.map { event -> [String] in
                        switch event {
                        case .fixed(let before, let after): return ["fixed", before, after]
                        case .suggested(let before, let after): return ["suggested", before, after]
                        }
                    })
            }
            records.append(
                LearningDump(
                    group: group,
                    segs: segments.map {
                        Seg(text: $0.text, on: Self.short($0.intended), active: Self.short($0.active))
                    },
                    sessions: sessions, auto: Self.d(thresholds.auto),
                    suggest: Self.d(thresholds.suggest), screens: screens, events: events,
                    items: learner.items().map { [$0.id, $0.kind.rawValue, $0.lang?.rawValue ?? ""] }))
        }

        // Nothing is ever fixed: every mistake is left standing, as when a fix did not happen.
        let never = (auto: 2.0, suggest: 2.0)
        for (label, sentences, lang) in [
            ("ru", Corpus.russianSentences, Lang.ru), ("en", Corpus.englishSentences, .en),
            ("kk", Corpus.kazakhSentences, .kk),
        ] {
            for sentence in sentences {
                let text = sentence + " "
                // Typed correctly: only what no dictionary has may become the user's word.
                run("kept/\(label)", [Corpus.Segment(text: text, intended: lang, active: lang)])
                for active in Lang.allCases where active != lang {
                    let segment = Corpus.Segment(text: text, intended: lang, active: active)
                    // Mistakes nobody caught: never the user's vocabulary.
                    run("missed/\(label) on \(Self.short(active))", [segment], thresholds: never)
                    // The same with the usual thresholds, where most of them are fixed.
                    run("default/\(label) on \(Self.short(active))", [segment])
                }
            }
        }
        // Russian typed with the Kazakh layout's keys: correct Russian, not Kazakh vocabulary.
        for sentence in Corpus.russianSentences {
            run("kept/ru with kk keys", [Corpus.Segment(text: sentence + " ", intended: .kk, active: .kk)])
        }
        // Words no dictionary has, in context.
        for word in Corpus.russianUnknown {
            run("kept/ru unknown", [Corpus.Segment(text: "мой \(word) друг ", intended: .ru, active: .ru)])
        }
        for word in Corpus.englishUnknown {
            run("kept/en unknown", [Corpus.Segment(text: "my \(word) friend ", intended: .en, active: .en)])
        }
        for mixedCase in Corpus.mixedCases {
            run("default/mixed", mixedCase.segments)
        }
        // The sentences of LearnerTests.
        run(
            "tests/missed", [Corpus.Segment(text: "я написал так но потом всё ", intended: .ru, active: .en)],
            sessions: 3, thresholds: never)
        run(
            "tests/ru with kk keys",
            [Corpus.Segment(text: "ну короче вот так кароче сделаем ", intended: .kk, active: .kk)])

        try Self.writeLines(
            records,
            header: [
                "salt": Self.learningSalt.map { String(format: "%02x", $0) }.joined(),
                "day": "\(Int(Self.learningDay))",
            ],
            to: "learning.json")
    }

    // MARK: Replacement helpers

    struct ChunksDump: Encodable {
        let text: String
        let limit: Int
        let chunks: [String]
    }

    struct CapitalDump: Encodable {
        let screen: String
        let deleted: String
        let insert: String
        let outDeleted: String
        let outInsert: String
    }

    func dumpChunks() throws {
        var texts = [
            "", "a", "привет как дела у тебя, всё хорошо? ", "👨‍👩‍👧‍👦 сәлем 🇰🇿 hello",
            "e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}e\u{301}",
            String(repeating: "я", count: 45), "🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿🇰🇿", "\r\n\r\n\r\n",
            "𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠𝟠", "ab👍🏽cd👍🏽ef👍🏽gh👍🏽ij👍🏽kl👍🏽mn👍🏽op",
        ]
        texts += Corpus.russianSentences.prefix(10).map { $0 + " " }
        texts += Corpus.kazakhSentences.prefix(5)
        var chunks: [ChunksDump] = []
        for text in texts {
            for limit in [20, 8, 3, 2, 1] {
                chunks.append(ChunksDump(text: text, limit: limit, chunks: TextReplacer.chunks(text, limit: limit)))
            }
        }
        try Self.writeLines(chunks, header: [:], to: "chunks.json")

        var capitals: [CapitalDump] = []
        for (screen, deleted, insert) in [
            ("Ghbdtn ", "ghbdtn ", "привет "), ("ghbdtn ", "ghbdtn ", "привет "),
            ("GHBDTN ", "ghbdtn ", "привет "), ("Ghbdtn ", "Ghbdtn ", "Привет "),
            ("Ghbdtn ", "ghbdtn ", "25 "), ("Ьфьф ", "ьфьф ", "mama "), ("Ёжик", "ёжик", "`;br"),
            (" Ghbdtn", " ghbdtn", " привет"), ("Ǆemal", "ǆemal", "ǆемал"), ("ǅemal", "ǆemal", "ǆемал"),
            ("Ghbdtn ", "ghbdtn ", "ßtraße "), ("Ghbdtn ", "ghbdtn ", "ﬁle "), ("Ghbdtn ", "ghbdtn ", ""),
            ("Ghbdtn ", "ghbdtn ", "i̇stanbul "), ("Z nt,z ", "z nt,z ", "я тебя "),
            ("Pf ", "pf ", "за "), ("1bc", "1bc", "1ис"), ("Abc", "abc", "É"), ("Abc", "abc", "e\u{301}x"),
            ("Ghbdtn ", "ghbdtn ", "'привет "), ("Ghbdtn ", "ghbdtn ", "ǳ "),
        ] {
            let out = TextReplacer.carryingCapital(
                from: screen, into: Replacement(deletedText: deleted, insertText: insert))
            capitals.append(
                CapitalDump(
                    screen: screen, deleted: deleted, insert: insert, outDeleted: out.deletedText,
                    outInsert: out.insertText))
        }
        try Self.writeLines(capitals, header: [:], to: "capital.json")
    }

    // MARK: Text converter

    struct ConvertDump: Encodable {
        let text: String
        let from: String
        let to: String
        let out: String
    }

    struct BestDump: Encodable {
        let text: String
        let from: String
        let layout: String?
        let out: String?
    }

    struct CostDump: Encodable {
        let text: String
        let lang: String
        let cost: String?
    }

    func dumpTextConverter() throws {
        var texts = [
            "ghbdtn", "Ghbdtn, rfr ltkf?", "руддщ цщкдв", "с2леметс3з", "0аза0", "әғ", "25",
            "hello world", "привет мир", "сәлеметсіз бе", "iPhone 15 Pro", "ytn", "yt pyf. ",
            "Z nt,z k.,k.", "b vs dct", ":-) 👍", "", "   ", "ьщтшещк", "vjybnjh", "ЫФВФ",
            "Ntcn Ntcn", "q", "123 456", "ё", "`", "~!@#$%^&*()_+", "\"№;%:?*()",
        ]
        for word in Self.russianWords.prefix(40) { texts.append(abc.render(russian.strokes(for: word) ?? [])) }
        for word in Self.kazakhWords.prefix(40) { texts.append(russian.render(kazakh.strokes(for: word) ?? [])) }
        for word in Self.englishCommon.prefix(40) { texts.append(russian.render(abc.strokes(for: word) ?? [])) }

        var converts: [ConvertDump] = []
        var bests: [BestDump] = []
        var costs: [CostDump] = []
        let all = [("abc", abc!), ("ru", russian!), ("kk", kazakh!)]
        for text in texts {
            for (fromShort, from) in all {
                for (toShort, to) in all where toShort != fromShort {
                    converts.append(
                        ConvertDump(
                            text: text, from: fromShort, to: toShort,
                            out: TextConverter.convert(text, from: from, to: to)))
                }
                let best = TextConverter.bestConversion(
                    text, from: from, layouts: [abc, russian, kazakh], models: store)
                bests.append(
                    BestDump(
                        text: text, from: fromShort,
                        layout: best.map { $0.layout.sourceID }, out: best?.text))
            }
            for lang in Lang.allCases {
                costs.append(
                    CostDump(
                        text: text, lang: lang.rawValue,
                        cost: TextConverter.cost(of: text, lang: lang, models: store).map(Self.d)))
            }
        }
        try Self.writeLines(converts, header: [:], to: "textconv-convert.json")
        try Self.writeLines(bests, header: [:], to: "textconv-best.json")
        try Self.writeLines(costs, header: [:], to: "textconv-cost.json")
    }

    // MARK: Transliteration

    struct TranslitDump: Encodable {
        let input: String
        let direction: String
        let toCyrillicPractical: String
        let toCyrillicOfficial: String
        let toLatinPractical: String
        let toLatinOfficial: String
        let readings: [ReadingDump]
        let latinOnly: Bool
    }

    struct ReadingDump: Encodable {
        let lang: String
        let text: String
        let cost: String
    }

    struct LearnedDump: Encodable {
        let never: [String]
        let overrides: [String: String]
        let input: String
        let output: String
    }

    struct ChangedDump: Encodable {
        let before: String
        let after: String
        let changed: [String]
    }

    func dumpTranslit() throws {
        let translit = Transliterator(models: store)
        var inputs: [String] = [
            "qalaisyz", "qai", "jylgysyz", "kim", "qalai", "dese", "magan", "baribir", "eken",
            "siz", "bilmeisiz", "goi", "menin", "ishimde", "bolyp", "jatqanyn", "aitqan", "edi",
            "koz", "kore", "uirenedi", "dep", "alde", "olai", "aitpady", "ony", "bilmeidi",
            "ekenmin", "jylqy", "qysta", "jazda", "qoigan", "apai", "sanyrau", "qalgan",
            "qaharman", "ulken", "tulga", "bolamyn", "desenizder", "kop", "enbek", "etu", "kerek",
            "Zametki", "Spisok", "Pokupki", "Nastroiki", "Raspisanie", "Zagruzka", "Ssylka", "Papka",
            "kofe", "Poisk", "Pochta", "Vkladka", "Uvedomlenie", "Podpiska", "Izbrannoe",
            "Obnovlenie", "Shablon", "Okno", "rabote", "jaqsy", "ademi", "jasalgan", "zzxq",
            "whatsapp", "instagram", "Instagram", "slack", "notion", "figma", "iphone", "zoomga",
            "instagramda", "xkcd", "privet", "bugin", "keshke", "jospar", "arqyly", "qosu",
            "aldymen", "Sherhan", "SHERHAN", "Urgashy", "v", "v zale", "v papke", "v rabote",
            "(Пример)", "qalaisyz 25 + 3 = 28", "!!! ???", "", "assalaumagaleikum aga, qalaisyz?",
            "sälemetsız", "jaqsy", "kino", "äğñöūüş", "ÄĞÑÖŪÜŞ", "Qazaqstan", "QAZAQSTAN",
            "shch", "sh", "ch", "zh", "kh", "ts", "ya", "yu", "yo", "x", "w", "c", "hh", "ng",
            "don't", "it’s", "a'b", "ai", "oi", "ui", "ei", "iu", "ia", "io",
            "қалайсыз", "Ұрғашы", "Шерхан", "ҚАЗАҚСТАН", "аәбвгғдежзиклқлмнңоөпрстуұүфхшыі",
            "әғңөұүш", "жақсы", "сәлеметсіз", "сәлеметсіз бе, қалайсыз?", "қалайсыз аға",
            "ёлка щука цирк чай юла яблоко объявление мягкий", "Ёжик", "ЩУКА", "Юлия",
            "таһарат аллаһ", "мен сені сағындым", "iPhone қымбат", "Kazakh Latin 2021",
            "qalaisyz aga", "қалайсыз аға",
            """
            assalaumagaleikum aga qalaisyz?
            qai jylgysyz?
            kim qalai dese de magan baribir eken
            siz bilmeisiz goi
            menin ishimde ne bolyp jatqanyn
            Sherhan aga aitqan edi
            koz kore kore uirenedi dep
            """,
            """
            bugin keshke 3 jospar Vstrecha, Zametki, Raspisanie, Zadachi
            ertengi 5 ssylka zoom arqyly jiberemin
            Pochta qazir jaqsy, jumys bastaldy
            Zagruzka aldymen notion qosu kerek
            Podpiska (v rabote)
            Vkladka: spisok, v papke, zametki na ertengi
            Obnovlenie telegram keiin tapsyrma ushin qaita update
            """,
        ]
        for word in Self.kazakhWords {
            inputs.append(word)
            inputs.append(translit.toLatin(word, scheme: .practical))
            inputs.append(translit.toLatin(word, scheme: .official2021))
        }
        for word in Self.russianWords.prefix(80) {
            inputs.append(translit.toLatin(word, scheme: .practical))
        }
        var seen = Set<String>()
        var records: [TranslitDump] = []
        for input in inputs where seen.insert(input).inserted {
            let direction = Transliterator.direction(for: input)
            records.append(
                TranslitDump(
                    input: input, direction: direction == .toLatin ? "toLatin" : "toCyrillic",
                    toCyrillicPractical: translit.toCyrillic(input, scheme: .practical),
                    toCyrillicOfficial: translit.toCyrillic(input, scheme: .official2021),
                    toLatinPractical: translit.toLatin(input, scheme: .practical),
                    toLatinOfficial: translit.toLatin(input, scheme: .official2021),
                    readings: translit.readings(of: input).map {
                        ReadingDump(lang: $0.lang.rawValue, text: $0.text, cost: Self.d($0.cost))
                    },
                    latinOnly: TranslitTables.isLatinOnly(input)))
        }
        try Self.writeLines(records, header: [:], to: "translit.json")

        var learned: [LearnedDump] = []
        for (never, overrides) in [
            (["kofe", "pokupki"], [String: String]()),
            ([], ["jatqanyn": "жатқаның", "kofe": "кофе!"]),
            (["Kerek"], ["qalai": "ҚАЛАЙ"]),
        ] {
            let custom = Transliterator(models: store, overrides: overrides, neverConvert: Set(never))
            for input in ["kofe", "pokupki", "papka", "jatqanyn", "Jatqanyn", "kerek", "Kerek", "qalai", "kofega", "kofe papka kerek"] {
                learned.append(
                    LearnedDump(
                        never: never, overrides: overrides, input: input,
                        output: custom.toCyrillic(input, scheme: .practical)))
            }
        }
        try Self.writeLines(learned, header: [:], to: "translit-learned.json")

        var changed: [ChangedDump] = []
        for (before, after) in [
            ("kofe papka zoom", "кофе папка zoom"), ("a b", "а"), ("don't stop", "дон'т стоп"),
            ("", ""), ("one, two; three", "one, два; three"), ("Ab-cd", "Аб-cd"),
        ] {
            changed.append(
                ChangedDump(
                    before: before, after: after,
                    changed: translit.changedWords(from: before, to: after)))
        }
        try Self.writeLines(changed, header: [:], to: "translit-changed.json")
    }
}

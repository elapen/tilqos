import Combine
import XCTest

@testable import TilqosKit

/// The interface texts: every key in every language, the same placeholders everywhere, the
/// fallback to English, the choice of language, and the plural forms.
///
/// These tests read the real `Resources/<lang>.lproj` folders of the repository, so a texts
/// file that is incomplete or mismatched fails here before it reaches a user.
final class L10nTests: XCTestCase {
    static let packageRoot: URL = URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    static let resources = packageRoot.appendingPathComponent("Resources")

    override func setUp() {
        L10n.reset()
        L10n.useResourceRoots([Self.resources])
        L10n.useSystemLanguages(["en-US"])
    }

    override func tearDown() {
        L10n.reset()
    }

    // MARK: - The tables

    private func table(_ lang: Lang) -> [String: String] { L10n.table(of: lang) }

    func testTablesLoad() {
        for lang in Lang.allCases {
            XCTAssertGreaterThan(table(lang).count, 300, "\(lang.rawValue) table did not load")
        }
    }

    func testEveryKeyExistsInAllThreeLanguages() {
        let english = Set(table(.en).keys)
        for lang in [Lang.ru, .kk] {
            let keys = Set(table(lang).keys)
            XCTAssertEqual(
                english.subtracting(keys).sorted(), [],
                "keys missing from \(lang.rawValue).lproj")
            XCTAssertEqual(
                keys.subtracting(english).sorted(), [],
                "keys in \(lang.rawValue).lproj that en.lproj does not have")
        }
    }

    func testNoValueIsEmpty() {
        for lang in Lang.allCases {
            for (key, value) in table(lang) {
                XCTAssertFalse(
                    value.trimmingCharacters(in: .whitespaces).isEmpty,
                    "\(lang.rawValue): \(key) is empty")
            }
        }
    }

    /// `%@`/`%d` and their positional forms must be the same set in every language, or a
    /// translation would crash `String(format:)` or print the wrong thing.
    func testPlaceholdersMatchAcrossLanguages() {
        let english = table(.en)
        for lang in [Lang.ru, .kk] {
            for (key, text) in table(lang) {
                guard let source = english[key] else { continue }
                XCTAssertEqual(
                    Self.placeholders(in: text), Self.placeholders(in: source),
                    "\(lang.rawValue): \(key): placeholders differ from English")
            }
        }
    }

    /// (position, conversion) pairs, sorted; a plain `%@` takes the next free position.
    static func placeholders(in text: String) -> [String] {
        let pattern = try! NSRegularExpression(pattern: #"%(?:(\d+)\$)?(?:l)?([@dsfx%])"#)
        var next = 1
        var found: [String] = []
        for match in pattern.matches(in: text, range: NSRange(text.startIndex..., in: text)) {
            let conversion = String(text[Range(match.range(at: 2), in: text)!])
            if conversion == "%" { continue }
            var position = next
            if let range = Range(match.range(at: 1), in: text), let n = Int(text[range]) {
                position = n
            }
            next += 1
            found.append("\(position)\(conversion)")
        }
        return found.sorted()
    }

    func testPlaceholderParserUnderstandsBothForms() {
        XCTAssertEqual(Self.placeholders(in: "a %@ b %d"), ["1@", "2d"])
        XCTAssertEqual(Self.placeholders(in: "%2$@ then %1$@"), ["1@", "2@"])
        XCTAssertEqual(Self.placeholders(in: "100%% sure"), [])
    }

    /// The Kazakh and Russian files are translations, not copies of each other or of English.
    func testTranslationsAreNotCopies() {
        let english = table(.en)
        let russian = table(.ru)
        let kazakh = table(.kk)
        let sameAsEnglish = Set(english.keys).filter {
            russian[$0] == english[$0] && kazakh[$0] == english[$0]
        }
        // Names and symbols that are written the same in every language.
        XCTAssertLessThan(sameAsEnglish.count, 12, "untranslated: \(sameAsEnglish.sorted())")
        let sameAsRussian = Set(russian.keys).filter { kazakh[$0] == russian[$0] }
        XCTAssertLessThan(sameAsRussian.count, 20, "Kazakh equals Russian for: \(sameAsRussian.sorted())")
    }

    func testPluralKeysHaveEveryCategory() {
        for lang in Lang.allCases {
            let keys = Set(table(lang).keys)
            let bases = Set(
                keys.filter { $0.hasSuffix(".other") }.map { String($0.dropLast(".other".count)) })
            XCTAssertFalse(bases.isEmpty)
            for base in bases {
                for category in Plural.Category.allCases {
                    XCTAssertTrue(
                        keys.contains("\(base).\(category.rawValue)"),
                        "\(lang.rawValue): \(base) has no .\(category.rawValue) form")
                }
            }
        }
    }

    func testInfoPlistStringsExistInAllLanguages() throws {
        var keySets: [Set<String>] = []
        for lang in Lang.allCases {
            let url = Self.resources.appendingPathComponent("\(lang.rawValue).lproj/InfoPlist.strings")
            let table = try XCTUnwrap(NSDictionary(contentsOf: url) as? [String: String], lang.rawValue)
            XCTAssertEqual(table["CFBundleDisplayName"], "Tilqos")
            XCTAssertNotNil(table["NSHumanReadableCopyright"])
            keySets.append(Set(table.keys))
        }
        XCTAssertEqual(Set(keySets.map { $0.sorted() }).count, 1, "InfoPlist.strings keys differ")
    }

    // MARK: - The code uses the tables

    /// Every literal key in the sources exists, and every key in the table is used.
    func testSourcesUseExactlyTheKeysThatExist() throws {
        let english = Set(table(.en).keys)
        var used = Set<String>()
        let sources = Self.packageRoot.appendingPathComponent("Sources/TilqosKit")
        let pattern = try NSRegularExpression(
            pattern: #"(?:L10n\.(?:tr|plural|richText)\(\s*|\bkey:\s*)"([A-Za-z0-9_.]+)""#)
        let files = FileManager.default.enumerator(at: sources, includingPropertiesForKeys: nil)!
        for case let url as URL in files where url.pathExtension == "swift" {
            let text = try String(contentsOf: url, encoding: .utf8)
            for match in pattern.matches(in: text, range: NSRange(text.startIndex..., in: text)) {
                let key = String(text[Range(match.range(at: 1), in: text)!])
                // A plural call names the base; its forms are `<base>.one` and so on.
                if english.contains(key) || english.contains("\(key).other") {
                    used.insert(key)
                } else {
                    XCTFail("\(url.lastPathComponent) uses \(key), which en.lproj does not define")
                }
            }
        }
        let covered = english.filter { key in
            used.contains(key)
                || Plural.Category.allCases.contains {
                    key.hasSuffix(".\($0.rawValue)")
                        && used.contains(String(key.dropLast($0.rawValue.count + 1)))
                }
        }
        XCTAssertEqual(english.subtracting(covered).sorted(), [], "keys no code uses")
    }

    // MARK: - Choosing the language

    func testSystemLanguageResolution() {
        XCTAssertEqual(L10n.resolve(system: ["en-US"]), .en)
        XCTAssertEqual(L10n.resolve(system: ["ru-RU"]), .ru)
        XCTAssertEqual(L10n.resolve(system: ["kk-KZ"]), .kk)
        XCTAssertEqual(L10n.resolve(system: ["kk_KZ"]), .kk)
        XCTAssertEqual(L10n.resolve(system: ["RU"]), .ru)
        // Anything else is English.
        XCTAssertEqual(L10n.resolve(system: ["de-DE"]), .en)
        XCTAssertEqual(L10n.resolve(system: ["uk-UA", "fr-FR"]), .en)
        XCTAssertEqual(L10n.resolve(system: []), .en)
        // The first language the app has, like macOS does for any app.
        XCTAssertEqual(L10n.resolve(system: ["de-DE", "ru-RU", "en-US"]), .ru)
    }

    func testOverrideBeatsTheSystemLanguage() {
        L10n.useSystemLanguages(["ru-RU"])
        L10n.setting = .system
        XCTAssertEqual(L10n.language, .ru)
        L10n.setting = .kk
        XCTAssertEqual(L10n.language, .kk)
        L10n.setting = .en
        XCTAssertEqual(L10n.language, .en)
        L10n.setting = .system
        XCTAssertEqual(L10n.language, .ru)
    }

    func testChangingLanguagePostsOneNotification() {
        L10n.setting = .system
        var count = 0
        let token = NotificationCenter.default.addObserver(
            forName: L10n.didChange, object: nil, queue: nil
        ) { _ in count += 1 }
        defer { NotificationCenter.default.removeObserver(token) }
        L10n.setting = .ru
        L10n.setting = .ru  // no change
        L10n.setting = .kk
        XCTAssertEqual(count, 2)
    }

    func testTextFollowsTheLanguage() {
        L10n.setting = .en
        XCTAssertEqual(AppHealth.working.title, "Working")
        L10n.setting = .ru
        XCTAssertEqual(AppHealth.working.title, "Работает")
        L10n.setting = .kk
        XCTAssertEqual(AppHealth.working.title, "Жұмыс істеп тұр")
        XCTAssertEqual(SettingsPane.exceptions.title, "Ерекшеліктер")
        XCTAssertEqual(TranslitScheme.practical.displayName, "Практикалық (ASCII)")
    }

    func testLanguageSettingIsStoredInPrefs() {
        let scratch = ScratchDefaults.make()
        defer { scratch.remove() }
        let prefs = Prefs(defaults: scratch.defaults)
        XCTAssertEqual(prefs.uiLanguage, .system)
        prefs.uiLanguage = .kk
        XCTAssertEqual(Prefs(defaults: scratch.defaults).uiLanguage, .kk)
        scratch.defaults.set("klingon", forKey: "uiLanguage")
        XCTAssertEqual(prefs.uiLanguage, .system, "an unknown value means: follow the system")
    }

    // MARK: - Fallback

    /// A language whose table lacks a key shows the English text; a key nobody has shows
    /// itself, so the gap is visible.
    func testMissingKeyFallsBackToEnglishThenToTheKey() throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("tilqos-l10n-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: root) }
        for (lang, body) in [
            ("en", "\"a\" = \"English A\";\n\"b\" = \"English B\";\n\"n.one\" = \"%d file\";\n\"n.other\" = \"%d files\";\n"),
            ("kk", "\"a\" = \"Kazakh A\";\n"),
        ] {
            let folder = root.appendingPathComponent("\(lang).lproj")
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            try body.write(
                to: folder.appendingPathComponent("Localizable.strings"), atomically: true,
                encoding: .utf8)
        }
        L10n.useResourceRoots([root])
        L10n.setting = .kk
        XCTAssertEqual(L10n.tr("a"), "Kazakh A")
        XCTAssertEqual(L10n.tr("b"), "English B")
        XCTAssertEqual(L10n.tr("nobody.has.this"), "nobody.has.this")
        XCTAssertEqual(L10n.plural("n", 1), "1 file", "plural forms fall back to English too")
        XCTAssertEqual(L10n.plural("n", 3), "3 files")
        // Russian has no table at all.
        L10n.setting = .ru
        XCTAssertEqual(L10n.tr("a"), "English A")
    }

    func testNoResourcesAtAllShowsKeys() {
        L10n.useResourceRoots([URL(fileURLWithPath: "/nonexistent-tilqos-resources")])
        XCTAssertEqual(L10n.tr("general.sound.title"), "general.sound.title")
    }

    func testFormattedTextKeepsPositionalArguments() {
        L10n.setting = .ru
        XCTAssertEqual(L10n.tr("tutorial.progress", 2, 5), "Шаг 2 из 5")
        L10n.setting = .kk
        XCTAssertEqual(L10n.tr("tutorial.progress", 2, 5), "2-қадам, барлығы 5")
        L10n.setting = .en
        XCTAssertEqual(L10n.tr("tutorial.progress", 2, 5), "Step 2 of 5")
    }

    // MARK: - Plurals

    func testRussianPluralCategories() {
        let expected: [(Int, Plural.Category)] = [
            (0, .many), (1, .one), (2, .few), (4, .few), (5, .many), (11, .many), (12, .many),
            (14, .many), (21, .one), (22, .few), (25, .many), (101, .one), (111, .many),
            (112, .many), (1000, .many),
        ]
        for (count, category) in expected {
            XCTAssertEqual(Plural.category(count, in: .ru), category, "\(count)")
        }
    }

    func testEnglishAndKazakhPluralCategories() {
        for lang in [Lang.en, .kk] {
            XCTAssertEqual(Plural.category(1, in: lang), .one)
            XCTAssertEqual(Plural.category(0, in: lang), .other)
            XCTAssertEqual(Plural.category(2, in: lang), .other)
            XCTAssertEqual(Plural.category(21, in: lang), .other)
        }
    }

    func testPluralTextsInEachLanguage() {
        L10n.setting = .en
        XCTAssertEqual(L10n.plural("list.search.prompt", 1), "Search among 1 word")
        XCTAssertEqual(L10n.plural("list.search.prompt", 9), "Search among 9 words")
        L10n.setting = .ru
        XCTAssertEqual(L10n.plural("list.search.prompt", 1), "Поиск среди 1 слова")
        XCTAssertEqual(L10n.plural("list.search.prompt", 3), "Поиск среди 3 слов")
        XCTAssertEqual(L10n.plural("list.search.prompt", 9), "Поиск среди 9 слов")
        XCTAssertEqual(L10n.plural("list.search.prompt", 21), "Поиск среди 21 слова")
        XCTAssertEqual(L10n.plural("correction.minlen.detail", 3),
            "Короче 3 букв: в них слишком мало букв, чтобы судить.")
        L10n.setting = .kk
        XCTAssertEqual(L10n.plural("list.search.prompt", 9), "9 сөз ішінен іздеу")
    }

    // MARK: - Pieces

    @MainActor
    func testRelativeTimeUsesTheChosenLanguage() {
        let now = Date()
        let past = now.addingTimeInterval(-300)
        for lang in UILanguageSetting.allCases where lang != .system {
            L10n.setting = lang
            let text = StatusMenu.ago(past, now: now)
            XCTAssertFalse(text.isEmpty)
            XCTAssertEqual(StatusMenu.ago(now, now: now), L10n.tr("menu.justNow"))
        }
        L10n.setting = .ru
        XCTAssertTrue(StatusMenu.ago(past, now: now).contains("мин"), StatusMenu.ago(past, now: now))
    }

    func testLicenseFilesAreFoundInTheRepository() {
        LicenseFiles.roots = [Self.packageRoot]
        defer { LicenseFiles.roots = nil }
        XCTAssertNotNil(LicenseFiles.url(for: .license))
        XCTAssertNotNil(LicenseFiles.url(for: .notices))
        XCTAssertNotNil(LicenseFiles.folder)
        LicenseFiles.roots = [Self.packageRoot.appendingPathComponent("no-such-folder")]
        XCTAssertNil(LicenseFiles.url(for: .license))
    }

    // MARK: - Live switching and the texts around the app

    /// Changing the language redraws an open Settings window: its views redraw when the model
    /// says it changed, so the model has to say so.
    @MainActor
    func testSettingsModelRedrawsWhenTheLanguageChanges() {
        let scratch = ScratchDefaults.make()
        defer { scratch.remove() }
        let model = SettingsModel(
            prefs: Prefs(defaults: scratch.defaults), inputSources: InputSourceManager(),
            pipeline: nil)
        var redraws = 0
        let subscription = model.objectWillChange.sink { redraws += 1 }
        defer { subscription.cancel() }

        L10n.setting = .ru
        RunLoop.main.run(until: Date().addingTimeInterval(0.1))
        XCTAssertGreaterThan(redraws, 0)
        XCTAssertEqual(SettingsPane.general.title, "Основные")

        // The picker's binding stores the choice and applies it in one go.
        model.uiLanguage.wrappedValue = .kk
        XCTAssertEqual(model.prefs.uiLanguage, .kk)
        XCTAssertEqual(L10n.language, .kk)
        XCTAssertEqual(SettingsPane.general.title, "Негізгі")
    }

    func testInfoPlistDeclaresTheThreeLanguages() throws {
        let url = Self.resources.appendingPathComponent("Info.plist")
        let plist = try XCTUnwrap(NSDictionary(contentsOf: url) as? [String: Any])
        XCTAssertEqual(plist["CFBundleDevelopmentRegion"] as? String, "en")
        XCTAssertEqual(plist["CFBundleLocalizations"] as? [String], ["en", "ru", "kk"])
    }

    /// The DMG install text exists in all three languages and has both variants of step 2.
    func testInstallerTextsExistInAllLanguages() throws {
        for lang in ["en", "ru", "kk"] {
            let url = Self.resources.appendingPathComponent("Installer/Install.\(lang).txt")
            let text = try String(contentsOf: url, encoding: .utf8)
            for marker in ["[[NOT_NOTARIZED]]", "[[/NOT_NOTARIZED]]", "[[NOTARIZED]]", "[[/NOTARIZED]]"] {
                XCTAssertTrue(text.contains(marker), "\(lang): \(marker) is missing")
            }
            for fact in ["5", "2", "10", "64"] {
                XCTAssertTrue(text.contains(fact), "\(lang): the privacy section lacks \(fact)")
            }
        }
    }

    /// Statements that were true of an older version, or never were, must not come back: typed
    /// text is not "never saved", the learned-word hashes are not "irreversible", and password
    /// protection is not "in any app".
    func testNoStalePrivacyClaimsInAnyText() throws {
        let banned = [
            "необратим", "irreversible", "не сохраняется и никуда", "never saved",
            "never written to disk", "никогда не пишется на диск", "в любом приложении",
            "in any app", "ешқашан дискіге жазылмайды", "қайтымсыз", "from seven steps",
            "из семи шагов",
        ]
        var files: [URL] = []
        for lang in ["en", "ru", "kk"] {
            files.append(Self.resources.appendingPathComponent("\(lang).lproj/Localizable.strings"))
            files.append(Self.resources.appendingPathComponent("Installer/Install.\(lang).txt"))
        }
        for url in files {
            let text = try String(contentsOf: url, encoding: .utf8).lowercased()
            for phrase in banned {
                XCTAssertFalse(text.contains(phrase), "\(url.lastPathComponent) says “\(phrase)”")
            }
        }
    }
}


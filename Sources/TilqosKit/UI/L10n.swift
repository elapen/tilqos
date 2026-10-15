import Foundation
import SwiftUI

/// The interface language as the user chose it.
public enum UILanguageSetting: String, CaseIterable, Sendable {
    /// Follow macOS: the first of the user's preferred languages that is English, Russian or
    /// Kazakh, and English when none of them is.
    case system
    case en
    case ru
    case kk

    /// The language this setting pins, or nil for "same as system".
    public var pinned: Lang? {
        switch self {
        case .system: return nil
        case .en: return .en
        case .ru: return .ru
        case .kk: return .kk
        }
    }
}

/// Everything the user reads goes through here.
///
/// Strings live in `Resources/<lang>.lproj/Localizable.strings` (copied into the app bundle
/// by `build.sh`, like the models). Keys are semantic (`general.enabled.title`), English is the
/// source language and the fallback, and a key that is missing everywhere is returned as is so
/// the gap is visible instead of blank.
///
/// Why not `NSLocalizedString`/`Bundle.localizedString`: the language can be changed inside
/// the app without a restart, and the system API follows the system's language list instead.
/// The tables are plain dictionaries read once per language, so lookups are cheap enough for
/// SwiftUI bodies and menus.
///
/// Thread-safe: everything is guarded by one lock, and no AppKit/SwiftUI state is touched.
public enum L10n {
    /// Posted (on the thread that changed the setting) when the language in use changes.
    public static let didChange = Notification.Name("TilqosLanguageDidChange")

    // MARK: - State

    private struct State {
        var setting: UILanguageSetting = .system
        /// Overrides `Locale.preferredLanguages`; tests set it.
        var systemLanguages: [String]?
        /// Overrides where the `.lproj` folders are looked for; tests set it.
        var roots: [URL]?
        var tables: [Lang: [String: String]] = [:]
    }

    private static let lock = NSLock()
    nonisolated(unsafe) private static var state = State()

    private static func withState<T>(_ body: (inout State) -> T) -> T {
        lock.lock()
        defer { lock.unlock() }
        return body(&state)
    }

    // MARK: - Language

    /// The setting in force. Assigning posts `didChange` when the language actually changes.
    public static var setting: UILanguageSetting {
        get { withState { $0.setting } }
        set {
            let (before, after) = withState { state -> (Lang, Lang) in
                let before = resolve(state.setting, preferred: state.systemLanguages ?? preferredLanguages)
                state.setting = newValue
                let after = resolve(newValue, preferred: state.systemLanguages ?? preferredLanguages)
                return (before, after)
            }
            if before != after { NotificationCenter.default.post(name: didChange, object: nil) }
        }
    }

    /// The language strings are currently shown in.
    public static var language: Lang {
        withState { resolve($0.setting, preferred: $0.systemLanguages ?? preferredLanguages) }
    }

    private static var preferredLanguages: [String] { Locale.preferredLanguages }

    /// The first language tag (`ru-RU`, `kk_KZ`, `en`) that names English, Russian or Kazakh;
    /// English when none does. This is how macOS itself picks among an app's localisations.
    public static func resolve(system tags: [String]) -> Lang {
        for tag in tags {
            let code = tag.lowercased().split(whereSeparator: { $0 == "-" || $0 == "_" }).first
            if let code, let lang = Lang(languageCode: String(code)) { return lang }
        }
        return .en
    }

    static func resolve(_ setting: UILanguageSetting, preferred: [String]) -> Lang {
        setting.pinned ?? resolve(system: preferred)
    }

    /// For formatting numbers and dates the way the chosen language writes them.
    public static var locale: Locale { Locale(identifier: language.rawValue) }

    // MARK: - Lookup

    /// The text for `key` in the current language, else in English, else the key itself.
    public static func tr(_ key: String) -> String {
        let (lang, tables) = snapshot()
        return tables[lang]?[key] ?? tables[.en]?[key] ?? key
    }

    /// `tr` with `String(format:)` substitutions. Use positional specifiers (`%1$@`) when a
    /// text has more than one, so a translation may reorder them.
    public static func tr(_ key: String, _ arguments: CVarArg...) -> String {
        String(format: tr(key), locale: locale, arguments: arguments)
    }

    /// A text that depends on a count: looks up `key.<category>` for the language's plural
    /// category of `count` (falling back to `.other`) and substitutes the count for `%d`.
    public static func plural(_ key: String, _ count: Int) -> String {
        let (lang, tables) = snapshot()
        let category = Plural.category(count, in: lang)
        let format =
            tables[lang]?["\(key).\(category.rawValue)"] ?? tables[lang]?["\(key).other"]
            ?? tables[.en]?["\(key).\(category.rawValue)"] ?? tables[.en]?["\(key).other"] ?? key
        return String(format: format, locale: locale, count)
    }

    /// Tables for the current language and for English, loading them on first use.
    private static func snapshot() -> (Lang, [Lang: [String: String]]) {
        withState { state in
            let lang = resolve(state.setting, preferred: state.systemLanguages ?? preferredLanguages)
            for needed in [lang, .en] where state.tables[needed] == nil {
                state.tables[needed] = loadTable(needed, roots: state.roots ?? defaultRoots)
            }
            return (lang, state.tables)
        }
    }

    // MARK: - Resources

    /// The app's own resources. A plain run of the binary outside a bundle has none, and
    /// shows keys.
    private static var defaultRoots: [URL] { [Bundle.main.resourceURL].compactMap { $0 } }

    private static func loadTable(_ lang: Lang, roots: [URL]) -> [String: String] {
        for root in roots {
            let url = root.appendingPathComponent("\(lang.rawValue).lproj/Localizable.strings")
            if let table = NSDictionary(contentsOf: url) as? [String: String] { return table }
        }
        return [:]
    }

    /// The whole table of one language, loaded if need be (tests).
    static func table(of lang: Lang) -> [String: String] {
        withState { state in
            if state.tables[lang] == nil {
                state.tables[lang] = loadTable(lang, roots: state.roots ?? defaultRoots)
            }
            return state.tables[lang] ?? [:]
        }
    }

    // MARK: - Test hooks

    /// Points the lookup at other `.lproj` parents and drops what was cached.
    static func useResourceRoots(_ roots: [URL]?) {
        withState { state in
            state.roots = roots
            state.tables = [:]
        }
    }

    /// Replaces `Locale.preferredLanguages` for "same as system".
    static func useSystemLanguages(_ tags: [String]?) {
        withState { $0.systemLanguages = tags }
    }

    /// Back to the defaults: follow the system, look in the app bundle.
    static func reset() {
        withState { state in state = State() }
    }
}

// MARK: - Plural categories

/// The CLDR plural categories the app's three languages need.
enum Plural {
    enum Category: String, CaseIterable {
        case one, few, many, other
    }

    static func category(_ count: Int, in lang: Lang) -> Category {
        let n = abs(count)
        switch lang {
        case .ru:
            if n % 10 == 1 && n % 100 != 11 { return .one }
            if (2...4).contains(n % 10) && !(12...14).contains(n % 100) { return .few }
            return .many
        case .en, .kk:
            return n == 1 ? .one : .other
        }
    }
}

// MARK: - Rich text

extension L10n {
    /// A localised sentence with some parts shown as code: the format uses positional `%1$@`,
    /// `%2$@`… (or plain `%@`) and each is replaced by the matching entry of `codes`, drawn in
    /// a bold monospaced font. The translation decides where the code goes.
    static func richText(_ key: String, codes: [String]) -> Text {
        let format = tr(key)
        guard let pattern = try? NSRegularExpression(pattern: #"%(?:(\d+)\$)?@"#) else {
            return Text(format)
        }
        var result = AttributedString()
        var cursor = format.startIndex
        var sequential = 0
        for match in pattern.matches(in: format, range: NSRange(format.startIndex..., in: format)) {
            guard let range = Range(match.range, in: format) else { continue }
            result.append(AttributedString(String(format[cursor..<range.lowerBound])))
            var index = sequential
            if let positional = Range(match.range(at: 1), in: format), let n = Int(format[positional]) {
                index = n - 1
            }
            sequential += 1
            if codes.indices.contains(index) {
                var code = AttributedString(codes[index])
                code.font = .body.monospaced().weight(.semibold)
                result.append(code)
            }
            cursor = range.upperBound
        }
        result.append(AttributedString(String(format[cursor...])))
        return Text(result)
    }
}

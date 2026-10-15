import Foundation

/// Which Latin spelling of Kazakh to use.
public enum TranslitScheme: String, CaseIterable, Codable, Sendable {
    /// The official alphabet adopted by presidential decree in 2021: 31 letters using
    /// ä ğ ñ ö ū ü ş and the Turkish-style dotted/dotless i pair. One Cyrillic letter to one
    /// Latin letter, so it round-trips almost exactly.
    case official2021

    /// The plain-ASCII spelling people actually type in chat, where nothing carries a
    /// diacritic: `qalaisyz`, `sәlemetsіz` → `salemetsiz`. Lossy by construction — а and ә
    /// both become `a`, and ұ ү у all become `u` — so reading it back needs a language model.
    case practical

    public var displayName: String {
        switch self {
        case .official2021: return L10n.tr("scheme.official2021")
        case .practical: return L10n.tr("scheme.practical")
        }
    }

    public var explanation: String {
        switch self {
        case .official2021: return L10n.tr("scheme.official2021.detail")
        case .practical: return L10n.tr("scheme.practical.detail")
        }
    }
}

/// Letter tables for both directions.
///
/// The Cyrillic → Latin direction is a straight substitution in either scheme. The reverse
/// is only straightforward for the official alphabet; see `Transliterator` for how the
/// practical scheme is decoded.
public enum TranslitTables {
    // MARK: - Official 2021

    /// Cyrillic → official 2021 Latin.
    ///
    /// The nine letters that exist only in Russian loanwords (ё ц ч щ ъ ь э ю я) are outside
    /// the 31-letter standard; the values here are the conventional fallbacks so that mixed
    /// text survives instead of losing characters.
    public static let cyrillicToOfficial: [Character: String] = [
        "а": "a", "ә": "ä", "б": "b", "в": "v", "г": "g", "ғ": "ğ", "д": "d", "е": "e",
        "ж": "j", "з": "z", "и": "i", "й": "i", "к": "k", "қ": "q", "л": "l", "м": "m",
        "н": "n", "ң": "ñ", "о": "o", "ө": "ö", "п": "p", "р": "r", "с": "s", "т": "t",
        "у": "u", "ұ": "ū", "ү": "ü", "ф": "f", "х": "h", "һ": "h", "ш": "ş", "ы": "y",
        "і": "ı",
        // Outside the standard.
        "ё": "io", "ц": "ts", "ч": "ch", "щ": "şş", "ъ": "", "ь": "", "э": "e",
        "ю": "iu", "я": "ia",
    ]

    /// Official 2021 Latin → Cyrillic. Single-valued: the standard is a bijection apart from
    /// `i`, which stands for both и and й, and `h`, which stands for both х and һ.
    public static let officialToCyrillic: [Character: Character] = [
        "a": "а", "ä": "ә", "b": "б", "v": "в", "g": "г", "ğ": "ғ", "d": "д", "e": "е",
        "j": "ж", "z": "з", "i": "и", "ı": "і", "k": "к", "q": "қ", "l": "л", "m": "м",
        "n": "н", "ñ": "ң", "o": "о", "ö": "ө", "p": "п", "r": "р", "s": "с", "t": "т",
        "u": "у", "ū": "ұ", "ü": "ү", "f": "ф", "h": "х", "ş": "ш", "y": "ы",
    ]

    // MARK: - Practical

    /// Cyrillic → practical ASCII Latin. Deliberately many-to-one.
    public static let cyrillicToPractical: [Character: String] = [
        "а": "a", "ә": "a", "б": "b", "в": "v", "г": "g", "ғ": "g", "д": "d", "е": "e",
        "ж": "j", "з": "z", "и": "i", "й": "i", "к": "k", "қ": "q", "л": "l", "м": "m",
        "н": "n", "ң": "n", "о": "o", "ө": "o", "п": "p", "р": "r", "с": "s", "т": "t",
        "у": "u", "ұ": "u", "ү": "u", "ф": "f", "х": "h", "һ": "h", "ш": "sh", "ы": "y",
        "і": "i", "ё": "io", "ц": "ts", "ч": "ch", "щ": "sh", "ъ": "", "ь": "",
        "э": "e", "ю": "iu", "я": "ia",
    ]

    /// Practical Latin → every Cyrillic letter it could stand for, best guess first.
    ///
    /// This is where the ambiguity lives, and it is the reason a lookup table alone cannot
    /// read this spelling back: `u` alone has three readings.
    public static let practicalToCyrillic: [Character: [Character]] = [
        "a": ["а", "ә"],
        "b": ["б"],
        "c": ["с"],
        "d": ["д"],
        "e": ["е"],
        "f": ["ф"],
        "g": ["г", "ғ"],
        "h": ["х", "һ"],
        "i": ["і", "и", "й"],
        "j": ["ж"],
        "k": ["к"],
        "l": ["л"],
        "m": ["м"],
        "n": ["н", "ң"],
        "o": ["о", "ө"],
        "p": ["п"],
        "q": ["қ"],
        "r": ["р"],
        "s": ["с"],
        "t": ["т"],
        "u": ["ұ", "ү", "у"],
        "v": ["в"],
        "w": ["у"],
        "x": ["х"],
        "y": ["ы", "й"],
        "z": ["з"],
    ]

    /// Multi-letter sequences that must be matched before single letters.
    public static let practicalDigraphs: [String: [Character]] = [
        "sh": ["ш"],
        "ch": ["ч"],
        "shch": ["щ"],
    ]

    // MARK: - Russian written in Latin

    /// Latin → Cyrillic for *Russian* words typed in Latin.
    ///
    /// Necessary because real writing code-switches: a Kazakh sentence carries Russian and
    /// English words spelled the same ASCII way, and reading a Russian word such as `Zametki`
    /// under Kazakh rules puts Kazakh-only letters into it. Russian has no vowel harmony and no ә ө ұ ү і қ ғ ң, so it needs a
    /// table of its own — and the two readings are then compared and the better one wins.
    public static let practicalRussianToCyrillic: [Character: [Character]] = [
        "a": ["а"],
        "b": ["б"],
        "c": ["ц", "к"],
        "d": ["д"],
        "e": ["е", "э"],
        "f": ["ф"],
        "g": ["г"],
        "h": ["х"],
        "i": ["и", "й"],
        "j": ["ж", "й"],
        "k": ["к"],
        "l": ["л"],
        "m": ["м"],
        "n": ["н"],
        "o": ["о"],
        "p": ["п"],
        "q": ["к"],
        "r": ["р"],
        "s": ["с"],
        "t": ["т"],
        "u": ["у", "ю"],
        "v": ["в"],
        "w": ["в"],
        "x": ["х"],
        "y": ["ы", "й"],
        "z": ["з"],
    ]

    public static let practicalRussianDigraphs: [String: [Character]] = [
        "sh": ["ш"],
        "ch": ["ч"],
        "zh": ["ж"],
        "kh": ["х"],
        "ts": ["ц"],
        "shch": ["щ"],
        "ya": ["я"],
        "yu": ["ю"],
        "yo": ["ё"],
    ]

    // MARK: - Vowel harmony

    /// Kazakh vowel harmony: a native word draws its vowels from one row, not both. This is
    /// what makes the ambiguous vowels recoverable — once one vowel in a word is pinned, the
    /// rest follow.
    public static let frontVowels: Set<Character> = ["ә", "е", "ө", "ү", "і"]
    public static let backVowels: Set<Character> = ["а", "о", "ұ", "ы"]
    /// Borrowed into both rows, so they say nothing about harmony.
    public static let neutralVowels: Set<Character> = ["и", "у"]

    public static func isVowel(_ character: Character) -> Bool {
        frontVowels.contains(character) || backVowels.contains(character)
            || neutralVowels.contains(character)
    }

    /// ғ and қ belong to back-vowel words, г and к to front-vowel ones. Another free
    /// constraint, and the reason `q` in `qalaisyz` already tells us the word is a back one.
    public static let backConsonants: Set<Character> = ["ғ", "қ"]
    public static let frontConsonants: Set<Character> = ["г", "к"]

    /// Short, high-frequency words spelled out explicitly.
    ///
    /// A character trigram model has almost nothing to work with in a three-letter word, yet
    /// these are the words that appear most often — `ғой` and `гөй` are both perfectly
    /// well-formed Kazakh shapes, and only frequency tells them apart. Listing them is how
    /// real transliterators handle function words.
    public static let practicalWordOverrides: [String: String] = [
        "goi": "ғой", "gana": "ғана", "da": "да", "de": "де", "ta": "та", "te": "те",
        "ma": "ма", "me": "ме", "ba": "ба", "be": "бе", "pa": "па", "pe": "пе",
        "ne": "не", "ol": "ол", "bul": "бұл", "sol": "сол", "osy": "осы", "onda": "онда",
        "men": "мен", "sen": "сен", "biz": "біз", "siz": "сіз", "olar": "олар",
        "kim": "кім", "qai": "қай", "qalai": "қалай", "eken": "екен", "edi": "еді",
        "dep": "деп", "dese": "десе", "bar": "бар", "joq": "жоқ", "jane": "және",
        "ushin": "үшін", "keiin": "кейін", "endi": "енді", "ote": "өте", "oz": "өз",
        "ozi": "өзі", "aga": "аға", "apai": "апай", "koz": "көз", "kop": "көп",
        "alde": "әлде", "baribir": "бәрібір", "kerek": "керек", "sonda": "сонда",
        "sondyqtan": "сондықтан", "birak": "бірақ", "biraq": "бірақ", "tek": "тек",
    ]

    /// Words that stay in Latin: brands, apps and platform names.
    ///
    /// A list rather than a model, because measurement showed a model cannot do this job:
    /// `instagram` scores 3.96 against the real word `kofe` at 4.62, so no cost threshold
    /// separates them. Kept deliberately short — names people genuinely do write in Cyrillic
    /// (яндекс, каспи) are left out, and the learn-from-undo list covers whatever is missing.
    public static let latinOnlyWords: Set<String> = [
        // Apps and platforms
        "instagram", "whatsapp", "telegram", "tiktok", "youtube", "facebook", "twitter",
        "linkedin", "snapchat", "viber", "zoom", "skype", "discord", "reddit", "spotify",
        "netflix", "github", "gitlab", "figma", "notion", "slack", "trello", "jira", "canva",
        "google", "gmail", "chatgpt", "claude",
        // Hardware and software
        "iphone", "ipad", "macbook", "imac", "airpods", "android", "windows", "macos",
        "linux", "chrome", "safari", "firefox", "wifi", "bluetooth", "airdrop", "icloud",
        // Point of sale and delivery systems that show up in this kind of note
        "iiko", "drinkit", "poster", "rkeeper", "syrve", "wolt", "glovo", "bolt", "uber",
        // Payments
        "visa", "mastercard", "paypal", "stripe", "applepay",
        // File and web jargon
        "email", "url", "api", "pdf", "html", "css", "sql", "json", "http", "https",
    ]

    /// True when a word is a Latin-only name, allowing for a short attached suffix so
    /// `instagramda` (instagram + да) is recognised too.
    public static func isLatinOnly(_ word: String, extra: Set<String> = []) -> Bool {
        let lower = word.lowercased()
        if latinOnlyWords.contains(lower) || extra.contains(lower) { return true }
        // A known name plus up to four letters of Kazakh or Russian case ending.
        for length in stride(from: lower.count - 1, through: 4, by: -1) {
            guard lower.count - length <= 4 else { continue }
            let base = String(lower.prefix(length))
            if latinOnlyWords.contains(base) || extra.contains(base) { return true }
        }
        return false
    }

    /// Words where һ is correct rather than х. There are only a handful in Kazakh, so an
    /// explicit list beats asking a statistical model to guess.
    public static let hehWords: Set<String> = [
        "қаһарман", "гауһар", "жауһар", "шаһар", "аһ", "уһ", "жиһаз", "жиһан",
        "қаһар", "маһаббат", "шаһзада", "аллаһ", "таһарат",
    ]
}

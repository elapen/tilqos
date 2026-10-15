import Carbon.HIToolbox
import Foundation

/// The character each key produces under one keyboard layout, both directions.
///
/// Built at runtime from the layout data macOS itself ships, never hardcoded, so
/// the app works with whatever layouts the user has installed now or adds later.
public struct KeyMapTable: Sendable, Identifiable {
    /// Virtual key codes on macOS top out at 0x7E.
    public static let keyCodeCount = 128

    /// Modifier combinations ordinary typing actually uses.
    ///
    /// Option is excluded on purpose. Layouts diverge wildly on their Option layer —
    /// Kazakh puts Ukrainian ґ/є/ї there where Russian puts other symbols — but those
    /// characters never appear in a mistyped word, so comparing them would drown the
    /// signal we care about.
    public static let typingVariants: [ModState] = [[], .shift, .caps, [.shift, .caps]]

    /// The numeric keypad. It produces the same digits on every layout, so it must
    /// never win when resolving a character back to a keystroke — on a laptop those
    /// keys do not even exist.
    static let keypadKeyCodes: Set<UInt16> = {
        var codes: Set<UInt16> = [
            UInt16(kVK_ANSI_KeypadDecimal), UInt16(kVK_ANSI_KeypadMultiply),
            UInt16(kVK_ANSI_KeypadPlus), UInt16(kVK_ANSI_KeypadClear),
            UInt16(kVK_ANSI_KeypadDivide), UInt16(kVK_ANSI_KeypadEnter),
            UInt16(kVK_ANSI_KeypadMinus), UInt16(kVK_ANSI_KeypadEquals),
        ]
        for code in UInt16(kVK_ANSI_Keypad0)...UInt16(kVK_ANSI_Keypad9) { codes.insert(code) }
        return codes
    }()

    public let sourceID: String
    public let localizedName: String
    public let lang: Lang?

    /// `forward[stroke.tableIndex]` — the text a keystroke produces, or nil.
    private let forward: [String?]
    /// Whether that text is part of a word. Precomputed: the question is asked for every
    /// keystroke, on the tap's time budget, and for every stroke of every evaluation.
    private let wordStrokes: [Bool]
    /// Cheapest keystroke that produces a given character.
    private let reverse: [Character: KeyStroke]

    public var id: String { sourceID }

    public init(sourceID: String, localizedName: String, lang: Lang?, forward: [String?]) {
        precondition(
            forward.count == Self.keyCodeCount * ModState.variantCount,
            "forward table must be keyCodeCount * variantCount entries"
        )
        self.sourceID = sourceID
        self.localizedName = localizedName
        self.lang = lang
        self.forward = forward
        self.wordStrokes = forward.map { text in
            guard let text, !text.isEmpty else { return false }
            return text.allSatisfy(Tokenizer.isWordCharacter)
        }

        var reverse: [Character: KeyStroke] = [:]
        reverse.reserveCapacity(128)
        // Main block first, keypad only as a last resort; within each, the cheapest
        // modifier combination wins. So a character reachable both plainly and through
        // Option resolves to the plain keystroke, and "5" resolves to the number row
        // rather than to keypad 5.
        for keypadPass in [false, true] {
            for mods in ModState.byCost {
                for keyCode in 0..<Self.keyCodeCount {
                    let code = UInt16(keyCode)
                    guard Self.keypadKeyCodes.contains(code) == keypadPass else { continue }
                    let stroke = KeyStroke(keyCode: code, mods: mods)
                    guard let text = forward[stroke.tableIndex], text.count == 1 else { continue }
                    let ch = text[text.startIndex]
                    if reverse[ch] == nil { reverse[ch] = stroke }
                }
            }
        }
        self.reverse = reverse
    }

    // MARK: - Rendering

    /// The text a single keystroke produces under this layout.
    public func text(for stroke: KeyStroke) -> String? {
        guard stroke.tableIndex < forward.count else { return nil }
        return forward[stroke.tableIndex]
    }

    /// Whether the keystroke produces part of a word — a letter, a digit or an apostrophe.
    public func producesWordCharacter(_ stroke: KeyStroke) -> Bool {
        guard stroke.tableIndex < wordStrokes.count else { return false }
        return wordStrokes[stroke.tableIndex]
    }

    /// Re-renders a keystroke sequence under this layout. Keys that produce nothing
    /// here are dropped.
    public func render(_ strokes: [KeyStroke]) -> String {
        var out = ""
        out.reserveCapacity(strokes.count)
        for stroke in strokes {
            if let text = text(for: stroke) { out += text }
        }
        return out
    }

    // MARK: - Reverse

    /// The keystroke a user would press on this layout to get `character`.
    public func stroke(for character: Character) -> KeyStroke? {
        reverse[character]
    }

    /// Recovers the keystroke sequence behind already-typed text. Returns nil if any
    /// character is unreachable on this layout, which means the text did not come
    /// from here.
    public func strokes(for text: String) -> [KeyStroke]? {
        var out: [KeyStroke] = []
        out.reserveCapacity(text.count)
        for ch in text {
            guard let stroke = reverse[ch] else { return nil }
            out.append(stroke)
        }
        return out
    }

    /// Best-effort variant of `strokes(for:)` that keeps going past unmappable
    /// characters, reporting which positions it could not resolve.
    public func partialStrokes(for text: String) -> (strokes: [KeyStroke?], unresolved: Int) {
        var out: [KeyStroke?] = []
        var unresolved = 0
        out.reserveCapacity(text.count)
        for ch in text {
            let stroke = reverse[ch]
            if stroke == nil { unresolved += 1 }
            out.append(stroke)
        }
        return (out, unresolved)
    }

    // MARK: - Comparison

    /// Key codes whose character differs between two layouts.
    ///
    /// With the default `typingVariants`, Russian vs Kazakh returns exactly
    /// {10, 18...29, 50}: every letter key is identical. That is why confusing those
    /// two layouts only ever damages the Kazakh-specific letters and the digits, and
    /// never a single Russian letter.
    ///
    /// The caller is expected to cache the result — it walks the whole table.
    public func differingKeyCodes(
        from other: KeyMapTable,
        variants: [ModState] = KeyMapTable.typingVariants
    ) -> Set<UInt16> {
        var result: Set<UInt16> = []
        for keyCode in 0..<Self.keyCodeCount {
            for mods in variants {
                let index = keyCode * ModState.variantCount + Int(mods.rawValue)
                if forward[index] != other.forward[index] {
                    result.insert(UInt16(keyCode))
                    break
                }
            }
        }
        return result
    }
}

// MARK: - Building from Text Input Services

extension KeyMapTable {
    /// Reads a layout's character table out of macOS.
    ///
    /// Returns nil for input methods that expose no Unicode key layout (Emoji picker,
    /// Press-and-Hold, CJK input methods) — those cannot be re-rendered.
    public static func build(from source: TISInputSource) -> KeyMapTable? {
        guard let sourceID = stringProperty(source, kTISPropertyInputSourceID) else { return nil }
        guard let layoutPointer = TISGetInputSourceProperty(source, kTISPropertyUnicodeKeyLayoutData)
        else { return nil }

        let name = stringProperty(source, kTISPropertyLocalizedName) ?? sourceID
        let lang = primaryLanguage(source).flatMap(Lang.init(languageCode:))
        let data = Unmanaged<CFData>.fromOpaque(layoutPointer).takeUnretainedValue() as Data

        var forward = [String?](repeating: nil, count: keyCodeCount * ModState.variantCount)
        let keyboardType = UInt32(LMGetKbdType())

        data.withUnsafeBytes { raw in
            guard let layout = raw.bindMemory(to: UCKeyboardLayout.self).baseAddress else { return }
            var buffer = [UniChar](repeating: 0, count: 8)
            for keyCode in 0..<keyCodeCount {
                for variant in 0..<ModState.variantCount {
                    let mods = ModState(rawValue: UInt8(variant))
                    var deadKeyState: UInt32 = 0
                    var length = 0
                    let status = UCKeyTranslate(
                        layout,
                        UInt16(keyCode),
                        UInt16(kUCKeyActionDown),
                        mods.ucKeyModifiers,
                        keyboardType,
                        OptionBits(kUCKeyTranslateNoDeadKeysBit),
                        &deadKeyState,
                        buffer.count,
                        &length,
                        &buffer
                    )
                    guard status == noErr, length > 0 else { continue }
                    let text = String(utf16CodeUnits: buffer, count: length)
                    // Return, Tab, Escape and Delete report a character here but never
                    // take part in a word. Space is kept — it is the word boundary.
                    if let scalar = text.unicodeScalars.first,
                        text.unicodeScalars.count == 1,
                        scalar.value < 0x20 || scalar.value == 0x7F
                    {
                        continue
                    }
                    forward[keyCode * ModState.variantCount + variant] = text
                }
            }
        }

        return KeyMapTable(
            sourceID: sourceID, localizedName: name, lang: lang, forward: forward)
    }

    static func stringProperty(_ source: TISInputSource, _ key: CFString!) -> String? {
        guard let pointer = TISGetInputSourceProperty(source, key) else { return nil }
        return Unmanaged<CFString>.fromOpaque(pointer).takeUnretainedValue() as String
    }

    /// First entry of `kTISPropertyInputSourceLanguages` — "ru" for Russian, "kk" for
    /// Kazakh, "en" for ABC.
    static func primaryLanguage(_ source: TISInputSource) -> String? {
        guard let pointer = TISGetInputSourceProperty(source, kTISPropertyInputSourceLanguages)
        else { return nil }
        let languages = Unmanaged<CFArray>.fromOpaque(pointer).takeUnretainedValue() as? [String]
        return languages?.first
    }
}

import Carbon.HIToolbox
import CoreGraphics
import Foundation

public struct HotkeyModifiers: OptionSet, Codable, Hashable, Sendable {
    public let rawValue: UInt8
    public init(rawValue: UInt8) { self.rawValue = rawValue }

    public static let shift = HotkeyModifiers(rawValue: 1 << 0)
    public static let control = HotkeyModifiers(rawValue: 1 << 1)
    public static let option = HotkeyModifiers(rawValue: 1 << 2)
    public static let command = HotkeyModifiers(rawValue: 1 << 3)

    public init(cgFlags: CGEventFlags) {
        var result = HotkeyModifiers()
        if cgFlags.contains(.maskShift) { result.insert(.shift) }
        if cgFlags.contains(.maskControl) { result.insert(.control) }
        if cgFlags.contains(.maskAlternate) { result.insert(.option) }
        if cgFlags.contains(.maskCommand) { result.insert(.command) }
        self = result
    }

    public var display: String {
        var text = ""
        if contains(.control) { text += "⌃" }
        if contains(.option) { text += "⌥" }
        if contains(.shift) { text += "⇧" }
        if contains(.command) { text += "⌘" }
        return text
    }
}

public struct Hotkey: Codable, Hashable, Sendable {
    public var keyCode: UInt16
    public var modifiers: HotkeyModifiers

    public init(keyCode: UInt16, modifiers: HotkeyModifiers) {
        self.keyCode = keyCode
        self.modifiers = modifiers
    }

    /// A bare key with no modifiers would fire while typing, so at least one is required.
    public var isValid: Bool { !modifiers.isEmpty }

    public func matches(keyCode: UInt16, flags: CGEventFlags) -> Bool {
        self.keyCode == keyCode && modifiers == HotkeyModifiers(cgFlags: flags)
    }

    public var display: String {
        modifiers.display + (Hotkey.keyNames[keyCode] ?? "key\(keyCode)")
    }

    static let keyNames: [UInt16: String] = [
        UInt16(kVK_ANSI_A): "A", UInt16(kVK_ANSI_F): "F", UInt16(kVK_ANSI_L): "L",
        UInt16(kVK_ANSI_S): "S", UInt16(kVK_ANSI_T): "T", UInt16(kVK_ANSI_V): "V",
        UInt16(kVK_ANSI_Z): "Z",
        UInt16(kVK_ANSI_1): "1", UInt16(kVK_ANSI_2): "2", UInt16(kVK_ANSI_3): "3",
        UInt16(kVK_Space): "Space", UInt16(kVK_Tab): "Tab", UInt16(kVK_Escape): "Esc",
        UInt16(kVK_ANSI_Grave): "`",
    ]
}

public enum HotkeyAction: String, CaseIterable, Codable, Sendable {
    case fixLastWord
    case fixSelection
    case undoLastFix
    case forceEnglish
    case forceRussian
    case forceKazakh
    case convertClipboard
    case transliterate
    case toggleEnabled

    public var displayName: String {
        switch self {
        case .fixLastWord: return L10n.tr("hotkey.fixLastWord")
        case .fixSelection: return L10n.tr("hotkey.fixSelection")
        case .undoLastFix: return L10n.tr("hotkey.undoLastFix")
        case .forceEnglish: return L10n.tr("hotkey.switchTo", Lang.en.displayName)
        case .forceRussian: return L10n.tr("hotkey.switchTo", Lang.ru.displayName)
        case .forceKazakh: return L10n.tr("hotkey.switchTo", Lang.kk.displayName)
        case .convertClipboard: return L10n.tr("hotkey.convertClipboard")
        case .transliterate: return L10n.tr("hotkey.transliterate")
        case .toggleEnabled: return L10n.tr("hotkey.toggleEnabled")
        }
    }

    /// Option+Shift combinations, chosen because macOS and most apps leave them free.
    public var defaultHotkey: Hotkey {
        let optionShift: HotkeyModifiers = [.option, .shift]
        switch self {
        case .fixLastWord: return Hotkey(keyCode: UInt16(kVK_ANSI_F), modifiers: optionShift)
        case .fixSelection: return Hotkey(keyCode: UInt16(kVK_ANSI_S), modifiers: optionShift)
        case .undoLastFix: return Hotkey(keyCode: UInt16(kVK_ANSI_Z), modifiers: optionShift)
        case .forceEnglish: return Hotkey(keyCode: UInt16(kVK_ANSI_1), modifiers: optionShift)
        case .forceRussian: return Hotkey(keyCode: UInt16(kVK_ANSI_2), modifiers: optionShift)
        case .forceKazakh: return Hotkey(keyCode: UInt16(kVK_ANSI_3), modifiers: optionShift)
        case .convertClipboard: return Hotkey(keyCode: UInt16(kVK_ANSI_V), modifiers: optionShift)
        case .transliterate: return Hotkey(keyCode: UInt16(kVK_ANSI_T), modifiers: optionShift)
        case .toggleEnabled: return Hotkey(keyCode: UInt16(kVK_ANSI_L), modifiers: optionShift)
        }
    }
}

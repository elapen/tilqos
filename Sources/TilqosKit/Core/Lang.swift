import Foundation

/// Writing system. The first cascade of the detector decides only this.
public enum Script: Sendable, Hashable {
    case latin
    case cyrillic
}

/// The three languages the detector models.
///
/// Layouts whose language is outside this set still get key tables built (so
/// rendering and manual conversion work), but they are not scoring candidates —
/// there is no dictionary or trigram model for them.
public enum Lang: String, CaseIterable, Codable, Sendable {
    case en
    case ru
    case kk

    public var script: Script {
        switch self {
        case .en: return .latin
        case .ru, .kk: return .cyrillic
        }
    }

    /// Two-to-three character badge for the menu bar.
    public var badge: String {
        switch self {
        case .en: return "EN"
        case .ru: return "РУ"
        case .kk: return "ҚАЗ"
        }
    }

    public var displayName: String {
        switch self {
        case .en: return "English"
        case .ru: return "Русский"
        case .kk: return "Қазақша"
        }
    }

    /// Maps a `kTISPropertyInputSourceLanguages` code onto a modelled language.
    public init?(languageCode: String) {
        switch languageCode.lowercased() {
        case "en": self = .en
        case "ru": self = .ru
        case "kk": self = .kk
        default: return nil
        }
    }
}

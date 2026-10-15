import AppKit
import Carbon.HIToolbox
import SwiftUI

/// The app's own colours, so the windows, the hint panel and the icon read as one thing.
///
/// Used sparingly — for the mark and for the one action the hint panel asks for. Everything
/// the user interacts with in windows follows the system accent colour instead, like any
/// other Mac app.
enum Brand {
    static let start = Color(red: 0.478, green: 0.502, blue: 1.0)
    static let end = Color(red: 0.208, green: 0.157, blue: 0.706)
    static var gradient: LinearGradient {
        LinearGradient(colors: [start, end], startPoint: .topLeading, endPoint: .bottomTrailing)
    }

    /// Solid brand indigo, dark enough for white text on top to stay legible: about 5.8:1,
    /// above the 4.5:1 that small text needs.
    static let accent = NSColor(srgbRed: 0.318, green: 0.310, blue: 0.902, alpha: 1)
}

/// The real bundle icon, so windows match what the user sees in Finder and in the System
/// Settings list they are about to be sent to.
struct AppIconView: View {
    let size: CGFloat

    var body: some View {
        Image(nsImage: NSApp.applicationIconImage)
            .resizable()
            .interpolation(.high)
            .frame(width: size, height: size)
            .accessibilityHidden(true)
    }
}

// MARK: - Keycaps

/// A key drawn as a key, so shortcuts read as something to press rather than as text.
struct Keycap: View {
    let text: String
    var prominent = false

    var body: some View {
        Text(text)
            .font(.system(size: 11.5, weight: .medium).monospacedDigit())
            .foregroundStyle(prominent ? AnyShapeStyle(.white) : AnyShapeStyle(.primary))
            .padding(.horizontal, 6)
            .frame(minWidth: 22, minHeight: 20)
            .background(
                RoundedRectangle(cornerRadius: 5, style: .continuous)
                    .fill(
                        prominent
                            ? AnyShapeStyle(Color(nsColor: Brand.accent))
                            : AnyShapeStyle(.background.opacity(0.9)))
            )
            .overlay(
                RoundedRectangle(cornerRadius: 5, style: .continuous)
                    .strokeBorder(prominent ? .clear : Color.primary.opacity(0.14), lineWidth: 1)
            )
            .shadow(color: .black.opacity(prominent ? 0 : 0.06), radius: 0, y: 1)
            .accessibilityLabel(KeyNames.spoken(text))
    }
}

/// A whole shortcut as one keycap: `⌥⇧Z` reads faster than three separate keys.
struct ShortcutKeycap: View {
    let hotkey: Hotkey?

    var body: some View {
        if let hotkey {
            Keycap(text: KeyNames.display(hotkey))
        }
    }
}

// MARK: - Sidebar icons

/// White glyph on a coloured rounded square, the way System Settings marks its panes.
struct SettingsIcon: View {
    let symbol: String
    let color: Color
    var size: CGFloat = 22

    var body: some View {
        RoundedRectangle(cornerRadius: size * 0.26, style: .continuous)
            .fill(color.gradient)
            .frame(width: size, height: size)
            .overlay(
                Image(systemName: symbol)
                    .font(.system(size: size * 0.55, weight: .semibold))
                    .foregroundStyle(.white)
            )
            .accessibilityHidden(true)
    }
}

// MARK: - Status

/// Overall state of the app, derived once and shown the same way everywhere — menu,
/// status icon, settings.
public enum AppHealth: Equatable, Sendable {
    /// Correcting as configured.
    case working
    /// Running, but automatic detection is off; only the hotkeys act.
    case hotkeysOnly
    /// Switched off by the user.
    case disabled
    /// Can read keys but cannot replace text: suggestions only.
    case suggestionsOnly
    /// Cannot read the keyboard at all.
    case noPermission
    /// Permission is there but the keyboard tap is not running.
    case tapStopped

    public static func evaluate(
        enabled: Bool, smartMode: Bool, permissions: Permissions.Status, tapRunning: Bool
    ) -> AppHealth {
        guard permissions.canDetect else { return .noPermission }
        guard tapRunning else { return .tapStopped }
        guard enabled else { return .disabled }
        guard permissions.canApply else { return .suggestionsOnly }
        return smartMode ? .working : .hotkeysOnly
    }

    /// Needs the user to do something before the app can work.
    public var needsAttention: Bool {
        switch self {
        case .noPermission, .tapStopped, .suggestionsOnly: return true
        case .working, .hotkeysOnly, .disabled: return false
        }
    }

    public var title: String {
        switch self {
        case .working: return L10n.tr("health.working")
        case .hotkeysOnly: return L10n.tr("health.hotkeysOnly")
        case .disabled: return L10n.tr("health.disabled")
        case .suggestionsOnly: return L10n.tr("health.suggestionsOnly")
        case .noPermission: return L10n.tr("health.noPermission")
        case .tapStopped: return L10n.tr("health.tapStopped")
        }
    }

    var color: NSColor {
        switch self {
        case .working: return .systemGreen
        case .hotkeysOnly: return .systemBlue
        case .disabled: return .secondaryLabelColor
        case .suggestionsOnly, .noPermission, .tapStopped: return .systemOrange
        }
    }
}

/// Coloured dot plus a word, for places where state has to be readable at a glance.
struct StatusLabel: View {
    let health: AppHealth

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(Color(nsColor: health.color))
                .frame(width: 8, height: 8)
            Text(health.title)
                .foregroundStyle(
                    health.needsAttention ? Color(nsColor: health.color) : .secondary)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(L10n.tr("status.label", health.title))
    }
}

// MARK: - Key names

/// Human names for key codes.
///
/// The core's own table only covers the default bindings; a recorded shortcut can use any
/// key, and showing it as `key37` would be useless. Letters use their ABC-layout name,
/// which is how shortcuts are conventionally written on a Mac whatever the active layout.
enum KeyNames {
    static let names: [Int: String] = [
        kVK_ANSI_A: "A", kVK_ANSI_B: "B", kVK_ANSI_C: "C", kVK_ANSI_D: "D", kVK_ANSI_E: "E",
        kVK_ANSI_F: "F", kVK_ANSI_G: "G", kVK_ANSI_H: "H", kVK_ANSI_I: "I", kVK_ANSI_J: "J",
        kVK_ANSI_K: "K", kVK_ANSI_L: "L", kVK_ANSI_M: "M", kVK_ANSI_N: "N", kVK_ANSI_O: "O",
        kVK_ANSI_P: "P", kVK_ANSI_Q: "Q", kVK_ANSI_R: "R", kVK_ANSI_S: "S", kVK_ANSI_T: "T",
        kVK_ANSI_U: "U", kVK_ANSI_V: "V", kVK_ANSI_W: "W", kVK_ANSI_X: "X", kVK_ANSI_Y: "Y",
        kVK_ANSI_Z: "Z",
        kVK_ANSI_0: "0", kVK_ANSI_1: "1", kVK_ANSI_2: "2", kVK_ANSI_3: "3", kVK_ANSI_4: "4",
        kVK_ANSI_5: "5", kVK_ANSI_6: "6", kVK_ANSI_7: "7", kVK_ANSI_8: "8", kVK_ANSI_9: "9",
        kVK_ANSI_Minus: "-", kVK_ANSI_Equal: "=", kVK_ANSI_LeftBracket: "[",
        kVK_ANSI_RightBracket: "]", kVK_ANSI_Semicolon: ";", kVK_ANSI_Quote: "'",
        kVK_ANSI_Comma: ",", kVK_ANSI_Period: ".", kVK_ANSI_Slash: "/",
        kVK_ANSI_Backslash: "\\", kVK_ANSI_Grave: "`", kVK_ISO_Section: "§",
        kVK_Space: "Space", kVK_Tab: "Tab", kVK_Return: "↩", kVK_Escape: "Esc",
        kVK_Delete: "⌫", kVK_ForwardDelete: "⌦",
        kVK_LeftArrow: "←", kVK_RightArrow: "→", kVK_UpArrow: "↑", kVK_DownArrow: "↓",
        kVK_Home: "↖", kVK_End: "↘", kVK_PageUp: "⇞", kVK_PageDown: "⇟",
        kVK_F1: "F1", kVK_F2: "F2", kVK_F3: "F3", kVK_F4: "F4", kVK_F5: "F5", kVK_F6: "F6",
        kVK_F7: "F7", kVK_F8: "F8", kVK_F9: "F9", kVK_F10: "F10", kVK_F11: "F11",
        kVK_F12: "F12",
    ]

    static func key(_ keyCode: UInt16) -> String? {
        names[Int(keyCode)]
    }

    /// `⌥⇧Z`, in the order macOS menus use.
    static func display(_ hotkey: Hotkey) -> String {
        hotkey.modifiers.display + (key(hotkey.keyCode) ?? "?")
    }

    /// What a menu's `keyEquivalent` needs for this key, when it can draw it at all.
    static func menuKeyEquivalent(_ keyCode: UInt16) -> String? {
        switch Int(keyCode) {
        case kVK_Space: return " "
        case kVK_Tab: return "\t"
        case kVK_Return: return "\r"
        case kVK_Escape: return "\u{1b}"
        case kVK_Delete: return "\u{8}"
        default:
            guard let name = key(keyCode), name.count == 1 else { return nil }
            return name.lowercased()
        }
    }

    /// VoiceOver reads `⌥⇧Z` as "Z"; spell the modifiers out instead.
    static func spoken(_ text: String) -> String {
        var parts: [String] = []
        var rest = Substring(text)
        let modifiers: [(Character, String)] = [
            ("⌃", "Control"), ("⌥", "Option"), ("⇧", "Shift"), ("⌘", "Command"),
        ]
        while let first = rest.first, let match = modifiers.first(where: { $0.0 == first }) {
            parts.append(match.1)
            rest = rest.dropFirst()
        }
        let key = rest.trimmingCharacters(in: .whitespaces)
        switch key {
        case "⇥", "Tab": parts.append("Tab")
        case "␣", "Space": parts.append(L10n.tr("key.space.spoken"))
        case "": break
        default: parts.append(key)
        }
        return parts.joined(separator: " ")
    }
}

// MARK: - Windows

/// Keeps the Dock icon in step with our windows.
///
/// A menu bar app has to become a regular one for its windows to take focus, and should go
/// back as soon as the last of them closes — but not before, or closing Settings would
/// yank focus from the tutorial behind it.
@MainActor
enum AppWindows {
    static func present(_ window: NSWindow) {
        NSApp.setActivationPolicy(.regular)
        NSApp.activate()
        window.makeKeyAndOrderFront(nil)
    }

    /// Call from `windowWillClose`; the closing window still counts as visible there.
    static func windowWillClose(_ closing: NSWindow) {
        let others = NSApp.windows.filter {
            $0 !== closing && $0.isVisible && $0.styleMask.contains(.titled)
                && !($0 is NSPanel)
        }
        if others.isEmpty {
            NSApp.setActivationPolicy(.accessory)
        }
    }
}

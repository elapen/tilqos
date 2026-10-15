import Foundation

/// Per-application behaviour: how much the app may do, and which Cyrillic layout it
/// should return to.
public final class AppRules {
    /// Never touched, and not removable from the list. Typing into a password manager is
    /// exactly the case where a helpful correction is a security problem.
    public static let alwaysDisabled: Set<String> = [
        "com.apple.keychainaccess",
        "com.1password.1password",
        "com.1password.1password7",
        "com.agilebits.onepassword7",
        "com.bitwarden.desktop",
        "org.keepassxc.keepassxc",
        "com.dashlane.dashlanephonefinal",
        "in.sinew.Enpass-Desktop",
        "com.apple.SecurityAgent",
        "com.apple.loginwindow",
    ]

    /// Shipped defaults for apps where automatic rewriting is more nuisance than help.
    public static let defaultSuggestOnly: Set<String> = [
        "com.apple.Terminal",
        "com.googlecode.iterm2",
        "com.mitchellh.ghostty",
        "net.kovidgoyal.kitty",
        "dev.warp.Warp-Stable",
        "org.alacritty",
    ]

    /// Apps where Tab is load-bearing — completion, indentation, cell navigation. The
    /// accept key falls back to the secondary binding in these.
    public static let tabCritical: Set<String> = [
        "com.apple.Terminal",
        "com.googlecode.iterm2",
        "com.mitchellh.ghostty",
        "net.kovidgoyal.kitty",
        "dev.warp.Warp-Stable",
        "org.alacritty",
        "com.microsoft.VSCode",
        "com.microsoft.VSCodeInsiders",
        "com.visualstudio.code.oss",
        "com.todesktop.230313mzl4w4u92",
        "com.jetbrains.intellij",
        "com.jetbrains.pycharm",
        "com.sublimetext.4",
        "com.apple.dt.Xcode",
        "com.microsoft.Excel",
        "com.apple.iWork.Numbers",
    ]

    private let prefs: Prefs

    public init(prefs: Prefs = .shared) {
        self.prefs = prefs
    }

    // MARK: - Mode

    public func mode(for bundleID: String?) -> FixMode {
        guard let bundleID else { return prefs.defaultFixMode }
        if Self.alwaysDisabled.contains(bundleID) { return .off }
        if let raw = prefs.appRules[bundleID], let mode = FixMode(rawValue: raw) { return mode }
        if Self.defaultSuggestOnly.contains(bundleID) { return .suggest }
        return prefs.defaultFixMode
    }

    public func setMode(_ mode: FixMode?, for bundleID: String) {
        guard !Self.alwaysDisabled.contains(bundleID) else { return }
        var rules = prefs.appRules
        if let mode {
            rules[bundleID] = mode.rawValue
        } else {
            rules.removeValue(forKey: bundleID)
        }
        prefs.appRules = rules
    }

    public func isLocked(_ bundleID: String) -> Bool {
        Self.alwaysDisabled.contains(bundleID)
    }

    public func isTabCritical(_ bundleID: String?) -> Bool {
        guard let bundleID else { return false }
        return Self.tabCritical.contains(bundleID)
    }

    // MARK: - Cyrillic memory

    /// Remembers which Cyrillic layout was last used in an app.
    ///
    /// This is the whole answer to "Russian or Kazakh?" when the letters cannot tell them
    /// apart. Rather than guess, the app keeps using whichever one this app was last
    /// written in, and only moves when a word gives positive evidence.
    public func rememberCyrillic(layoutID: String, for bundleID: String?) {
        guard prefs.rememberLayoutPerApp, let bundleID else { return }
        var memory = prefs.appCyrillicMemory
        guard memory[bundleID] != layoutID else { return }
        memory[bundleID] = layoutID
        prefs.appCyrillicMemory = memory
    }

    public func rememberedCyrillic(for bundleID: String?) -> String? {
        guard prefs.rememberLayoutPerApp, let bundleID else { return nil }
        return prefs.appCyrillicMemory[bundleID]
    }
}

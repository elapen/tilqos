import ApplicationServices
import AppKit
import CoreGraphics

/// The two TCC grants this app needs, and the deep links to go grant them.
///
/// They are genuinely separate: reading keystrokes needs Input Monitoring, while
/// swallowing a key, injecting a replacement, or reading the caret through the
/// accessibility tree needs Accessibility. With only the first, the app can still
/// detect mistakes and show suggestions — it just cannot apply them.
public enum Permissions {
    // MARK: - Input Monitoring

    public static var hasInputMonitoring: Bool {
        CGPreflightListenEventAccess()
    }

    /// Prompts once. macOS shows its own dialog and returns the answer it already has;
    /// a denial is remembered until the user changes it in System Settings.
    @discardableResult
    public static func requestInputMonitoring() -> Bool {
        CGRequestListenEventAccess()
    }

    // MARK: - Accessibility

    public static var hasAccessibility: Bool {
        AXIsProcessTrusted()
    }

    /// Triggers the system prompt by asking the trust question with prompting enabled.
    @discardableResult
    public static func requestAccessibility() -> Bool {
        let key = kAXTrustedCheckOptionPrompt.takeUnretainedValue()
        let options = [key: true] as CFDictionary
        return AXIsProcessTrustedWithOptions(options)
    }

    // MARK: - Settings deep links

    public static func openInputMonitoringSettings() {
        open("x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent")
    }

    public static func openAccessibilitySettings() {
        open("x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
    }

    private static func open(_ urlString: String) {
        guard let url = URL(string: urlString) else { return }
        NSWorkspace.shared.open(url)
    }

    // MARK: - Summary

    public struct Status: Equatable, Sendable {
        public var inputMonitoring: Bool
        public var accessibility: Bool

        /// Either grant is enough to read keystrokes.
        ///
        /// Accessibility subsumes event-tap listening — an app that has it does not need
        /// Input Monitoring as well, and macOS will not even list it under Input Monitoring
        /// once Accessibility is on. Treating Input Monitoring as mandatory was wrong and
        /// stopped the app from ever starting.
        public var canDetect: Bool { inputMonitoring || accessibility }

        /// Applying a correction means writing through AX or posting events, both of which
        /// are Accessibility.
        public var canApply: Bool { accessibility }
    }

    public static var status: Status {
        Status(inputMonitoring: hasInputMonitoring, accessibility: hasAccessibility)
    }
}

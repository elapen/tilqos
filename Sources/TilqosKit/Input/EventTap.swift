import Carbon.HIToolbox
import CoreGraphics
import Foundation

/// Identifies events this app posted itself.
///
/// Without this, injecting a correction would feed the replacement text straight back
/// into the tap and the app would correct its own output forever.
public enum EventMarker {
    /// Written into `eventSourceUserData` on everything we post.
    public static let magic: Int64 = 0x5449_4C51_4F53_0001  // "TILQOS" + version

    public static func isOurs(_ event: CGEvent) -> Bool {
        event.getIntegerValueField(.eventSourceUserData) == magic
    }

    /// A source configured to stamp our marker onto every event created from it.
    public static func makeSource() -> CGEventSource? {
        guard let source = CGEventSource(stateID: .privateState) else { return nil }
        source.userData = magic
        return source
    }
}

/// Watches the keyboard through a `CGEventTap`.
///
/// Two constraints shape everything here. First, the callback runs on the path of
/// every keystroke in the system and has a hard time budget — blow it and macOS
/// silently disables the tap — so the callback does no real work beyond a handful of
/// comparisons. Second, the tap can be disabled behind our back, so its health is
/// polled and it is rebuilt when needed.
public final class EventTap {
    /// What the tap should do with a keystroke it just reported.
    public enum Verdict {
        /// Deliver the event to the app the user is typing into.
        case pass
        /// Consume it. Only ever used for the suggestion accept key.
        case swallow
    }

    /// Why the keystroke buffer is being discarded. Anything that means "the text I was
    /// accumulating no longer describes what is on screen".
    public enum ResetReason: String {
        case mouseClick
        case appSwitch
        case shortcut
        case navigationKey
        /// Escape: like other navigation keys it ends the line of thought, and it is also the
        /// user's explicit "no" to a suggestion on screen.
        case escape
        case secureInput
        case idle
        case disabled
        case applied
    }

    /// Consulted before anything else on a key press. Returning true swallows the event.
    ///
    /// Hotkeys are matched here rather than through Carbon's `RegisterEventHotKey`: the
    /// tap already sees every key with its modifiers, so a second global mechanism would
    /// be redundant. It also means hotkeys share the tap's fate, which is honest — the
    /// whole app needs Input Monitoring anyway.
    public var onHotkey: ((KeyStroke, CGEventFlags) -> Bool)?

    /// Called for every text-producing key press. Returns whether to swallow it.
    public var onStroke: ((KeyStroke) -> Verdict)?
    /// Called when the accumulated buffer must be thrown away.
    public var onReset: ((ResetReason) -> Void)?
    /// Called when a plain Delete is pressed, so the buffer can drop its last stroke.
    public var onBackspace: (() -> Void)?

    private var machPort: CFMachPort?
    private var runLoopSource: CFRunLoopSource?
    private var watchdog: Timer?
    private let prefs: Prefs

    /// Set while we are posting our own events, as a belt-and-braces guard next to the
    /// source marker: some apps re-post events through their own source, which strips
    /// the marker but happens inside this window.
    public var isInjecting = false

    private var secureInputActive = false

    /// Whether secure event input is on, i.e. some app has a password field in front. Replaceable
    /// so tests can simulate it without switching the real thing on for the whole machine.
    var secureInputProbe: () -> Bool = { IsSecureEventInputEnabled() }

    /// True while a password field (or anything else that asks for secure input) is in front.
    /// While it is true the tap reads nothing, `Pipeline` reads nothing, and nothing is logged.
    public var isSecureInputActive: Bool { secureInputProbe() }

    public init(prefs: Prefs = .shared) {
        self.prefs = prefs
    }

    deinit {
        stop()
    }

    // MARK: - Lifecycle

    public var isRunning: Bool {
        guard let machPort else { return false }
        return CGEvent.tapIsEnabled(tap: machPort)
    }

    /// Installs the tap. Returns false when Input Monitoring has not been granted —
    /// the caller is expected to send the user through onboarding rather than retry.
    @discardableResult
    public func start() -> Bool {
        guard machPort == nil else { return true }

        // Deliberately no preflight gate. Whether a tap can be created depends on Input
        // Monitoring *or* Accessibility, and the preflight for one says nothing about the
        // other, so the only trustworthy check is trying it.
        let status = Permissions.status
        Log.info(
            .tap,
            "starting tap: inputMonitoring=\(status.inputMonitoring) "
                + "accessibility=\(status.accessibility)")

        let mask: CGEventMask =
            (1 << CGEventType.keyDown.rawValue)
            | (1 << CGEventType.flagsChanged.rawValue)
            | (1 << CGEventType.leftMouseDown.rawValue)
            | (1 << CGEventType.rightMouseDown.rawValue)

        // .defaultTap rather than .listenOnly: swallowing the accept key requires the
        // ability to modify the stream, which is also what pulls in the Accessibility
        // grant on top of Input Monitoring.
        guard
            let port = CGEvent.tapCreate(
                tap: .cgSessionEventTap,
                place: .headInsertEventTap,
                options: .defaultTap,
                eventsOfInterest: mask,
                callback: eventTapCallback,
                userInfo: Unmanaged.passUnretained(self).toOpaque()
            )
        else {
            Log.error(.tap, "CGEvent.tapCreate returned nil")
            return false
        }

        machPort = port
        let source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, port, 0)
        runLoopSource = source
        CFRunLoopAddSource(CFRunLoopGetMain(), source, .commonModes)
        CGEvent.tapEnable(tap: port, enable: true)

        startWatchdog()
        Log.info(.tap, "tap installed")
        return true
    }

    public func stop() {
        watchdog?.invalidate()
        watchdog = nil
        if let runLoopSource {
            CFRunLoopRemoveSource(CFRunLoopGetMain(), runLoopSource, .commonModes)
        }
        runLoopSource = nil
        if let machPort {
            CGEvent.tapEnable(tap: machPort, enable: false)
            CFMachPortInvalidate(machPort)
        }
        machPort = nil
    }

    /// Tears the tap down and builds a new one. Used when the code identity changed
    /// under us, which leaves a tap that looks installed but never fires.
    public func restart() {
        Log.warn(.tap, "restarting tap")
        stop()
        _ = start()
    }

    // MARK: - Watchdog

    /// A tap can be disabled without the callback ever being told: a timeout that
    /// arrives while the process is suspended, or a TCC re-evaluation after the binary
    /// is re-signed. Polling is the only reliable way to notice.
    private func startWatchdog() {
        watchdog?.invalidate()
        let timer = Timer(timeInterval: 5.0, repeats: true) { [weak self] _ in
            self?.checkHealth()
        }
        // Tolerance lets the timer coalesce with other wakeups so an idle app really is idle.
        timer.tolerance = 1.0
        RunLoop.main.add(timer, forMode: .common)
        watchdog = timer
    }

    private func checkHealth() {
        guard let machPort else { return }
        guard !CGEvent.tapIsEnabled(tap: machPort) else { return }
        Log.warn(.tap, "watchdog found tap disabled; re-enabling")
        CGEvent.tapEnable(tap: machPort, enable: true)
        if !CGEvent.tapIsEnabled(tap: machPort) {
            restart()
        }
    }

    // MARK: - Callback

    fileprivate func handle(
        proxy: CGEventTapProxy, type: CGEventType, event: CGEvent
    ) -> Unmanaged<CGEvent>? {
        process(type, event)
    }

    /// What the tap does with one event: the event to deliver, or nil to swallow it.
    ///
    /// Internal rather than private so the one decision everything else rests on — that a
    /// swallowed key really is swallowed — can be tested without a live tap.
    func process(_ type: CGEventType, _ event: CGEvent) -> Unmanaged<CGEvent>? {
        let passthrough = Unmanaged.passUnretained(event)

        // macOS telling us it pulled the plug. Re-arm immediately; these carry no key.
        if type == .tapDisabledByTimeout || type == .tapDisabledByUserInput {
            Log.warn(.tap, "tap disabled by \(type == .tapDisabledByTimeout ? "timeout" : "user input")")
            if let machPort { CGEvent.tapEnable(tap: machPort, enable: true) }
            onReset?(.disabled)
            return nil
        }

        // Our own injected events, and anything posted while we are injecting.
        if isInjecting || EventMarker.isOurs(event) { return passthrough }

        // A password field is focused. Stop looking at keys entirely and drop whatever
        // was buffered — this is the one case where correctness means doing nothing. That
        // includes the log: when a password field was in front is itself something to keep
        // to ourselves, so neither this nor the end of it is written anywhere.
        if secureInputProbe() {
            if !secureInputActive {
                secureInputActive = true
                onReset?(.secureInput)
            }
            return passthrough
        }
        secureInputActive = false

        switch type {
        case .leftMouseDown, .rightMouseDown:
            guard prefs.enabled else { return passthrough }
            // The caret moved somewhere we cannot see. The buffer no longer describes
            // the text in front of it.
            onReset?(.mouseClick)
            return passthrough

        case .flagsChanged:
            return passthrough

        case .keyDown:
            // The verdict is explicit on purpose. This used to be `handleKeyDown(...) ??
            // passthrough` with nil meaning "swallow" — and the `??` turned every swallow back
            // into a pass. The accept key then reached the app alongside the correction (a Tab
            // in the text, or focus moving away mid-edit), and every hotkey typed its character.
            return handleKeyDown(event: event) == .swallow ? nil : passthrough

        default:
            return passthrough
        }
    }

    private func handleKeyDown(event: CGEvent) -> Verdict {
        let keyCode = UInt16(event.getIntegerValueField(.keyboardEventKeycode))
        let flags = event.flags

        // Hotkeys first: some of them deliberately carry Command, which the next check
        // would otherwise treat as somebody else's shortcut. And before the master switch,
        // or the hotkey that turns the app back on could never be heard.
        if let onHotkey, onHotkey(KeyStroke(keyCode: keyCode, mods: ModState(cgFlags: flags)), flags)
        {
            Log.debug(.tap, "hotkey: swallowed")
            return .swallow
        }

        guard prefs.enabled else { return .pass }

        // Command or Control means this is a shortcut, not text. Whatever the shortcut
        // does — paste, undo, select-all — the buffer stops matching the screen.
        if flags.contains(.maskCommand) || flags.contains(.maskControl) {
            onReset?(.shortcut)
            return .pass
        }

        if keyCode == UInt16(kVK_Delete) {
            onBackspace?()
            return .pass
        }

        if keyCode == UInt16(kVK_Escape) {
            onReset?(.escape)
            return .pass
        }

        if Self.navigationKeyCodes.contains(keyCode) {
            onReset?(.navigationKey)
            return .pass
        }

        let stroke = KeyStroke(keyCode: keyCode, mods: ModState(cgFlags: flags))
        return onStroke?(stroke) ?? .pass
    }

    /// Keys that move the caret or commit the line. Each invalidates the buffer.
    static let navigationKeyCodes: Set<UInt16> = [
        UInt16(kVK_Return), UInt16(kVK_ANSI_KeypadEnter), UInt16(kVK_Escape),
        UInt16(kVK_LeftArrow), UInt16(kVK_RightArrow), UInt16(kVK_UpArrow),
        UInt16(kVK_DownArrow), UInt16(kVK_Home), UInt16(kVK_End),
        UInt16(kVK_PageUp), UInt16(kVK_PageDown), UInt16(kVK_ForwardDelete),
    ]
}

/// Free function so the C function pointer carries no Swift context of its own.
private func eventTapCallback(
    proxy: CGEventTapProxy,
    type: CGEventType,
    event: CGEvent,
    userInfo: UnsafeMutableRawPointer?
) -> Unmanaged<CGEvent>? {
    guard let userInfo else { return Unmanaged.passUnretained(event) }
    let tap = Unmanaged<EventTap>.fromOpaque(userInfo).takeUnretainedValue()
    return tap.handle(proxy: proxy, type: type, event: event)
}

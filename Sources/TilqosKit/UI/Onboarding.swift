import AppKit
import SwiftUI

/// Polls the two TCC grants while the onboarding window is open.
///
/// Polling rather than observing, because macOS gives no notification when a permission is
/// granted in System Settings — the app simply starts working, and the user needs to see
/// that happen.
@MainActor
final class PermissionWatcher: ObservableObject {
    @Published var status = Permissions.status
    private var timer: Timer?
    private var languageObserver: NSObjectProtocol?

    init() {
        // The text in the window follows the interface language; the view only redraws on
        // changes of this object.
        languageObserver = NotificationCenter.default.addObserver(
            forName: L10n.didChange, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.objectWillChange.send() }
        }
    }

    deinit {
        if let languageObserver { NotificationCenter.default.removeObserver(languageObserver) }
    }

    /// Called once when replacing text becomes possible.
    var onReady: (() -> Void)?

    func start() {
        stop()
        let timer = Timer(timeInterval: 1.0, repeats: true) { [weak self] _ in
            MainActor.assumeIsolated { self?.refresh() }
        }
        RunLoop.main.add(timer, forMode: .common)
        self.timer = timer
        refresh()
    }

    func stop() {
        timer?.invalidate()
        timer = nil
    }

    private func refresh() {
        let latest = Permissions.status
        guard latest != status else { return }
        let wasReady = status.canApply
        status = latest
        if latest.canApply && !wasReady { onReady?() }
    }
}

/// One task, shown as the three things to do in System Settings — including a picture of
/// the exact row to switch on, because "find the app in the list" is where people get lost.
struct OnboardingView: View {
    @ObservedObject var watcher: PermissionWatcher
    let openSettings: () -> Void
    let onFinish: () -> Void

    private var granted: Bool { watcher.status.canApply }

    var body: some View {
        VStack(spacing: 0) {
            VStack(spacing: 18) {
                header
                if granted {
                    ready.transition(.opacity)
                } else {
                    steps.transition(.opacity)
                }
            }
            .padding(.horizontal, 28)
            .padding(.top, 34)
            .padding(.bottom, 20)
            // The short "done" state sits in the middle rather than under an empty half-window.
            .frame(maxHeight: .infinity, alignment: granted ? .center : .top)

            footer
        }
        .frame(width: 460, height: 520)
        .animation(.easeInOut(duration: 0.25), value: granted)
    }

    private var header: some View {
        VStack(spacing: 10) {
            ZStack(alignment: .bottomTrailing) {
                AppIconView(size: 72)
                if granted {
                    Image(systemName: "checkmark.circle.fill")
                        .font(.system(size: 24))
                        .symbolRenderingMode(.palette)
                        .foregroundStyle(.white, .green)
                        .background(Circle().fill(.background).padding(2))
                        .offset(x: 4, y: 4)
                        .transition(.scale.combined(with: .opacity))
                }
            }
            Text(granted ? L10n.tr("onboarding.title.ready") : L10n.tr("onboarding.title.missing"))
                .font(.title2.weight(.semibold))
                .multilineTextAlignment(.center)
            Text(
                granted
                    ? L10n.tr("onboarding.subtitle.ready")
                    : L10n.tr("onboarding.subtitle.missing")
            )
            .foregroundStyle(.secondary)
            .multilineTextAlignment(.center)
            .fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .combine)
    }

    private var steps: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(spacing: 0) {
                StepRow(number: 1, title: L10n.tr("onboarding.step1")) {
                    Button(L10n.tr("onboarding.step1.button"), action: openSettings)
                        .controlSize(.regular)
                        .keyboardShortcut(.defaultAction)
                }
                Divider().padding(.leading, 44)
                StepRow(number: 2, title: L10n.tr("onboarding.step2")) { EmptyView() }
                SettingsRowMock()
                    .padding(.leading, 44)
                    .padding(.trailing, 14)
                    .padding(.bottom, 12)
                Divider().padding(.leading, 44)
                StepRow(number: 3, title: L10n.tr("onboarding.step3")) {
                    ProgressView().controlSize(.small)
                        .accessibilityLabel(L10n.tr("onboarding.waiting"))
                }
            }
            .background(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .fill(.background.secondary))
            .overlay(
                RoundedRectangle(cornerRadius: 12, style: .continuous)
                    .strokeBorder(.separator.opacity(0.6)))

            // Wraps: the Russian and Kazakh texts do not fit on one line.
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Image(systemName: "lock.fill")
                Text(L10n.tr("onboarding.privacy"))
                    .fixedSize(horizontal: false, vertical: true)
            }
            .font(.callout)
            .foregroundStyle(.secondary)

            DisclosureGroup(L10n.tr("onboarding.trouble")) {
                VStack(alignment: .leading, spacing: 6) {
                    Text(L10n.tr("onboarding.trouble.missing"))
                    Text(L10n.tr("onboarding.trouble.on"))
                    Text(L10n.tr("onboarding.trouble.input"))
                }
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
                .padding(.top, 6)
            }
            .font(.callout)
        }
    }

    private var ready: some View {
        VStack(spacing: 10) {
            Label(L10n.tr("onboarding.ready.title"), systemImage: "checkmark.shield.fill")
                .font(.headline)
                .foregroundStyle(.green)
            Text(L10n.tr("onboarding.ready.text"))
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
        }
        .padding(.vertical, 18)
        .frame(maxWidth: .infinity)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(Color.green.opacity(0.08)))
    }

    private var footer: some View {
        VStack(spacing: 0) {
            Divider()
            HStack {
                if !granted {
                    Text(L10n.tr("onboarding.footer"))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                if granted {
                    Button(L10n.tr("onboarding.start"), action: onFinish)
                        .keyboardShortcut(.defaultAction)
                        .controlSize(.large)
                } else {
                    Button(L10n.tr("onboarding.later"), action: onFinish)
                        .controlSize(.large)
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
    }
}

private struct StepRow<Accessory: View>: View {
    let number: Int
    let title: String
    @ViewBuilder let accessory: () -> Accessory

    var body: some View {
        HStack(spacing: 12) {
            Text("\(number)")
                .font(.system(size: 12, weight: .bold).monospacedDigit())
                .foregroundStyle(.white)
                .frame(width: 20, height: 20)
                .background(Circle().fill(Color.accentColor))
                .accessibilityHidden(true)
            Text(title)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 8)
            accessory()
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 11)
        .accessibilityElement(children: .contain)
        .accessibilityLabel(L10n.tr("onboarding.step.label", number, title))
    }
}

/// A drawing of the row the user is looking for in System Settings — icon, name, switch —
/// so it can be recognised at a glance in a long list.
private struct SettingsRowMock: View {
    var body: some View {
        HStack(spacing: 8) {
            AppIconView(size: 20)
            Text("Tilqos").font(.callout)
            Spacer()
            Capsule()
                .fill(Color.accentColor)
                .frame(width: 30, height: 18)
                .overlay(alignment: .trailing) {
                    Circle().fill(.white).padding(2).shadow(radius: 0.5)
                }
        }
        .padding(.horizontal, 10)
        .padding(.vertical, 7)
        .background(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .fill(.background))
        .overlay(
            RoundedRectangle(cornerRadius: 8, style: .continuous)
                .strokeBorder(.separator.opacity(0.8)))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(L10n.tr("onboarding.mock.label"))
    }
}

@MainActor
public final class OnboardingWindowController: NSWindowController, NSWindowDelegate {
    private let watcher = PermissionWatcher()
    private let onGranted: () -> Void
    private let onClose: () -> Void
    private var didFinish = false
    private var languageObserver: NSObjectProtocol?

    /// `onGranted` fires as soon as the permission appears, so the app can start working
    /// straight away; `onClose` fires once, when the window goes.
    public init(onGranted: @escaping () -> Void, onClose: @escaping () -> Void) {
        self.onGranted = onGranted
        self.onClose = onClose
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 460, height: 520),
            styleMask: [.titled, .closable, .fullSizeContentView],
            backing: .buffered,
            defer: false)
        window.title = L10n.tr("onboarding.window")
        window.titleVisibility = .hidden
        window.titlebarAppearsTransparent = true
        window.isMovableByWindowBackground = true
        window.isReleasedWhenClosed = false
        super.init(window: window)
        window.delegate = self

        window.contentView = NSHostingView(
            rootView: OnboardingView(
                watcher: watcher,
                openSettings: { [weak self] in self?.openSystemSettings() },
                onFinish: { [weak self] in self?.close() }))
        window.center()
        languageObserver = NotificationCenter.default.addObserver(
            forName: L10n.didChange, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.window?.title = L10n.tr("onboarding.window") }
        }
        watcher.onReady = { [weak self] in
            self?.onGranted()
            // Bring the window back from behind System Settings, so the user sees it worked.
            if let window = self?.window { AppWindows.present(window) }
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not used")
    }

    private var didRequestThisLaunch = false

    public func present() {
        watcher.start()
        guard let window else { return }
        AppWindows.present(window)
        registerWithTCC()
    }

    /// Asks for Accessibility as soon as onboarding opens.
    ///
    /// Asking is the *only* thing that puts an app into the System Settings list — the
    /// preflight checks deliberately do not, so without this there is no row for the user to
    /// switch on. Only Accessibility is requested: it covers everything the app does, and
    /// prompting for Input Monitoring as well would put up a second dialog for a permission
    /// that Accessibility already subsumes.
    private func registerWithTCC() {
        guard !didRequestThisLaunch else { return }
        didRequestThisLaunch = true
        guard !Permissions.status.accessibility else { return }
        _ = Permissions.requestAccessibility()
    }

    private func openSystemSettings() {
        Permissions.requestAccessibility()
        Permissions.openAccessibilitySettings()
    }

    public func windowWillClose(_ notification: Notification) {
        guard !didFinish else { return }
        didFinish = true
        watcher.stop()
        Prefs.shared.didCompleteOnboarding = true
        if let window { AppWindows.windowWillClose(window) }
        if let languageObserver { NotificationCenter.default.removeObserver(languageObserver) }
        languageObserver = nil
        // After the window has gone, so a tutorial opened in response is not closed with it.
        DispatchQueue.main.async { [onClose] in onClose() }
    }
}

import AppKit
import ServiceManagement
import SwiftUI

/// One page of the tutorial.
struct TutorialStep: Identifiable {
    let id: Int
    let title: String
}

@MainActor
final class TutorialModel: ObservableObject {
    @Published var current = 0
    /// Text typed into each practice field, keyed by step.
    @Published var practice: [Int: String] = [:]
    @Published private(set) var layoutName: String?
    @Published private(set) var layoutLang: Lang?

    var steps: [TutorialStep] {
        [
            TutorialStep(id: 0, title: L10n.tr("tutorial.step0")),
            TutorialStep(id: 1, title: L10n.tr("tutorial.step1")),
            TutorialStep(id: 2, title: L10n.tr("tutorial.step2")),
            TutorialStep(id: 3, title: L10n.tr("tutorial.step3")),
            TutorialStep(id: 4, title: L10n.tr("tutorial.step4")),
        ]
    }

    private let inputSources: InputSourceManager?
    let prefs: Prefs
    private var languageObserver: NSObjectProtocol?

    init(inputSources: InputSourceManager? = nil, prefs: Prefs = .shared) {
        self.inputSources = inputSources
        self.prefs = prefs
        refreshLayout()
        languageObserver = NotificationCenter.default.addObserver(
            forName: L10n.didChange, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.objectWillChange.send() }
        }
    }

    deinit {
        if let languageObserver { NotificationCenter.default.removeObserver(languageObserver) }
    }

    var isFirst: Bool { current == 0 }
    var isLast: Bool { current == 4 }

    func next() { if !isLast { current += 1 } }
    func back() { if !isFirst { current -= 1 } }

    func refreshLayout() {
        let table = inputSources?.current
        layoutName = table?.localizedName
        layoutLang = table?.lang
    }

    func canSwitch(to lang: Lang) -> Bool {
        inputSources?.table(for: lang) != nil
    }

    func switchLayout(to lang: Lang) {
        inputSources?.select(lang: lang)
        refreshLayout()
    }

    func hotkey(_ action: HotkeyAction) -> Hotkey? {
        prefs.hotkeys[action]
    }

    var launchAtLogin: Bool {
        get { SMAppService.mainApp.status == .enabled }
        set {
            do {
                newValue
                    ? try SMAppService.mainApp.register()
                    : try SMAppService.mainApp.unregister()
            } catch {
                Log.error(.ui, "launch at login: \(error.localizedDescription)")
            }
            objectWillChange.send()
        }
    }
}

struct TutorialView: View {
    @ObservedObject var model: TutorialModel
    let onFinish: () -> Void

    var body: some View {
        VStack(spacing: 0) {
            progress
                .padding(.top, 40)
                .padding(.horizontal, 32)

            VStack(alignment: .leading, spacing: 18) {
                switch model.current {
                case 0: wrongLayout
                case 1: kazakh
                case 2: suggestions
                case 3: transliteration
                default: done
                }
            }
            .padding(.horizontal, 32)
            .padding(.top, 22)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .topLeading)
            .id(model.current)
            .transition(.opacity)

            footer
        }
        .frame(width: 580, height: 540)
        .animation(.easeInOut(duration: 0.2), value: model.current)
    }

    // MARK: - Progress

    private var progress: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 6) {
                ForEach(model.steps) { step in
                    Capsule()
                        .fill(
                            step.id <= model.current
                                ? AnyShapeStyle(Color.accentColor)
                                : AnyShapeStyle(Color.secondary.opacity(0.2))
                        )
                        .frame(height: 4)
                        // Doubles as navigation for anyone who wants to skip ahead.
                        .contentShape(Rectangle().inset(by: -6))
                        .onTapGesture { model.current = step.id }
                        .accessibilityElement()
                        .accessibilityLabel(L10n.tr("tutorial.progress.label", step.id + 1, step.title))
                        .accessibilityAddTraits(step.id == model.current ? [.isSelected, .isButton] : .isButton)
                        .accessibilityAction { model.current = step.id }
                }
            }
            Text(L10n.tr("tutorial.progress", model.current + 1, model.steps.count))
                .font(.caption)
                .foregroundStyle(.secondary)
        }
    }

    private func heading(_ title: String, _ subtitle: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(.title.weight(.bold))
                .accessibilityAddTraits(.isHeader)
            Text(subtitle)
                .font(.body)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    // MARK: - Steps

    private var wrongLayout: some View {
        VStack(alignment: .leading, spacing: 18) {
            heading(L10n.tr("tutorial.s0.heading"), L10n.tr("tutorial.s0.text"))
            BeforeAfter(before: "ghbdtn", after: "привет")
            PracticeCard(
                model: model, step: 0, expected: "привет", layout: .en,
                instruction: L10n.richText("tutorial.typeAndSpace", codes: ["ghbdtn"]))
        }
    }

    private var kazakh: some View {
        VStack(alignment: .leading, spacing: 18) {
            heading(L10n.tr("tutorial.s1.heading"), L10n.tr("tutorial.s1.text"))
            BeforeAfter(before: "с2леметс3з", after: "сәлеметсіз")
            PracticeCard(
                model: model, step: 1, expected: "сәлеметсіз", layout: .ru,
                instruction: L10n.richText("tutorial.typeAndSpace", codes: ["с2леметс3з"]))
        }
    }

    private var suggestions: some View {
        VStack(alignment: .leading, spacing: 18) {
            heading(L10n.tr("tutorial.s2.heading"), L10n.tr("tutorial.s2.text"))
            HintPreview(
                content: .init(
                    before: "ghbdtn", after: "привет",
                    style: .suggestion(acceptKey: model.prefs.acceptKey.keycapTitle)))
                .shadow(color: .black.opacity(0.12), radius: 8, y: 3)
                .padding(.leading, 2)
            VStack(alignment: .leading, spacing: 12) {
                KeyFact(
                    key: model.prefs.acceptKey.keycapTitle, prominent: true,
                    text: L10n.tr("tutorial.s2.accept"))
                KeyFact(
                    key: model.hotkey(.undoLastFix).map(KeyNames.display) ?? "⌥⇧Z",
                    text: L10n.tr("tutorial.s2.undo"))
            }
            Text(L10n.tr("tutorial.s2.note"))
                .font(.callout)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private var transliteration: some View {
        VStack(alignment: .leading, spacing: 18) {
            heading(
                L10n.tr("tutorial.s3.heading"),
                L10n.tr(
                    "tutorial.s3.text",
                    model.hotkey(.transliterate).map(KeyNames.display) ?? "⌥⇧T"))
            BeforeAfter(before: "qalaisyz", after: "қалайсыз")
            PracticeCard(
                model: model, step: 3, expected: "қалайсыз", layout: .en,
                instruction: L10n.richText(
                    "tutorial.s3.instruction",
                    codes: [
                        "qalaisyz", "⌘A", model.hotkey(.transliterate).map(KeyNames.display) ?? "⌥⇧T",
                    ]))
        }
    }

    private var done: some View {
        VStack(alignment: .leading, spacing: 18) {
            heading(L10n.tr("tutorial.s4.heading"), L10n.tr("tutorial.s4.text"))
            HStack(spacing: 10) {
                Image(nsImage: StatusIcon.keycap("РУ"))
                    .renderingMode(.template)
                    .foregroundStyle(.primary)
                    .accessibilityHidden(true)
                Text(L10n.tr("tutorial.s4.icon"))
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 9)
            .background(
                RoundedRectangle(cornerRadius: 9, style: .continuous).fill(.quaternary.opacity(0.5)))

            VStack(spacing: 0) {
                ForEach(
                    [
                        (HotkeyAction.undoLastFix, HotkeysPane.title(for: .undoLastFix)),
                        (.fixLastWord, HotkeysPane.title(for: .fixLastWord)),
                        (.transliterate, HotkeysPane.title(for: .transliterate)),
                        (.toggleEnabled, HotkeysPane.title(for: .toggleEnabled)),
                    ], id: \.0
                ) { action, title in
                    HStack {
                        Text(title)
                        Spacer()
                        ShortcutKeycap(hotkey: model.hotkey(action))
                    }
                    .padding(.vertical, 7)
                    .padding(.horizontal, 12)
                    if action != .toggleEnabled { Divider().padding(.leading, 12) }
                }
            }
            .background(
                RoundedRectangle(cornerRadius: 10, style: .continuous).fill(.background.secondary))
            .overlay(
                RoundedRectangle(cornerRadius: 10, style: .continuous)
                    .strokeBorder(.separator.opacity(0.6)))

            Toggle(
                L10n.tr("tutorial.s4.launch"),
                isOn: Binding(get: { model.launchAtLogin }, set: { model.launchAtLogin = $0 }))
        }
    }

    // MARK: - Footer

    private var footer: some View {
        VStack(spacing: 0) {
            Divider()
            HStack {
                if !model.isFirst {
                    Button(L10n.tr("tutorial.back")) { model.back() }
                        .controlSize(.large)
                } else {
                    Button(L10n.tr("tutorial.skip"), action: onFinish)
                        .controlSize(.large)
                        .buttonStyle(.borderless)
                        .foregroundStyle(.secondary)
                }
                Spacer()
                if model.isLast {
                    Button(L10n.tr("tutorial.finish"), action: onFinish)
                        .keyboardShortcut(.defaultAction)
                        .controlSize(.large)
                } else {
                    Button(L10n.tr("tutorial.next")) { model.next() }
                        .keyboardShortcut(.defaultAction)
                        .controlSize(.large)
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 14)
        }
    }
}

// MARK: - Pieces

private struct KeyFact: View {
    let key: String
    var prominent = false
    let text: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            Keycap(text: key, prominent: prominent)
                .frame(minWidth: 56, alignment: .leading)
            Text(text)
                .fixedSize(horizontal: false, vertical: true)
        }
    }
}

/// `before → after`, the shape the app's own suggestion card uses.
private struct BeforeAfter: View {
    let before: String
    let after: String

    var body: some View {
        HStack(spacing: 12) {
            Text(before)
                .font(.system(.title3, design: .monospaced))
                .foregroundStyle(.secondary)
                .strikethrough(color: .secondary.opacity(0.7))
            Image(systemName: "arrow.right")
                .font(.callout.weight(.semibold))
                .foregroundStyle(.tertiary)
            Text(after)
                .font(.system(.title3, design: .monospaced).weight(.semibold))
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous).fill(.background.secondary))
        .overlay(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .strokeBorder(.separator.opacity(0.6)))
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(L10n.tr("tutorial.becomes", before, after))
    }
}

/// A real text field driven by the real engine.
///
/// Nothing here is simulated: the event tap is system-wide, so it sees this field like any
/// other and corrects it for real. That is the whole point — practising against a mock would
/// prove nothing about whether the app works on this machine.
private struct PracticeCard: View {
    @ObservedObject var model: TutorialModel
    let step: Int
    let expected: String
    /// The layout the exercise has to be typed in. Being on the wrong one is the usual
    /// reason practice "does not work", so the card says which one is active and offers to
    /// switch.
    let layout: Lang
    let instruction: Text

    @FocusState private var focused: Bool

    private var text: Binding<String> {
        Binding(
            get: { model.practice[step] ?? "" },
            set: { model.practice[step] = $0 })
    }

    private var succeeded: Bool {
        text.wrappedValue.localizedCaseInsensitiveContains(expected)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 7) {
                Image(systemName: succeeded ? "checkmark.circle.fill" : "keyboard")
                    .foregroundStyle(succeeded ? Color.green : Color.accentColor)
                if succeeded {
                    Text(L10n.tr("tutorial.success")).fontWeight(.semibold).foregroundStyle(.green)
                } else {
                    instruction
                }
                Spacer(minLength: 0)
            }

            TextField("", text: text, prompt: Text(L10n.tr("tutorial.practice.prompt")))
                .textFieldStyle(.plain)
                .font(.system(.title3, design: .monospaced))
                .focused($focused)
                .padding(.horizontal, 10)
                .padding(.vertical, 8)
                .background(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .fill(Color(nsColor: .textBackgroundColor)))
                .overlay(
                    RoundedRectangle(cornerRadius: 8, style: .continuous)
                        .strokeBorder(
                            succeeded
                                ? Color.green
                                : (focused ? Color.accentColor : Color.secondary.opacity(0.3)),
                            lineWidth: succeeded || focused ? 2 : 1))
                .accessibilityLabel(L10n.tr("tutorial.practice.label"))

            if !succeeded { layoutStatus }
        }
        .padding(14)
        .background(
            RoundedRectangle(cornerRadius: 12, style: .continuous)
                .fill(succeeded ? Color.green.opacity(0.07) : Color.accentColor.opacity(0.06)))
        .animation(.easeInOut(duration: 0.2), value: succeeded)
        .onAppear { focused = true }
    }

    @ViewBuilder private var layoutStatus: some View {
        if model.layoutLang == layout {
            Label(L10n.tr("tutorial.layout.ok", layout.displayName), systemImage: "checkmark")
                .font(.callout)
                .foregroundStyle(.secondary)
        } else {
            HStack(spacing: 8) {
                Label(
                    model.layoutName.map {
                        L10n.tr("tutorial.layout.needNow", layout.displayName, $0)
                    } ?? L10n.tr("tutorial.layout.need", layout.displayName),
                    systemImage: "exclamationmark.circle"
                )
                .foregroundStyle(.orange)
                Spacer(minLength: 0)
                if model.canSwitch(to: layout) {
                    Button(L10n.tr("tutorial.layout.switch")) {
                        model.switchLayout(to: layout)
                        focused = true
                    }
                    .controlSize(.small)
                }
            }
            .font(.callout)
        }
    }
}

// MARK: - Window

@MainActor
public final class TutorialWindowController: NSWindowController, NSWindowDelegate {
    private let model: TutorialModel
    private let onFinish: () -> Void
    private var languageObserver: NSObjectProtocol?

    public init(
        inputSources: InputSourceManager, prefs: Prefs = .shared,
        onFinish: @escaping () -> Void
    ) {
        self.model = TutorialModel(inputSources: inputSources, prefs: prefs)
        self.onFinish = onFinish
        let window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 580, height: 540),
            styleMask: [.titled, .closable, .fullSizeContentView],
            backing: .buffered,
            defer: false)
        window.title = L10n.tr("tutorial.window")
        window.titleVisibility = .hidden
        window.titlebarAppearsTransparent = true
        window.isMovableByWindowBackground = true
        window.isReleasedWhenClosed = false
        super.init(window: window)
        window.delegate = self

        window.contentView = NSHostingView(
            rootView: TutorialView(model: model) { [weak self] in self?.close() })
        window.center()
        languageObserver = NotificationCenter.default.addObserver(
            forName: L10n.didChange, object: nil, queue: .main
        ) { [weak self] _ in
            MainActor.assumeIsolated { self?.window?.title = L10n.tr("tutorial.window") }
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not used")
    }

    public func present() {
        // A tutorial with typing practice needs real keyboard focus, so the app has to become
        // a regular one for as long as the window is up.
        model.refreshLayout()
        guard let window else { return }
        AppWindows.present(window)
    }

    /// Called by the app when the keyboard layout changes, so the practice card can say
    /// whether the right one is active.
    public func layoutDidChange() {
        model.refreshLayout()
    }

    public func windowWillClose(_ notification: Notification) {
        // Closing counts as done: whoever closes it has seen it, and it stays one click away
        // in the menu.
        model.prefs.didCompleteTutorial = true
        if let window { AppWindows.windowWillClose(window) }
        if let languageObserver { NotificationCenter.default.removeObserver(languageObserver) }
        languageObserver = nil
        DispatchQueue.main.async { [onFinish] in onFinish() }
    }
}

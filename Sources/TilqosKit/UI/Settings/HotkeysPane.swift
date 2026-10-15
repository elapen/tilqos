import AppKit
import Carbon.HIToolbox
import SwiftUI

struct HotkeysPane: View {
    @ObservedObject var model: SettingsModel

    private static var groups: [(String, [HotkeyAction])] {
        [
            (
                L10n.tr("hotkeys.group.correction"),
                [.fixLastWord, .fixSelection, .convertClipboard, .undoLastFix]
            ),
            (L10n.tr("hotkeys.group.transliteration"), [.transliterate]),
            (L10n.tr("hotkeys.group.layout"), [.forceEnglish, .forceRussian, .forceKazakh]),
            ("Tilqos", [.toggleEnabled]),
        ]
    }

    var body: some View {
        Form {
            ForEach(Self.groups, id: \.0) { title, actions in
                Section(title) {
                    ForEach(actions, id: \.self) { action in
                        HotkeyRow(model: model, action: action)
                    }
                }
            }
            Section {
                Button(L10n.tr("hotkeys.reset")) { model.resetHotkeys() }
                    .disabled(HotkeyAction.allCases.allSatisfy { model.hotkey($0) == $0.defaultHotkey })
            } footer: {
                Text(L10n.tr("hotkeys.footer"))
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            }
        }
        .formStyle(.grouped)
    }

    static func title(for action: HotkeyAction) -> String {
        switch action {
        case .fixLastWord: return L10n.tr("hotkey.fixLastWord")
        case .fixSelection: return L10n.tr("hotkey.fixSelection")
        case .undoLastFix: return L10n.tr("hotkey.undoLastFix")
        case .convertClipboard: return L10n.tr("hotkey.convertClipboard")
        case .transliterate: return L10n.tr("hotkey.transliterate")
        case .forceEnglish: return Lang.en.displayName
        case .forceRussian: return Lang.ru.displayName
        case .forceKazakh: return Lang.kk.displayName
        case .toggleEnabled: return L10n.tr("hotkey.toggleEnabled")
        }
    }

    static func detail(for action: HotkeyAction) -> String? {
        switch action {
        case .fixLastWord: return L10n.tr("hotkey.fixLastWord.detail")
        case .fixSelection: return L10n.tr("hotkey.fixSelection.detail")
        case .undoLastFix: return L10n.tr("hotkey.undoLastFix.detail")
        case .convertClipboard: return L10n.tr("hotkey.convertClipboard.detail")
        case .transliterate: return L10n.tr("hotkey.transliterate.detail")
        case .forceEnglish, .forceRussian, .forceKazakh, .toggleEnabled: return nil
        }
    }
}

private struct HotkeyRow: View {
    @ObservedObject var model: SettingsModel
    let action: HotkeyAction
    @State private var problem: String?

    var body: some View {
        let hotkey = model.hotkey(action)
        LabeledContent {
            ShortcutRecorder(
                hotkey: hotkey,
                isDefault: hotkey == action.defaultHotkey,
                title: HotkeysPane.title(for: action),
                recording: Binding(
                    get: { model.recordingHotkey == action },
                    set: { isOn in
                        if isOn {
                            model.recordingHotkey = action
                        } else if model.recordingHotkey == action {
                            model.recordingHotkey = nil
                        }
                    }),
                problem: $problem,
                commit: { model.setHotkey($0, for: action) },
                reset: { model.resetHotkey(action) })
        } label: {
            FormLabel(
                HotkeysPane.title(for: action),
                problem ?? HotkeysPane.detail(for: action),
                detailColor: problem == nil ? .secondary : .orange)
        }
    }
}

/// Click, press the new combination, done.
///
/// Recording goes through a local event monitor, which only sees keys while this window is
/// in front — exactly when recording makes sense. Whether this recorder is the one
/// listening is decided by the model, so arming one disarms any other.
struct ShortcutRecorder: View {
    let hotkey: Hotkey
    let isDefault: Bool
    let title: String
    @Binding var recording: Bool
    @Binding var problem: String?
    let commit: (Hotkey) -> String?
    let reset: () -> Void

    @State private var monitor: Any?

    var body: some View {
        HStack(spacing: 6) {
            if !isDefault && !recording {
                Button(action: reset) {
                    Image(systemName: "arrow.uturn.backward.circle.fill")
                        .symbolRenderingMode(.hierarchical)
                        .foregroundStyle(.secondary)
                        .font(.system(size: 15))
                }
                .buttonStyle(.borderless)
                .help(L10n.tr("hotkey.restore.help", KeyNames.display(hotkey)))
                .accessibilityLabel(L10n.tr("hotkey.restore.label"))
            }
            Button(action: toggle) {
                Text(recording ? L10n.tr("hotkey.recording") : KeyNames.display(hotkey))
                    .font(
                        recording
                            ? .callout
                            : .system(size: 12.5, weight: .medium).monospacedDigit()
                    )
                    .foregroundStyle(recording ? Color.accentColor : Color.primary)
                    .frame(minWidth: 120)
                    .padding(.vertical, 4)
                    .padding(.horizontal, 8)
                    .background(
                        RoundedRectangle(cornerRadius: 6, style: .continuous)
                            .fill(recording ? Color.accentColor.opacity(0.12) : Color.primary.opacity(0.05)))
                    .overlay(
                        RoundedRectangle(cornerRadius: 6, style: .continuous)
                            .strokeBorder(
                                recording ? Color.accentColor : Color.primary.opacity(0.15),
                                lineWidth: recording ? 1.5 : 1))
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("\(title): \(KeyNames.spoken(KeyNames.display(hotkey)))")
            .accessibilityHint(
                recording ? L10n.tr("hotkey.recording.hint") : L10n.tr("hotkey.change.hint"))
        }
        .onChange(of: recording, initial: true) { _, isOn in
            isOn ? listen() : stopListening()
        }
        .onDisappear {
            stopListening()
            if recording { recording = false }
        }
    }

    private func toggle() {
        if !recording { problem = nil }
        recording.toggle()
    }

    private func listen() {
        guard monitor == nil else { return }
        monitor = NSEvent.addLocalMonitorForEvents(matching: [.keyDown]) { event in
            handle(event)
            return nil
        }
    }

    private func stopListening() {
        if let monitor { NSEvent.removeMonitor(monitor) }
        monitor = nil
    }

    private func handle(_ event: NSEvent) {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        if event.keyCode == UInt16(kVK_Escape), flags.subtracting(.function).isEmpty {
            problem = nil
            recording = false
            return
        }
        var modifiers: HotkeyModifiers = []
        if flags.contains(.control) { modifiers.insert(.control) }
        if flags.contains(.option) { modifiers.insert(.option) }
        if flags.contains(.shift) { modifiers.insert(.shift) }
        if flags.contains(.command) { modifiers.insert(.command) }

        guard !modifiers.isDisjoint(with: [.control, .option, .command]) else {
            problem = L10n.tr("hotkey.problem.modifier")
            return
        }
        guard KeyNames.key(event.keyCode) != nil else {
            problem = L10n.tr("hotkey.problem.key")
            return
        }
        if let refusal = commit(Hotkey(keyCode: event.keyCode, modifiers: modifiers)) {
            problem = refusal
            return
        }
        problem = nil
        recording = false
    }
}

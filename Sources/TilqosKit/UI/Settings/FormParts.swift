import AppKit
import ServiceManagement
import SwiftUI

/// A setting's name with a plain-language line under it, the way System Settings explains
/// its switches.
struct FormLabel: View {
    let title: String
    var detail: String?
    var detailColor: Color = .secondary

    init(_ title: String, _ detail: String? = nil, detailColor: Color = .secondary) {
        self.title = title
        self.detail = detail
        self.detailColor = detailColor
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(title)
            if let detail {
                Text(detail)
                    .font(.subheadline)
                    .foregroundStyle(detailColor)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }
}

/// Empty lists say what would be there and how it gets filled, instead of just "Empty".
struct EmptyListNote: View {
    let symbol: String
    let title: String
    let text: String

    var body: some View {
        VStack(spacing: 6) {
            Image(systemName: symbol)
                .font(.system(size: 22))
                .foregroundStyle(.tertiary)
            Text(title).font(.headline).foregroundStyle(.secondary)
            Text(text)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .fixedSize(horizontal: false, vertical: true)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 12)
        .accessibilityElement(children: .combine)
    }
}

/// Trailing "remove" control for list rows.
struct RemoveButton: View {
    let label: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: "xmark.circle.fill")
                .symbolRenderingMode(.palette)
                .foregroundStyle(Color(nsColor: .windowBackgroundColor), .secondary)
                .font(.system(size: 14))
                .contentShape(Rectangle())
        }
        .buttonStyle(.borderless)
        .help(L10n.tr("common.remove"))
        .accessibilityLabel(L10n.tr("common.removeItem", label))
    }
}

/// "Clear all" for a word list: the button, the confirmation and what it deletes.
struct WordListClear {
    let button: String
    let title: String
    let message: String
    let perform: () -> Void
}

/// A word list with search, add and remove: the shape every exception list here shares.
///
/// Reusable on purpose — the learned-words list goes into the Exceptions pane as one more
/// of these.
struct WordListSection: View {
    let title: String
    let words: [String]
    let placeholder: String
    let emptySymbol: String
    let emptyTitle: String
    let emptyText: String
    var footer: String?
    var monospaced = true
    var clear: WordListClear?
    let onAdd: (String) -> Void
    let onRemove: (String) -> Void

    @State private var query = ""
    @State private var newWord = ""
    @State private var confirmingClear = false

    /// Search only earns its place once scrolling through the list becomes a chore.
    private var searchable: Bool { words.count > 8 }

    private var filtered: [String] {
        let trimmed = query.trimmingCharacters(in: .whitespaces)
        guard searchable, !trimmed.isEmpty else { return words }
        return words.filter { $0.localizedCaseInsensitiveContains(trimmed) }
    }

    var body: some View {
        Section {
            if words.isEmpty {
                EmptyListNote(symbol: emptySymbol, title: emptyTitle, text: emptyText)
            } else {
                if searchable {
                    HStack(spacing: 6) {
                        Image(systemName: "magnifyingglass").foregroundStyle(.secondary)
                        TextField(
                            L10n.tr("common.search"), text: $query,
                            prompt: Text(L10n.plural("list.search.prompt", words.count)))
                            .textFieldStyle(.plain)
                            .labelsHidden()
                        if !query.isEmpty {
                            Button {
                                query = ""
                            } label: {
                                Image(systemName: "xmark.circle.fill").foregroundStyle(.tertiary)
                            }
                            .buttonStyle(.borderless)
                            .accessibilityLabel(L10n.tr("common.clearSearch"))
                        }
                    }
                }
                ForEach(filtered, id: \.self) { word in
                    HStack {
                        Text(word)
                            .font(monospaced ? .body.monospaced() : .body)
                            .textSelection(.enabled)
                        Spacer()
                        RemoveButton(label: word) { onRemove(word) }
                    }
                }
                if filtered.isEmpty {
                    Text(L10n.tr("common.nothingFound"))
                        .foregroundStyle(.secondary)
                        .frame(maxWidth: .infinity, alignment: .center)
                }
            }
            HStack {
                TextField(L10n.tr("common.newWord"), text: $newWord, prompt: Text(placeholder))
                    .labelsHidden()
                    .onSubmit(add)
                Button(L10n.tr("common.add"), action: add)
                    .disabled(newWord.trimmingCharacters(in: .whitespaces).isEmpty)
            }
            if let clear {
                // The words are plain text on disk, so there is a way to delete them all.
                Button(clear.button, role: .destructive) { confirmingClear = true }
                    .disabled(words.isEmpty)
                    .confirmationDialog(clear.title, isPresented: $confirmingClear) {
                        Button(L10n.tr("common.clearAll"), role: .destructive, action: clear.perform)
                        Button(L10n.tr("common.cancel"), role: .cancel) {}
                    } message: {
                        Text(clear.message)
                    }
            }
        } header: {
            HStack {
                Text(title)
                if !words.isEmpty {
                    Text("\(words.count)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 6)
                        .padding(.vertical, 1)
                        .background(Capsule().fill(.quaternary))
                }
            }
        } footer: {
            if let footer {
                Text(footer)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
        }
    }

    private func add() {
        let word = newWord.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !word.isEmpty else { return }
        onAdd(word)
        newWord = ""
    }
}

/// A two-to-three letter language chip, the same shape as the menu bar icon.
struct LangBadge: View {
    let lang: Lang

    var body: some View {
        Text(lang.badge)
            .font(.system(size: 10.5, weight: .semibold))
            .kerning(0.3)
            .foregroundStyle(.secondary)
            .frame(minWidth: 30, minHeight: 18)
            .padding(.horizontal, 2)
            .overlay(
                RoundedRectangle(cornerRadius: 4, style: .continuous)
                    .strokeBorder(.secondary.opacity(0.7), lineWidth: 1.2))
            .accessibilityHidden(true)
    }
}

extension FixMode {
    /// Names for the per-app picker: what the app will do, rather than a mode name.
    var settingsTitle: String {
        switch self {
        case .auto: return L10n.tr("mode.pane.auto")
        case .suggest: return L10n.tr("mode.pane.suggest")
        case .off: return L10n.tr("mode.pane.off")
        }
    }
}

enum SystemSettingsLinks {
    static func openKeyboard() {
        guard let url = URL(string: "x-apple.systempreferences:com.apple.Keyboard-Settings.extension")
        else { return }
        NSWorkspace.shared.open(url)
    }

    static func openLoginItems() {
        SMAppService.openSystemSettingsLoginItems()
    }
}

import SwiftUI

struct TransliterationPane: View {
    @ObservedObject var model: SettingsModel
    @State private var newLatin = ""
    @State private var newCyrillic = ""

    var body: some View {
        Form {
            Section {
                Picker(L10n.tr("translit.latin"), selection: model.binding(\.translitScheme) { $0.translitScheme = $1 }) {
                    ForEach(TranslitScheme.allCases, id: \.self) { scheme in
                        VStack(alignment: .leading, spacing: 2) {
                            Text(scheme.displayName)
                            Text(model.translitSample(scheme))
                                .font(.subheadline.monospaced())
                                .foregroundStyle(.secondary)
                        }
                        .padding(.vertical, 2)
                        .tag(scheme)
                    }
                }
                .pickerStyle(.radioGroup)
                .labelsHidden()
                Text(model.translitScheme.explanation)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            } header: {
                Text(L10n.tr("translit.latin"))
            } footer: {
                Text(L10n.tr("translit.footer", KeyNames.display(model.hotkey(.transliterate))))
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            }

            Section {
                if model.translitOverrides.isEmpty {
                    EmptyListNote(
                        symbol: "character.book.closed",
                        title: L10n.tr("translit.overrides.emptyTitle"),
                        text: L10n.tr("translit.overrides.emptyText"))
                }
                ForEach(model.translitOverrides, id: \.latin) { entry in
                    HStack {
                        Text(entry.latin).font(.body.monospaced())
                        Image(systemName: "arrow.right")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(.tertiary)
                        Text(entry.cyrillic)
                        Spacer()
                        RemoveButton(label: entry.latin) { model.removeTranslitOverride(entry.latin) }
                    }
                    .accessibilityElement(children: .combine)
                }
                HStack {
                    TextField(
                        L10n.tr("translit.overrides.latinField"), text: $newLatin,
                        prompt: Text(L10n.tr("translit.overrides.latinPrompt")))
                        .labelsHidden()
                    Image(systemName: "arrow.right")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(.tertiary)
                    TextField(
                        L10n.tr("translit.overrides.cyrillicField"), text: $newCyrillic,
                        prompt: Text(L10n.tr("translit.overrides.cyrillicPrompt")))
                        .labelsHidden()
                        .onSubmit(addOverride)
                    Button(L10n.tr("common.add"), action: addOverride)
                        .disabled(trimmed(newLatin).isEmpty || trimmed(newCyrillic).isEmpty)
                }
            } header: {
                Text(L10n.tr("translit.overrides.header"))
            }

            WordListSection(
                title: L10n.tr("translit.never.title"),
                words: model.neverConvert,
                placeholder: L10n.tr("translit.never.placeholder"),
                emptySymbol: "textformat.abc",
                emptyTitle: L10n.tr("common.emptyYet"),
                emptyText: L10n.tr("translit.never.emptyText"),
                clear: WordListClear(
                    button: L10n.tr("translit.never.clear.button"),
                    title: L10n.tr("translit.never.clear.title"),
                    message: L10n.tr("translit.never.clear.message"),
                    perform: model.clearNeverConvertWords),
                onAdd: model.addNeverConvert,
                onRemove: model.removeNeverConvert)
        }
        .formStyle(.grouped)
    }

    private func trimmed(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func addOverride() {
        let latin = trimmed(newLatin)
        let cyrillic = trimmed(newCyrillic)
        guard !latin.isEmpty, !cyrillic.isEmpty else { return }
        model.addTranslitOverride(latin: latin, cyrillic: cyrillic)
        newLatin = ""
        newCyrillic = ""
    }
}

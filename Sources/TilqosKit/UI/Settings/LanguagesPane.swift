import SwiftUI

struct LanguagesPane: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        Form {
            Section {
                ForEach(Lang.allCases, id: \.self) { lang in
                    LanguageRow(model: model, lang: lang)
                }
            } header: {
                Text(L10n.tr("languages.header"))
            } footer: {
                Text(L10n.tr("languages.footer"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }

            Section {
                Text(L10n.tr("languages.cyrillic.note"))
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

                Picker(
                    L10n.tr("languages.cyrillic.picker"),
                    selection: model.binding(\.cyrillicPreference) { $0.cyrillicPreference = $1 }
                ) {
                    ForEach(CyrillicPreference.allCases, id: \.self) {
                        Text($0.settingsTitle).tag($0)
                    }
                }
                Toggle(
                    isOn: model.binding(\.rememberLayoutPerApp) { $0.rememberLayoutPerApp = $1 }
                ) {
                    FormLabel(L10n.tr("languages.remember.title"), L10n.tr("languages.remember.detail"))
                }
            } header: {
                Text(L10n.tr("languages.cyrillic.header"))
            }

            Section {
                ForEach(model.inputSources.tables) { table in
                    LabeledContent(table.localizedName) {
                        if let lang = table.lang {
                            LangBadge(lang: lang)
                        } else {
                            Text(L10n.tr("languages.notUsed")).foregroundStyle(.tertiary)
                        }
                    }
                }
                Button(L10n.tr("languages.manage")) { SystemSettingsLinks.openKeyboard() }
            } header: {
                Text(L10n.tr("languages.system.header"))
            }
        }
        .formStyle(.grouped)
    }
}

private struct LanguageRow: View {
    @ObservedObject var model: SettingsModel
    let lang: Lang

    var body: some View {
        let layouts = model.inputSources.tables.filter { $0.lang == lang }
        let hasModel = model.modelLanguages.contains(lang)
        LabeledContent {
            if layouts.isEmpty {
                Button(L10n.tr("languages.add")) { SystemSettingsLinks.openKeyboard() }
            } else {
                Toggle(lang.displayName, isOn: model.languageBinding(lang))
                    .labelsHidden()
                    .toggleStyle(.switch)
                    .disabled(!hasModel)
            }
        } label: {
            HStack(spacing: 10) {
                LangBadge(lang: lang)
                FormLabel(lang.displayName, detail(layouts: layouts, hasModel: hasModel),
                    detailColor: layouts.isEmpty || !hasModel ? .orange : .secondary)
            }
        }
        .accessibilityElement(children: .contain)
    }

    private func detail(layouts: [KeyMapTable], hasModel: Bool) -> String {
        if layouts.isEmpty { return L10n.tr("languages.detail.missing") }
        if !hasModel { return L10n.tr("languages.detail.noModel") }
        return L10n.tr("languages.detail.layouts", layouts.map(\.localizedName).joined(separator: ", "))
    }
}

extension CyrillicPreference {
    var settingsTitle: String {
        switch self {
        case .automatic: return L10n.tr("languages.cyrillic.perApp")
        case .preferKazakh: return L10n.tr("languages.cyrillic.kazakh")
        case .preferRussian: return L10n.tr("languages.cyrillic.russian")
        }
    }
}

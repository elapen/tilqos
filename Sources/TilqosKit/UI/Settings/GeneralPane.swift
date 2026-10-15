import SwiftUI

struct GeneralPane: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        Form {
            Section {
                HStack(spacing: 14) {
                    AppIconView(size: 56)
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Tilqos").font(.title3.weight(.semibold))
                        StatusLabel(health: model.health).font(.callout)
                        Text(L10n.tr("general.version", model.version))
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                    }
                    Spacer()
                    if model.health.needsAttention {
                        Button(L10n.tr("general.fix")) { model.pane = .diagnostics }
                    }
                }
                .padding(.vertical, 4)
            }

            Section {
                Toggle(isOn: model.binding(\.enabled) { $0.enabled = $1 }) {
                    FormLabel(L10n.tr("general.enabled.title"), L10n.tr("general.enabled.detail"))
                }
                Toggle(
                    isOn: Binding(
                        get: { model.launchAtLogin }, set: { model.setLaunchAtLogin($0) })
                ) {
                    FormLabel(
                        L10n.tr("general.launch.title"), launchDetail,
                        detailColor: model.launchAtLoginProblem == nil ? .secondary : .orange)
                }
                if model.launchAtLoginNeedsApproval {
                    Button(L10n.tr("general.launch.openItems")) { SystemSettingsLinks.openLoginItems() }
                }
            }

            Section {
                Picker(selection: model.uiLanguage) {
                    Text(L10n.tr("general.language.system")).tag(UILanguageSetting.system)
                    // Each language is named in itself, so it can be found whatever is showing now.
                    ForEach(Lang.allCases, id: \.self) { lang in
                        Text(lang.displayName).tag(UILanguageSetting(rawValue: lang.rawValue) ?? .system)
                    }
                } label: {
                    FormLabel(L10n.tr("general.language.picker"), L10n.tr("general.language.detail"))
                }
            } header: {
                Text(L10n.tr("general.language.header"))
            }

            Section(L10n.tr("general.menubar.header")) {
                Toggle(isOn: model.binding(\.showLayoutBadge) { $0.showLayoutBadge = $1 }) {
                    FormLabel(L10n.tr("general.badge.title"), L10n.tr("general.badge.detail"))
                }
            }

            Section(L10n.tr("general.after.header")) {
                Toggle(isOn: model.binding(\.showHUD) { $0.showHUD = $1 }) {
                    FormLabel(L10n.tr("general.hud.title"), L10n.tr("general.hud.detail"))
                }
                if model.prefs.showHUD {
                    HStack {
                        Spacer()
                        HintPreview(
                            content: .init(
                                before: "ghbdtn", after: "привет",
                                style: .applied(
                                    undoKey: KeyNames.display(model.hotkey(.undoLastFix)))))
                            .shadow(color: .black.opacity(0.1), radius: 6, y: 2)
                        Spacer()
                    }
                    .padding(.vertical, 4)
                    .accessibilityHidden(true)
                }
                Toggle(isOn: model.binding(\.playSound) { $0.playSound = $1 }) {
                    FormLabel(L10n.tr("general.sound.title"), L10n.tr("general.sound.detail"))
                }
            }

            Section {
                Label {
                    Text(L10n.tr("general.privacy"))
                        .fixedSize(horizontal: false, vertical: true)
                } icon: {
                    Image(systemName: "lock.shield.fill").foregroundStyle(.green)
                }
                .font(.subheadline)
                .foregroundStyle(.secondary)
            }
        }
        .formStyle(.grouped)
    }

    private var launchDetail: String? {
        if let problem = model.launchAtLoginProblem { return problem }
        if model.launchAtLoginNeedsApproval {
            return L10n.tr("general.launch.approval")
        }
        return nil
    }
}

import AppKit
import SwiftUI

struct AppsPane: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        Form {
            Section {
                if model.appRules.isEmpty {
                    EmptyListNote(
                        symbol: "square.grid.2x2",
                        title: L10n.tr("apps.rules.emptyTitle"),
                        text: L10n.tr("apps.rules.emptyText"))
                }
                ForEach(model.appRules) { rule in
                    AppRuleView(model: model, rule: rule)
                }
                addMenu
            } header: {
                Text(L10n.tr("apps.rules.header"))
            } footer: {
                Text(
                    L10n.tr(
                        "apps.rules.footer",
                        model.prefs.defaultFixMode.settingsTitle.lowercased(with: L10n.locale))
                )
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            }

            Section {
                ForEach(model.lockedApps) { rule in
                    HStack(spacing: 10) {
                        Image(nsImage: AppInfo.icon(for: rule.bundleID))
                            .resizable()
                            .frame(width: 22, height: 22)
                            .accessibilityHidden(true)
                        Text(rule.name)
                        Spacer()
                        Label(L10n.tr("apps.locked.row"), systemImage: "lock.fill")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    .help(rule.bundleID)
                }
            } header: {
                Text(L10n.tr("apps.locked.header"))
            } footer: {
                Text(L10n.tr("apps.locked.footer"))
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            }
        }
        .formStyle(.grouped)
    }

    private var addMenu: some View {
        Menu {
            let running = model.addableRunningApps
            if !running.isEmpty {
                Section(L10n.tr("apps.add.running")) {
                    ForEach(running, id: \.processIdentifier) { app in
                        Button {
                            if let id = app.bundleIdentifier { model.addApp(id) }
                        } label: {
                            Label {
                                Text(app.localizedName ?? app.bundleIdentifier ?? "")
                            } icon: {
                                Image(nsImage: Self.menuIcon(app.icon))
                            }
                        }
                    }
                }
                Divider()
            }
            Button(L10n.tr("apps.add.choose")) { model.chooseApp() }
        } label: {
            Label(L10n.tr("apps.add.label"), systemImage: "plus")
        }
        .menuStyle(.borderlessButton)
        .fixedSize()
    }

    private static func menuIcon(_ icon: NSImage?) -> NSImage {
        let image = (icon?.copy() as? NSImage) ?? NSWorkspace.shared.icon(for: .application)
        image.size = NSSize(width: 16, height: 16)
        return image
    }
}

private struct AppRuleView: View {
    @ObservedObject var model: SettingsModel
    let rule: AppRuleRow

    var body: some View {
        HStack(spacing: 10) {
            Image(nsImage: AppInfo.icon(for: rule.bundleID))
                .resizable()
                .frame(width: 24, height: 24)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 1) {
                Text(rule.name)
                if rule.isShippedDefault && !rule.isExplicit {
                    Text(L10n.tr("apps.terminalDefault"))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }
            .help(rule.bundleID)
            Spacer()
            Picker(
                L10n.tr("apps.mode.label", rule.name),
                selection: Binding(
                    get: { rule.mode },
                    set: { model.setMode($0, for: rule.bundleID) })
            ) {
                ForEach(FixMode.allCases.reversed(), id: \.self) {
                    Text($0.settingsTitle).tag($0)
                }
            }
            .labelsHidden()
            .fixedSize()
            if rule.isExplicit {
                if rule.isShippedDefault {
                    Button {
                        model.setMode(nil, for: rule.bundleID)
                    } label: {
                        Image(systemName: "arrow.uturn.backward.circle.fill")
                            .symbolRenderingMode(.hierarchical)
                            .foregroundStyle(.secondary)
                            .font(.system(size: 15))
                    }
                    .buttonStyle(.borderless)
                    .help(L10n.tr("apps.restore.help"))
                    .accessibilityLabel(L10n.tr("apps.restore.label", rule.name))
                } else {
                    RemoveButton(label: rule.name) { model.setMode(nil, for: rule.bundleID) }
                }
            }
        }
    }
}

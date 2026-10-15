import AppKit
import SwiftUI

struct DiagnosticsPane: View {
    @ObservedObject var model: SettingsModel
    @State private var copied = false

    private static let logCommand =
        "log stream --predicate 'subsystem == \"io.github.elapen.tilqos\"'"

    var body: some View {
        let permissions = model.permissions
        Form {
            Section {
                PermissionRow(
                    title: L10n.tr("diagnostics.accessibility.title"),
                    detail: permissions.accessibility
                        ? L10n.tr("diagnostics.accessibility.granted")
                        : L10n.tr("diagnostics.accessibility.missing"),
                    ok: permissions.accessibility,
                    open: {
                        Permissions.requestAccessibility()
                        Permissions.openAccessibilitySettings()
                    })
                PermissionRow(
                    title: L10n.tr("diagnostics.input.title"),
                    detail: permissions.accessibility
                        ? L10n.tr("diagnostics.input.notNeeded")
                        : (permissions.inputMonitoring
                            ? L10n.tr("diagnostics.input.granted")
                            : L10n.tr("diagnostics.input.fallback")),
                    ok: permissions.accessibility || permissions.inputMonitoring,
                    optional: true,
                    open: {
                        Permissions.requestInputMonitoring()
                        Permissions.openInputMonitoringSettings()
                    })
            } header: {
                Text(L10n.tr("diagnostics.access.header"))
            } footer: {
                Text(L10n.tr("diagnostics.access.footer"))
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            }

            Section(L10n.tr("diagnostics.work.header")) {
                LabeledContent(L10n.tr("diagnostics.tap")) {
                    HStack(spacing: 8) {
                        if !model.tapRunning, permissions.canDetect {
                            Button(L10n.tr("diagnostics.tap.restart")) { model.restartTap() }
                                .controlSize(.small)
                        }
                        Label(
                            model.tapRunning
                                ? L10n.tr("diagnostics.tap.running")
                                : L10n.tr("diagnostics.tap.stopped"),
                            systemImage: model.tapRunning ? "checkmark.circle.fill" : "xmark.octagon.fill"
                        )
                        .foregroundStyle(model.tapRunning ? .green : .red)
                    }
                }
                LabeledContent(L10n.tr("diagnostics.models")) {
                    Text(
                        model.modelLanguages.isEmpty
                            ? L10n.tr("diagnostics.models.none")
                            : Lang.allCases.filter(model.modelLanguages.contains)
                                .map(\.displayName).joined(separator: ", "))
                }
                LabeledContent(L10n.tr("diagnostics.layout")) {
                    Text(model.inputSources.current?.localizedName ?? "—")
                }
                LabeledContent(L10n.tr("diagnostics.undoCount")) {
                    Text("\(model.pipeline?.undoCount ?? 0)").monospacedDigit()
                }
            }

            if !model.learnedStrategies.isEmpty {
                Section {
                    ForEach(model.learnedStrategies.sorted(by: { $0.key < $1.key }), id: \.key) {
                        entry in
                        LabeledContent(AppInfo.name(for: entry.key)) {
                            Text(
                                entry.value == .accessibility
                                    ? L10n.tr("diagnostics.strategy.accessibility")
                                    : L10n.tr("diagnostics.strategy.synthetic"))
                        }
                    }
                } header: {
                    Text(L10n.tr("diagnostics.strategy.header"))
                } footer: {
                    Text(L10n.tr("diagnostics.strategy.footer"))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
            }

            Section {
                Toggle(isOn: model.binding(\.diagnosticsEnabled) { $0.diagnosticsEnabled = $1 }) {
                    FormLabel(
                        L10n.tr("diagnostics.verbose.title"), L10n.tr("diagnostics.verbose.detail"))
                }
                LabeledContent {
                    Button(copied ? L10n.tr("diagnostics.log.copied") : L10n.tr("diagnostics.log.copy")) {
                        NSPasteboard.general.clearContents()
                        NSPasteboard.general.setString(Self.logCommand, forType: .string)
                        copied = true
                    }
                } label: {
                    FormLabel(L10n.tr("diagnostics.log.title"), L10n.tr("diagnostics.log.detail"))
                }
                LabeledContent(L10n.tr("diagnostics.version"), value: model.version)
            } header: {
                Text(L10n.tr("diagnostics.developer.header"))
            }
        }
        .formStyle(.grouped)
    }
}

private struct PermissionRow: View {
    let title: String
    let detail: String
    let ok: Bool
    var optional = false
    let open: () -> Void

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: ok ? "checkmark.circle.fill" : (optional ? "circle.dashed" : "exclamationmark.triangle.fill"))
                .font(.system(size: 17))
                .foregroundStyle(ok ? .green : (optional ? .secondary : .orange))
                .accessibilityHidden(true)
            FormLabel(title, detail)
            Spacer()
            if !ok {
                Button(L10n.tr("common.openSettings"), action: open)
                    .accessibilityHint(L10n.tr("diagnostics.open.hint", title))
            }
        }
    }
}

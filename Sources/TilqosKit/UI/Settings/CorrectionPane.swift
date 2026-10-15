import SwiftUI

struct CorrectionPane: View {
    @ObservedObject var model: SettingsModel
    @State private var showsDetails = false

    private var smartMode: Bool { model.prefs.smartMode }

    var body: some View {
        Form {
            Section {
                Toggle(isOn: model.binding(\.smartMode) { $0.smartMode = $1 }) {
                    FormLabel(L10n.tr("correction.smart.title"), L10n.tr("correction.smart.detail"))
                }
                Picker(selection: model.binding(\.defaultFixMode) { $0.defaultFixMode = $1 }) {
                    ForEach(FixMode.allCases.reversed(), id: \.self) {
                        Text($0.settingsTitle).tag($0)
                    }
                } label: {
                    FormLabel(L10n.tr("correction.default.title"), L10n.tr("correction.default.detail"))
                }
                .disabled(!smartMode)
            }

            Section {
                Picker(L10n.tr("correction.bold.picker"), selection: sensitivity) {
                    ForEach(Sensitivity.presets) { Text($0.title).tag($0) }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
                Text(model.sensitivity.explanation)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)

                DisclosureGroup(L10n.tr("correction.fine"), isExpanded: $showsDetails) {
                    ThresholdRow(
                        title: L10n.tr("correction.auto.title"),
                        detail: L10n.tr("correction.auto.detail"),
                        value: model.autoThreshold, range: 0.5...1.0)
                    ThresholdRow(
                        title: L10n.tr("correction.suggest.title"),
                        detail: L10n.tr("correction.suggest.detail"),
                        value: model.suggestThreshold, range: 0.1...0.9)
                    Stepper(value: model.binding(\.minWordLength) { $0.minWordLength = $1 }, in: 2...8) {
                        FormLabel(
                            L10n.tr("correction.minlen.title"),
                            L10n.plural("correction.minlen.detail", model.prefs.minWordLength))
                    }
                }
            } header: {
                Text(L10n.tr("correction.bold.header"))
            }
            .disabled(!smartMode)

            Section {
                LabeledContent(L10n.tr("correction.preview.label")) {
                    HintPreview(
                        content: .init(
                            before: "ghbdtn", after: "привет",
                            style: .suggestion(acceptKey: model.prefs.acceptKey.keycapTitle)))
                        .shadow(color: .black.opacity(0.1), radius: 6, y: 2)
                        .padding(.vertical, 4)
                        .accessibilityHidden(true)
                }
                Picker(selection: model.binding(\.acceptKey) { $0.acceptKey = $1 }) {
                    ForEach(AcceptKey.allCases, id: \.self) { Text($0.keycapTitle).tag($0) }
                } label: {
                    FormLabel(L10n.tr("correction.accept.title"), L10n.tr("correction.accept.detail"))
                }
                Picker(L10n.tr("correction.timeout"), selection: model.binding(\.suggestionTimeout) { $0.suggestionTimeout = $1 }) {
                    ForEach(timeouts, id: \.self) { seconds in
                        Text(L10n.tr("common.seconds", Int(seconds))).tag(seconds)
                    }
                }
            } header: {
                Text(L10n.tr("correction.hint.header"))
            } footer: {
                Text(L10n.tr("correction.hint.footer"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            .disabled(!smartMode)
        }
        .formStyle(.grouped)
    }

    private var sensitivity: Binding<Sensitivity> {
        Binding(get: { model.sensitivity }, set: { model.sensitivity = $0 })
    }

    /// The offered choices, plus whatever is stored if it is not one of them.
    private var timeouts: [Double] {
        var values: [Double] = [3, 4, 6, 10]
        let current = model.prefs.suggestionTimeout
        if current > 0, !values.contains(current) { values.append(current) }
        return values.sorted()
    }
}

private struct ThresholdRow: View {
    let title: String
    let detail: String
    let value: Binding<Double>
    let range: ClosedRange<Double>

    var body: some View {
        LabeledContent {
            HStack(spacing: 10) {
                Slider(value: value, in: range, step: 0.05)
                    .frame(minWidth: 140)
                    .accessibilityValue(L10n.tr("correction.percent", Int((value.wrappedValue * 100).rounded())))
                Text("\(Int((value.wrappedValue * 100).rounded())) %")
                    .monospacedDigit()
                    .foregroundStyle(.secondary)
                    .frame(width: 44, alignment: .trailing)
            }
        } label: {
            FormLabel(title, detail)
        }
    }
}

import SwiftUI

/// Words Tilqos must leave alone.
///
/// Laid out as a stack of sections so each kind of list gets the same treatment: the typed
/// exceptions, then what the engine has learned from the user on its own.
struct ExceptionsPane: View {
    @ObservedObject var model: SettingsModel

    var body: some View {
        Form {
            WordListSection(
                title: L10n.tr("exceptions.title"),
                words: model.exceptions,
                placeholder: L10n.tr("exceptions.placeholder"),
                emptySymbol: "text.badge.checkmark",
                emptyTitle: L10n.tr("common.emptyYet"),
                emptyText: L10n.tr(
                    "exceptions.emptyText", KeyNames.display(model.hotkey(.undoLastFix))),
                footer: L10n.tr("exceptions.footer"),
                clear: WordListClear(
                    button: L10n.tr("exceptions.clear.button"),
                    title: L10n.tr("exceptions.clear.title"),
                    message: L10n.tr("exceptions.clear.message"),
                    perform: model.clearExceptionWords),
                onAdd: model.addException,
                onRemove: model.removeException)

            LearningSection(model: model)
        }
        .formStyle(.grouped)
    }
}

/// What the engine learned from the user's own actions.
///
/// There is deliberately no word list here: the learner keeps salted hashes, never the words
/// themselves, so the section reports how much it knows and lets the user forget a word by
/// typing it — or reset everything at once.
private struct LearningSection: View {
    @ObservedObject var model: SettingsModel
    @State private var wordToForget = ""
    @State private var forgetResult: String?
    @State private var confirmingReset = false

    private var learner: Learner { model.learner }

    var body: some View {
        let counts = learner.counts
        Section {
            Toggle(isOn: enabled) {
                FormLabel(L10n.tr("learning.toggle.title"), L10n.tr("learning.toggle.detail"))
            }

            if counts.total == 0 {
                EmptyListNote(
                    symbol: "brain",
                    title: L10n.tr("learning.empty.title"),
                    text: L10n.tr("learning.empty.text"))
            } else {
                CountRow(
                    title: L10n.tr("learning.never.title"), detail: L10n.tr("learning.never.detail"),
                    value: counts.neverConvert)
                CountRow(
                    title: L10n.tr("learning.always.title"), detail: L10n.tr("learning.always.detail"),
                    value: counts.alwaysConvert)
                CountRow(
                    title: L10n.tr("learning.user.title"), detail: L10n.tr("learning.user.detail"),
                    value: counts.userWords)
                if counts.pending > 0 {
                    CountRow(
                        title: L10n.tr("learning.pending.title"),
                        detail: L10n.tr("learning.pending.detail"),
                        value: counts.pending)
                }
            }

            LabeledContent {
                HStack(spacing: 6) {
                    TextField(
                        L10n.tr("learning.forget.field"), text: $wordToForget,
                        prompt: Text(L10n.tr("learning.forget.prompt")))
                        .labelsHidden()
                        .onSubmit(forgetWord)
                    Button(L10n.tr("learning.forget.button"), action: forgetWord)
                        .disabled(trimmedWord.isEmpty)
                }
                .frame(maxWidth: 280)
            } label: {
                FormLabel(L10n.tr("learning.forget.title"), forgetResult)
            }

            Button(L10n.tr("learning.reset.button"), role: .destructive) {
                confirmingReset = true
            }
            .disabled(counts.total == 0)
            .confirmationDialog(
                L10n.tr("learning.reset.title"), isPresented: $confirmingReset
            ) {
                Button(L10n.tr("learning.reset.confirm"), role: .destructive) {
                    model.resetLearnedVocabulary()
                    forgetResult = nil
                }
                Button(L10n.tr("common.cancel"), role: .cancel) {}
            } message: {
                Text(L10n.tr("learning.reset.message"))
            }
        } header: {
            Text(L10n.tr("learning.header"))
        } footer: {
            Text(L10n.tr("learning.footer"))
        }
    }

    private var trimmedWord: String {
        wordToForget.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var enabled: Binding<Bool> {
        Binding(
            get: { learner.isEnabled },
            set: { value in
                learner.isEnabled = value
                model.refresh()
            })
    }

    private func forgetWord() {
        let word = trimmedWord
        guard !word.isEmpty else { return }
        forgetResult =
            learner.forget(word: word)
            ? L10n.tr("learning.forget.done", word)
            : L10n.tr("learning.forget.none", word)
        wordToForget = ""
        model.refresh()
    }
}

/// A count with a line saying where the entries come from.
private struct CountRow: View {
    let title: String
    let detail: String
    let value: Int

    var body: some View {
        LabeledContent {
            Text("\(value)")
                .monospacedDigit()
                .foregroundStyle(.secondary)
        } label: {
            FormLabel(title, detail)
        }
        .accessibilityElement(children: .combine)
    }
}

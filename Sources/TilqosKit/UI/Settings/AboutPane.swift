import AppKit
import SwiftUI

/// The license texts that ship with the app (`build.sh` copies them into `Contents/Resources`).
enum LicenseFiles {
    enum Document: String {
        case license = "LICENSE"
        case notices = "THIRD_PARTY_NOTICES.md"
    }

    /// Where to look. Tests point this at the repository root, which has the same layout.
    nonisolated(unsafe) static var roots: [URL]?

    private static var searchRoots: [URL] {
        roots ?? [Bundle.main.resourceURL].compactMap { $0 }
    }

    static func url(for document: Document) -> URL? {
        firstExisting(document.rawValue)
    }

    /// The folder with the full texts of the licenses the data files are under.
    static var folder: URL? { firstExisting("LICENSES") }

    private static func firstExisting(_ name: String) -> URL? {
        for root in searchRoots {
            let url = root.appendingPathComponent(name)
            if FileManager.default.fileExists(atPath: url.path) { return url }
        }
        return nil
    }

    /// Shows a document in TextEdit (the notices are Markdown, which has no default viewer on
    /// a fresh Mac and would otherwise open in whatever claims the extension). Returns false
    /// when the file is not there.
    @MainActor @discardableResult
    static func open(_ document: Document) -> Bool {
        guard let url = url(for: document) else { return false }
        if let textEdit = NSWorkspace.shared.urlForApplication(
            withBundleIdentifier: "com.apple.TextEdit")
        {
            NSWorkspace.shared.open(
                [url], withApplicationAt: textEdit, configuration: NSWorkspace.OpenConfiguration())
        } else if !NSWorkspace.shared.open(url) {
            NSWorkspace.shared.activateFileViewerSelecting([url])
        }
        return true
    }

    @MainActor @discardableResult
    static func revealFolder() -> Bool {
        guard let url = folder ?? self.url(for: .license) else { return false }
        NSWorkspace.shared.activateFileViewerSelecting([url])
        return true
    }
}

/// What the app is, what it does with your typing, and under which licenses it comes.
struct AboutPane: View {
    @ObservedObject var model: SettingsModel
    @State private var licensesMissing = false

    var body: some View {
        Form {
            Section {
                HStack(spacing: 14) {
                    AppIconView(size: 56)
                    VStack(alignment: .leading, spacing: 3) {
                        Text("Tilqos").font(.title3.weight(.semibold))
                        Text(L10n.tr("about.version", model.version))
                            .font(.callout)
                            .foregroundStyle(.secondary)
                        Text(L10n.tr("about.tagline"))
                            .font(.caption)
                            .foregroundStyle(.tertiary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    Spacer()
                }
                .padding(.vertical, 4)
            }

            Section {
                Text(L10n.tr("about.privacy.intro"))
                    .fixedSize(horizontal: false, vertical: true)
                PrivacyPoint(symbol: "memorychip", key: "about.privacy.memory")
                PrivacyPoint(symbol: "arrow.uturn.backward", key: "about.privacy.undo")
                PrivacyPoint(symbol: "doc.on.clipboard", key: "about.privacy.clipboard")
                PrivacyPoint(symbol: "lock.shield", key: "about.privacy.secure")
                PrivacyPoint(symbol: "text.badge.xmark", key: "about.privacy.log")
                PrivacyPoint(symbol: "internaldrive", key: "about.privacy.storage")
                PrivacyPoint(symbol: "wifi.slash", key: "about.privacy.network")
            } header: {
                Text(L10n.tr("about.privacy.header"))
            }

            Section {
                Text(L10n.tr("about.licenses.text"))
                    .fixedSize(horizontal: false, vertical: true)
                Text(L10n.tr("about.copyright"))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                // Side by side when they fit, one under the other when a language needs more room.
                ViewThatFits(in: .horizontal) {
                    HStack { licenseButtons; Spacer() }
                    VStack(alignment: .leading) { licenseButtons }
                }
                if licensesMissing {
                    Text(L10n.tr("about.licenses.missing"))
                        .font(.subheadline)
                        .foregroundStyle(.orange)
                        .fixedSize(horizontal: false, vertical: true)
                }
            } header: {
                Text(L10n.tr("about.licenses.header"))
            }
        }
        .formStyle(.grouped)
    }

    @ViewBuilder private var licenseButtons: some View {
        Button(L10n.tr("about.licenses.mit")) { show(.license) }
        Button(L10n.tr("about.licenses.notices")) { show(.notices) }
        Button(L10n.tr("about.licenses.folder")) {
            licensesMissing = !LicenseFiles.revealFolder()
        }
    }

    private func show(_ document: LicenseFiles.Document) {
        licensesMissing = !LicenseFiles.open(document)
    }
}

/// One privacy statement with a symbol in front.
private struct PrivacyPoint: View {
    let symbol: String
    let key: String

    var body: some View {
        Label {
            Text(L10n.tr(key))
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
        } icon: {
            Image(systemName: symbol)
                .foregroundStyle(.green)
                .frame(width: 18)
        }
    }
}

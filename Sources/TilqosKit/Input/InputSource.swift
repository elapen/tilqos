import Carbon.HIToolbox
import Foundation

/// Owns the set of installed keyboard layouts and the currently selected one.
///
/// Tables are rebuilt when the user adds or removes a layout in System Settings, so
/// nothing here is baked in at compile time.
public final class InputSourceManager: NSObject {
    public static let shared = InputSourceManager()

    /// Every enabled input source that exposes a Unicode key layout, in system order.
    public private(set) var tables: [KeyMapTable] = []
    /// Input source handles, kept separately so `KeyMapTable` stays a value type.
    private var sources: [String: TISInputSource] = [:]
    public private(set) var currentSourceID: String?

    /// Fired when the user (or we) switch layout.
    public var onSelectionChange: ((KeyMapTable?) -> Void)?
    /// Fired when the set of installed layouts changes.
    public var onLayoutSetChange: (() -> Void)?

    public override init() {
        super.init()
    }

    // MARK: - Lifecycle

    public func start() {
        reloadTables()
        reloadCurrent()

        // Selector-based with `.deliverImmediately` on purpose: distributed notifications are
        // coalesced and deferred by default, and a menu-bar-only app can wait a long time for
        // them. The keystroke path no longer depends on these — see `liveCurrentTable()` — but
        // the menu bar badge does.
        let center = DistributedNotificationCenter.default()
        center.addObserver(
            self, selector: #selector(selectedSourceChanged),
            name: Notification.Name(kTISNotifySelectedKeyboardInputSourceChanged as String),
            object: nil, suspensionBehavior: .deliverImmediately)
        center.addObserver(
            self, selector: #selector(enabledSourcesChanged),
            name: Notification.Name(kTISNotifyEnabledKeyboardInputSourcesChanged as String),
            object: nil, suspensionBehavior: .deliverImmediately)
    }

    @objc private func selectedSourceChanged() {
        reloadCurrent()
        onSelectionChange?(current)
    }

    @objc private func enabledSourcesChanged() {
        reloadTables()
        reloadCurrent()
        onLayoutSetChange?()
    }

    public func stop() {
        DistributedNotificationCenter.default().removeObserver(self)
    }

    // MARK: - Queries

    public var current: KeyMapTable? {
        guard let currentSourceID else { return nil }
        return tables.first { $0.sourceID == currentSourceID }
    }

    /// The selected layout, read straight from the system rather than from cached state.
    ///
    /// Called on every keystroke, which is affordable: the underlying Text Input Services
    /// call measures at about 0.12 µs. Doing it this way is not an optimisation but a
    /// correctness fix — relying on the change notification alone left the app believing the
    /// user was still on the previous layout, so `ghbdtn` typed on ABC was read as `привет`
    /// typed on Russian, which is perfectly correct text and therefore never corrected.
    public func liveCurrentTable() -> KeyMapTable? {
        guard let source = TISCopyCurrentKeyboardInputSource()?.takeRetainedValue(),
            let id = KeyMapTable.stringProperty(source, kTISPropertyInputSourceID),
            let table = table(forSourceID: id)
        else {
            // A non-layout input method is in front; keep whatever we last knew.
            return current
        }

        if id != currentSourceID {
            currentSourceID = id
            // Off the keystroke path: rebuilding the menu is far too heavy for a callback with
            // a hard time budget, and the layout only changes occasionally.
            DispatchQueue.main.async { [weak self] in
                self?.onSelectionChange?(table)
            }
        }
        return table
    }

    /// Modelled languages the user actually has layouts for.
    public var availableLanguages: Set<Lang> {
        Set(tables.compactMap(\.lang))
    }

    public func table(forSourceID id: String) -> KeyMapTable? {
        tables.first { $0.sourceID == id }
    }

    public func table(for lang: Lang) -> KeyMapTable? {
        tables.first { $0.lang == lang }
    }

    public var cyrillicTables: [KeyMapTable] {
        tables.filter { $0.lang?.script == .cyrillic }
    }

    public var latinTables: [KeyMapTable] {
        tables.filter { $0.lang?.script == .latin }
    }

    /// Tables the detector may propose, filtered by what the user enabled.
    public func candidateTables(limitedTo languages: Set<Lang>) -> [KeyMapTable] {
        tables.filter { table in
            guard let lang = table.lang else { return false }
            return languages.contains(lang)
        }
    }

    // MARK: - Switching

    @discardableResult
    public func select(sourceID: String) -> Bool {
        guard let source = sources[sourceID] else { return false }
        let status = TISSelectInputSource(source)
        if status == noErr {
            currentSourceID = sourceID
            return true
        }
        return false
    }

    @discardableResult
    public func select(_ table: KeyMapTable) -> Bool {
        select(sourceID: table.sourceID)
    }

    @discardableResult
    public func select(lang: Lang) -> Bool {
        guard let table = table(for: lang) else { return false }
        return select(table)
    }

    // MARK: - Reload

    public func reloadTables() {
        var tables: [KeyMapTable] = []
        var sources: [String: TISInputSource] = [:]

        // nil filter + includeAllInstalled:false → the sources the user has enabled.
        if let list = TISCreateInputSourceList(nil, false)?.takeRetainedValue()
            as? [TISInputSource]
        {
            for source in list {
                guard let table = KeyMapTable.build(from: source) else { continue }
                tables.append(table)
                sources[table.sourceID] = source
            }
        }

        self.tables = tables
        self.sources = sources
    }

    public func reloadCurrent() {
        guard let source = TISCopyCurrentKeyboardInputSource()?.takeRetainedValue() else {
            currentSourceID = nil
            return
        }
        if let id = KeyMapTable.stringProperty(source, kTISPropertyInputSourceID),
            sources[id] != nil
        {
            currentSourceID = id
            return
        }
        // A non-layout source (an IME) is active. Fall back to the keyboard layout
        // backing it, which is what typed characters actually come from.
        if let layoutSource = TISCopyCurrentKeyboardLayoutInputSource()?.takeRetainedValue() {
            currentSourceID = KeyMapTable.stringProperty(layoutSource, kTISPropertyInputSourceID)
        } else {
            currentSourceID = nil
        }
    }
}

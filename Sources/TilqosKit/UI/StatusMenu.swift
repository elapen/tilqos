import AppKit

/// Everything the status menu shows, gathered so the menu is a pure function of it — built
/// fresh each time it opens, and renderable in tests without a running app.
struct StatusMenuState {
    var health: AppHealth
    var enabled: Bool
    var layoutName: String?
    var currentLang: Lang?
    var installedLangs: Set<Lang>
    var lastFix: AppliedFix?
    var frontApp: FrontApp?
    var hotkeys: [HotkeyAction: Hotkey]
    var translitScheme: TranslitScheme
    var now = Date()
}

/// The app the user was in when they opened the menu.
struct FrontApp {
    var bundleID: String
    var name: String
    var icon: NSImage?
    var mode: FixMode
    var locked: Bool
}

struct StatusMenuActions {
    var setEnabled: (Bool) -> Void = { _ in }
    var fixPermissions: () -> Void = {}
    var restartTap: () -> Void = {}
    /// bundle ID, and whether Tilqos should stay out of that app.
    var setAppExcluded: (String, Bool) -> Void = { _, _ in }
    var undo: () -> Void = {}
    var perform: (HotkeyAction) -> Void = { _ in }
    var transliterate: (Transliterator.Direction?) -> Void = { _ in }
    var setScheme: (TranslitScheme) -> Void = { _ in }
    var openSettings: () -> Void = {}
    var openTutorial: () -> Void = {}
    var quit: () -> Void = {}
}

/// Builds the status item's menu.
///
/// Ordered by how often each part is needed: state and the on/off switch first, then the
/// two things people reach for mid-typing — "not in this app" and "undo that" — then the
/// manual commands, then the rarely used rest.
@MainActor
enum StatusMenu {
    static func populate(_ menu: NSMenu, state: StatusMenuState, actions: StatusMenuActions) {
        // The header is kept when the menu is rebuilt while open: its switch may be the very
        // control that triggered the rebuild, and it is under the pointer.
        let header = menu.items.first { $0.view is MenuHeaderView } ?? NSMenuItem()
        let headerView = header.view as? MenuHeaderView ?? MenuHeaderView()
        headerView.update(state: state)
        headerView.onToggle = actions.setEnabled
        header.view = headerView

        menu.autoenablesItems = false
        for item in menu.items where item !== header { menu.removeItem(item) }
        if header.menu == nil { menu.addItem(header) }

        addProblem(to: menu, state: state, actions: actions)
        menu.addItem(.separator())

        if let app = state.frontApp {
            menu.addItem(appItem(app, actions: actions))
        }
        menu.addItem(undoItem(state: state, actions: actions))
        menu.addItem(.separator())

        for (hotkey, title, symbol) in [
            (HotkeyAction.fixLastWord, L10n.tr("menu.fixLastWord"), "character.cursor.ibeam"),
            (.fixSelection, L10n.tr("menu.fixSelection"), "text.viewfinder"),
            (.convertClipboard, L10n.tr("menu.convertClipboard"), "doc.on.clipboard"),
        ] {
            let item = ActionMenuItem(title: title, symbol: symbol) { actions.perform(hotkey) }
            item.isEnabled = state.enabled
            attachShortcut(state.hotkeys[hotkey], to: item)
            menu.addItem(item)
        }

        let translit = submenuItem(L10n.tr("menu.transliteration"), symbol: "character.textbox")
        translit.submenu = translitMenu(state: state, actions: actions)
        translit.isEnabled = state.enabled
        menu.addItem(translit)

        let layouts = submenuItem(L10n.tr("menu.layout"), symbol: "keyboard")
        layouts.submenu = layoutMenu(state: state, actions: actions)
        menu.addItem(layouts)
        menu.addItem(.separator())

        let settings = ActionMenuItem(
            title: L10n.tr("menu.settings"), symbol: "gearshape", action: actions.openSettings)
        settings.keyEquivalent = ","
        settings.keyEquivalentModifierMask = .command
        menu.addItem(settings)
        menu.addItem(
            ActionMenuItem(
                title: L10n.tr("menu.tutorial"), symbol: "graduationcap", action: actions.openTutorial))
        menu.addItem(.separator())
        let quit = ActionMenuItem(title: L10n.tr("menu.quit"), symbol: nil, action: actions.quit)
        quit.keyEquivalent = "q"
        quit.keyEquivalentModifierMask = .command
        menu.addItem(quit)
    }

    // MARK: - Sections

    /// A problem gets a row of its own with the fix on it, above everything else: every
    /// other symptom of a missing permission looks like "nothing happens".
    private static func addProblem(
        to menu: NSMenu, state: StatusMenuState, actions: StatusMenuActions
    ) {
        let item: ActionMenuItem
        let detail: String
        switch state.health {
        case .noPermission:
            item = ActionMenuItem(
                title: L10n.tr("menu.problem.noPermission"), symbol: nil,
                action: actions.fixPermissions)
            detail = L10n.tr("menu.problem.noPermission.detail")
        case .suggestionsOnly:
            item = ActionMenuItem(
                title: L10n.tr("menu.problem.suggestionsOnly"), symbol: nil,
                action: actions.fixPermissions)
            detail = L10n.tr("menu.problem.suggestionsOnly.detail")
        case .tapStopped:
            item = ActionMenuItem(
                title: L10n.tr("menu.problem.tapStopped"), symbol: nil, action: actions.restartTap)
            detail = L10n.tr("menu.problem.tapStopped.detail")
        case .working, .hotkeysOnly, .disabled:
            return
        }
        item.image = coloredSymbol("exclamationmark.triangle.fill", .systemOrange)
        setSubtitle(detail, on: item)
        menu.addItem(.separator())
        menu.addItem(item)
    }

    private static func appItem(_ app: FrontApp, actions: StatusMenuActions) -> NSMenuItem {
        if app.locked {
            let item = NSMenuItem(
                title: L10n.tr("menu.app.locked", app.name), action: nil, keyEquivalent: "")
            item.isEnabled = false
            item.image = app.icon.map(menuIcon)
            item.toolTip = L10n.tr("menu.app.locked.tip")
            return item
        }
        let excluded = app.mode == .off
        let item = ActionMenuItem(title: L10n.tr("menu.app.exclude", app.name), symbol: nil) {
            actions.setAppExcluded(app.bundleID, !excluded)
        }
        item.state = excluded ? .on : .off
        item.image = app.icon.map(menuIcon)
        if app.mode == .suggest {
            setSubtitle(L10n.tr("menu.app.suggestOnly"), on: item)
        }
        return item
    }

    private static func undoItem(state: StatusMenuState, actions: StatusMenuActions) -> NSMenuItem {
        let item = ActionMenuItem(
            title: L10n.tr("menu.undo"), symbol: "arrow.uturn.backward", action: actions.undo)
        attachShortcut(state.hotkeys[.undoLastFix], to: item)
        guard let fix = state.lastFix else {
            item.isEnabled = false
            return item
        }
        let change =
            "\(HintView.trimmed(fix.originalWord, limit: 18)) → "
            + HintView.trimmed(fix.correctedWord, limit: 18)
        let what = fix.kind == .transliteration ? L10n.tr("menu.undo.translit", change) : change
        if #available(macOS 14.4, *) {
            item.subtitle = "\(what) · \(ago(fix.time, now: state.now))"
        } else {
            item.title = L10n.tr("menu.undo.item", what)
        }
        return item
    }

    private static func translitMenu(state: StatusMenuState, actions: StatusMenuActions) -> NSMenu {
        let menu = NSMenu()
        menu.autoenablesItems = false
        let auto = ActionMenuItem(
            title: L10n.tr("menu.translit.auto"), symbol: "arrow.left.arrow.right"
        ) {
            actions.transliterate(nil)
        }
        attachShortcut(state.hotkeys[.transliterate], to: auto)
        setSubtitle(L10n.tr("menu.translit.auto.detail"), on: auto)
        menu.addItem(auto)
        // No icons on these two: there is no symbol for "Cyrillic", and a Latin stand-in
        // would say the opposite of what the item does.
        menu.addItem(
            ActionMenuItem(title: L10n.tr("menu.translit.toCyrillic"), symbol: nil) {
                actions.transliterate(.toCyrillic)
            })
        menu.addItem(
            ActionMenuItem(title: L10n.tr("menu.translit.toLatin"), symbol: nil) {
                actions.transliterate(.toLatin)
            })
        menu.addItem(.separator())
        menu.addItem(.sectionHeader(title: L10n.tr("translit.latin")))
        for scheme in TranslitScheme.allCases {
            let item = ActionMenuItem(title: scheme.displayName, symbol: nil) {
                actions.setScheme(scheme)
            }
            item.state = state.translitScheme == scheme ? .on : .off
            setSubtitle(scheme.explanation, on: item)
            menu.addItem(item)
        }
        return menu
    }

    private static func layoutMenu(state: StatusMenuState, actions: StatusMenuActions) -> NSMenu {
        let menu = NSMenu()
        menu.autoenablesItems = false
        for (hotkey, lang) in [
            (HotkeyAction.forceEnglish, Lang.en), (.forceRussian, .ru), (.forceKazakh, .kk),
        ] {
            let item = ActionMenuItem(title: lang.displayName, symbol: nil) {
                actions.perform(hotkey)
            }
            item.image = StatusIcon.keycap(lang.badge)
            item.state = state.currentLang == lang ? .on : .off
            let installed = state.installedLangs.contains(lang)
            item.isEnabled = installed
            if !installed { setSubtitle(L10n.tr("languages.detail.missing"), on: item) }
            attachShortcut(state.hotkeys[hotkey], to: item)
            menu.addItem(item)
        }
        return menu
    }

    // MARK: - Helpers

    private static func submenuItem(_ title: String, symbol: String) -> NSMenuItem {
        let item = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        item.image = symbolImage(symbol)
        return item
    }

    nonisolated static func symbolImage(_ name: String) -> NSImage? {
        let configuration = NSImage.SymbolConfiguration(pointSize: 13, weight: .regular)
        let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)?
            .withSymbolConfiguration(configuration)
        image?.isTemplate = true
        return image
    }

    private static func coloredSymbol(_ name: String, _ color: NSColor) -> NSImage? {
        let configuration = NSImage.SymbolConfiguration(pointSize: 13, weight: .regular)
            .applying(.init(paletteColors: [color]))
        let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)?
            .withSymbolConfiguration(configuration)
        image?.isTemplate = false
        return image
    }

    private static func menuIcon(_ icon: NSImage) -> NSImage {
        let copy = icon.copy() as? NSImage ?? icon
        copy.size = NSSize(width: 16, height: 16)
        return copy
    }

    /// Second line under the title where the system can draw one (macOS 14.4+); older
    /// systems show it as a tooltip rather than cramming it into the title.
    static func setSubtitle(_ text: String, on item: NSMenuItem) {
        if #available(macOS 14.4, *) {
            item.subtitle = text
        } else {
            item.toolTip = text
        }
    }

    /// Shows a hotkey on the right of a menu row, so it can be learned instead of hiding in
    /// a settings pane.
    static func attachShortcut(_ hotkey: Hotkey?, to item: NSMenuItem) {
        guard let hotkey, let key = KeyNames.menuKeyEquivalent(hotkey.keyCode) else { return }
        item.keyEquivalent = key
        var mask: NSEvent.ModifierFlags = []
        if hotkey.modifiers.contains(.command) { mask.insert(.command) }
        if hotkey.modifiers.contains(.option) { mask.insert(.option) }
        if hotkey.modifiers.contains(.shift) { mask.insert(.shift) }
        if hotkey.modifiers.contains(.control) { mask.insert(.control) }
        item.keyEquivalentModifierMask = mask
    }

    static func ago(_ date: Date, now: Date) -> String {
        let seconds = now.timeIntervalSince(date)
        if seconds < 10 { return L10n.tr("menu.justNow") }
        let formatter = RelativeDateTimeFormatter()
        formatter.locale = L10n.locale
        formatter.unitsStyle = .short
        return formatter.localizedString(for: date, relativeTo: now)
    }
}

// MARK: - Items

/// A menu item that runs a closure, so the menu does not need a responder chain target
/// for every command.
final class ActionMenuItem: NSMenuItem {
    private let handler: () -> Void

    init(title: String, symbol: String?, action handler: @escaping () -> Void) {
        self.handler = handler
        super.init(title: title, action: #selector(fire), keyEquivalent: "")
        target = self
        if let symbol { image = StatusMenu.symbolImage(symbol) }
    }

    required init(coder: NSCoder) {
        fatalError("init(coder:) is not used")
    }

    @objc private func fire() {
        handler()
    }
}

/// Name, state and the master switch, the way the Wi-Fi and Bluetooth menus open.
///
/// The switch works without closing the menu, so the effect — the status line changing —
/// is visible right where it was flipped.
final class MenuHeaderView: NSView {
    var onToggle: ((Bool) -> Void)?

    private let title = NSTextField(labelWithString: "Tilqos")
    private let status = NSTextField(labelWithString: "")
    private let toggle = NSSwitch()

    init() {
        super.init(frame: NSRect(x: 0, y: 0, width: 300, height: 50))
        autoresizingMask = [.width]

        title.font = .systemFont(ofSize: 13, weight: .semibold)
        status.font = .systemFont(ofSize: 11.5)
        status.lineBreakMode = .byTruncatingTail
        toggle.controlSize = .small
        toggle.target = self
        toggle.action = #selector(toggled)
        toggle.setAccessibilityLabel(L10n.tr("menu.toggle.label"))

        let text = NSStackView(views: [title, status])
        text.orientation = .vertical
        text.alignment = .leading
        text.spacing = 1
        for view in [text, toggle] as [NSView] {
            view.translatesAutoresizingMaskIntoConstraints = false
            addSubview(view)
        }
        NSLayoutConstraint.activate([
            text.leadingAnchor.constraint(equalTo: leadingAnchor, constant: 14),
            text.centerYAnchor.constraint(equalTo: centerYAnchor),
            text.trailingAnchor.constraint(lessThanOrEqualTo: toggle.leadingAnchor, constant: -12),
            toggle.trailingAnchor.constraint(equalTo: trailingAnchor, constant: -14),
            toggle.centerYAnchor.constraint(equalTo: centerYAnchor),
        ])
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not used")
    }

    func update(state: StatusMenuState) {
        toggle.state = state.enabled ? .on : .off
        let health = state.health
        let line = NSMutableAttributedString(
            string: "● ",
            attributes: [.foregroundColor: health.color, .font: NSFont.systemFont(ofSize: 9)])
        var words = health.title
        if health == .working || health == .hotkeysOnly, let layout = state.layoutName {
            words += " · \(layout)"
        }
        line.append(
            NSAttributedString(
                string: words,
                attributes: [
                    .foregroundColor: health.needsAttention ? health.color : .secondaryLabelColor,
                    .font: NSFont.systemFont(ofSize: 11.5),
                ]))
        status.attributedStringValue = line
        toggle.setAccessibilityLabel(L10n.tr("menu.toggle.label"))
        setAccessibilityLabel(L10n.tr("status.tooltip", words))
    }

    @objc private func toggled() {
        onToggle?(toggle.state == .on)
    }
}

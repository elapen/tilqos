import AppKit
import ObjectiveC
import SwiftUI
import XCTest

@testable import TilqosKit

/// Renders every window of the app to PNG, in light and dark and in each interface language
/// (English, Russian, Kazakh), for reviewing UI changes. Russian and Kazakh texts are the long
/// ones, so this is where truncation and overflow show up.
///
/// Skipped unless `TILQOS_SNAPSHOTS` names an output directory:
///
///     TILQOS_SNAPSHOTS=$PWD/build/screenshots swift test --filter UISnapshotTests
///
/// File names end in `-<appearance>-<language>`. `TILQOS_SNAPSHOT_LANGS=ru,kk` renders only
/// some languages.
///
/// Nothing here starts an event tap, asks for a permission or touches the app's own
/// defaults domain: views are built from a throwaway `UserDefaults` suite and hosted in
/// invisible windows parked far off screen.
final class UISnapshotTests: XCTestCase {
    private var outputDirectory: URL!

    override func setUpWithError() throws {
        guard let path = ProcessInfo.processInfo.environment["TILQOS_SNAPSHOTS"] else {
            throw XCTSkip("set TILQOS_SNAPSHOTS=<dir> to render UI snapshots")
        }
        outputDirectory = URL(fileURLWithPath: path, isDirectory: true)
        try FileManager.default.createDirectory(
            at: outputDirectory, withIntermediateDirectories: true)
        MainActor.assumeIsolated { Snapshot.prepareApplication() }
        // The texts and licenses sit next to the models in the real bundle.
        let packageRoot = MainActor.assumeIsolated { Snapshot.packageRoot }
        L10n.useResourceRoots([packageRoot.appendingPathComponent("Resources")])
        LicenseFiles.roots = [packageRoot]
    }

    override func tearDown() {
        ScratchDefaults.removeAll()
        L10n.reset()
        LicenseFiles.roots = nil
    }

    // MARK: - Suggestion panel

    /// The card as it appears at the caret. Rendered with the opaque background: an
    /// offscreen capture cannot blur what is behind a window, and the opaque fill is the
    /// closest stand-in for the material over a text field.
    @MainActor
    func testSuggestionPanel() throws {
        for lang in Snapshot.languages { try renderSuggestionPanel(lang) }
    }

    @MainActor
    private func renderSuggestionPanel(_ lang: Lang) throws {
        Snapshot.use(lang)
        let variants: [(String, HintView.Content)] = [
            ("suggestion", .init(before: "ghbdtn", after: "привет", style: .suggestion(acceptKey: "Tab"))),
            (
                "suggestion-tip",
                .init(
                    before: "ghbdtn", after: "привет", style: .suggestion(acceptKey: "Tab"),
                    tip: SuggestionPanel.tip(for: .tab))
            ),
            (
                "suggestion-optionspace",
                .init(before: "с2леметс3з", after: "сәлеметсіз", style: .suggestion(acceptKey: "⌥ Space"))
            ),
            ("applied", .init(before: "ghbdtn", after: "привет", style: .applied(undoKey: "⌥⇧Z"))),
            (
                "suggestion-long",
                .init(
                    before: "Ghbdtn rfr ltkf e nt,z ctujlyz", after: "Привет как дела у тебя сегодня",
                    style: .suggestion(acceptKey: "Tab"))
            ),
        ]
        for appearance in Snapshot.appearances {
            for (name, content) in variants {
                let view = HintView(background: .opaque)
                view.configure(content)
                // The timeout line, frozen at its starting length.
                if name == "suggestion" { view.startCountdown(4) }
                let image = Snapshot.captureDetached(view, appearance: appearance.name)
                let scene = Snapshot.caretScene(panel: image, appearance: appearance.name)
                try Snapshot.write(
                    scene, "panel-\(name)-\(appearance.suffix)-\(lang.rawValue)",
                    in: outputDirectory)
            }
        }
    }

    // MARK: - Settings

    @MainActor
    func testSettings() throws {
        for lang in Snapshot.languages { try renderSettings(lang) }
    }

    @MainActor
    private func renderSettings(_ lang: Lang) throws {
        Snapshot.use(lang)
        let prefs = Prefs(defaults: Snapshot.scratchDefaults())
        prefs.wordExceptions = [
            "ghbdtn", "iphone", "qwerty", "zoom", "figma", "jira", "slack", "notion", "lfq",
            "rjl",
        ]
        prefs.translitNeverConvert = ["instagram", "kaspi"]
        prefs.showHUD = true
        prefs.appRules = [
            "com.apple.Safari": FixMode.suggest.rawValue,
            "com.apple.dt.Xcode": FixMode.off.rawValue,
        ]
        let sources = InputSourceManager()
        sources.reloadTables()
        sources.reloadCurrent()

        for appearance in Snapshot.appearances {
            let model = SettingsModel(prefs: prefs, inputSources: sources, pipeline: nil)
            // The state of a working install, not of the test runner.
            model.readPermissions = { Permissions.Status(inputMonitoring: false, accessibility: true) }
            model.readTapRunning = { true }
            model.readVersion = { "1.0.0" }
            model.learner = Snapshot.sampleLearner()
            let window = SnapshotWindow(
                contentRect: NSRect(x: 0, y: 0, width: 760, height: 560),
                styleMask: [.titled, .closable, .miniaturizable, .resizable, .fullSizeContentView],
                backing: .buffered, defer: false)
            window.title = L10n.tr("settings.title")
            window.toolbarStyle = .unified
            window.appearance = NSAppearance(named: appearance.name)
            window.contentViewController = NSHostingController(rootView: SettingsView(model: model))
            window.setContentSize(NSSize(width: 760, height: 560))
            Snapshot.park(window)

            for pane in SettingsPane.allCases {
                model.pane = pane
                Snapshot.settle(0.5)
                let frame = window.contentView!.superview!
                let image = Snapshot.capture(frame, scale: 1)
                let sidebar = Snapshot.sidebarOverlay(
                    in: window, model: model, appearance: appearance.name)
                try Snapshot.write(
                    Snapshot.composite(image, over: sidebar),
                    "settings-\(SettingsPane.allCases.firstIndex(of: pane)!)-\(pane.rawValue)-\(appearance.suffix)-\(lang.rawValue)",
                    in: outputDirectory)
            }

            // Long panes scroll in the real window; a tall one shows everything at once, which
            // is what reveals text that is cut off or wraps badly. Light only: dark changes
            // colours, not layout.
            window.setContentSize(NSSize(width: 760, height: 1600))
            for pane in SettingsPane.allCases where appearance.name == .aqua {
                model.pane = pane
                Snapshot.settle(0.5)
                let sidebar = Snapshot.sidebarOverlay(
                    in: window, model: model, appearance: appearance.name)
                try Snapshot.write(
                    Snapshot.composite(
                        Snapshot.capture(window.contentView!.superview!, scale: 1), over: sidebar),
                    "settings-\(SettingsPane.allCases.firstIndex(of: pane)!)-\(pane.rawValue)-full-\(appearance.suffix)-\(lang.rawValue)",
                    in: outputDirectory)
            }
            window.orderOut(nil)
        }
    }

    // MARK: - Onboarding

    @MainActor
    func testOnboarding() throws {
        for lang in Snapshot.languages { try renderOnboarding(lang) }
    }

    @MainActor
    private func renderOnboarding(_ lang: Lang) throws {
        Snapshot.use(lang)
        for appearance in Snapshot.appearances {
            for (state, status) in [
                ("missing", Permissions.Status(inputMonitoring: false, accessibility: false)),
                ("granted", Permissions.Status(inputMonitoring: false, accessibility: true)),
            ] {
                let watcher = PermissionWatcher()
                watcher.status = status
                let host = Snapshot.host(
                    OnboardingView(watcher: watcher, openSettings: {}, onFinish: {}),
                    size: NSSize(width: 460, height: 520), appearance: appearance.name,
                    title: L10n.tr("onboarding.window"), hideTitle: true)
                try Snapshot.write(
                    Snapshot.capture(host.frameView, scale: 1),
                    "onboarding-\(state)-\(appearance.suffix)-\(lang.rawValue)",
                    in: outputDirectory)
                host.close()
            }
        }
    }

    // MARK: - Tutorial

    @MainActor
    func testTutorial() throws {
        for lang in Snapshot.languages { try renderTutorial(lang) }
    }

    @MainActor
    private func renderTutorial(_ lang: Lang) throws {
        Snapshot.use(lang)
        let sources = InputSourceManager()
        sources.reloadTables()
        sources.reloadCurrent()
        let prefs = Prefs(defaults: Snapshot.scratchDefaults())
        for appearance in Snapshot.appearances {
            let model = TutorialModel(inputSources: sources, prefs: prefs)
            let host = Snapshot.host(
                TutorialView(model: model, onFinish: {}), size: NSSize(width: 580, height: 540),
                appearance: appearance.name, title: L10n.tr("tutorial.window"), hideTitle: true)
            for step in model.steps {
                model.current = step.id
                Snapshot.settle()
                try Snapshot.write(
                    Snapshot.capture(host.frameView, scale: 1),
                    "tutorial-\(step.id)-\(appearance.suffix)-\(lang.rawValue)",
                    in: outputDirectory)
            }
            // A finished exercise, to see the success state.
            model.current = 0
            model.practice[0] = "привет "
            Snapshot.settle()
            try Snapshot.write(
                Snapshot.capture(host.frameView, scale: 1),
                "tutorial-0-done-\(appearance.suffix)-\(lang.rawValue)", in: outputDirectory)
            host.close()
        }
    }

    // MARK: - Menu

    @MainActor
    func testStatusMenu() throws {
        for lang in Snapshot.languages { try renderStatusMenu(lang) }
    }

    @MainActor
    private func renderStatusMenu(_ lang: Lang) throws {
        Snapshot.use(lang)
        let fix = AppliedFix(
            replacement: Replacement(deletedText: "ghbdtn", insertText: "привет"),
            bundleID: "com.apple.Safari", time: Date().addingTimeInterval(-95),
            originalWord: "ghbdtn", correctedWord: "привет", previousLayoutID: nil,
            strategy: .accessibility)
        let safari = FrontApp(
            bundleID: "com.apple.Safari", name: "Safari",
            icon: AppInfo.icon(for: "com.apple.Safari"), mode: .auto, locked: false)
        func state(_ health: AppHealth, enabled: Bool = true, fix: AppliedFix? = nil)
            -> StatusMenuState
        {
            StatusMenuState(
                health: health, enabled: enabled, layoutName: "Русская — ПК", currentLang: .ru,
                installedLangs: [.en, .ru, .kk], lastFix: fix, frontApp: safari,
                hotkeys: Prefs(defaults: Snapshot.scratchDefaults()).hotkeys,
                translitScheme: .practical)
        }
        let states: [(String, StatusMenuState)] = [
            ("ok", state(.working, fix: fix)),
            ("disabled", state(.disabled, enabled: false)),
            ("nopermission", state(.noPermission)),
        ]
        for appearance in Snapshot.appearances {
            for (name, state) in states {
                let menu = NSMenu()
                StatusMenu.populate(menu, state: state, actions: StatusMenuActions())
                let image = MenuSnapshot.render(menu, appearance: appearance.name)
                try Snapshot.write(
                    image, "menu-\(name)-\(appearance.suffix)-\(lang.rawValue)",
                    in: outputDirectory)
            }
        }
    }

    /// The status item in each state, drawn into a strip of menu bar.
    @MainActor
    func testStatusIcon() throws {
        let icons: [(NSImage, Bool)] = [
            (StatusIcon.image(badge: "EN", health: .working), false),
            (StatusIcon.image(badge: "РУ", health: .working), false),
            (StatusIcon.image(badge: "ҚАЗ", health: .working), false),
            (StatusIcon.image(badge: "РУ", health: .disabled), true),
            (StatusIcon.image(badge: "РУ", health: .noPermission), false),
            (StatusIcon.image(badge: nil, health: .working), false),
        ]
        for appearance in Snapshot.appearances {
            let dark = appearance.name == .darkAqua
            let image = Snapshot.drawn(
                size: NSSize(width: 360, height: 24), appearance: appearance.name, scale: 3
            ) { bounds in
                (dark ? NSColor(white: 0.16, alpha: 1) : NSColor(white: 0.93, alpha: 1)).setFill()
                bounds.fill()
                var x: CGFloat = 16
                for (icon, dimmed) in icons {
                    let tinted = NSImage(size: icon.size, flipped: false) { rect in
                        icon.draw(in: rect)
                        (dark ? NSColor.white : NSColor.black).set()
                        rect.fill(using: .sourceAtop)
                        return true
                    }
                    tinted.draw(
                        in: NSRect(
                            x: x, y: (bounds.height - icon.size.height) / 2,
                            width: icon.size.width, height: icon.size.height),
                        from: .zero, operation: .sourceOver, fraction: dimmed ? 0.35 : 1)
                    x += icon.size.width + 26
                }
            }
            try Snapshot.write(image, "statusicon-\(appearance.suffix)", in: outputDirectory)
        }
    }
}

// MARK: - Rendering helpers

@MainActor
enum Snapshot {
    struct Appearance {
        let name: NSAppearance.Name
        let suffix: String
    }

    static let appearances = [
        Appearance(name: .aqua, suffix: "light"), Appearance(name: .darkAqua, suffix: "dark"),
    ]

    private static var prepared = false

    /// The interface languages to render: all three, or the ones `TILQOS_SNAPSHOT_LANGS` lists.
    static var languages: [Lang] {
        guard let list = ProcessInfo.processInfo.environment["TILQOS_SNAPSHOT_LANGS"] else {
            return Lang.allCases
        }
        let wanted = list.split(separator: ",").compactMap { Lang(rawValue: String($0)) }
        return wanted.isEmpty ? Lang.allCases : wanted
    }

    /// Switches every `L10n` text to `lang`, as the Settings picker does.
    static func use(_ lang: Lang) {
        L10n.setting = UILanguageSetting(rawValue: lang.rawValue) ?? .system
    }

    /// A test process has no running application, so controls would draw in their inactive
    /// style. Pretending the app is active makes the renders match what users see.
    static func prepareApplication() {
        guard !prepared else { return }
        prepared = true
        let app = NSApplication.shared
        if let method = class_getInstanceMethod(
            NSApplication.self, #selector(getter: NSApplication.isActive))
        {
            let block: @convention(block) (AnyObject) -> Bool = { _ in true }
            method_setImplementation(method, imp_implementationWithBlock(block))
        }
        // The real app has its models loaded by the time anyone opens a window.
        ModelStore.shared.load(from: packageRoot.appendingPathComponent("Resources/Models"))
        let icon = packageRoot.appendingPathComponent("Resources/icon-1024.png")
        if let image = NSImage(contentsOf: icon) { app.applicationIconImage = image }
    }

    static var packageRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    }

    /// Fresh defaults under the temp directory; swept by `UISnapshotTests.tearDown`.
    static func scratchDefaults() -> UserDefaults {
        ScratchDefaults.make().defaults
    }

    /// A learner with a little of everything, in its own scratch store.
    static func sampleLearner() -> Learner {
        let learner = Learner(defaults: scratchDefaults())
        for word in ["tot", "ult", "her"] { learner.rejected(typed: word) }
        for word in ["ghbdtn", "rfr", "cgfcb,j", "lf", "yfgbie"] {
            learner.confirmed(typed: word, to: .ru)
        }
        for _ in 0..<Learner.vocabularySightings {
            for word in ["kaspi", "zoom"] { learner.kept(typed: word, lang: .en) }
        }
        learner.ignored(typed: "yjhv")
        return learner
    }

    final class Host {
        let window: NSWindow
        init(window: NSWindow) { self.window = window }
        var frameView: NSView { window.contentView!.superview! }
        func close() { window.orderOut(nil) }
    }

    static func host<V: View>(
        _ view: V, size: NSSize, appearance: NSAppearance.Name, title: String,
        hideTitle: Bool = false
    ) -> Host {
        let window = SnapshotWindow(
            contentRect: NSRect(origin: .zero, size: size),
            styleMask: [.titled, .closable, .fullSizeContentView],
            backing: .buffered, defer: false)
        window.title = title
        window.titlebarAppearsTransparent = true
        if hideTitle { window.titleVisibility = .hidden }
        window.appearance = NSAppearance(named: appearance)
        window.contentView = NSHostingView(rootView: view)
        window.setContentSize(size)
        park(window)
        return Host(window: window)
    }

    /// On screen as far as AppKit is concerned, so lists and focus behave normally, but
    /// invisible and click-through, and nowhere near the user's displays.
    static func park(_ window: NSWindow) {
        window.alphaValue = 0
        window.ignoresMouseEvents = true
        window.setFrameOrigin(NSPoint(x: -30000, y: -30000))
        window.makeKeyAndOrderFront(nil)
        settle()
    }

    static func settle(_ seconds: TimeInterval = 0.35) {
        RunLoop.main.run(until: Date().addingTimeInterval(seconds))
    }

    static func capture(_ view: NSView, scale: CGFloat) -> NSImage {
        view.layoutSubtreeIfNeeded()
        view.displayIfNeeded()
        let bounds = view.bounds
        let rep = NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: Int(bounds.width * scale),
            pixelsHigh: Int(bounds.height * scale), bitsPerSample: 8, samplesPerPixel: 4,
            hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0,
            bitsPerPixel: 0)!
        rep.size = bounds.size
        view.cacheDisplay(in: bounds, to: rep)
        let image = NSImage(size: bounds.size)
        image.addRepresentation(rep)
        return image
    }

    /// Captures a free-standing view at its fitting size, with a transparent surround so it
    /// can be composited like a floating panel.
    static func captureDetached(
        _ view: NSView, appearance: NSAppearance.Name, size: NSSize? = nil
    ) -> NSImage {
        let window = SnapshotWindow(
            contentRect: NSRect(x: 0, y: 0, width: 700, height: 200),
            styleMask: [.borderless], backing: .buffered, defer: false)
        window.isOpaque = false
        window.backgroundColor = .clear
        window.appearance = NSAppearance(named: appearance)
        let container = NSView(frame: NSRect(x: 0, y: 0, width: 700, height: 200))
        window.contentView = container
        container.addSubview(view)
        view.frame = NSRect(origin: .zero, size: size ?? view.fittingSize)
        park(window)
        let image = capture(view, scale: 2)
        window.orderOut(nil)
        return image
    }

    /// The split view's sidebar is drawn through vibrancy, which a cached display cannot
    /// capture — its rows come out blank. A flat stand-in with the same rows is laid over
    /// the column instead, so the renders still show what the sidebar lists and selects.
    static func sidebarOverlay(
        in window: NSWindow, model: SettingsModel, appearance: NSAppearance.Name
    ) -> (NSImage, NSRect)? {
        guard let split = find(NSSplitView.self, in: window.contentView!),
            let column = split.arrangedSubviews.first
        else { return nil }
        let width = column.frame.width
        let height = window.contentView!.bounds.height
        let dark = appearance == .darkAqua
        let host = Snapshot.host(
            SidebarStandIn(selected: model.pane, dark: dark)
                .frame(width: width, height: height),
            size: NSSize(width: width, height: height),
            appearance: appearance, title: "", hideTitle: true)
        host.window.isOpaque = false
        host.window.backgroundColor = .clear
        settle(0.2)
        let image = capture(host.window.contentView!, scale: 1)
        host.close()
        return (image, NSRect(x: 0, y: 0, width: width, height: height))
    }

    static func composite(_ base: NSImage, over overlay: (NSImage, NSRect)?) -> NSImage {
        guard let (image, rect) = overlay else { return base }
        return drawn(size: base.size, appearance: .aqua, scale: 1) { bounds in
            base.draw(in: bounds)
            image.draw(in: rect, from: .zero, operation: .sourceOver, fraction: 1)
            NSColor.black.withAlphaComponent(0.08).setFill()
            NSRect(x: rect.maxX, y: 0, width: 1, height: rect.height).fill()
        }
    }

    static func find<T: NSView>(_ type: T.Type, in view: NSView) -> T? {
        if let match = view as? T { return match }
        for subview in view.subviews {
            if let match = find(type, in: subview) { return match }
        }
        return nil
    }

    /// Puts a panel render under a line of text with a caret, the way it appears in use.
    static func caretScene(panel: NSImage, appearance: NSAppearance.Name) -> NSImage {
        let font = NSFont.systemFont(ofSize: 15)
        let sample = "Сегодня напишу вам ghbdtn "
        let textWidth = (sample as NSString).size(withAttributes: [.font: font]).width
        let size = NSSize(
            width: max(24 + textWidth - 8 + panel.size.width + 24, 420),
            height: panel.size.height + 90)
        return drawn(size: size, appearance: appearance) { bounds in
            NSColor.textBackgroundColor.setFill()
            bounds.fill()
            let text = NSAttributedString(
                string: sample, attributes: [.font: font, .foregroundColor: NSColor.textColor])
            let origin = NSPoint(x: 24, y: bounds.height - 48)
            text.draw(at: origin)
            let caretX = origin.x + text.size().width
            NSColor.controlAccentColor.setFill()
            NSRect(x: caretX, y: origin.y - 2, width: 2, height: 21).fill()
            let panelOrigin = NSPoint(x: caretX - 12, y: origin.y - 8 - panel.size.height)
            panel.draw(
                in: NSRect(origin: panelOrigin, size: panel.size), from: .zero,
                operation: .sourceOver, fraction: 1)
        }
    }

    static func drawn(
        size: NSSize, appearance: NSAppearance.Name, scale: CGFloat = 2,
        _ body: (NSRect) -> Void
    ) -> NSImage {
        let rep = NSBitmapImageRep(
            bitmapDataPlanes: nil, pixelsWide: Int(size.width * scale),
            pixelsHigh: Int(size.height * scale), bitsPerSample: 8, samplesPerPixel: 4,
            hasAlpha: true, isPlanar: false, colorSpaceName: .deviceRGB, bytesPerRow: 0,
            bitsPerPixel: 0)!
        rep.size = size
        NSGraphicsContext.saveGraphicsState()
        NSGraphicsContext.current = NSGraphicsContext(bitmapImageRep: rep)
        NSAppearance(named: appearance)!.performAsCurrentDrawingAppearance {
            body(NSRect(origin: .zero, size: size))
        }
        NSGraphicsContext.restoreGraphicsState()
        let image = NSImage(size: size)
        image.addRepresentation(rep)
        return image
    }

    static func write(_ image: NSImage, _ name: String, in directory: URL) throws {
        guard let rep = image.representations.first as? NSBitmapImageRep,
            let data = rep.representation(using: .png, properties: [:])
        else { throw CocoaError(.fileWriteUnknown) }
        try data.write(to: directory.appendingPathComponent("\(name).png"))
    }
}

/// Same rows as `SettingsSidebar`, drawn without vibrancy. The top stays transparent so
/// the window's traffic lights show through.
private struct SidebarStandIn: View {
    let selected: SettingsPane
    let dark: Bool

    var body: some View {
        VStack(alignment: .leading, spacing: 1) {
            ForEach(SettingsPane.allCases) { pane in
                HStack(spacing: 8) {
                    SettingsIcon(symbol: pane.symbol, color: pane.color, size: 20)
                    Text(pane.title)
                        .foregroundStyle(pane == selected ? Color.white : Color.primary)
                    Spacer()
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 5)
                .background(
                    RoundedRectangle(cornerRadius: 7, style: .continuous)
                        .fill(pane == selected ? Color.accentColor : .clear))
            }
            Spacer()
        }
        .padding(.top, 58)
        .padding(.horizontal, 10)
        .background(alignment: .top) {
            VStack(spacing: 0) {
                Color.clear.frame(height: 52)
                Color(nsColor: dark ? NSColor(white: 0.17, alpha: 1) : NSColor(white: 0.92, alpha: 1))
            }
        }
    }
}

/// Always reports itself key, so controls render as they do in the focused window.
final class SnapshotWindow: NSWindow {
    /// Taller than the screen is fine here: a tall render shows a whole long pane at once.
    override func constrainFrameRect(_ frameRect: NSRect, to screen: NSScreen?) -> NSRect {
        frameRect
    }
    override var isKeyWindow: Bool { true }
    override var isMainWindow: Bool { true }
    override var canBecomeKey: Bool { true }
}

// MARK: - Menu renderer

/// Draws an `NSMenu` the way macOS lays it out, because a real menu cannot be rendered
/// off screen. Close enough to judge structure, wording and grouping.
@MainActor
enum MenuSnapshot {
    static func render(_ menu: NSMenu, appearance: NSAppearance.Name) -> NSImage {
        var menus = [menu]
        for item in menu.items { if let sub = item.submenu { menus.append(sub) } }
        let rendered = menus.map { draw($0, appearance: appearance) }
        let gap: CGFloat = 16
        let width = rendered.map(\.size.width).reduce(0, +) + gap * CGFloat(rendered.count + 1)
        let height = (rendered.map(\.size.height).max() ?? 0) + gap * 2
        return Snapshot.drawn(size: NSSize(width: width, height: height), appearance: appearance)
        { bounds in
            (appearance == .darkAqua
                ? NSColor(white: 0.13, alpha: 1) : NSColor(white: 0.86, alpha: 1)).setFill()
            bounds.fill()
            var x = gap
            for image in rendered {
                image.draw(
                    in: NSRect(
                        x: x, y: bounds.height - gap - image.size.height,
                        width: image.size.width, height: image.size.height),
                    from: .zero, operation: .sourceOver, fraction: 1)
                x += image.size.width + gap
            }
        }
    }

    private static let font = NSFont.menuFont(ofSize: 13)
    private static let rowHeight: CGFloat = 22
    private static let headerHeight: CGFloat = 20
    private static let separatorHeight: CGFloat = 11

    private static func height(of item: NSMenuItem) -> CGFloat {
        if item.isHidden { return 0 }
        if let view = item.view { return view.frame.height }
        if item.isSeparatorItem { return separatorHeight }
        if item.isSectionHeader { return headerHeight }
        if #available(macOS 14.4, *), item.subtitle != nil { return rowHeight + 15 }
        return rowHeight
    }

    private static func keyText(_ item: NSMenuItem) -> String {
        guard !item.keyEquivalent.isEmpty else { return "" }
        var text = ""
        let mask = item.keyEquivalentModifierMask
        if mask.contains(.control) { text += "⌃" }
        if mask.contains(.option) { text += "⌥" }
        if mask.contains(.shift) { text += "⇧" }
        if mask.contains(.command) { text += "⌘" }
        return text + item.keyEquivalent.uppercased()
    }

    private static func title(_ item: NSMenuItem) -> NSAttributedString {
        if let attributed = item.attributedTitle {
            let copy = NSMutableAttributedString(attributedString: attributed)
            copy.enumerateAttribute(
                .font, in: NSRange(location: 0, length: copy.length)
            ) { value, range, _ in
                if value == nil { copy.addAttribute(.font, value: font, range: range) }
            }
            return copy
        }
        let color: NSColor = item.isEnabled ? .labelColor : .tertiaryLabelColor
        return NSAttributedString(
            string: item.title, attributes: [.font: font, .foregroundColor: color])
    }

    private static func draw(_ menu: NSMenu, appearance: NSAppearance.Name) -> NSImage {
        let items = menu.items.filter { !$0.isHidden }
        let hasImages = items.contains { $0.image != nil }
        let leading: CGFloat = 12 + 14 + (hasImages ? 22 : 0)
        var width: CGFloat = 200
        for item in items {
            if let view = item.view {
                width = max(width, view.frame.width)
                continue
            }
            var row = leading + title(item).size().width + 14
            if #available(macOS 14.4, *), let subtitle = item.subtitle {
                let size = (subtitle as NSString).size(withAttributes: [
                    .font: NSFont.systemFont(ofSize: 11)
                ])
                row = max(row, leading + size.width + 14)
            }
            let key = keyText(item)
            if !key.isEmpty {
                row += 28 + (key as NSString).size(withAttributes: [.font: font]).width
            }
            if item.submenu != nil { row += 24 }
            width = max(width, row)
        }
        width = ceil(width)
        let contentHeight = items.map(height(of:)).reduce(0, +) + 10
        let size = NSSize(width: width + 30, height: contentHeight + 30)

        // Custom views are captured on their own, at the menu's width.
        var viewImages: [ObjectIdentifier: NSImage] = [:]
        for item in items {
            guard let view = item.view else { continue }
            view.frame.size.width = width
            viewImages[ObjectIdentifier(item)] = Snapshot.captureDetached(
                view, appearance: appearance, size: view.frame.size)
        }

        return Snapshot.drawn(size: size, appearance: appearance) { bounds in
            let body = bounds.insetBy(dx: 15, dy: 15)
            let shape = NSBezierPath(roundedRect: body, xRadius: 10, yRadius: 10)
            NSGraphicsContext.saveGraphicsState()
            let shadow = NSShadow()
            shadow.shadowBlurRadius = 12
            shadow.shadowOffset = NSSize(width: 0, height: -3)
            shadow.shadowColor = NSColor.black.withAlphaComponent(0.3)
            shadow.set()
            (appearance == .darkAqua
                ? NSColor(white: 0.19, alpha: 1) : NSColor(white: 0.96, alpha: 1)).setFill()
            shape.fill()
            NSGraphicsContext.restoreGraphicsState()
            NSColor.separatorColor.setStroke()
            shape.lineWidth = 0.5
            shape.stroke()

            var y = body.maxY - 5
            for item in items {
                let h = height(of: item)
                y -= h
                let row = NSRect(x: body.minX, y: y, width: body.width, height: h)
                if let image = viewImages[ObjectIdentifier(item)] {
                    image.draw(
                        in: NSRect(x: row.minX, y: row.minY, width: image.size.width, height: h),
                        from: .zero, operation: .sourceOver, fraction: 1)
                    continue
                }
                if item.isSeparatorItem {
                    NSColor.separatorColor.setFill()
                    NSRect(x: row.minX + 12, y: row.midY, width: row.width - 24, height: 1).fill()
                    continue
                }
                if item.isSectionHeader {
                    let text = NSAttributedString(
                        string: item.title,
                        attributes: [
                            .font: NSFont.systemFont(ofSize: 11, weight: .semibold),
                            .foregroundColor: NSColor.secondaryLabelColor,
                        ])
                    text.draw(at: NSPoint(x: row.minX + 14, y: row.minY + 3))
                    continue
                }
                let textColor: NSColor = item.isEnabled ? .labelColor : .tertiaryLabelColor
                let titleY = row.maxY - 19
                let indent = CGFloat(item.indentationLevel) * 10
                if item.state == .on {
                    NSAttributedString(
                        string: "✓",
                        attributes: [
                            .font: NSFont.systemFont(ofSize: 12, weight: .semibold),
                            .foregroundColor: textColor,
                        ]
                    ).draw(at: NSPoint(x: row.minX + 10 + indent, y: titleY + 1))
                }
                if let image = item.image {
                    let tinted = tint(image, textColor)
                    let ratio = image.size.width / max(image.size.height, 1)
                    let h = min(16, 20 / max(ratio, 0.01))
                    let w = h * ratio
                    tinted.draw(
                        in: NSRect(
                            x: row.minX + 26 + indent + (20 - w) / 2, y: titleY + (17 - h) / 2,
                            width: w, height: h),
                        from: .zero, operation: .sourceOver, fraction: item.isEnabled ? 1 : 0.5)
                }
                title(item).draw(at: NSPoint(x: row.minX + leading + indent, y: titleY))
                if #available(macOS 14.4, *), let subtitle = item.subtitle {
                    NSAttributedString(
                        string: subtitle,
                        attributes: [
                            .font: NSFont.systemFont(ofSize: 11),
                            .foregroundColor: NSColor.secondaryLabelColor,
                        ]
                    ).draw(at: NSPoint(x: row.minX + leading + indent, y: titleY - 15))
                }
                let key = keyText(item)
                if !key.isEmpty {
                    let text = NSAttributedString(
                        string: key,
                        attributes: [.font: font, .foregroundColor: NSColor.secondaryLabelColor])
                    text.draw(at: NSPoint(x: row.maxX - 14 - text.size().width, y: titleY))
                }
                if item.submenu != nil {
                    let text = NSAttributedString(
                        string: "›",
                        attributes: [
                            .font: NSFont.systemFont(ofSize: 15, weight: .semibold),
                            .foregroundColor: textColor,
                        ])
                    text.draw(at: NSPoint(x: row.maxX - 22, y: titleY - 2))
                }
            }
        }
    }

    private static func tint(_ image: NSImage, _ color: NSColor) -> NSImage {
        guard image.isTemplate else { return image }
        let result = NSImage(size: image.size, flipped: false) { rect in
            image.draw(in: rect)
            color.set()
            rect.fill(using: .sourceAtop)
            return true
        }
        return result
    }
}

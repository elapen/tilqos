import AppKit
import TilqosKit

/// An `@main` type rather than top-level code in `main.swift`: top-level code is not
/// main-actor isolated, and everything AppKit here touches is.
@main
struct TilqosApp {
    /// `NSApplication.delegate` is a weak reference, so the controller has to be held here
    /// or it would be released the moment `main` stopped using it.
    @MainActor private static var controller: AppController?

    @MainActor
    static func main() {
        let application = NSApplication.shared
        // LSUIElement in Info.plist already keeps us out of the Dock; setting the policy
        // explicitly makes the bare binary behave the same way during development.
        application.setActivationPolicy(.accessory)

        let controller = AppController()
        Self.controller = controller
        application.delegate = controller
        application.run()
    }
}

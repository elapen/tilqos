import Foundation
import XCTest

/// Throwaway `UserDefaults` for tests, stored in a temporary directory instead of
/// `~/Library/Preferences`.
///
/// Why not `UserDefaults(suiteName: "SomeName")`? A named suite is backed by
/// `~/Library/Preferences/SomeName.plist`, and `removePersistentDomain(forName:)` only
/// *empties* it: the file stays behind. Every test run used to leave hundreds of such
/// files. `UserDefaults(suiteName:)` also accepts an absolute path, which this helper uses
/// so that the whole domain lives under the temporary directory and disappears with it.
///
/// Tests must not create a named suite themselves (`ScratchDefaultsTests` enforces that);
/// call `ScratchDefaults.make()` and `remove()` the result in `tearDown` / `defer`.
/// Anything a test forgets is still swept when the test bundle finishes.
final class ScratchDefaults {
    let defaults: UserDefaults
    private let suiteName: String
    private let directory: URL
    private var removed = false

    private init() {
        directory = FileManager.default.temporaryDirectory
            .appendingPathComponent("tilqos-test-defaults-\(UUID().uuidString)", isDirectory: true)
        try? FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        suiteName = directory.appendingPathComponent("defaults").path
        defaults = UserDefaults(suiteName: suiteName)!
    }

    /// Empties the domain and deletes its directory. Safe to call more than once.
    func remove() {
        guard !removed else { return }
        removed = true
        defaults.removePersistentDomain(forName: suiteName)
        try? FileManager.default.removeItem(at: directory)
        Self.lock.lock()
        Self.live.removeAll { $0 === self }
        Self.lock.unlock()
    }

    /// Whether the backing directory is still on disk (for the hygiene tests).
    var directoryExists: Bool { FileManager.default.fileExists(atPath: directory.path) }

    // MARK: - Registry

    private static let lock = NSLock()
    private static var live: [ScratchDefaults] = []
    private static let observer = BundleObserver()

    /// A fresh, empty store, registered for cleanup.
    static func make() -> ScratchDefaults {
        let scratch = ScratchDefaults()
        lock.lock()
        live.append(scratch)
        lock.unlock()
        // Registered once, on first use: removes whatever tests did not remove themselves.
        _ = observer
        return scratch
    }

    /// Removes every store that is still alive.
    static func removeAll() {
        lock.lock()
        let pending = live
        lock.unlock()
        pending.forEach { $0.remove() }
    }

    private final class BundleObserver: NSObject, XCTestObservation {
        override init() {
            super.init()
            XCTestObservationCenter.shared.addTestObserver(self)
        }

        func testBundleDidFinish(_ testBundle: Bundle) {
            ScratchDefaults.removeAll()
        }
    }
}

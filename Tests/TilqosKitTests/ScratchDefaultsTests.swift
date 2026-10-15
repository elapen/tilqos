import XCTest

/// Pins the test-hygiene rule: tests never write into `~/Library/Preferences`.
///
/// A named `UserDefaults` suite is a plist in that folder, and `removePersistentDomain`
/// leaves the (emptied) file behind, so a test run used to litter it with hundreds of files.
/// Tests use `ScratchDefaults` instead, which keeps everything under the temp directory.
final class ScratchDefaultsTests: XCTestCase {
    override func tearDown() {
        ScratchDefaults.removeAll()
    }

    func testStoreRoundTripsInsideTheTempDirectoryAndIsRemoved() {
        let scratch = ScratchDefaults.make()
        scratch.defaults.set("value", forKey: "key")
        scratch.defaults.set(["a", "b"], forKey: "list")
        XCTAssertEqual(scratch.defaults.string(forKey: "key"), "value")
        XCTAssertEqual(scratch.defaults.stringArray(forKey: "list"), ["a", "b"])
        XCTAssertTrue(scratch.directoryExists)

        scratch.remove()
        XCTAssertFalse(scratch.directoryExists, "removing the store must delete its directory")
        scratch.remove()  // idempotent
    }

    func testStoresAreIndependent() {
        let first = ScratchDefaults.make()
        let second = ScratchDefaults.make()
        first.defaults.set(1, forKey: "n")
        XCTAssertNil(second.defaults.object(forKey: "n"))
    }

    func testRemoveAllSweepsEveryLiveStore() {
        let stores = (0..<3).map { _ in ScratchDefaults.make() }
        XCTAssertTrue(stores.allSatisfy(\.directoryExists))
        ScratchDefaults.removeAll()
        XCTAssertTrue(stores.allSatisfy { !$0.directoryExists })
    }

    /// The rule itself: no test source creates a named suite or touches the standard domain
    /// directly. (This file and ScratchDefaults.swift are the only places that mention them.)
    func testNoTestTouchesTheRealPreferencesFolder() throws {
        let testsDirectory = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
        let packageRoot = testsDirectory.deletingLastPathComponent().deletingLastPathComponent()
        let parityDirectory = packageRoot.appendingPathComponent("windows/parity")

        let forbidden = try NSRegularExpression(
            pattern: #"UserDefaults\s*\(\s*(suiteName|\))|UserDefaults\s*\.\s*standard"#)
        let exempt: Set<String> = ["ScratchDefaults.swift", "ScratchDefaultsTests.swift"]

        var offenders: [String] = []
        for directory in [testsDirectory, parityDirectory] {
            let names = (try? FileManager.default.contentsOfDirectory(atPath: directory.path)) ?? []
            for name in names where name.hasSuffix(".swift") && !exempt.contains(name) {
                let text = try String(contentsOf: directory.appendingPathComponent(name), encoding: .utf8)
                for (index, line) in text.components(separatedBy: "\n").enumerated() {
                    let code = line.trimmingCharacters(in: .whitespaces)
                    if code.hasPrefix("//") { continue }
                    let range = NSRange(code.startIndex..., in: code)
                    if forbidden.firstMatch(in: code, range: range) != nil {
                        offenders.append("\(name):\(index + 1): \(code)")
                    }
                }
            }
        }
        XCTAssertTrue(
            offenders.isEmpty,
            "use ScratchDefaults.make() instead of a real defaults domain:\n"
                + offenders.joined(separator: "\n"))
    }
}

import XCTest

/// P6: no network, no telemetry, no updater. The app uses no networking API, imports none, and
/// the package has no dependencies that could bring one in.
///
/// (Opening a link in the user's browser after a click is the system's doing, not the app's;
/// this scan is about the app's own code reaching out.)
final class PrivacyNetworkTests: XCTestCase {
    func testNoSourceFileUsesTheNetworkOrStartsProcesses() throws {
        let forbidden = try NSRegularExpression(
            pattern: [
                #"\bURLSession\b"#, #"\bURLRequest\b"#, #"\bNSURLConnection\b"#, #"\bNSURLSession\b"#,
                #"\bURLProtocol\b"#, #"\bNWConnection\b"#, #"\bNWListener\b"#, #"\bNWPathMonitor\b"#,
                #"\bCFNetwork\b"#, #"\bCFSocket\b"#, #"\bCFReadStream\b"#, #"\bCFWriteStream\b"#,
                #"\bNSNetService\b"#, #"\bNetService\b"#, #"\bMultipeerConnectivity\b"#, #"\bWebKit\b"#,
                #"\bSparkle\b"#, #"\bSCNetworkReachability\b"#, #"\bgethostbyname\b"#,
                #"\bgetaddrinfo\b"#, #"\bimport\s+Network\b"#, #"\.dataTask\b"#, #"\.downloadTask\b"#,
                #"\.uploadTask\b"#, #"\bProcess\s*\(\s*\)"#, #"\bNSAppleScript\b"#,
                #"\bContentsOf\s*:\s*URL\s*\(\s*string"#,
            ].joined(separator: "|"))
        var offenders: [String] = []
        for (name, text) in try PrivacyTestSupport.appSources() {
            for (index, line) in text.components(separatedBy: "\n").enumerated() {
                let code = line.trimmingCharacters(in: .whitespaces)
                if code.hasPrefix("//") { continue }
                if forbidden.firstMatch(in: code, range: NSRange(code.startIndex..., in: code)) != nil {
                    offenders.append("\(name):\(index + 1): \(code)")
                }
            }
        }
        XCTAssertTrue(offenders.isEmpty, "network or process APIs found:\n" + offenders.joined(separator: "\n"))
    }

    func testPackageHasNoDependencies() throws {
        let manifest = PrivacyTestSupport.packageRoot.appendingPathComponent("Package.swift")
        let text = try String(contentsOf: manifest, encoding: .utf8)
        let code = text.components(separatedBy: "\n")
            .filter { !$0.trimmingCharacters(in: .whitespaces).hasPrefix("//") }
            .joined(separator: "\n")
        XCTAssertFalse(code.contains(".package("), "no third-party dependencies")
        XCTAssertFalse(code.contains("package(url"), "no third-party dependencies")
    }
}

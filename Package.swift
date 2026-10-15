// swift-tools-version:6.0
import PackageDescription

// The logic lives in a library target so the test target can import it. The
// executable is a thin shell — see Sources/Tilqos/App.swift.
//
// Swift 5 language mode is deliberate: the event-tap C callback, the TIS
// notifications and the shared app state are all main-thread-confined by
// construction, and Swift 6 strict concurrency cannot express that without
// wrapping every Carbon/CoreGraphics handle in an unsafe shim.
let package = Package(
    name: "Tilqos",
    platforms: [.macOS(.v14)],
    targets: [
        .target(
            name: "TilqosKit",
            path: "Sources/TilqosKit",
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
        .executableTarget(
            name: "Tilqos",
            dependencies: ["TilqosKit"],
            path: "Sources/Tilqos",
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
        .executableTarget(
            name: "GenModel",
            dependencies: ["TilqosKit"],
            path: "Sources/GenModel",
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
        .testTarget(
            name: "TilqosKitTests",
            dependencies: ["TilqosKit"],
            path: "Tests/TilqosKitTests",
            swiftSettings: [.swiftLanguageMode(.v5)]
        ),
    ]
)

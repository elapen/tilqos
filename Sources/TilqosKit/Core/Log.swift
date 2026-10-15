import Foundation
import os

/// Logging for a keyboard utility needs one rule above all others: never record what
/// the user typed. Everything here logs shapes and counts — lengths, languages,
/// decisions — and never characters, clipboard contents or window titles. That holds at every
/// level and in every build.
///
/// How the rule is kept:
/// - The message of a call is public and is built from numbers and fixed words only. A test
///   (`PrivacyLogTests`) lists every interpolation in every call and fails when a new one
///   appears, so a new log line has to be reviewed.
/// - A value that comes from the user's machine and is not a number (the identifier of the app
///   in front) goes in `private:`. It reaches the unified log marked `<private>` and the
///   stderr echo and the test sink only ever see the redacted form.
/// - Diagnostics are off by default (`Prefs.diagnosticsEnabled`). While off, only warnings and
///   errors are written: failures of the app itself, which carry nothing about the user.
///
/// Goes to the unified log (`log stream --predicate 'subsystem == "io.github.elapen.tilqos"'`)
/// and additionally to stderr when `TILQOS_DEBUG=1`, which is how the
/// `./build.sh run` log file gets populated.
public enum Log {
    public static let subsystem = "io.github.elapen.tilqos"

    public enum Category: String {
        case app
        case tap
        case layout
        case detect
        case inject
        case ui
    }

    public enum Level: String {
        case debug = "DEBUG"
        case info = "INFO"
        case warn = "WARN"
        case error = "ERROR"
    }

    /// What every sink sees in place of a `private:` value.
    public static let redacted = "<private>"

    private static let loggers: [Category: Logger] = {
        var result: [Category: Logger] = [:]
        for category in [
            Category.app, .tap, .layout, .detect, .inject, .ui,
        ] {
            result[category] = Logger(subsystem: subsystem, category: category.rawValue)
        }
        return result
    }()

    private static let echoToStderr = ProcessInfo.processInfo.environment["TILQOS_DEBUG"] == "1"

    // MARK: - Switches

    private static let lock = NSLock()
    private static var _diagnostics = false
    private static var _sink: ((Level, Category, String) -> Void)?

    /// Whether `info` and `debug` lines are written at all. Set from `Prefs.diagnosticsEnabled`.
    public static var diagnosticsEnabled: Bool {
        get { lock.withLock { _diagnostics } }
        set { lock.withLock { _diagnostics = newValue } }
    }

    /// Receives every line that would be written, already redacted. For tests; nil in the app.
    static var sink: ((Level, Category, String) -> Void)? {
        get { lock.withLock { _sink } }
        set { lock.withLock { _sink = newValue } }
    }

    private static func wanted(_ level: Level) -> Bool {
        switch level {
        case .warn, .error: return true
        case .info, .debug: return diagnosticsEnabled || echoToStderr
        }
    }

    // MARK: - Writing

    /// Notice rather than info on purpose: os_log keeps `info` in memory only, so lifecycle
    /// messages would vanish before anyone could read them back with `log show`.
    public static func info(_ category: Category, _ message: String, private detail: String? = nil) {
        guard wanted(.info) else { return }
        emit(.info, category, message, detail)
    }

    /// The per-word trail — decisions, routes, timings — for diagnosing a misbehaving app.
    /// Debug level, so nothing is kept unless someone is watching:
    /// `log stream --level debug --predicate 'subsystem == "io.github.elapen.tilqos"'`.
    /// Shapes only, like everything else here: never the characters.
    public static func debug(
        _ category: Category, _ message: @autoclosure () -> String, private detail: String? = nil
    ) {
        // The message is not even built when nobody is going to read it: the per-word trail runs
        // on every keystroke.
        guard wanted(.debug) else { return }
        emit(.debug, category, message(), detail)
    }

    public static func warn(_ category: Category, _ message: String, private detail: String? = nil) {
        emit(.warn, category, message, detail)
    }

    public static func error(_ category: Category, _ message: String, private detail: String? = nil) {
        emit(.error, category, message, detail)
    }

    private static func emit(_ level: Level, _ category: Category, _ message: String, _ detail: String?) {
        if let logger = loggers[category] {
            // The detail is its own interpolation, so the system itself redacts it.
            if let detail {
                switch level {
                case .debug: logger.debug("\(message, privacy: .public) \(detail, privacy: .private)")
                case .info: logger.notice("\(message, privacy: .public) \(detail, privacy: .private)")
                case .warn: logger.warning("\(message, privacy: .public) \(detail, privacy: .private)")
                case .error: logger.error("\(message, privacy: .public) \(detail, privacy: .private)")
                }
            } else {
                switch level {
                case .debug: logger.debug("\(message, privacy: .public)")
                case .info: logger.notice("\(message, privacy: .public)")
                case .warn: logger.warning("\(message, privacy: .public)")
                case .error: logger.error("\(message, privacy: .public)")
                }
            }
        }
        // What leaves this function by any other road is the redacted form only.
        let line = detail == nil ? message : message + " " + redacted
        sink?(level, category, line)
        guard echoToStderr else { return }
        FileHandle.standardError.write(
            Data("[\(level.rawValue)] \(category.rawValue): \(line)\n".utf8))
    }
}

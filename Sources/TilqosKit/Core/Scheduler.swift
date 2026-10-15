import Foundation

/// A one-shot timer that can be called off.
public protocol ScheduledTask: AnyObject {
    func cancel()
}

/// The only clock and timer the privacy-relevant code uses: the idle reset of the keystroke
/// buffer, the expiry of the undo list, the polling of the clipboard.
///
/// A protocol so tests drive time by hand instead of sleeping. Everything is main-thread: the
/// event tap, the run loop and the timers below all live there.
public protocol TimerScheduler: AnyObject {
    /// Monotonic seconds. Only differences mean anything.
    var now: TimeInterval { get }
    /// Runs `work` once, `delay` seconds from now, unless the returned task is cancelled first.
    @discardableResult
    func schedule(after delay: TimeInterval, _ work: @escaping () -> Void) -> ScheduledTask
}

/// The real thing: dispatch timers on the main queue, system uptime as the clock.
public final class MainQueueScheduler: TimerScheduler {
    public static let shared = MainQueueScheduler()

    public init() {}

    public var now: TimeInterval { ProcessInfo.processInfo.systemUptime }

    @discardableResult
    public func schedule(after delay: TimeInterval, _ work: @escaping () -> Void) -> ScheduledTask {
        let task = Task()
        let timer = DispatchSource.makeTimerSource(queue: .main)
        // A little slack lets the system coalesce wake-ups; the privacy promise is "about five
        // seconds", not "to the millisecond".
        timer.schedule(deadline: .now() + max(0, delay), leeway: .milliseconds(100))
        // Strong on purpose: the caller may drop the task without keeping it. The cycle
        // task -> source -> handler -> task ends when the timer fires or is cancelled.
        timer.setEventHandler {
            guard !task.cancelled else { return }
            task.cancel()
            work()
        }
        task.source = timer
        timer.resume()
        return task
    }

    private final class Task: ScheduledTask {
        var source: DispatchSourceTimer?
        var cancelled = false

        func cancel() {
            cancelled = true
            source?.cancel()
            source = nil
        }
    }
}

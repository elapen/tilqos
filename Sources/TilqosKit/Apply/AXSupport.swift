import ApplicationServices
import AppKit

/// Thin wrappers over the accessibility API.
///
/// Every call crosses a process boundary to the app being typed into, and that app may
/// be busy or hung. A messaging timeout is therefore set on every element we touch:
/// without it, a single unresponsive app would freeze our main thread and take the
/// event tap down with it.
enum AX {
    /// How long to wait for another process to answer. Short on purpose — a correction
    /// that arrives late is worse than one that does not arrive.
    static let messagingTimeout: Float = 0.25

    static var isTrusted: Bool { AXIsProcessTrusted() }

    static func focusedElement() -> AXUIElement? {
        let systemWide = AXUIElementCreateSystemWide()
        AXUIElementSetMessagingTimeout(systemWide, messagingTimeout)
        guard let focused = copy(systemWide, kAXFocusedUIElementAttribute) else { return nil }
        guard CFGetTypeID(focused) == AXUIElementGetTypeID() else { return nil }
        let element = unsafeDowncast(focused, to: AXUIElement.self)
        AXUIElementSetMessagingTimeout(element, messagingTimeout)
        return element
    }

    static func copy(_ element: AXUIElement, _ attribute: String) -> CFTypeRef? {
        var value: CFTypeRef?
        guard AXUIElementCopyAttributeValue(element, attribute as CFString, &value) == .success
        else { return nil }
        return value
    }

    static func string(_ element: AXUIElement, _ attribute: String) -> String? {
        copy(element, attribute) as? String
    }

    static func isSettable(_ element: AXUIElement, _ attribute: String) -> Bool {
        var settable: DarwinBoolean = false
        guard
            AXUIElementIsAttributeSettable(element, attribute as CFString, &settable) == .success
        else { return false }
        return settable.boolValue
    }

    // MARK: - Text ranges

    static func selectedRange(_ element: AXUIElement) -> CFRange? {
        guard let value = copy(element, kAXSelectedTextRangeAttribute),
            CFGetTypeID(value) == AXValueGetTypeID()
        else { return nil }
        var range = CFRange()
        guard AXValueGetValue(unsafeDowncast(value, to: AXValue.self), .cfRange, &range) else {
            return nil
        }
        return range
    }

    @discardableResult
    static func setSelectedRange(_ element: AXUIElement, _ range: CFRange) -> Bool {
        var range = range
        guard let value = AXValueCreate(.cfRange, &range) else { return false }
        return AXUIElementSetAttributeValue(
            element, kAXSelectedTextRangeAttribute as CFString, value) == .success
    }

    @discardableResult
    static func setSelectedText(_ element: AXUIElement, _ text: String) -> Bool {
        AXUIElementSetAttributeValue(
            element, kAXSelectedTextAttribute as CFString, text as CFTypeRef) == .success
    }

    static func selectedText(_ element: AXUIElement) -> String? {
        string(element, kAXSelectedTextAttribute)
    }

    /// The text in a range, without fetching the whole value. Not every app implements it.
    static func string(_ element: AXUIElement, for range: CFRange) -> String? {
        var range = range
        guard let rangeValue = AXValueCreate(.cfRange, &range) else { return nil }
        var result: CFTypeRef?
        guard
            AXUIElementCopyParameterizedAttributeValue(
                element, kAXStringForRangeParameterizedAttribute as CFString, rangeValue,
                &result) == .success
        else { return nil }
        return result as? String
    }

    static func numberOfCharacters(_ element: AXUIElement) -> Int? {
        copy(element, kAXNumberOfCharactersAttribute) as? Int
    }

    /// Screen rectangle of a character range, used to place the suggestion panel at the
    /// caret. Not every app implements this parameterized attribute.
    static func boundsForRange(_ element: AXUIElement, _ range: CFRange) -> CGRect? {
        var range = range
        guard let rangeValue = AXValueCreate(.cfRange, &range) else { return nil }
        var result: CFTypeRef?
        guard
            AXUIElementCopyParameterizedAttributeValue(
                element, kAXBoundsForRangeParameterizedAttribute as CFString, rangeValue,
                &result) == .success,
            let result, CFGetTypeID(result) == AXValueGetTypeID()
        else { return nil }
        var rect = CGRect.zero
        guard AXValueGetValue(unsafeDowncast(result, to: AXValue.self), .cgRect, &rect) else {
            return nil
        }
        return rect
    }

    // MARK: - Frontmost app

    static var frontmostBundleID: String? {
        NSWorkspace.shared.frontmostApplication?.bundleIdentifier
    }
}

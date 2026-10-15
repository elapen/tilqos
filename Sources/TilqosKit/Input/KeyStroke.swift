import Carbon.HIToolbox
import Foundation

/// The modifier combinations that change which character a key produces.
///
/// Command and Control are deliberately absent: they produce no character, and a
/// keystroke carrying either is a shortcut, not text.
public struct ModState: OptionSet, Hashable, Sendable {
    public let rawValue: UInt8
    public init(rawValue: UInt8) { self.rawValue = rawValue }

    public static let shift = ModState(rawValue: 1 << 0)
    public static let option = ModState(rawValue: 1 << 1)
    public static let caps = ModState(rawValue: 1 << 2)

    /// All 8 combinations, in ascending raw order. Used to size the key tables.
    public static let variantCount = 8

    /// `modifierKeyState` for `UCKeyTranslate`: Carbon modifier bits shifted right by 8.
    var ucKeyModifiers: UInt32 {
        var m: UInt32 = 0
        if contains(.shift) { m |= UInt32(shiftKey >> 8) }
        if contains(.option) { m |= UInt32(optionKey >> 8) }
        if contains(.caps) { m |= UInt32(alphaLock >> 8) }
        return m
    }

    /// Ordering used when building the reverse (character → keystroke) map.
    /// Fewer modifiers wins, so a character reachable both plainly and via Option
    /// resolves to the plain keystroke.
    static let byCost: [ModState] = [
        [], .shift, .caps, [.shift, .caps],
        .option, [.option, .shift], [.option, .caps], [.option, .shift, .caps],
    ]

    public init(cgFlags: CGEventFlags) {
        var m = ModState()
        if cgFlags.contains(.maskShift) { m.insert(.shift) }
        if cgFlags.contains(.maskAlternate) { m.insert(.option) }
        if cgFlags.contains(.maskAlphaShift) { m.insert(.caps) }
        self = m
    }
}

/// One physical key press, stored as the key plus the modifiers that affect its
/// character — never as the character itself.
///
/// This is the central data decision of the app. Kazakh digits are only reachable
/// through Option, so a character-to-character mapping table (what RuSwitcher and
/// Bzz use) cannot round-trip Kazakh text. Keeping the keystroke lets any layout
/// re-render the same input losslessly.
public struct KeyStroke: Hashable, Sendable {
    public var keyCode: UInt16
    public var mods: ModState

    public init(keyCode: UInt16, mods: ModState = []) {
        self.keyCode = keyCode
        self.mods = mods
    }

    /// Index into a layout's flat character table.
    var tableIndex: Int { Int(keyCode) * ModState.variantCount + Int(mods.rawValue) }
}

/// A keystroke as the tap recorded it: what was pressed, when, and which layout was
/// active at that moment.
///
/// The layout has to travel with the stroke. A user writing a mixed paragraph switches
/// layout mid-sentence, so reconstructing what is actually on screen means rendering
/// each stroke under the layout it was typed on, not under whatever is active now.
public struct RecordedStroke: Sendable {
    public var stroke: KeyStroke
    public var time: TimeInterval
    public var layoutID: String

    public init(stroke: KeyStroke, time: TimeInterval, layoutID: String) {
        self.stroke = stroke
        self.time = time
        self.layoutID = layoutID
    }
}

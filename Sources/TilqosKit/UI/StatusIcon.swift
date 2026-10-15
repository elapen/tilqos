import AppKit

/// The menu bar image: the current layout as a small keycap, or a warning when the app
/// cannot work.
///
/// Drawn rather than set as button text so the badge keeps one shape whatever the language —
/// `EN`, `РУ` and `ҚАЗ` all sit in the same outlined key — and so the three states differ
/// in shape, not just in colour: an outlined key when working, the same key dimmed when
/// switched off, and a warning triangle when a permission is missing. A template image, so
/// the menu bar tints it for light, dark and coloured wallpapers alike.
enum StatusIcon {
    static func image(badge: String?, health: AppHealth) -> NSImage {
        if health == .noPermission || health == .tapStopped || health == .suggestionsOnly {
            return symbol("exclamationmark.triangle", description: health.title)
        }
        guard let badge else {
            return symbol("keyboard", description: "Tilqos")
        }
        return keycap(badge)
    }

    private static func symbol(_ name: String, description: String) -> NSImage {
        let configuration = NSImage.SymbolConfiguration(pointSize: 14, weight: .medium)
        let image =
            NSImage(systemSymbolName: name, accessibilityDescription: description)?
            .withSymbolConfiguration(configuration) ?? NSImage()
        image.isTemplate = true
        return image
    }

    static func keycap(_ text: String) -> NSImage {
        let font = NSFont.systemFont(ofSize: 10.5, weight: .semibold)
        let attributes: [NSAttributedString.Key: Any] = [
            .font: font, .foregroundColor: NSColor.black, .kern: 0.3,
        ]
        let textSize = (text as NSString).size(withAttributes: attributes)
        let height: CGFloat = 15
        let width = max(ceil(textSize.width) + 9, 22)
        let image = NSImage(size: NSSize(width: width, height: height), flipped: false) { rect in
            let key = rect.insetBy(dx: 0.75, dy: 0.75)
            let path = NSBezierPath(roundedRect: key, xRadius: 3.5, yRadius: 3.5)
            path.lineWidth = 1.3
            NSColor.black.setStroke()
            path.stroke()
            (text as NSString).draw(
                at: NSPoint(
                    x: rect.midX - textSize.width / 2,
                    y: rect.midY - textSize.height / 2 + 0.5),
                withAttributes: attributes)
            return true
        }
        image.isTemplate = true
        image.accessibilityDescription = text
        return image
    }
}

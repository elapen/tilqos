import AppKit
import SwiftUI

/// The small "before → after" card near the caret.
///
/// One view for every place it appears — the live suggestion, the confirmation after an
/// automatic fix, and the previews in Settings and the tutorial — so what the tutorial
/// teaches is exactly what later shows up while typing.
final class HintView: NSView {
    enum Style: Equatable {
        /// An offer: the accept key is the one thing to do, so it is the one coloured element.
        case suggestion(acceptKey: String)
        /// Already applied: says so, and names the key that takes it back.
        case applied(undoKey: String?)
    }

    struct Content: Equatable {
        var before: String
        var after: String
        var style: Style
        /// A one-line explanation for the first few times, then never again.
        var tip: String? = nil
    }

    /// Opaque backgrounds are for previews inside windows, where there is nothing behind
    /// the card to blur.
    enum Background {
        case material
        case opaque
    }

    static let cornerRadius: CGFloat = 11

    private let effect = NSVisualEffectView()
    private let tint = NSView()
    private let statusIcon = NSImageView()
    private let beforeLabel = NSTextField(labelWithString: "")
    private let arrow = NSImageView()
    private let afterLabel = NSTextField(labelWithString: "")
    private let keycap = KeycapView()
    private let keyLabel = NSTextField(labelWithString: "")
    private let tipLabel = NSTextField(wrappingLabelWithString: "")
    private let row = NSStackView()
    private let column = NSStackView()
    private let border = HairlineBorderView()
    private let countdown = CALayer()

    init(background: Background = .material) {
        super.init(frame: NSRect(x: 0, y: 0, width: 240, height: 36))
        wantsLayer = true
        layer?.cornerRadius = Self.cornerRadius
        layer?.cornerCurve = .continuous
        layer?.masksToBounds = true

        effect.material = .popover
        effect.blendingMode = .behindWindow
        effect.state = .active
        effect.maskImage = Self.mask(radius: Self.cornerRadius)
        effect.isHidden = background == .opaque
        tint.wantsLayer = true
        for view in [effect, tint] {
            view.translatesAutoresizingMaskIntoConstraints = false
            addSubview(view)
            pin(view)
        }
        self.backgroundStyle = background

        statusIcon.image = NSImage(
            systemSymbolName: "checkmark.circle.fill", accessibilityDescription: nil)
        statusIcon.symbolConfiguration = NSImage.SymbolConfiguration(
            pointSize: 14, weight: .semibold
        ).applying(.init(paletteColors: [.white, .systemGreen]))
        statusIcon.setContentHuggingPriority(.required, for: .horizontal)

        // The original stays readable but visibly struck out, so the eye lands on the
        // replacement without the user having to remember what they typed.
        //
        // Both texts are single-line and give way from the head when the card hits its
        // maximum width: the end of the text is what sits at the caret. The original gives
        // way first, since the replacement is the part being offered.
        for (label, resistance) in [(beforeLabel, 740), (afterLabel, 745)] {
            label.maximumNumberOfLines = 1
            label.cell?.usesSingleLineMode = true
            label.lineBreakMode = .byTruncatingHead
            label.setContentCompressionResistancePriority(
                NSLayoutConstraint.Priority(Float(resistance)), for: .horizontal)
        }
        beforeLabel.font = .systemFont(ofSize: 13)
        beforeLabel.textColor = .secondaryLabelColor

        arrow.image = NSImage(systemSymbolName: "arrow.right", accessibilityDescription: nil)
        arrow.symbolConfiguration = .init(pointSize: 10.5, weight: .semibold)
        arrow.contentTintColor = .tertiaryLabelColor
        arrow.setContentHuggingPriority(.required, for: .horizontal)

        afterLabel.font = .systemFont(ofSize: 13, weight: .semibold)
        afterLabel.textColor = .labelColor

        keyLabel.font = .systemFont(ofSize: 12)
        keyLabel.textColor = .secondaryLabelColor

        tipLabel.font = .systemFont(ofSize: 11)
        tipLabel.textColor = .secondaryLabelColor
        tipLabel.maximumNumberOfLines = 2
        tipLabel.preferredMaxLayoutWidth = 320

        let keySpacer = NSView()
        keySpacer.translatesAutoresizingMaskIntoConstraints = false
        keySpacer.widthAnchor.constraint(equalToConstant: 2).isActive = true

        row.orientation = .horizontal
        row.alignment = .centerY
        row.spacing = 7
        row.setViews(
            [statusIcon, beforeLabel, arrow, afterLabel, keySpacer, keycap, keyLabel],
            in: .leading)
        row.setCustomSpacing(4, after: keycap)

        column.orientation = .vertical
        column.alignment = .leading
        column.spacing = 5
        column.setViews([row, tipLabel], in: .top)
        column.edgeInsets = NSEdgeInsets(top: 7, left: 12, bottom: 8, right: 9)
        column.translatesAutoresizingMaskIntoConstraints = false
        addSubview(column)
        pin(column)

        border.translatesAutoresizingMaskIntoConstraints = false
        addSubview(border)
        pin(border)

        countdown.backgroundColor = Brand.accent.withAlphaComponent(0.55).cgColor
        countdown.anchorPoint = CGPoint(x: 0, y: 0.5)
        countdown.isHidden = true
        layer?.addSublayer(countdown)

        setAccessibilityElement(true)
        setAccessibilityRole(.group)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is not used")
    }

    private var backgroundStyle: Background = .material {
        didSet { updateColors() }
    }

    private func pin(_ view: NSView) {
        NSLayoutConstraint.activate([
            view.leadingAnchor.constraint(equalTo: leadingAnchor),
            view.trailingAnchor.constraint(equalTo: trailingAnchor),
            view.topAnchor.constraint(equalTo: topAnchor),
            view.bottomAnchor.constraint(equalTo: bottomAnchor),
        ])
    }

    // MARK: - Content

    private(set) var content = Content(before: "", after: "", style: .applied(undoKey: nil))

    func configure(_ content: Content) {
        self.content = content
        let before = Self.trimmed(content.before)
        // An attributed value brings its own paragraph style, which would otherwise wrap.
        let paragraph = NSMutableParagraphStyle()
        paragraph.lineBreakMode = .byTruncatingHead
        beforeLabel.attributedStringValue = NSAttributedString(
            string: before,
            attributes: [
                .font: NSFont.systemFont(ofSize: 13),
                .foregroundColor: NSColor.secondaryLabelColor,
                .strikethroughStyle: NSUnderlineStyle.single.rawValue,
                .strikethroughColor: NSColor.secondaryLabelColor.withAlphaComponent(0.7),
                .paragraphStyle: paragraph,
            ])
        afterLabel.stringValue = Self.trimmed(content.after)
        beforeLabel.isHidden = before.isEmpty
        arrow.isHidden = before.isEmpty

        switch content.style {
        case .suggestion(let key):
            statusIcon.isHidden = true
            keycap.isHidden = false
            keycap.configure(text: key, prominent: true)
            keyLabel.isHidden = true
        case .applied(let undoKey):
            statusIcon.isHidden = false
            keycap.isHidden = undoKey == nil
            keycap.configure(text: undoKey ?? "", prominent: false)
            keyLabel.stringValue = L10n.tr("hint.undo")
            keyLabel.isHidden = undoKey == nil
        }

        tipLabel.stringValue = content.tip ?? ""
        tipLabel.isHidden = content.tip == nil
        setAccessibilityLabel(Self.spokenDescription(content))
        needsLayout = true
    }

    /// Forgets the words on the card. A hidden panel must not keep what was typed in memory
    /// until the next suggestion: the labels, the stored content and the spoken description
    /// are all emptied.
    func reset() {
        content = Content(before: "", after: "", style: .applied(undoKey: nil))
        beforeLabel.attributedStringValue = NSAttributedString(string: "")
        beforeLabel.stringValue = ""
        afterLabel.stringValue = ""
        keyLabel.stringValue = ""
        tipLabel.stringValue = ""
        keycap.configure(text: "", prominent: false)
        setAccessibilityLabel(nil)
        needsLayout = true
    }

    /// What VoiceOver says, since the arrow and the keycap mean nothing read aloud.
    static func spokenDescription(_ content: Content) -> String {
        switch content.style {
        case .suggestion(let key):
            return L10n.tr(
                "hint.spoken.suggestion", content.before, content.after, KeyNames.spoken(key))
        case .applied(let undoKey):
            var text = L10n.tr("hint.spoken.applied", content.before, content.after)
            if let undoKey { text += L10n.tr("hint.spoken.undo", KeyNames.spoken(undoKey)) }
            return text
        }
    }

    /// Long multi-word corrections would make the card unusably wide; the tail is the part
    /// next to the caret, so that is what stays.
    static func trimmed(_ text: String, limit: Int = 34) -> String {
        let collapsed = text.replacingOccurrences(of: "\n", with: " ")
        guard collapsed.count > limit else { return collapsed }
        return "…" + String(collapsed.suffix(limit))
    }

    override var fittingSize: NSSize {
        layoutSubtreeIfNeeded()
        var size = column.fittingSize
        size.width = min(max(ceil(size.width), 96), 540)
        size.height = max(ceil(size.height), 34)
        return size
    }

    // MARK: - Countdown

    /// A hairline along the bottom edge that runs out with the timeout, so the card going
    /// away is expected rather than abrupt.
    func startCountdown(_ duration: TimeInterval) {
        guard duration > 0 else { return stopCountdown() }
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        countdown.removeAllAnimations()
        countdown.isHidden = false
        layoutCountdown()
        CATransaction.commit()

        let animation = CABasicAnimation(keyPath: "transform.scale.x")
        animation.fromValue = 1
        animation.toValue = 0
        animation.duration = duration
        animation.timingFunction = CAMediaTimingFunction(name: .linear)
        animation.fillMode = .forwards
        animation.isRemovedOnCompletion = false
        countdown.add(animation, forKey: "countdown")
    }

    func stopCountdown() {
        countdown.removeAllAnimations()
        countdown.isHidden = true
    }

    private func layoutCountdown() {
        let height: CGFloat = 2
        countdown.bounds = CGRect(x: 0, y: 0, width: bounds.width, height: height)
        countdown.position = CGPoint(x: 0, y: height / 2)
    }

    override func layout() {
        super.layout()
        CATransaction.begin()
        CATransaction.setDisableActions(true)
        layoutCountdown()
        CATransaction.commit()
    }

    // MARK: - Appearance

    override func viewDidChangeEffectiveAppearance() {
        super.viewDidChangeEffectiveAppearance()
        updateColors()
    }

    private func updateColors() {
        effectiveAppearance.performAsCurrentDrawingAppearance {
            let isDark = effectiveAppearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
            switch backgroundStyle {
            case .material:
                // A light wash over the blur keeps the text legible over busy content.
                tint.layer?.backgroundColor =
                    (isDark
                    ? NSColor(white: 0.12, alpha: 0.35)
                    : NSColor(white: 1, alpha: 0.45)).cgColor
            case .opaque:
                tint.layer?.backgroundColor =
                    (isDark ? NSColor(white: 0.2, alpha: 1) : NSColor(white: 0.995, alpha: 1))
                    .cgColor
            }
        }
        border.needsDisplay = true
    }

    private static func mask(radius: CGFloat) -> NSImage {
        let side = radius * 2 + 1
        let image = NSImage(size: NSSize(width: side, height: side), flipped: false) { rect in
            NSColor.black.setFill()
            NSBezierPath(roundedRect: rect, xRadius: radius, yRadius: radius).fill()
            return true
        }
        image.capInsets = NSEdgeInsets(top: radius, left: radius, bottom: radius, right: radius)
        image.resizingMode = .stretch
        return image
    }
}

/// Hairline edge that keeps the card distinct from a background of the same brightness.
private final class HairlineBorderView: NSView {
    override func draw(_ dirtyRect: NSRect) {
        let isDark = effectiveAppearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
        let path = NSBezierPath(
            roundedRect: bounds.insetBy(dx: 0.5, dy: 0.5),
            xRadius: HintView.cornerRadius - 0.5, yRadius: HintView.cornerRadius - 0.5)
        path.lineWidth = 1
        (isDark ? NSColor(white: 1, alpha: 0.16) : NSColor(white: 0, alpha: 0.12)).setStroke()
        path.stroke()
    }

    override func hitTest(_ point: NSPoint) -> NSView? { nil }
}

/// A key drawn as a key.
final class KeycapView: NSView {
    private var text = ""
    private var prominent = false
    private var attributed: NSAttributedString {
        NSAttributedString(
            string: text,
            attributes: [
                .font: NSFont.systemFont(ofSize: 11.5, weight: prominent ? .semibold : .medium),
                .foregroundColor: prominent ? NSColor.white : NSColor.labelColor,
            ])
    }

    func configure(text: String, prominent: Bool) {
        self.text = text
        self.prominent = prominent
        invalidateIntrinsicContentSize()
        needsDisplay = true
    }

    override var intrinsicContentSize: NSSize {
        let size = attributed.size()
        return NSSize(width: max(ceil(size.width) + 12, 22), height: 20)
    }

    override func draw(_ dirtyRect: NSRect) {
        let rect = bounds.insetBy(dx: 0.5, dy: 0.5)
        let path = NSBezierPath(roundedRect: rect, xRadius: 5, yRadius: 5)
        if prominent {
            Brand.accent.setFill()
            path.fill()
        } else {
            let isDark = effectiveAppearance.bestMatch(from: [.aqua, .darkAqua]) == .darkAqua
            (isDark ? NSColor(white: 1, alpha: 0.12) : NSColor(white: 0, alpha: 0.05)).setFill()
            path.fill()
            (isDark ? NSColor(white: 1, alpha: 0.18) : NSColor(white: 0, alpha: 0.16)).setStroke()
            path.lineWidth = 1
            path.stroke()
        }
        let string = attributed
        let size = string.size()
        string.draw(
            at: NSPoint(x: bounds.midX - size.width / 2, y: bounds.midY - size.height / 2 + 0.5))
    }
}

// MARK: - SwiftUI preview

/// The live card inside SwiftUI windows, so previews cannot drift from the real thing.
struct HintPreview: NSViewRepresentable {
    let content: HintView.Content

    func makeNSView(context: Context) -> HintView {
        let view = HintView(background: .opaque)
        view.configure(content)
        view.setContentHuggingPriority(.required, for: .horizontal)
        view.setContentHuggingPriority(.required, for: .vertical)
        return view
    }

    func updateNSView(_ view: HintView, context: Context) {
        if view.content != content { view.configure(content) }
        view.invalidateIntrinsicContentSize()
    }

    func sizeThatFits(_ proposal: ProposedViewSize, nsView: HintView, context: Context) -> CGSize? {
        nsView.fittingSize
    }
}

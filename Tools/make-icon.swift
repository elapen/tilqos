import AppKit
import CoreGraphics
import CoreText
import Foundation

// Draws the app icon and writes every size an .iconset needs.
//
//   swift Tools/make-icon.swift <output .iconset directory>
//
// The mark is a keycap carrying Ә — the letter that only exists on the Kazakh layout, and
// the one thing this app does that other layout switchers do not. It stays legible at
// 16 pt because the white keycap silhouette carries the shape even when the glyph blurs.

let brandTop = CGColor(srgbRed: 0.478, green: 0.502, blue: 1.0, alpha: 1)
let brandBottom = CGColor(srgbRed: 0.208, green: 0.157, blue: 0.706, alpha: 1)
let glyphColor = CGColor(srgbRed: 0.216, green: 0.180, blue: 0.647, alpha: 1)

/// macOS icon grid: a 1024 canvas whose rounded body occupies the middle 824 points.
let canvas: CGFloat = 1024
let bodyInset: CGFloat = 100
let bodyRadius: CGFloat = 185

func makeContext(_ pixels: Int) -> CGContext {
    guard
        let context = CGContext(
            data: nil, width: pixels, height: pixels, bitsPerComponent: 8, bytesPerRow: 0,
            space: CGColorSpace(name: CGColorSpace.sRGB)!,
            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
    else { fatalError("cannot create bitmap context") }
    let scale = CGFloat(pixels) / canvas
    context.scaleBy(x: scale, y: scale)
    return context
}

func roundedPath(_ rect: CGRect, _ radius: CGFloat) -> CGPath {
    CGPath(roundedRect: rect, cornerWidth: radius, cornerHeight: radius, transform: nil)
}

func drawGradient(_ context: CGContext, in path: CGPath, from: CGColor, to: CGColor) {
    let box = path.boundingBox
    context.saveGState()
    context.addPath(path)
    context.clip()
    let gradient = CGGradient(
        colorsSpace: CGColorSpace(name: CGColorSpace.sRGB)!,
        colors: [from, to] as CFArray, locations: [0, 1])!
    // Top to bottom; CoreGraphics y grows upward, so "start" is the top edge.
    context.drawLinearGradient(
        gradient, start: CGPoint(x: box.midX, y: box.maxY),
        end: CGPoint(x: box.midX, y: box.minY), options: [])
    context.restoreGState()
}

func draw(into context: CGContext) {
    let body = CGRect(
        x: bodyInset, y: bodyInset, width: canvas - bodyInset * 2, height: canvas - bodyInset * 2)
    let bodyPath = roundedPath(body, bodyRadius)

    drawGradient(context, in: bodyPath, from: brandTop, to: brandBottom)

    // A soft sheen across the upper half, so the surface reads as glass rather than flat.
    context.saveGState()
    context.addPath(bodyPath)
    context.clip()
    let sheen = CGGradient(
        colorsSpace: CGColorSpace(name: CGColorSpace.sRGB)!,
        colors: [
            CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 0.28),
            CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 0),
        ] as CFArray, locations: [0, 1])!
    context.drawRadialGradient(
        sheen,
        startCenter: CGPoint(x: body.midX, y: body.maxY + body.height * 0.18),
        startRadius: 0,
        endCenter: CGPoint(x: body.midX, y: body.maxY + body.height * 0.18),
        endRadius: body.width * 0.95, options: [])
    context.restoreGState()

    // Two overlapping keycaps: the pair says "switches between layouts" on its own, with no
    // extra ornament to turn to mush at 16 pt. The back one is Latin, the front one carries
    // Ә — the letter that only exists on the Kazakh layout.
    //
    // Everything from here is clipped to the body, so a cap or its shadow can never bleed
    // past the rounded silhouette.
    context.saveGState()
    context.addPath(bodyPath)
    context.clip()

    let back = centeredSquare(
        in: body, side: 0.40, dx: -0.105, dy: 0.105)
    let backPath = roundedPath(back, back.width * 0.245)
    context.addPath(backPath)
    context.setFillColor(CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 0.33))
    context.fillPath()
    drawGlyph(
        "A", in: back, context: context, weight: .semibold,
        color: CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 0.95), ratio: 0.58)

    let front = centeredSquare(in: body, side: 0.46, dx: 0.10, dy: -0.10)
    let frontPath = roundedPath(front, front.width * 0.245)

    context.saveGState()
    context.setShadow(
        offset: CGSize(width: -body.width * 0.006, height: -body.height * 0.014),
        blur: body.width * 0.045,
        color: CGColor(srgbRed: 0.05, green: 0.03, blue: 0.22, alpha: 0.38))
    context.addPath(frontPath)
    context.setFillColor(CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 1))
    context.fillPath()
    context.restoreGState()

    // Very slight top-to-bottom shading keeps the cap from looking like a flat sticker.
    drawGradient(
        context, in: frontPath,
        from: CGColor(srgbRed: 1, green: 1, blue: 1, alpha: 1),
        to: CGColor(srgbRed: 0.906, green: 0.914, blue: 0.964, alpha: 1))

    drawGlyph("Ә", in: front, context: context, weight: .semibold, color: glyphColor, ratio: 0.60)
    context.restoreGState()
}

/// A square whose side and centre offset are fractions of the body, so the layout scales
/// with the icon rather than being tuned per size.
func centeredSquare(in body: CGRect, side: CGFloat, dx: CGFloat, dy: CGFloat) -> CGRect {
    let length = body.width * side
    return CGRect(
        x: body.midX + body.width * dx - length / 2,
        y: body.midY + body.height * dy - length / 2,
        width: length, height: length)
}

/// Centres a glyph on its cap using the real typographic bounds, not the font's line box —
/// otherwise the visual centre sits noticeably high.
func drawGlyph(
    _ text: String, in cap: CGRect, context: CGContext,
    weight: NSFont.Weight, color: CGColor, ratio: CGFloat
) {
    let font = NSFont.systemFont(ofSize: cap.height * ratio, weight: weight)
    let attributed = NSAttributedString(
        string: text,
        attributes: [.font: font, .foregroundColor: NSColor(cgColor: color)!])
    let line = CTLineCreateWithAttributedString(attributed)
    let bounds = CTLineGetBoundsWithOptions(line, .useOpticalBounds)

    context.saveGState()
    context.textPosition = CGPoint(
        x: cap.midX - bounds.width / 2 - bounds.minX,
        y: cap.midY - bounds.height / 2 - bounds.minY)
    CTLineDraw(line, context)
    context.restoreGState()
}

func writePNG(_ image: CGImage, to url: URL) {
    guard
        let destination = CGImageDestinationCreateWithURL(
            url as CFURL, "public.png" as CFString, 1, nil)
    else { fatalError("cannot write \(url.path)") }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else { fatalError("PNG encode failed") }
}

// MARK: - Entry point

let arguments = Array(CommandLine.arguments.dropFirst())
guard let outputPath = arguments.first else {
    FileHandle.standardError.write(Data("usage: make-icon.swift <output .iconset>\n".utf8))
    exit(1)
}
let outputURL = URL(fileURLWithPath: outputPath)
try? FileManager.default.createDirectory(at: outputURL, withIntermediateDirectories: true)

/// Every entry Apple's iconset format expects: base size plus its @2x companion.
let variants: [(base: Int, scale: Int)] = [
    (16, 1), (16, 2), (32, 1), (32, 2), (128, 1), (128, 2), (256, 1), (256, 2), (512, 1), (512, 2),
]

for variant in variants {
    let pixels = variant.base * variant.scale
    let context = makeContext(pixels)
    draw(into: context)
    guard let image = context.makeImage() else { fatalError("render failed at \(pixels)px") }
    let suffix = variant.scale == 1 ? "" : "@\(variant.scale)x"
    let name = "icon_\(variant.base)x\(variant.base)\(suffix).png"
    writePNG(image, to: outputURL.appendingPathComponent(name))
}

// A standalone 1024 PNG, handy for the README and for anything that wants a plain image.
let previewContext = makeContext(1024)
draw(into: previewContext)
if let image = previewContext.makeImage() {
    writePNG(image, to: outputURL.deletingLastPathComponent().appendingPathComponent("icon-1024.png"))
}

print("wrote \(variants.count) sizes to \(outputURL.lastPathComponent)")

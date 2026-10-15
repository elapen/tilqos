import Foundation

/// Re-reads text that already exists — a selection, or the clipboard — as if it had been
/// typed on a different layout.
///
/// This is a guess by nature: unlike the keystroke buffer, finished text carries no record
/// of which keys produced it, so the source layout has to be assumed. Characters the source
/// layout cannot produce are passed through untouched, which keeps punctuation, emoji and
/// text from a third language intact.
public enum TextConverter {
    public static func convert(_ text: String, from source: KeyMapTable, to target: KeyMapTable)
        -> String
    {
        var result = ""
        result.reserveCapacity(text.count)
        for character in text {
            guard let stroke = source.stroke(for: character),
                let produced = target.text(for: stroke)
            else {
                result.append(character)
                continue
            }
            result += produced
        }
        return result
    }

    /// Every reading of `text`, one per candidate layout, excluding no-ops.
    public static func candidates(
        _ text: String, from source: KeyMapTable, layouts: [KeyMapTable]
    ) -> [(layout: KeyMapTable, text: String)] {
        layouts.compactMap { layout in
            guard layout.sourceID != source.sourceID else { return nil }
            let converted = convert(text, from: source, to: layout)
            guard converted != text else { return nil }
            return (layout, converted)
        }
    }

    /// Mean per-word cost of a passage under one language, for ranking conversions.
    /// Returns nil when the passage has no scorable word.
    public static func cost(of text: String, lang: Lang, models: ModelStore) -> Double? {
        guard let model = models.model(for: lang) else { return nil }
        let words = text.split(whereSeparator: { !Tokenizer.isWordCharacter($0) })
        var total = 0.0
        var counted = 0
        for word in words {
            let candidate = String(word)
            if WordScorer.isNumeric(candidate) {
                total += WordScorer.numericCost
                counted += 1
            } else if let score = model.score(candidate) {
                total += score.penalizedCost
                counted += 1
            }
        }
        guard counted > 0 else { return nil }
        return total / Double(counted)
    }

    /// The best re-reading of `text`, or nil when leaving it alone scores at least as well.
    public static func bestConversion(
        _ text: String, from source: KeyMapTable, layouts: [KeyMapTable], models: ModelStore
    ) -> (layout: KeyMapTable, text: String)? {
        guard let sourceLang = source.lang,
            let asIs = cost(of: text, lang: sourceLang, models: models)
        else { return nil }

        var best: (layout: KeyMapTable, text: String)?
        var bestCost = asIs
        for candidate in candidates(text, from: source, layouts: layouts) {
            guard let lang = candidate.layout.lang,
                let cost = cost(of: candidate.text, lang: lang, models: models),
                cost < bestCost
            else { continue }
            bestCost = cost
            best = candidate
        }
        return best
    }
}

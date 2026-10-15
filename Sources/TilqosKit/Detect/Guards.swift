import Foundation

/// Decides which words must never be rewritten, whatever the models think.
///
/// This is where most of the app's perceived quality lives. A missed correction is a
/// shrug; a mangled URL or a rewritten identifier in a code editor is the reason people
/// uninstall utilities like this one. When in doubt, protect.
public enum Guards {
    public enum SkipReason: String, Sendable {
        case tooShort
        case exception
        case allDigits
        case acronym
        case email
        case url
        case path
        case codeIdentifier
        case sigil
        case mixedLayouts
    }

    /// Separator characters that mark whatever follows as machine text rather than prose.
    static let sigils: Set<Character> = ["$", "`", "#", "\\", "~", "@", "%", "&"]

    private static let patterns: [(SkipReason, NSRegularExpression)] = {
        let sources: [(SkipReason, String)] = [
            (.email, #"[\w.+-]+@[\w-]+\.[\w.-]+"#),
            (.url, #"[a-zA-Z][a-zA-Z0-9+.\-]*://\S+"#),
            (
                .url,
                #"\b(?:www\.)?[\w-]+\.(?:com|net|org|io|dev|app|co|ru|kz|uk|de|fr|me|ai|sh|xyz|info|biz|edu|gov)\b"#
            ),
            (.path, #"(?:^|\s)(?:~|\.{1,2})?/[\w./~-]+"#),
            (.path, #"\\[\w\\.-]+"#),
            // snake_case, camelCase, and anything that looks like a call.
            (.codeIdentifier, #"\b\w+(?:_\w+)+\b"#),
            (.codeIdentifier, #"\b[a-z][a-z0-9]*(?:[A-Z][a-z0-9]*)+\b"#),
            (.codeIdentifier, #"\b\w+\("#),
        ]
        return sources.compactMap { reason, pattern in
            guard let regex = try? NSRegularExpression(pattern: pattern) else {
                Log.error(.detect, "guard pattern failed to compile: \(pattern)")
                return nil
            }
            return (reason, regex)
        }
    }()

    /// Maps word-token indices to the reason their *surroundings* put them off limits.
    ///
    /// Only context-dependent reasons live here — `user@example.com` tokenizes into
    /// six innocent-looking words and only the joined text reveals it is an address.
    /// Checks that need nothing but the word itself belong in `wordLevelReason`, which the
    /// caller applies separately: it takes parameters this function cannot know about,
    /// and conflating the two once let the length floor veto a decisive numeric reading.
    public static func protected(tokens: [Tokenizer.Token]) -> [Int: SkipReason] {
        var result: [Int: SkipReason] = [:]

        // Reconstruct the line, recording each token's span in UTF-16 units so regex
        // match ranges can be mapped back onto tokens.
        var text = ""
        var spans: [Range<Int>] = []
        for token in tokens {
            let start = text.utf16.count
            text += token.displayed
            spans.append(start..<text.utf16.count)
        }

        for (index, token) in tokens.enumerated() where token.isWord {
            // A word introduced by $ ` # \ ~ @ is machine text.
            if let previous = index > 0 ? tokens[index - 1] : nil,
                !previous.isWord,
                let last = previous.displayed.last,
                sigils.contains(last)
            {
                result[index] = .sigil
            }
        }

        guard !text.isEmpty else { return result }
        let full = NSRange(location: 0, length: text.utf16.count)

        for (reason, regex) in patterns {
            for match in regex.matches(in: text, options: [], range: full) {
                let matched = match.range.lowerBound..<match.range.upperBound
                for (index, span) in spans.enumerated() where tokens[index].isWord {
                    guard result[index] == nil else { continue }
                    if span.overlaps(matched) { result[index] = reason }
                }
            }
        }
        return result
    }

    /// Checks that need only the word itself.
    ///
    /// `allowNumeric` exists because an all-digit token is the one case where a very short
    /// word carries a decisive signal: `әғ` rendering as `25` is a certain mistake, not a
    /// statistical guess, so the length floor must not hide it.
    static func wordLevelReason(
        _ token: Tokenizer.Token,
        minLength: Int,
        exceptions: Set<String>,
        allowNumeric: Bool = false
    ) -> SkipReason? {
        let word = token.displayed

        if word.allSatisfy(\.isNumber) {
            // Already a plain number; nothing about it can be a layout mistake.
            return .allDigits
        }
        if exceptions.contains(word.lowercased()) { return .exception }
        if !allowNumeric && word.count < minLength { return .tooShort }
        // FYI, HTTP, ГОСТ — initialisms are not words the models know.
        if word.count > 1, word.contains(where: \.isLetter),
            word == word.uppercased(), word != word.lowercased()
        {
            return .acronym
        }
        return nil
    }
}

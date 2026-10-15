import Foundation
import TilqosKit

// Builds a trigram model from a word list.
//
//   GenModel <en|ru|kk> <input word list> <output .bin>
//
// Accepts hunspell .dic files (the leading count line and the "/FLAGS" affix suffixes are
// stripped) as well as plain one-word-per-line lists such as /usr/share/dict/words.

func fail(_ message: String) -> Never {
    FileHandle.standardError.write(Data("error: \(message)\n".utf8))
    exit(1)
}

/// Splits on any newline. In Swift a CRLF pair is a single Character, so splitting on "\n"
/// silently matches nothing in a CRLF file — which is what the Kazakh dictionary uses.
func lines(of url: URL) -> [String] {
    guard let raw = try? String(contentsOf: url, encoding: .utf8) else {
        fail("cannot read \(url.path) as UTF-8")
    }
    return raw.split(whereSeparator: \.isNewline).map {
        // Strip the UTF-8 BOM from the first line.
        $0.hasPrefix("\u{FEFF}") ? String($0.dropFirst()) : String($0)
    }
}

/// Builds the suffix-stripping data: hashed stems plus the real affix rules.
func buildMorphology(dictionary: URL, affixes: URL, output: URL) {
    var stems: [UInt32] = []
    var stemCount = 0
    for line in lines(of: dictionary) {
        let entry = line.trimmingCharacters(in: .whitespaces)
        guard !entry.isEmpty, !entry.allSatisfy(\.isNumber) else { continue }
        // "аба/ABbCcDd" — the flags after the slash say which suffixes apply. They are dropped:
        // ignoring them makes the check slightly generous, which is the safe direction for
        // something used as evidence rather than as a filter.
        let stem = String(entry.split(separator: "/", maxSplits: 1)[0]).lowercased()
        guard !stem.isEmpty else { continue }
        stems.append(Morphology.hash(stem))
        stemCount += 1
    }

    // "SFX <flag> <strip> <affix> <condition>"; a line whose 4th field is a number is the
    // group header rather than a rule.
    var suffixes: [String: Set<String>] = [:]
    var ruleCount = 0
    for line in lines(of: affixes) where line.hasPrefix("SFX") {
        let fields = line.split(separator: " ", omittingEmptySubsequences: true).map(String.init)
        guard fields.count >= 4, Int(fields[3]) == nil else { continue }
        let stripping = fields[2] == "0" ? "" : fields[2]
        let affix = String(fields[3].split(separator: "/", maxSplits: 1)[0])
        guard affix != "0", !affix.isEmpty else { continue }
        suffixes[affix, default: []].insert(stripping)
        ruleCount += 1
    }

    // The personal endings (жіктік жалғау) are missing from the source affix file, which is a
    // genuine gap rather than a modelling choice: without `-мін` the form `екенмін` does not
    // decompose at all, while `екенмің` decomposes through the accidental chain `екен + м + ің`
    // and so looks better supported than the correct spelling. This is a closed, well-defined
    // set of morphemes, so listing it is filling in data, not tuning.
    let personalEndings = [
        "мын", "мін", "бын", "бін", "пын", "пін",  // 1sg
        "мыз", "міз", "быз", "біз", "пыз", "піз",  // 1pl
        "сың", "сің", "сыз", "сіз",  // 2sg, 2sg polite
        "сыңдар", "сіңдер", "сыздар", "сіздер",  // 2pl
    ]
    var addedEndings = 0
    for ending in personalEndings where suffixes[ending] == nil {
        suffixes[ending] = [""]
        addedEndings += 1
    }

    guard !stems.isEmpty, !suffixes.isEmpty else {
        fail("no stems or no suffixes found — check the input files")
    }

    let morphology = Morphology(
        stems: stems, suffixes: suffixes.mapValues { Array($0).sorted() })
    do {
        try morphology.encode().write(to: output, options: .atomic)
    } catch {
        fail("cannot write \(output.path): \(error.localizedDescription)")
    }

    let size = morphology.encode().count
    print(
        """
        morphology: \(stemCount) stems, \(suffixes.count) distinct suffixes \
        from \(ruleCount) rules (+\(addedEndings) personal endings the source file omits)
          longest suffix: \(suffixes.keys.map(\.count).max() ?? 0) characters
          wrote \(output.lastPathComponent) (\(size / 1024) KB)
        """
    )
}

/// Builds the word-form lexicon: hashed stems with their affix flags, plus the affix rules, plus
/// hand-written extra words and the frequency tier. See `Lexicon`.
func buildLexicon(lang: Lang, dictionary: URL, affixes: URL, extra: [URL], core: [URL], output: URL)
{
    func read(_ url: URL) -> String {
        guard let text = try? String(contentsOf: url, encoding: .utf8) else {
            fail("cannot read \(url.path) as UTF-8")
        }
        return text
    }
    // Word lists are whitespace-separated words; `#` starts a comment that runs to the end of
    // the line.
    func list(_ urls: [URL]) -> [String] {
        urls.flatMap { lines(of: $0) }
            .map { String($0.split(separator: "#", maxSplits: 1, omittingEmptySubsequences: false)[0]) }
            .flatMap { $0.split(whereSeparator: \.isWhitespace).map(String.init) }
    }
    let extraWords = list(extra)
    let coreWords = list(core)
    let lexicon = Lexicon.build(
        lang: lang, dictionary: read(dictionary), affixes: read(affixes), extra: extraWords,
        core: coreWords)
    let data = lexicon.encode()
    do {
        try data.write(to: output, options: .atomic)
    } catch {
        fail("cannot write \(output.path): \(error.localizedDescription)")
    }

    // Every core word must be recognised, or the frequency tier and the lexicon disagree about
    // what counts as a word.
    let unknownCore = coreWords.filter { !lexicon.contains($0) }
    print(
        """
        \(lang.rawValue) lexicon: \(lexicon.stemCount) stems, \(lexicon.ruleCount) affix rules, \
        \(extraWords.count) extra words, \(lexicon.coreCount) core words
          wrote \(output.lastPathComponent) (\(data.count / 1024) KB)
        """)
    if !unknownCore.isEmpty {
        print("  note: core words the dictionary does not derive: \(unknownCore.joined(separator: " "))")
    }
}

let arguments = Array(CommandLine.arguments.dropFirst())

// Lexicon mode:
//   GenModel lexicon <en|ru|kk> <.dic> <.aff> <output .lex> [--extra <list>]... [--core <list>]...
if arguments.first == "lexicon" {
    guard arguments.count >= 5, let lang = Lang(rawValue: arguments[1]) else {
        fail(
            "usage: GenModel lexicon <en|ru|kk> <.dic> <.aff> <output .lex> "
                + "[--extra <list>]... [--core <list>]...")
    }
    var extra: [URL] = []
    var core: [URL] = []
    var index = 5
    while index + 1 < arguments.count {
        let url = URL(fileURLWithPath: arguments[index + 1])
        switch arguments[index] {
        case "--extra": extra.append(url)
        case "--core": core.append(url)
        default: fail("unknown option \(arguments[index])")
        }
        index += 2
    }
    buildLexicon(
        lang: lang, dictionary: URL(fileURLWithPath: arguments[2]),
        affixes: URL(fileURLWithPath: arguments[3]), extra: extra, core: core,
        output: URL(fileURLWithPath: arguments[4]))
    exit(0)
}

// Second mode: build the morphology file from a hunspell dictionary and its affix rules.
if arguments.first == "morph" {
    guard arguments.count == 4 else {
        fail("usage: GenModel morph <dictionary .dic> <affix .aff> <output .bin>")
    }
    buildMorphology(
        dictionary: URL(fileURLWithPath: arguments[1]),
        affixes: URL(fileURLWithPath: arguments[2]),
        output: URL(fileURLWithPath: arguments[3]))
    exit(0)
}

guard arguments.count == 3 else {
    fail(
        """
        usage: GenModel <en|ru|kk> <input word list> <output .bin>
               GenModel morph <dictionary .dic> <affix .aff> <output .bin>
        """)
}
guard let lang = Lang(rawValue: arguments[0]) else {
    fail("unknown language '\(arguments[0])' (expected en, ru or kk)")
}

let inputURL = URL(fileURLWithPath: arguments[1])
let outputURL = URL(fileURLWithPath: arguments[2])
let alphabet = Alphabet.forScript(lang.script)
let size = alphabet.symbolCount

guard let raw = try? String(contentsOf: inputURL, encoding: .utf8) else {
    fail("cannot read \(inputURL.path) as UTF-8")
}

// counts[(a * size + b) * size + c] and the marginal for each (a, b) context.
var counts = [UInt32](repeating: 0, count: size * size * size)
var contextCounts = [UInt32](repeating: 0, count: size * size)

let boundary = alphabet.boundary
var wordsUsed = 0
var runsUsed = 0
var linesSkipped = 0

/// Counts one run of in-alphabet letters, padded so word starts and endings are modelled.
func count(run: [Int]) {
    guard !run.isEmpty else { return }
    var padded = [boundary, boundary]
    padded.append(contentsOf: run)
    padded.append(boundary)
    for index in 2..<padded.count {
        let a = padded[index - 2]
        let b = padded[index - 1]
        let c = padded[index]
        counts[(a * size + b) * size + c] += 1
        contextCounts[a * size + b] += 1
    }
    runsUsed += 1
}

// Split on any newline rather than on "\n": in Swift a CRLF pair is a single Character,
// so `split(separator: "\n")` silently matches nothing in a CRLF file — which is exactly
// what the Kazakh dictionary uses.
for line in raw.split(whereSeparator: \.isNewline) {
    // Strip the UTF-8 BOM, hunspell affix flags, and surrounding whitespace.
    var entry = line.hasPrefix("\u{FEFF}") ? String(line.dropFirst()) : String(line)
    if let slash = entry.firstIndex(of: "/") { entry = String(entry[entry.startIndex..<slash]) }
    entry = entry.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    guard !entry.isEmpty else { continue }
    // The hunspell header line is just a word count.
    if entry.allSatisfy(\.isNumber) { continue }

    // Split into maximal runs of alphabet letters, so "кто-то" contributes both halves
    // rather than being thrown away.
    var run: [Int] = []
    var contributed = false
    for character in entry {
        if let index = alphabet.index(of: character) {
            run.append(index)
        } else {
            if !run.isEmpty { count(run: run); contributed = true }
            run.removeAll(keepingCapacity: true)
        }
    }
    if !run.isEmpty { count(run: run); contributed = true }

    if contributed { wordsUsed += 1 } else { linesSkipped += 1 }
}

guard runsUsed > 0 else { fail("no usable words found in \(inputURL.lastPathComponent)") }

// Add-one smoothing over the symbol inventory, so an unseen trigram in a well-attested
// context still costs more than one in a context we have never seen either.
var costs = [UInt8](repeating: 0, count: counts.count)
let maxStored = Double(UInt8.max)
for a in 0..<size {
    for b in 0..<size {
        let context = Double(contextCounts[a * size + b]) + Double(size)
        for c in 0..<size {
            let index = (a * size + b) * size + c
            let probability = (Double(counts[index]) + 1.0) / context
            let bits = -log2(probability)
            costs[index] = UInt8(min(maxStored, max(0, (bits * TrigramModel.costScale).rounded())))
        }
    }
}

let model = TrigramModel(lang: lang, alphabet: alphabet, costs: costs)
do {
    try model.encode().write(to: outputURL, options: .atomic)
} catch {
    fail("cannot write \(outputURL.path): \(error.localizedDescription)")
}

let observed = counts.reduce(0) { $0 + ($1 > 0 ? 1 : 0) }
print(
    """
    \(lang.rawValue): \(wordsUsed) entries, \(runsUsed) runs, \(linesSkipped) skipped
      alphabet: \(alphabet.letterCount) letters + boundary = \(size) symbols
      trigrams: \(observed) of \(counts.count) observed \
    (\(String(format: "%.1f", Double(observed) / Double(counts.count) * 100))%)
      wrote \(outputURL.lastPathComponent) (\(costs.count + TrigramModel.headerSize) bytes)
    """
)

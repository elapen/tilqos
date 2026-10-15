import Foundation

/// Holds the trigram models and word lexicons, and loads them off the startup path.
///
/// About 2 MB in all, and reading and decoding it has no business happening before the menu
/// bar icon appears, so `loadInBackground` is what the app calls and the detector simply
/// reports "not ready" until it finishes.
public final class ModelStore {
    public static let shared = ModelStore()

    private let queue = DispatchQueue(label: "io.github.elapen.tilqos.models")
    private var models: [Lang: TrigramModel] = [:]
    private var lexicons: [Lang: Lexicon] = [:]
    private var morphology: Morphology?
    private var loaded = false
    private var loadCount = 0

    public init() {}

    public var isLoaded: Bool { queue.sync { loaded } }

    /// Changes every time the data is (re)loaded. Anything derived from the models — cached
    /// word judgements in particular — is only valid for the generation it was computed on.
    public var generation: Int { queue.sync { loadCount } }

    public func model(for lang: Lang) -> TrigramModel? {
        queue.sync { models[lang] }
    }

    /// The language's word forms. Nil when the file is missing, in which case the detector
    /// falls back to the trigram models alone — the behaviour before lexicons existed.
    public func lexicon(for lang: Lang) -> Lexicon? {
        queue.sync { lexicons[lang] }
    }

    /// Kazakh suffix-stripping data. Only Kazakh has it: it is the language whose forms a
    /// character model cannot judge, being agglutinative.
    public func kazakhMorphology() -> Morphology? {
        queue.sync { morphology }
    }

    public var availableLanguages: Set<Lang> {
        queue.sync { Set(models.keys) }
    }

    /// Default location: the app bundle's resource directory.
    public static var bundledModelDirectory: URL? {
        Bundle.main.resourceURL
    }

    @discardableResult
    public func load(from directory: URL) -> Set<Lang> {
        var loadedModels: [Lang: TrigramModel] = [:]
        for lang in Lang.allCases {
            let url = directory.appendingPathComponent("\(lang.rawValue).bin")
            guard let data = try? Data(contentsOf: url) else {
                Log.warn(.detect, "no model for \(lang.rawValue) at \(url.lastPathComponent)")
                continue
            }
            do {
                loadedModels[lang] = try TrigramModel.decode(data)
            } catch {
                Log.error(.detect, "\(lang.rawValue) model rejected: \(error)")
            }
        }

        var loadedLexicons: [Lang: Lexicon] = [:]
        for lang in Lang.allCases {
            // `.bin` like every other data file, because that is what the build bundles.
            let url = directory.appendingPathComponent("\(lang.rawValue)-lex.bin")
            guard let data = try? Data(contentsOf: url) else {
                Log.warn(.detect, "no lexicon for \(lang.rawValue)")
                continue
            }
            do {
                loadedLexicons[lang] = try Lexicon.decode(data)
            } catch {
                Log.error(.detect, "\(lang.rawValue) lexicon rejected: \(error)")
            }
        }

        var loadedMorphology: Morphology?
        let morphURL = directory.appendingPathComponent("kk-morph.bin")
        if let data = try? Data(contentsOf: morphURL) {
            do {
                loadedMorphology = try Morphology.decode(data)
            } catch {
                Log.error(.detect, "morphology rejected: \(error)")
            }
        }

        queue.sync {
            models = loadedModels
            lexicons = loadedLexicons
            morphology = loadedMorphology
            loaded = true
            loadCount += 1
        }
        Log.info(
            .detect,
            "lexicons loaded: "
                + loadedLexicons.values.map { "\($0.lang.rawValue)=\($0.stemCount)" }.sorted()
                .joined(separator: ","))
        if let loadedMorphology {
            Log.info(
                .detect,
                "morphology: \(loadedMorphology.stemCount) stems, "
                    + "\(loadedMorphology.suffixCount) suffixes")
        }
        Log.info(
            .detect,
            "models loaded: \(loadedModels.keys.map(\.rawValue).sorted().joined(separator: ","))")
        return Set(loadedModels.keys)
    }

    /// Loads on a utility queue and calls back on the main queue.
    public func loadInBackground(
        from directory: URL? = ModelStore.bundledModelDirectory,
        completion: ((Set<Lang>) -> Void)? = nil
    ) {
        guard let directory else {
            Log.error(.detect, "no resource directory to load models from")
            completion?([])
            return
        }
        DispatchQueue.global(qos: .utility).async { [weak self] in
            let langs = self?.load(from: directory) ?? []
            DispatchQueue.main.async { completion?(langs) }
        }
    }
}

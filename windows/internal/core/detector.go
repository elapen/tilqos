package core

// Port of Detect/Detector.swift.

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

// WordChange is one word an outcome changes, as the user will see it. Feeds learning.
type WordChange struct {
	// Typed is the word as it is on screen now.
	Typed      string
	Corrected  string
	TypedLang  Lang
	TargetLang Lang
}

// Outcome is a correction the detector is prepared to stand behind.
type Outcome struct {
	Replacement Replacement
	Corrections []WordCorrection
	Confidence  float64
	// Before and After are the affected region as it looks now, and as it would look.
	Before string
	After  string
	// TargetLang is the language of the last corrected word — what to switch the keyboard to.
	TargetLang     Lang
	TargetLayoutID string
	Changes        []WordChange
	// WordRanges are the stroke ranges of the changed words themselves, without the
	// punctuation re-rendered after them.
	WordRanges [][2]int
}

// ActionKind is what the detector recommends.
type ActionKind int

const (
	ActionNone ActionKind = iota
	// ActionSuggest: confident enough to offer, not to impose.
	ActionSuggest
	ActionFix
)

func (k ActionKind) String() string {
	switch k {
	case ActionSuggest:
		return "suggest"
	case ActionFix:
		return "fix"
	}
	return "none"
}

// Action is the detector's verdict; Outcome is nil for ActionNone.
type Action struct {
	Kind    ActionKind
	Outcome *Outcome
}

// shifted is the same action with its stroke indices moved, for results computed on a window.
func (a Action) shifted(offset int) Action {
	if a.Outcome == nil || offset == 0 {
		return a
	}
	outcome := *a.Outcome
	outcome.Corrections = make([]WordCorrection, len(a.Outcome.Corrections))
	for i, correction := range a.Outcome.Corrections {
		outcome.Corrections[i] = WordCorrection{Lo: correction.Lo + offset, Hi: correction.Hi + offset,
			TargetLayoutID: correction.TargetLayoutID}
	}
	outcome.WordRanges = make([][2]int, len(a.Outcome.WordRanges))
	for i, r := range a.Outcome.WordRanges {
		outcome.WordRanges[i] = [2]int{r[0] + offset, r[1] + offset}
	}
	return Action{Kind: a.Kind, Outcome: &outcome}
}

// Scope selects which words an evaluation considers.
type Scope int

const (
	// ScopeLastWord is just the word a separator has closed — the per-keystroke path.
	ScopeLastWord Scope = iota
	// ScopeTail is the trailing run of words, for catching mistakes that already slipped past.
	ScopeTail
)

// Context is everything one evaluation needs.
type Context struct {
	Recorded []RecordedStroke
	// CandidateLayouts are the layouts the detector may propose, already filtered by the
	// user's language choices.
	CandidateLayouts []*KeyMapTable
	Resolve          Resolver
	MinWordLength    int
	Exceptions       StringSet
	AutoThreshold    float64
	SuggestThreshold float64
	// MaxWords is how many trailing words the wide scan considers.
	MaxWords int
	// PrecedingText is text in front of the buffer, read from the text field, so the first
	// words after a click still have context ("" for none). In memory only; never logged.
	PrecedingText string
	// Learned is what the user taught; nil when learning is off.
	Learned LearnedVocabulary
	// Manual: the user asked for a fix (the fix-last-word hotkey): pick the best reading
	// even if the word on screen is a real word, and ignore the length floor.
	Manual bool
}

// Swift defaults for Context.
const (
	DefaultMinWordLength    = 3
	DefaultAutoThreshold    = 0.80
	DefaultSuggestThreshold = 0.45
	DefaultMaxWords         = 24
)

// NewContext fills in the Swift defaults.
func NewContext(recorded []RecordedStroke, candidateLayouts []*KeyMapTable, resolve Resolver) Context {
	return Context{
		Recorded:         recorded,
		CandidateLayouts: candidateLayouts,
		Resolve:          resolve,
		MinWordLength:    DefaultMinWordLength,
		Exceptions:       StringSet{},
		AutoThreshold:    DefaultAutoThreshold,
		SuggestThreshold: DefaultSuggestThreshold,
		MaxWords:         DefaultMaxWords,
	}
}

// Tuning. Every number was set against the regression corpus (CorpusTests), and each one
// says which way it errs.
const (
	// ConfidenceCertain clears the default automatic threshold (0.80).
	ConfidenceCertain   = 0.95
	ConfidenceConfident = 0.9
	// ConfidenceOfferOnly clears the default suggestion threshold (0.45) but not the default
	// automatic one: shown, not imposed — unless the user lowers the automatic threshold.
	ConfidenceOfferOnly = 0.6
	// contextWeight is how much each neighbouring word in a script tilts the trigram gap, in
	// bits. Small on purpose: context may tip a close call, but a real English word inside
	// Russian text must stay English.
	contextWeight = 0.35
	// knownAltTolerance is how far a trigram gap may lean against a known conversion before
	// it is refused.
	knownAltTolerance = -1.0
	// shortGarbageGap is the trigram gap that counts as "the typed text is garbage" for
	// three-letter words.
	shortGarbageGap = 0.8
	// unknownOfferGap: neither reading is a word, so only offered, and only when the gap is
	// at least this wide.
	unknownOfferGap = 1.8
	// backfillLimit is how many unfixed garbled words before a fixed one may be pulled into
	// the same fix.
	backfillLimit = 4
	// LastWordWindow is how many trailing strokes the last-word path looks at: the word, the
	// four it may pull in, and context for them.
	LastWordWindow = 64
)

// Detector turns a buffer of keystrokes into a decision: leave it, offer a fix, or apply one.
//
// The decision rests on evidence in this order, strongest first:
//
//  1. Is the text on screen a real word? If it is, it is never rewritten automatically —
//     `еще`, `тот`, `her` stay what they are, whatever a character model thinks of them.
//  2. Is the other reading a real word? Layout garble is not a word in any language
//     (`ghbdtn`), while its conversion is (`привет`).
//  3. Frequency, for short words: a short conversion must land on one of the language's
//     most frequent words (`ult` → `где`).
//  4. Context. The script of the words around it tilts the decision, never decides it.
//  5. Structure. A digit wedged inside a Cyrillic word (`с2лем`) is Kazakh typed on the
//     Russian layout, whatever the dictionaries say.
//  6. The character trigram models, last — for words no dictionary has, and then only as an
//     offer, never an automatic change.
type Detector struct {
	scorer *WordScorer
}

func NewDetector(models *Models) *Detector {
	return &Detector{scorer: NewWordScorer(models)}
}

// Scorer exposes the word scorer (tests and diagnostics).
func (d *Detector) Scorer() *WordScorer { return d.scorer }

// IsDictionaryWord: the shipped dictionaries (lexicon, frequency tier, brand list) know the
// word — what the user teaches does not count here.
func (d *Detector) IsDictionaryWord(word string, lang Lang) bool {
	return d.scorer.IsKnown(word, lang, nil)
}

// Confidence maps a trigram cost gap in bits onto 0...1.
//
// Calibrated against the separation measured on the shipped models: real words score
// 2.9–3.9 bits per trigram and layout garble 5.6–7.3, so a genuine mistake shows a gap of
// roughly 1.7–3.5 while correctly typed text sits at or below zero.
func Confidence(gap float64) float64 {
	if math.IsInf(gap, 0) || math.IsNaN(gap) {
		if gap > 0 {
			return 1
		}
		return 0
	}
	return swiftMin(1, swiftMax(0, (gap-0.4)/1.6))
}

// EndsWord reports whether a keystroke ends a word: it produces no letter on the layout it
// was typed on, nor on any other candidate layout.
//
// The second half is what keeps `,skj` whole. On ABC `,` is punctuation, but it is the key
// for `б`, and a Russian word typed on the wrong layout contains it mid-word: `было`, `тебе`
// (`nt,t`), `люблю` (`k.,k.`), `хорошо` (`[jhjij`).
func EndsWord(stroke KeyStroke, layout *KeyMapTable, candidates []*KeyMapTable) bool {
	if layout == nil {
		return false
	}
	produced, ok := layout.Text(stroke)
	if !ok || produced == "" || layout.ProducesWordCharacter(stroke) {
		return false
	}
	for _, candidate := range candidates {
		if candidate.ProducesWordCharacter(stroke) {
			return false
		}
	}
	return true
}

// IsWordText: non-empty and nothing but word characters.
func IsWordText(text string) bool {
	return text != "" && AllChars(text, IsWordCharacter)
}

// MARK: - Evaluation

// world is everything one evaluation works from.
type world struct {
	context   Context
	languages []Lang
	layoutFor map[Lang]*KeyMapTable
	slots     []slot
	prior     contextPrior
}

// slot is a word-shaped run of strokes between separators that are separators on every
// layout.
type slot struct {
	lo, hi int
	// layoutID is the layout every stroke was typed on; hasLayoutID is false when the word
	// mixes layouts, in which case it is pinned as it is.
	layoutID    string
	hasLayoutID bool
	typedLayout *KeyMapTable
	typedLang   Lang
	// mainRun is the longest run of strokes that are letters on the typed layout — the word
	// the typed layout itself sees, without the punctuation keys around it.
	mainRun     [2]int
	hasMainRun  bool
	mainRunText string
	// script of the text on screen, for context; ScriptNone for digits and punctuation.
	script Script
	// screen is the word as it is on screen, punctuation keys included.
	screen string
	// guardReason is set when the surroundings put the word off limits (URL, address,
	// code, sigil).
	guardReason SkipReason
}

// judgement is the verdict on one word: the reading it should become and how sure we are.
type judgement struct {
	slotIndex  int
	typed      Reading
	target     Reading
	confidence float64
}

// Evaluate runs the detector over the context.
func (d *Detector) Evaluate(context Context, scope Scope) Action {
	none := Action{Kind: ActionNone}
	if len(context.Recorded) == 0 {
		return none
	}

	// One layout per language, in a stable order.
	var languages []Lang
	layoutFor := map[Lang]*KeyMapTable{}
	for _, layout := range context.CandidateLayouts {
		if layout.Lang == LangNone {
			continue
		}
		if _, seen := layoutFor[layout.Lang]; seen {
			continue
		}
		languages = append(languages, layout.Lang)
		layoutFor[layout.Lang] = layout
	}
	if len(languages) <= 1 {
		return none
	}
	layouts := make([]*KeyMapTable, len(languages))
	for i, lang := range languages {
		layouts[i] = layoutFor[lang]
	}

	// Every stroke is resolved several times per evaluation; resolve each layout once.
	cache := map[string]*KeyMapTable{}
	for _, layout := range context.CandidateLayouts {
		cache[layout.SourceID] = layout
	}
	original := context.Resolve
	for _, entry := range context.Recorded {
		if _, ok := cache[entry.LayoutID]; !ok {
			if table := original(entry.LayoutID); table != nil {
				cache[entry.LayoutID] = table
			}
		}
	}
	context.Resolve = func(id string) *KeyMapTable {
		if table, ok := cache[id]; ok {
			return table
		}
		return original(id)
	}

	// The last-word path only needs the end of the buffer. Cut at a separator so no word is
	// split, and shift the result back afterwards.
	offset := 0
	if scope == ScopeLastWord && len(context.Recorded) > LastWordWindow {
		cut := len(context.Recorded) - LastWordWindow
		for cut < len(context.Recorded) {
			entry := context.Recorded[cut]
			cut++
			if typed := context.Resolve(entry.LayoutID); typed != nil && EndsWord(entry.Stroke, typed, layouts) {
				break
			}
		}
		offset = cut
		context.Recorded = context.Recorded[cut:]
		if len(context.Recorded) == 0 {
			return none
		}
	}

	slots := d.segment(context, layouts)
	if len(slots) == 0 {
		return none
	}
	preceding := context.PrecedingText
	if offset > 0 {
		preceding = ""
	}
	w := &world{context: context, languages: languages, layoutFor: layoutFor, slots: slots,
		prior: newContextPrior(preceding)}

	var action Action
	switch scope {
	case ScopeLastWord:
		action = d.evaluateLast(w)
	default:
		action = d.evaluateTail(w)
	}
	return action.shifted(offset)
}

// MARK: - Last word

func (d *Detector) evaluateLast(w *world) Action {
	index := len(w.slots) - 1
	support := func(script Script) int { return d.supportBefore(script, index, w) }
	judged, ok := d.judge(index, w, support)
	if !ok || !(judged.confidence > 0) {
		return Action{Kind: ActionNone}
	}
	judgements := []judgement{judged}
	// A confident fix vouches for its neighbours: `yt pyf.` is `не знаю`, although `yt`
	// alone was too short to touch.
	if judged.confidence >= w.context.AutoThreshold || w.context.Manual {
		judgements = append(d.backfill(judged, w), judgements...)
	}
	return d.action(judgements, w)
}

// backfill: unfixed words right before a fixed one, typed on the same wrong layout, whose
// other reading is a real word. Stops at the first word that does not qualify.
func (d *Detector) backfill(anchor judgement, w *world) []judgement {
	anchorSlot := w.slots[anchor.slotIndex]
	targetLayout := w.layoutFor[anchor.target.Lang]
	if targetLayout == nil {
		return nil
	}
	var result []judgement
	for index := anchor.slotIndex - 1; index >= 0 && len(result) < backfillLimit; index-- {
		s := w.slots[index]
		skip := d.skipReason(s, w.context)
		if s.hasLayoutID != anchorSlot.hasLayoutID || s.layoutID != anchorSlot.layoutID ||
			!(skip == SkipNone || skip == SkipTooShort) || s.typedLayout == nil || s.typedLang == LangNone {
			break
		}
		typed := d.scorer.Reading(w.context.Recorded, s.lo, s.hi, s.typedLayout, s.typedLang, w.context.Learned, false)
		target := d.scorer.Reading(w.context.Recorded, s.lo, s.hi, targetLayout, anchor.target.Lang, w.context.Learned, false)
		// Next to a sure fix, a rare word on the same layout that reads as a frequent word of
		// the target language is garble too: `yt` (a rare English abbreviation) before
		// `pyf.` is `не` before `знаю`. A frequent typed word is never pulled in.
		typedIsWord := typed.IsKnown && !typed.HasOddEdges
		if !target.IsClean || CanonEqual(target.Text, typed.Text) || !target.IsKnown ||
			!(target.Length >= 3 || target.IsCore) ||
			!(!typedIsWord || (target.IsCore && !typed.IsCore)) ||
			learnedKeep(w.context.Learned, typed.Core) {
			break
		}
		result = append([]judgement{{slotIndex: index, typed: typed, target: target, confidence: anchor.confidence}}, result...)
	}
	return result
}

func learnedKeep(learned LearnedVocabulary, typed string) bool {
	if learned == nil {
		return false
	}
	decision, ok := learned.Decision(typed)
	return ok && decision.Kind == DecisionKeep
}

// MARK: - Tail

// evaluateTail looks at every recent word, with context from both sides, in two passes: sure
// fixes first, so the weaker calls see the corrected neighbours rather than the garble.
func (d *Detector) evaluateTail(w *world) Action {
	first := max(0, len(w.slots)-w.context.MaxWords)
	assigned := map[int]judgement{}
	for pass := 0; pass < 2; pass++ {
		floor := w.context.SuggestThreshold
		if pass == 0 {
			floor = w.context.AutoThreshold
		}
		for index := first; index < len(w.slots); index++ {
			if _, done := assigned[index]; done {
				continue
			}
			support := func(script Script) int { return d.supportAround(script, index, w, assigned) }
			judged, ok := d.judge(index, w, support)
			if !ok || !(judged.confidence >= floor) {
				continue
			}
			assigned[index] = judged
		}
	}
	keys := make([]int, 0, len(assigned))
	for key := range assigned {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	judgements := make([]judgement, 0, len(keys))
	for _, key := range keys {
		judgements = append(judgements, assigned[key])
	}
	return d.action(judgements, w)
}

// MARK: - Judging one word

func (d *Detector) judge(index int, w *world, support func(Script) int) (judgement, bool) {
	s := w.slots[index]
	skip := d.skipReason(s, w.context)
	if !(skip == SkipNone || (skip == SkipTooShort && w.context.Manual)) || s.typedLayout == nil || s.typedLang == LangNone {
		return judgement{}, false
	}
	context := w.context

	// The whole run first, then only the part the typed layout calls a word: `ghbdtn,` is
	// better read as `привет,` than as `приветб`.
	options := [][2]int{{s.lo, s.hi}}
	if s.hasMainRun && s.mainRun != [2]int{s.lo, s.hi} {
		options = append(options, s.mainRun)
	}
	typedReadings := make([]Reading, len(options))
	for i, option := range options {
		typedReadings[i] = d.scorer.Reading(context.Recorded, option[0], option[1], s.typedLayout, s.typedLang, context.Learned, true)
	}
	var best judgement
	found := false
	for _, lang := range w.languages {
		if lang == s.typedLang {
			continue
		}
		layout := w.layoutFor[lang]
		if layout == nil {
			continue
		}
		for i, option := range options {
			typed := typedReadings[i]
			target := d.scorer.Reading(context.Recorded, option[0], option[1], layout, lang, context.Learned, true)
			var confidence float64
			if context.Manual {
				confidence = manualConfidence(typed, target)
			} else {
				confidence = d.confidence(typed, target, context.MinWordLength, context.Learned, support)
			}
			bestConfidence := 0.0
			if found {
				bestConfidence = best.confidence
			}
			if confidence > bestConfidence {
				best = judgement{slotIndex: index, typed: typed, target: target, confidence: confidence}
				found = true
			}
		}
	}
	return best, found
}

// confidence is the evidence ladder from the type's documentation, as numbers.
func (d *Detector) confidence(typed, target Reading, minLength int, learned LearnedVocabulary,
	support func(Script) int) float64 {
	if !target.IsClean || CanonEqual(target.Text, typed.Text) || typed.Core == "" {
		return 0
	}

	targetSupport := support(target.Lang.Script())
	typedSupport := support(typed.Lang.Script())

	// What the user taught us outranks everything we could infer — except that a conversion
	// learned in one language's company is only offered, not imposed, amid the other's:
	// accepting `tot` → `еще` in a Russian chat says nothing about `a tot of whisky`.
	if learned != nil {
		if decision, ok := learned.Decision(typed.Core); ok {
			switch {
			case decision.Kind == DecisionKeep:
				return 0
			case decision.Kind == DecisionConvert && decision.Lang == target.Lang:
				if typedSupport > targetSupport {
					return ConfidenceOfferOnly
				}
				return 1
			}
		}
	}

	// The text on screen is a word as it stands — unless it only looks like one because a
	// letter of the other layout was read as punctuation around it (`to\` is `ещё`).
	typedIsWord := typed.IsKnown && !typed.HasOddEdges

	if typed.IsNumeric {
		return 0
	}
	// `әғ` → `25`: a number is unambiguous, and too short for any length floor.
	if target.IsNumeric {
		if typedIsWord {
			return 0
		}
		return ConfidenceCertain
	}
	if target.Length < minLength {
		return 0
	}
	// The conversion keeps the product from being fused into the addition (FMA on arm64).
	lean := float64(float64(targetSupport-typedSupport) * contextWeight)
	gap := typed.Cost - target.Cost + lean

	if typedIsWord {
		// Both readings are words — `tot` is English, and `еще` is Russian. Only frequency
		// together with context can argue for a change, and then the user decides. With no
		// context at all there is nothing to go on, so nothing is offered.
		if target.IsKnown && target.IsCore && !typed.IsCore && targetSupport > typedSupport {
			return ConfidenceOfferOnly
		}
		return 0
	}

	// Kazakh typed on the Russian layout: its letters come out as digits and a hyphen in the
	// middle of a Cyrillic word (`с2лем`, `к-к`). That is structure, not statistics.
	cyrillicJunk := typed.Lang.Script() == ScriptCyrillic && target.Lang.Script() == ScriptCyrillic &&
		typed.InteriorJunk > 0 && target.InteriorJunk == 0

	if target.IsKnown && cyrillicJunk {
		return ConfidenceCertain
	}

	if target.IsKnown {
		switch {
		case target.Length >= 4:
			if gap > knownAltTolerance {
				return ConfidenceCertain
			}
			if gap > knownAltTolerance-1 {
				return ConfidenceOfferOnly
			}
			return 0
		case target.Length == 3:
			if target.IsCore {
				if gap >= shortGarbageGap || targetSupport > typedSupport {
					return ConfidenceConfident
				}
				return ConfidenceOfferOnly
			}
			if gap >= shortGarbageGap && targetSupport > typedSupport {
				return ConfidenceOfferOnly
			}
			return 0
		default:
			if target.IsCore && targetSupport > typedSupport {
				return ConfidenceOfferOnly
			}
			return 0
		}
	}

	// Neither reading is a word, but the structural signal still speaks.
	if cyrillicJunk {
		return Confidence(gap)
	}
	// Otherwise only the character model speaks, and it may only offer. Punctuation inside
	// the typed text inflates its cost without making the other reading any more of a word,
	// so when the pieces between it are words — `foo.bar` is a file name — nothing is
	// offered. When they are not (`pfib,bcm` is `зашибись`), the gap stands.
	if target.Length >= 5 && gap >= unknownOfferGap && (typed.InteriorJunk == 0 || !d.hasKnownPieces(typed)) {
		return ConfidenceOfferOnly
	}
	return 0
}

// hasKnownPieces: the letters between the punctuation of a typed word are words themselves.
func (d *Detector) hasKnownPieces(reading Reading) bool {
	// reading.core.split(whereSeparator: { !$0.isLetter && $0 != "'" }), empty pieces omitted.
	var pieces []string
	var piece strings.Builder
	it := graphemeIter{s: reading.Core}
	for c := it.next(); c != ""; c = it.next() {
		if !IsLetterChar(c) && c != "'" {
			if piece.Len() > 0 {
				pieces = append(pieces, piece.String())
				piece.Reset()
			}
			continue
		}
		piece.WriteString(c)
	}
	if piece.Len() > 0 {
		pieces = append(pieces, piece.String())
	}
	for _, p := range pieces {
		if CharCount(p) >= 2 && d.scorer.IsKnown(p, reading.Lang, nil) {
			return true
		}
	}
	return false
}

// manualConfidence: the user says the word is wrong, so the only question is which reading
// is best. Ranked, not thresholded: a known word beats an unknown one, a frequent one beats
// a rare one, and the character model breaks the remaining ties.
func manualConfidence(typed, target Reading) float64 {
	if !(target.IsClean || target.IsNumeric) || CanonEqual(target.Text, typed.Text) {
		return 0
	}
	score := 0.5
	if target.IsKnown {
		score += 0.3
	}
	if target.IsCore {
		score += 0.1
	}
	if !math.IsInf(target.Cost, 0) && !math.IsNaN(target.Cost) {
		score += swiftMax(0, 0.09-target.Cost/100)
	}
	return score
}

// MARK: - Context

// supportBefore counts consecutive words right before index written in script, then the
// text in front of the buffer if the buffer runs out. Capped at three: beyond that more
// context adds nothing.
func (d *Detector) supportBefore(script Script, index int, w *world) int {
	count := 0
	position := index - 1
	for position >= 0 && count < 3 {
		wordScript := w.slots[position].script
		if wordScript == ScriptNone {
			position--
			continue
		}
		if wordScript != script {
			return count
		}
		count++
		position--
	}
	if count >= 3 {
		return count
	}
	for i := len(w.prior.scripts) - 1; i >= 0; i-- {
		if w.prior.scripts[i] != script || count >= 3 {
			break
		}
		count++
	}
	return count
}

// supportAround counts words within two positions on either side, as they will read once
// assigned is applied.
func (d *Detector) supportAround(script Script, index int, w *world, assigned map[int]judgement) int {
	count := 0
	for position := index - 2; position <= index+2; position++ {
		if position == index || position < 0 || position >= len(w.slots) {
			continue
		}
		wordScript := w.slots[position].script
		if judged, ok := assigned[position]; ok {
			wordScript = judged.target.Lang.Script()
		}
		if wordScript != ScriptNone && wordScript == script {
			count++
		}
	}
	if index < 2 {
		scripts := w.prior.scripts
		take := 2 - index
		if take > len(scripts) {
			take = len(scripts)
		}
		for _, wordScript := range scripts[len(scripts)-take:] {
			if wordScript == script {
				count++
			}
		}
	}
	return min(count, 3)
}

// MARK: - Segmentation

func (d *Detector) segment(context Context, layouts []*KeyMapTable) []slot {
	recorded := context.Recorded
	hard := make([]bool, len(recorded))
	typedWord := make([]bool, len(recorded))
	typedTexts := make([]string, len(recorded))
	for index, entry := range recorded {
		hard[index] = true
		typed := context.Resolve(entry.LayoutID)
		if typed == nil {
			continue
		}
		produced, ok := typed.Text(entry.Stroke)
		if !ok || produced == "" {
			continue
		}
		typedTexts[index] = produced
		if typed.ProducesWordCharacter(entry.Stroke) {
			typedWord[index] = true
			hard[index] = false
		} else {
			hard[index] = true
			for _, layout := range layouts {
				if layout.ProducesWordCharacter(entry.Stroke) {
					hard[index] = false
					break
				}
			}
		}
	}

	// Guards speak in the typed layout's own tokens; a slot is off limits if any of its
	// tokens is. Sigils are the exception, handled per slot below: `$PATH` is machine text,
	// but the `\` in `ghbi\k` is the `ё` of `пришёл`, in the middle of a word.
	protected := guardedRanges(typedTexts, typedWord)

	var slots []slot
	index := 0
	for index < len(recorded) {
		if hard[index] {
			index++
			continue
		}
		start := index
		for index < len(recorded) && !hard[index] {
			index++
		}
		lo, hi := start, index

		s := slot{lo: lo, hi: hi}
		s.layoutID, s.hasLayoutID = recorded[lo].LayoutID, true
		for _, entry := range recorded[lo:hi] {
			if entry.LayoutID != s.layoutID {
				s.layoutID, s.hasLayoutID = "", false
				break
			}
		}
		if s.hasLayoutID {
			s.typedLayout = context.Resolve(s.layoutID)
			if s.typedLayout != nil {
				s.typedLang = s.typedLayout.Lang
			}
		}

		// Longest run of typed-layout letters; the later one wins a tie.
		runStart := -1
		for position := lo; position <= hi; position++ {
			isWord := position < hi && typedWord[position]
			if isWord && runStart < 0 {
				runStart = position
			}
			if !isWord && runStart >= 0 {
				length := 0
				if s.hasMainRun {
					length = s.mainRun[1] - s.mainRun[0]
				}
				if length <= position-runStart {
					s.mainRun, s.hasMainRun = [2]int{runStart, position}, true
				}
				runStart = -1
			}
		}

		var screen strings.Builder
		for position := lo; position < hi; position++ {
			screen.WriteString(typedTexts[position])
		}
		s.screen = screen.String()
		if s.hasMainRun {
			var run strings.Builder
			for position := s.mainRun[0]; position < s.mainRun[1]; position++ {
				run.WriteString(typedTexts[position])
			}
			s.mainRunText = run.String()
		}
		s.script = scriptOf(s.screen)

		// A sigil introduces machine text only at the start of a word: right before the slot,
		// or as its first character.
		before := ""
		if start > 0 {
			before = typedTexts[start-1]
		}
		startsWithSigil := sigils[LastChar(before)] || sigils[FirstChar(s.screen)]

		for _, guarded := range protected {
			if rangesOverlap(guarded.span, [2]int{lo, hi}) {
				s.guardReason = guarded.reason
				break
			}
		}
		if s.guardReason == SkipNone && startsWithSigil {
			s.guardReason = SkipSigil
		}
		slots = append(slots, s)
	}
	return slots
}

// skipReason says why a word must be left alone, if it must. Word-level checks run only for
// the words actually being judged — most of the buffer is only context.
func (d *Detector) skipReason(s slot, context Context) SkipReason {
	if s.guardReason != SkipNone {
		return s.guardReason
	}
	core := CoreOf(s.screen)
	if core == "" {
		return SkipTooShort
	}
	if AllChars(core, IsNumberChar) {
		return SkipAllDigits
	}
	if len(context.Exceptions) > 0 {
		if context.Exceptions.Contains(Lower(core)) {
			return SkipException
		}
		if s.hasMainRun && context.Exceptions.Contains(Lower(s.mainRunText)) {
			return SkipException
		}
	}
	// FYI, HTTP, ГОСТ — initialisms are not words the models know.
	var letters strings.Builder
	count := 0
	it := graphemeIter{s: core}
	for c := it.next(); c != ""; c = it.next() {
		if IsLetterChar(c) {
			letters.WriteString(c)
			count++
		}
	}
	text := letters.String()
	if count > 1 && CanonEqual(text, Upper(text)) && !CanonEqual(text, Lower(text)) {
		return SkipAcronym
	}
	return SkipNone
}

type guardedRange struct {
	span   [2]int
	reason SkipReason
}

// guardTriggers are the characters at least one guard pattern needs; see guardedRanges.
var guardTriggers = map[rune]bool{'@': true, ':': true, '.': true, '/': true, '\\': true, '_': true, '(': true}

// guardedRanges are the stroke ranges the guards put off limits: addresses, URLs, paths,
// code.
//
// Every pattern needs a character plain prose rarely has — `@ : . / \ _ (`, or a capital
// after a lowercase letter — so text without one skips the regular expressions entirely.
func guardedRanges(typedTexts []string, typedWord []bool) []guardedRange {
	needsGuards := false
	afterLowercase := false
scan:
	for _, text := range typedTexts {
		for _, r := range text {
			isUpper := r >= 0x41 && r <= 0x5A
			isLower := r >= 0x61 && r <= 0x7A
			isDigit := r >= 0x30 && r <= 0x39
			if guardTriggers[r] || (isUpper && afterLowercase) {
				needsGuards = true
				break scan
			}
			afterLowercase = isLower || (afterLowercase && isDigit)
		}
	}
	if !needsGuards {
		return nil
	}

	// The typed layout's own tokens, built from the texts already rendered.
	var tokens []Token
	for index := 0; index < len(typedTexts); {
		start := index
		isWord := typedWord[index]
		var text strings.Builder
		for index < len(typedTexts) && typedWord[index] == isWord {
			text.WriteString(typedTexts[index])
			index++
		}
		tokens = append(tokens, Token{IsWord: isWord, Lo: start, Hi: index, Displayed: text.String()})
	}
	protected := ProtectedTokens(tokens)
	keys := make([]int, 0, len(protected))
	for key, reason := range protected {
		if reason != SkipSigil {
			keys = append(keys, key)
		}
	}
	sort.Ints(keys)
	result := make([]guardedRange, 0, len(keys))
	for _, key := range keys {
		result = append(result, guardedRange{span: [2]int{tokens[key].Lo, tokens[key].Hi}, reason: protected[key]})
	}
	return result
}

// scriptOf is the script of the first letter. Plain range checks: this runs for every word
// on every evaluation, and the only scripts that matter here are ASCII Latin and Cyrillic.
func scriptOf(text string) Script {
	for _, r := range text {
		if r >= 0x0400 && r <= 0x04FF {
			return ScriptCyrillic
		}
		if (r >= 0x41 && r <= 0x5A) || (r >= 0x61 && r <= 0x7A) {
			return ScriptLatin
		}
	}
	return ScriptNone
}

// MARK: - Building the outcome

func (d *Detector) action(judgements []judgement, w *world) Action {
	none := Action{Kind: ActionNone}
	if len(judgements) == 0 {
		return none
	}
	last := judgements[len(judgements)-1]
	context := w.context

	corrections := make([]WordCorrection, 0, len(judgements)+2)
	for _, judged := range judgements {
		corrections = append(corrections, WordCorrection{Lo: judged.target.Lo, Hi: judged.target.Hi,
			TargetLayoutID: judged.target.LayoutID})
	}
	// Punctuation typed on the same wrong layout right after a corrected word was meant for
	// the target layout too: `ghbdtn^` is `привет,`. Re-rendered only where both sides are
	// punctuation — a letter on either side means it was never meant as a separator — and
	// only up to a word that stays as it is.
	corrected := map[int]bool{}
	for _, judged := range judgements {
		corrected[judged.slotIndex] = true
	}
	for _, judged := range judgements {
		s := w.slots[judged.slotIndex]
		next := judged.slotIndex + 1
		if !(next >= len(w.slots) || corrected[next]) {
			continue
		}
		targetLayout := w.layoutFor[judged.target.Lang]
		if targetLayout == nil {
			continue
		}
		end := len(context.Recorded)
		if next < len(w.slots) {
			end = w.slots[next].lo
		}
		for position := s.hi; position < end; position++ {
			entry := context.Recorded[position]
			if !s.hasLayoutID || entry.LayoutID != s.layoutID {
				continue
			}
			typedTable := context.Resolve(entry.LayoutID)
			if typedTable == nil {
				continue
			}
			typed, ok := typedTable.Text(entry.Stroke)
			target, ok2 := targetLayout.Text(entry.Stroke)
			if !ok || !ok2 || typed == "" || target == "" || CanonEqual(typed, target) ||
				IsWordText(typed) || IsWordText(target) {
				continue
			}
			corrections = append(corrections, WordCorrection{Lo: position, Hi: position + 1,
				TargetLayoutID: targetLayout.SourceID})
		}
	}
	sort.SliceStable(corrections, func(i, j int) bool { return corrections[i].Lo < corrections[j].Lo })

	replacement, ok := PlanReplacement(context.Recorded, corrections, context.Resolve)
	if !ok {
		return none
	}

	confidence := judgements[0].confidence
	for _, judged := range judgements[1:] {
		if judged.confidence < confidence {
			confidence = judged.confidence
		}
	}
	threshold := context.SuggestThreshold
	if context.Manual {
		threshold = 0
	}
	if !(confidence > 0 && confidence >= threshold) {
		return none
	}

	before, after := preview(context.Recorded, corrections, context.Resolve)
	outcome := &Outcome{
		Replacement:    replacement,
		Corrections:    corrections,
		Confidence:     confidence,
		Before:         before,
		After:          after,
		TargetLang:     last.target.Lang,
		TargetLayoutID: last.target.LayoutID,
	}
	for _, judged := range judgements {
		outcome.Changes = append(outcome.Changes, WordChange{Typed: judged.typed.Core, Corrected: judged.target.Core,
			TypedLang: judged.typed.Lang, TargetLang: judged.target.Lang})
		outcome.WordRanges = append(outcome.WordRanges, [2]int{judged.target.Lo, judged.target.Hi})
	}
	if context.Manual || confidence >= context.AutoThreshold {
		return Action{Kind: ActionFix, Outcome: outcome}
	}
	return Action{Kind: ActionSuggest, Outcome: outcome}
}

// preview is the affected span, before and after, for showing the user what would change.
func preview(recorded []RecordedStroke, corrections []WordCorrection, resolve Resolver) (string, string) {
	start, ok1 := minLower(corrections)
	end, ok2 := maxUpper(corrections)
	if !ok1 || !ok2 {
		return "", ""
	}
	if end > len(recorded) {
		end = len(recorded)
	}
	var before, after strings.Builder
	for index := start; index < end; index++ {
		entry := recorded[index]
		asTyped := ""
		if table := resolve(entry.LayoutID); table != nil {
			asTyped, _ = table.Text(entry.Stroke)
		}
		before.WriteString(asTyped)
		if correction, found := correctionAt(corrections, index); found {
			if target := resolve(correction.TargetLayoutID); target != nil {
				if text, ok := target.Text(entry.Stroke); ok {
					after.WriteString(text)
				} else {
					after.WriteString(asTyped)
				}
				continue
			}
		}
		after.WriteString(asTyped)
	}
	return TrimWhitespaces(before.String()), TrimWhitespaces(after.String())
}

// TrimWhitespaces is trimmingCharacters(in: .whitespaces): General Category Zs and tab, but
// not line breaks.
func TrimWhitespaces(text string) string {
	return strings.TrimFunc(text, func(r rune) bool { return r == '\t' || unicode.Is(unicode.Zs, r) })
}

// contextPrior is the scripts of the words just before the buffer, from the text field
// itself.
//
// The buffer is thrown away at every click, Enter and pause, so the first word typed after
// one has no neighbours — exactly when a wrong layout is most likely. The text around the
// caret says what language the user is writing in. Only the scripts of the last few words
// are kept; the text itself is dropped as soon as they are known.
type contextPrior struct {
	scripts []Script
}

func newContextPrior(text string) contextPrior {
	if text == "" {
		return contextPrior{}
	}
	words := splitWords(text)
	if len(words) > 3 {
		words = words[len(words)-3:]
	}
	var scripts []Script
	for _, word := range words {
		if script := scriptOf(word); script != ScriptNone {
			scripts = append(scripts, script)
		}
	}
	return contextPrior{scripts: scripts}
}

// splitWords is text.split(whereSeparator: { !Tokenizer.isWordCharacter($0) }).
func splitWords(text string) []string {
	var words []string
	var current strings.Builder
	it := graphemeIter{s: text}
	for c := it.next(); c != ""; c = it.next() {
		if IsWordCharacter(c) {
			current.WriteString(c)
			continue
		}
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}
	return words
}

package golden

// The regression corpus (Tests/TilqosKitTests/Corpus.swift and CorpusTests.swift):
// the runs dumped to corpus.json, the harness that replays them through core.TypingEngine
// exactly the way the Swift HarnessEngine drives the Swift one, and the CorpusTests tallies
// and quality bar. Shared by the parity test (macOS tables) and the Windows-layout run.

import (
	"fmt"
	"strings"

	"tilqos/internal/core"
)

// CorpusRun is one CorpusTests run: what was typed, with which thresholds and field text,
// and what ended up on screen in Swift. Group names the tally the run belongs to; Kind says
// how the tally reads it (stay, convert, sentence-stay, sentence-convert, mixed).
type CorpusRun struct {
	Group    string     `json:"group"`
	Kind     string     `json:"kind"`
	Word     *string    `json:"word"`
	Expected string     `json:"expected"`
	Segs     []Segment  `json:"segs"`
	Auto     string     `json:"auto"`
	Suggest  string     `json:"suggest"`
	Field    *string    `json:"field"`
	Screen   string     `json:"screen"`
	Events   [][]string `json:"events"`
}

// CorpusHarness is HarnessEngine: drives TypingEngine the way the Windows engine does,
// minus the side effects — every fix is applied to the buffer and switches the active
// layout. Tables are keyed "abc", "ru", "kk".
type CorpusHarness struct {
	Tables     map[string]*core.KeyMapTable
	Resolve    core.Resolver
	Candidates []*core.KeyMapTable
	Engine     *core.TypingEngine
	Active     *core.KeyMapTable
	Learned    core.LearnedVocabulary
	Preceding  string
	Auto       float64
	Suggest    float64
	Events     [][]string
	clock      float64
}

func NewCorpusHarness(models *core.Models, tables map[string]*core.KeyMapTable, resolve core.Resolver) *CorpusHarness {
	return &CorpusHarness{
		Tables: tables, Resolve: resolve,
		Candidates: []*core.KeyMapTable{tables["abc"], tables["ru"], tables["kk"]},
		Engine:     core.NewTypingEngine(core.NewDetector(models), nil),
		Active:     tables["abc"],
		Auto:       core.DefaultAutoThreshold, Suggest: core.DefaultSuggestThreshold,
	}
}

func (h *CorpusHarness) Screen() string { return h.Engine.ScreenText(h.Resolve) }

func (h *CorpusHarness) Backspace() {
	h.clock += 0.08
	h.Engine.Backspace()
}

func (h *CorpusHarness) TypeStroke(stroke core.KeyStroke) {
	h.clock += 0.08
	end, ok := h.Engine.Record(stroke, h.Active, h.clock, h.Candidates)
	if !ok {
		return
	}
	settings := core.NewTypingSettings(h.Candidates, h.Resolve)
	settings.AutoThreshold, settings.SuggestThreshold = h.Auto, h.Suggest
	settings.Learned = h.Learned
	decision := h.Engine.Decide(end, settings, h.Preceding)
	switch decision.Kind {
	case core.DecideApply:
		h.Engine.DidApply(decision.Outcome, true, h.clock)
		h.Events = append(h.Events, []string{"fixed", decision.Outcome.Before, decision.Outcome.After})
		if target := h.Resolve(decision.Outcome.TargetLayoutID); target != nil {
			h.Active = target
		}
	case core.DecideOffer:
		h.Events = append(h.Events, []string{"suggested", decision.Outcome.Before, decision.Outcome.After})
	}
}

// TypeText types text on the keys of layout on, character by character, as the Swift
// simulation does. False when a character cannot be typed there.
func (h *CorpusHarness) TypeText(text, on string) bool {
	table := h.Tables[on]
	for _, character := range core.Graphemes(text) {
		stroke, ok := table.Stroke(character)
		if !ok {
			return false
		}
		h.TypeStroke(stroke)
	}
	return true
}

// Typeable reports whether every segment can be typed on its layout.
func Typeable(tables map[string]*core.KeyMapTable, segments []Segment) bool {
	for _, segment := range segments {
		if _, ok := tables[segment.On].Strokes(segment.Text); !ok {
			return false
		}
	}
	return true
}

// Simulate is CorpusTests.simulate: each segment sets the active layout, then its text is
// typed on the keys of the intended layout.
func (h *CorpusHarness) Simulate(segments []Segment) bool {
	for _, segment := range segments {
		h.Active = h.Tables[segment.Active]
		if !h.TypeText(segment.Text, segment.On) {
			return false
		}
	}
	return true
}

func suggestionCount(events [][]string) int {
	n := 0
	for _, event := range events {
		if event[0] == "suggested" {
			n++
		}
	}
	return n
}

// Tally is one line of the CorpusTests report.
type Tally struct {
	Group, Kind                                     string
	Total, Changed, Suggested, Fixed, Wrong, Missed int
	Examples                                        []string
}

// Add counts one run the way mustStay, mustConvert and the sentence loops do.
func (t *Tally) Add(run CorpusRun, screen string, events [][]string) {
	t.Total++
	switch run.Kind {
	case "stay", "sentence-stay":
		if screen != run.Expected {
			t.Changed++
			t.Examples = append(t.Examples, fmt.Sprintf("%q→%q", run.Expected, screen))
		} else if run.Kind == "stay" && suggestionCount(events) > 0 {
			t.Suggested++
		}
		if run.Kind == "sentence-stay" {
			t.Suggested += suggestionCount(events)
		}
	case "convert":
		word := ""
		if run.Word != nil {
			word = *run.Word
		}
		offered := false
		for _, event := range events {
			if event[0] == "suggested" && (strings.HasPrefix(event[2], word) || event[2] == word) {
				offered = true
			}
		}
		switch {
		case screen == run.Expected:
			t.Fixed++
		case offered:
			t.Suggested++
		case len(events)-suggestionCount(events) > 0:
			t.Wrong++
			t.Examples = append(t.Examples, fmt.Sprintf("%s⇒%s", word, strings.TrimSpace(screen)))
		default:
			t.Missed++
			t.Examples = append(t.Examples, word)
		}
	case "sentence-convert", "mixed":
		if screen == run.Expected {
			t.Fixed++
		} else {
			t.Examples = append(t.Examples, fmt.Sprintf("«%s»→«%s»", run.Expected, screen))
		}
	}
}

func (t *Tally) String() string {
	pct := func(n int) float64 {
		if t.Total == 0 {
			return 0
		}
		return float64(n) / float64(t.Total) * 100
	}
	shown := t.Examples
	if len(shown) > 8 {
		shown = shown[:8]
	}
	examples := ""
	if len(shown) > 0 {
		examples = "  e.g. " + strings.Join(shown, ", ")
	}
	switch t.Kind {
	case "stay":
		return fmt.Sprintf("%s: %d/%d changed (%.1f%%), %d suggested%s", t.Group, t.Changed, t.Total, pct(t.Changed), t.Suggested, examples)
	case "sentence-stay":
		return fmt.Sprintf("%s: %d/%d sentences changed, %d suggestions%s", t.Group, t.Changed, t.Total, t.Suggested, examples)
	case "convert":
		return fmt.Sprintf("%s: fixed %d/%d (%.1f%%), suggested %d (%.1f%%), missed %d, wrong %d%s", t.Group, t.Fixed, t.Total,
			pct(t.Fixed), t.Suggested, pct(t.Suggested), t.Missed, t.Wrong, examples)
	default:
		return fmt.Sprintf("%s: %d/%d right%s", t.Group, t.Fixed, t.Total, examples)
	}
}

// CorpusReport is every tally, in the order CorpusTests reports them.
type CorpusReport struct {
	Order   []string
	Tallies map[string]*Tally
}

func NewCorpusReport() *CorpusReport { return &CorpusReport{Tallies: map[string]*Tally{}} }

func (r *CorpusReport) Add(run CorpusRun, screen string, events [][]string) {
	if r.Tallies[run.Group] == nil {
		r.Tallies[run.Group] = &Tally{Group: run.Group, Kind: run.Kind}
		r.Order = append(r.Order, run.Group)
	}
	r.Tallies[run.Group].Add(run, screen, events)
}

func (r *CorpusReport) String() string {
	lines := make([]string, 0, len(r.Order))
	for _, group := range r.Order {
		lines = append(lines, r.Tallies[group].String())
	}
	return strings.Join(lines, "\n")
}

// Headline is what the README quotes: each correctly typed word once (the "after context"
// repeats aside), the four plain wrong-layout lists, and every wrong-layout sentence.
type Headline struct {
	StayChanged, StayTotal         int
	ConvertFixed, ConvertTotal     int
	SentencesFixed, SentencesTotal int
}

func (h Headline) String() string {
	return fmt.Sprintf("correct words rewritten: %d/%d; wrong-layout words fixed: %d/%d (%.1f%%); "+
		"wrong-layout sentences right: %d/%d", h.StayChanged, h.StayTotal, h.ConvertFixed, h.ConvertTotal,
		float64(h.ConvertFixed)/float64(max(1, h.ConvertTotal))*100, h.SentencesFixed, h.SentencesTotal)
}

var headlineConvert = map[string]bool{
	"report/convert/ru typed on en": true, "report/convert/en typed on ru": true,
	"report/convert/kk typed on ru": true, "report/convert/kk typed on en": true,
}

func (r *CorpusReport) Headline() Headline {
	var h Headline
	for _, group := range r.Order {
		t := r.Tallies[group]
		switch {
		case strings.HasPrefix(group, "report/stay/") && !strings.HasSuffix(group, "after context"):
			h.StayTotal += t.Total
			h.StayChanged += t.Changed
		case headlineConvert[group]:
			h.ConvertTotal += t.Total
			h.ConvertFixed += t.Fixed
		case strings.HasPrefix(group, "report/sentences-wrong/"):
			h.SentencesTotal += t.Total
			h.SentencesFixed += t.Fixed
		}
	}
	return h
}

// Violations holds the report to the CorpusTests quality bar and lists what fails it. The
// bar is asymmetric on purpose: a correctly typed word must never be rewritten; misses are
// allowed, within limits. strictRates applies the catch-rate floors as well, which only the
// macOS tables are calibrated for.
func (r *CorpusReport) Violations(strictRates bool) []string {
	var failures []string
	fail := func(format string, args ...any) { failures = append(failures, fmt.Sprintf(format, args...)) }
	firstSix := map[string]bool{
		"report/convert/ru typed on en": true, "report/convert/en typed on ru": true,
		"report/convert/kk typed on ru": true, "report/convert/kk typed on en": true,
		"report/convert/ru typed on en after ru context": true, "report/convert/en typed on ru after en context": true,
	}
	staySuggested := 0
	for _, group := range r.Order {
		t := r.Tallies[group]
		switch {
		case strings.HasPrefix(group, "report/stay/"):
			if t.Changed != 0 {
				fail("correct words were rewritten: %s", t)
			}
			staySuggested += t.Suggested
		case strings.HasPrefix(group, "report/sentences-correct/"):
			if t.Changed != 0 || t.Suggested != 0 {
				fail("sentences typed correctly were changed or drew offers: %s", t)
			}
		case strings.HasPrefix(group, "report/convert/"):
			if t.Wrong != 0 {
				fail("wrong conversions: %s", t)
			}
			if strictRates && firstSix[group] && float64(t.Fixed+t.Suggested)/float64(max(1, t.Total)) < 0.95 {
				fail("too many misses: %s", t)
			}
		case strings.HasPrefix(group, "report/sentences-wrong/"):
			if strictRates && float64(t.Fixed)/float64(max(1, t.Total)) < 0.95 {
				fail("wrong-layout sentences: %s", t)
			}
		case group == "report/mixed":
			if strictRates && t.Fixed != t.Total {
				fail("mixed and context cases: %s", t)
			}
		case strings.HasPrefix(group, "context/") && t.Kind == "stay":
			if t.Changed != 0 {
				fail("context rewrote correct words: %s", t)
			}
		}
	}
	if staySuggested > 2 {
		fail("too many offers on correct words: %d", staySuggested)
	}
	if strictRates {
		alone := r.Tallies["context/short ru typed on en, empty field"]
		withRussian := r.Tallies["context/short ru typed on en, Russian text before"]
		if alone == nil || withRussian == nil || withRussian.Fixed < alone.Fixed || withRussian.Wrong != 0 {
			fail("context from the text field: alone %v, with Russian %v", alone, withRussian)
		}
	}
	for _, preset := range []string{"cautious", "balanced"} {
		changed := 0
		for _, group := range r.Order {
			if strings.HasPrefix(group, "presets/"+preset+"/stay/") {
				changed += r.Tallies[group].Changed
			}
		}
		if changed != 0 {
			fail("preset %s rewrote %d correct words", preset, changed)
		}
	}
	if h := r.Headline(); h.StayChanged != 0 {
		fail("%d correct words rewritten", h.StayChanged)
	}
	return failures
}

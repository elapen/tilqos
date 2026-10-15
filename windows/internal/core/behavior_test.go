package core_test

// The Swift behaviour tests (DetectorTests, TokenizerTests, ReplacementPlannerTests,
// TrigramModelTests), ported one for one. They run on the macOS tables from the golden
// dump, so they assert the same facts the Swift suite does.

import (
	"bytes"
	"math"
	"testing"
	"time"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

type session struct {
	t        *testing.T
	tables   map[string]*core.KeyMapTable
	recorded []core.RecordedStroke
	clock    float64
}

func (s *session) typeOn(text, intendedOn, active string) {
	s.t.Helper()
	strokes, ok := s.tables[intendedOn].Strokes(text)
	if !ok {
		s.t.Fatalf("%s cannot type %q", intendedOn, text)
	}
	for _, stroke := range strokes {
		s.clock += 0.05
		s.recorded = append(s.recorded, core.RecordedStroke{Stroke: stroke, Time: s.clock, LayoutID: s.tables[active].SourceID})
	}
}

func (s *session) typeCorrectly(text, layout string) { s.typeOn(text, layout, layout) }

type fixture struct {
	t        *testing.T
	tables   map[string]*core.KeyMapTable
	resolve  core.Resolver
	models   *core.Models
	detector *core.Detector
}

func newFixture(t *testing.T) *fixture {
	tables, resolve, _ := golden.Tables(t)
	models := golden.Models(t)
	return &fixture{t: t, tables: tables, resolve: resolve, models: models, detector: core.NewDetector(models)}
}

func (f *fixture) newSession() *session { return &session{t: f.t, tables: f.tables} }

func (f *fixture) evaluate(s *session, scope core.Scope, exceptions ...string) core.Action {
	context := core.NewContext(s.recorded, []*core.KeyMapTable{f.tables["abc"], f.tables["ru"], f.tables["kk"]}, f.resolve)
	context.Exceptions = core.NewStringSet(exceptions...)
	return f.detector.Evaluate(context, scope)
}

// MARK: - Latin typed where Cyrillic was meant

func TestGhbdtnBecomesPrivet(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("привет", "ru", "abc")
	s.typeCorrectly(" ", "abc")
	action := f.evaluate(s, core.ScopeTail)
	if action.Outcome == nil {
		t.Fatal("expected a correction")
	}
	o := action.Outcome
	if o.Before != "ghbdtn" || o.After != "привет" || o.TargetLang != core.LangRU || o.Replacement.InsertText != "привет " {
		t.Fatalf("got %+v", o)
	}
}

func TestCyrillicTypedWhereLatinWasMeant(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("keyboard", "abc", "ru")
	s.typeCorrectly(" ", "ru")
	action := f.evaluate(s, core.ScopeTail)
	if action.Outcome == nil || action.Outcome.After != "keyboard" || action.Outcome.TargetLang != core.LangEN {
		t.Fatalf("got %+v", action)
	}
}

// MARK: - Kazakh typed on the Russian layout

func TestKazakhGreetingWithDigitsIsCorrected(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("сәлеметсіз", "kk", "ru")
	s.typeCorrectly(" ", "ru")
	action := f.evaluate(s, core.ScopeTail)
	if action.Outcome == nil || action.Outcome.Before != "с2леметс3з" || action.Outcome.After != "сәлеметсіз" ||
		action.Outcome.TargetLang != core.LangKK {
		t.Fatalf("got %+v", action.Outcome)
	}
}

func TestQazaqWithDigitsIsCorrected(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("қазақ", "kk", "ru")
	s.typeCorrectly(" ", "ru")
	action := f.evaluate(s, core.ScopeTail)
	if action.Outcome == nil || action.Outcome.Before != "0аза0" || action.Outcome.After != "қазақ" {
		t.Fatalf("got %+v", action.Outcome)
	}
}

// The reverse mistake, and the reason numbers need their own cost.
func TestNumberTypedOnKazakhLayoutIsRestored(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeCorrectly("мне ", "ru")
	s.typeOn("25", "abc", "kk")
	s.typeCorrectly(" лет ", "ru")
	action := f.evaluate(s, core.ScopeTail)
	if action.Outcome == nil || action.Outcome.Before != "әғ" || action.Outcome.After != "25" {
		t.Fatalf("expected әғ to be read as 25, got %+v", action.Outcome)
	}
}

// MARK: - Cases that must be left alone

func TestRussianWordTypedOnKazakhLayoutIsLeftAlone(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("Астана ", "ru", "kk")
	if action := f.evaluate(s, core.ScopeTail); action.Kind != core.ActionNone {
		t.Fatalf("must not touch text that is already correct: %+v", action.Outcome)
	}
}

func TestCorrectlyTypedTextIsLeftAlone(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct{ text, layout string }{
		{"привет мир ", "ru"}, {"hello world ", "abc"}, {"сәлеметсіз бе ", "kk"},
	} {
		s := f.newSession()
		s.typeCorrectly(c.text, c.layout)
		if action := f.evaluate(s, core.ScopeTail); action.Kind != core.ActionNone {
			t.Errorf("%q (%s) was typed correctly and must not change: %+v", c.text, c.layout, action.Outcome)
		}
	}
}

func TestShortWordsAreLeftAlone(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct{ word, layout string }{{"ok ", "abc"}, {"да ", "ru"}, {"ия ", "ru"}} {
		s := f.newSession()
		s.typeCorrectly(c.word, c.layout)
		if action := f.evaluate(s, core.ScopeTail); action.Kind != core.ActionNone {
			t.Errorf("%q is too short to judge", c.word)
		}
	}
}

func TestMachineTextIsLeftAlone(t *testing.T) {
	f := newFixture(t)
	for _, text := range []string{
		"user@example.com ", "https://github.com/taem/hunspell-kk ",
		"let user_name = 1 ", "getUserName() ", "$PATH ",
	} {
		s := f.newSession()
		s.typeCorrectly(text, "abc")
		if action := f.evaluate(s, core.ScopeTail); action.Kind != core.ActionNone {
			t.Errorf("%q must never be rewritten: %+v", text, action.Outcome)
		}
	}
}

func TestWordExceptionsAreHonoured(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("привет", "ru", "abc")
	s.typeCorrectly(" ", "abc")
	if f.evaluate(s, core.ScopeTail).Outcome == nil {
		t.Fatal("baseline: this is normally corrected")
	}
	if action := f.evaluate(s, core.ScopeTail, "ghbdtn"); action.Kind != core.ActionNone {
		t.Fatal("an excepted word must be left alone")
	}
}

// MARK: - Mixed languages

func TestIsolatedEnglishWordInsideKazakhTextSurvives(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeCorrectly("менің ", "kk")
	s.typeCorrectly("iPhone", "abc")
	s.typeCorrectly(" телефоным ", "kk")
	if action := f.evaluate(s, core.ScopeTail); action.Kind != core.ActionNone {
		t.Fatalf("a deliberate foreign word must not be flipped: %+v", action.Outcome)
	}
}

func TestOnlyMistypedWordsInAMixedParagraphAreCorrected(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("привет как дела", "ru", "abc")
	s.typeCorrectly(" iPhone ", "abc")
	action := f.evaluate(s, core.ScopeTail)
	if action.Outcome == nil {
		t.Fatal("expected a correction")
	}
	if !bytes.Contains([]byte(action.Outcome.After), []byte("привет как дела")) ||
		bytes.Contains([]byte(action.Outcome.After), []byte("шЗрщту")) || action.Outcome.TargetLang != core.LangRU {
		t.Fatalf("got %q", action.Outcome.After)
	}
}

func TestLastWordScopeOnlyLooksAtTheClosingWord(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeCorrectly("hello ", "abc")
	s.typeOn("привет", "ru", "abc")
	s.typeCorrectly(" ", "abc")
	action := f.evaluate(s, core.ScopeLastWord)
	if action.Outcome == nil || action.Outcome.Before != "ghbdtn" || len(action.Outcome.Corrections) != 1 {
		t.Fatalf("got %+v", action.Outcome)
	}
}

// MARK: - Confidence

func TestConfidenceMapping(t *testing.T) {
	if core.Confidence(0) != 0 || core.Confidence(0.4) != 0 || !(core.Confidence(2.6) > 0.95) ||
		core.Confidence(100) != 1 || core.Confidence(math.Inf(1)) != 1 || core.Confidence(math.NaN()) != 0 {
		t.Fatal("confidence mapping changed")
	}
}

func TestRealMistakesClearTheAutoThreshold(t *testing.T) {
	f := newFixture(t)
	for _, c := range []struct{ intended, on string }{
		{"привет", "ru"}, {"клавиатура", "ru"}, {"keyboard", "abc"}, {"сәлеметсіз", "kk"},
	} {
		wrong := "abc"
		if c.on == "abc" {
			wrong = "ru"
		}
		s := f.newSession()
		s.typeOn(c.intended, c.on, wrong)
		s.typeCorrectly(" ", wrong)
		action := f.evaluate(s, core.ScopeLastWord)
		if action.Kind != core.ActionFix || action.Outcome.After != c.intended {
			t.Errorf("%q should be a confident fix, got %s %+v", c.intended, action.Kind, action.Outcome)
		}
	}
}

// MARK: - Degenerate input

func TestEmptyBufferProducesNothing(t *testing.T) {
	f := newFixture(t)
	if action := f.evaluate(f.newSession(), core.ScopeTail); action.Kind != core.ActionNone {
		t.Fatal("nothing typed, nothing to do")
	}
}

func TestSingleLayoutInstalledProducesNothing(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("привет", "ru", "abc")
	context := core.NewContext(s.recorded, []*core.KeyMapTable{f.tables["abc"]}, f.resolve)
	if action := f.detector.Evaluate(context, core.ScopeTail); action.Kind != core.ActionNone {
		t.Fatal("with one layout there is nothing to switch to")
	}
}

// MARK: - Tokenizer and buffer

func TestDigitsStayInsideAWord(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("сәлеметсіз", "kk", "ru")
	tokens := core.Tokenize(s.recorded, f.resolve)
	if len(tokens) != 1 || tokens[0].Displayed != "с2леметс3з" || !tokens[0].IsWord {
		t.Fatalf("got %+v", tokens)
	}
}

func TestIdleResetDiscardsStaleStrokes(t *testing.T) {
	buffer := core.NewStrokeBuffer(64, 5)
	buffer.Append(core.KeyStroke{Key: 0}, 0, "x")
	if buffer.Append(core.KeyStroke{Key: 1}, 1, "x") || buffer.Count() != 2 {
		t.Fatal("no reset expected within the idle window")
	}
	// A six second gap means whatever was buffered is no longer in front of the user.
	if !buffer.Append(core.KeyStroke{Key: 2}, 7, "x") || buffer.Count() != 1 {
		t.Fatal("expected an idle reset")
	}
}

func TestBufferNeverGrowsPastCapacity(t *testing.T) {
	buffer := core.NewStrokeBuffer(8, 5)
	for index := 0; index < 40; index++ {
		buffer.Append(core.KeyStroke{Key: uint16(index % 30)}, float64(index)*0.05, "x")
	}
	if buffer.Count() != 8 || buffer.Recorded()[7].Stroke.Key != 39%30 {
		t.Fatalf("count %d", buffer.Count())
	}
}

func TestBackspaceMirrorsIntoTheBuffer(t *testing.T) {
	buffer := core.NewStrokeBuffer(0, 0)
	buffer.Append(core.KeyStroke{Key: 0}, 0, "x")
	buffer.Append(core.KeyStroke{Key: 1}, 0.1, "x")
	buffer.RemoveLast()
	if buffer.Count() != 1 {
		t.Fatal("backspace not mirrored")
	}
	buffer.Reset()
	buffer.RemoveLast() // must not panic on an empty buffer
	if !buffer.IsEmpty() {
		t.Fatal("reset failed")
	}
}

// MARK: - Replacement planner

func mistyped(t *testing.T, tables map[string]*core.KeyMapTable, intended, typedOn, active string) []core.RecordedStroke {
	strokes, ok := tables[typedOn].Strokes(intended)
	if !ok {
		t.Fatalf("%s cannot type %q", typedOn, intended)
	}
	out := make([]core.RecordedStroke, len(strokes))
	for i, stroke := range strokes {
		out[i] = core.RecordedStroke{Stroke: stroke, Time: float64(i) * 0.05, LayoutID: tables[active].SourceID}
	}
	return out
}

func TestKazakhCorrectionPreservesTheCommonPrefix(t *testing.T) {
	tables, resolve, _ := golden.Tables(t)
	recorded := mistyped(t, tables, "сәлеметсіз", "kk", "ru")
	plan, ok := core.PlanReplacement(recorded, []core.WordCorrection{{Lo: 0, Hi: len(recorded), TargetLayoutID: tables["kk"].SourceID}}, resolve)
	if !ok || plan.DeletedText != "2леметс3з" || plan.InsertText != "әлеметсіз" || plan.DeleteCount() != 9 {
		t.Fatalf("got %+v", plan)
	}
}

func TestTrailingPunctuationIsNotReRendered(t *testing.T) {
	tables, resolve, _ := golden.Tables(t)
	recorded := mistyped(t, tables, "привет", "ru", "abc")
	wordLength := len(recorded)
	recorded = append(recorded, mistyped(t, tables, ".", "abc", "abc")...)
	plan, ok := core.PlanReplacement(recorded, []core.WordCorrection{{Lo: 0, Hi: wordLength, TargetLayoutID: tables["ru"].SourceID}}, resolve)
	if !ok || plan.DeletedText != "ghbdtn." || plan.InsertText != "привет." {
		t.Fatalf("got %+v", plan)
	}
}

func TestNoChangeYieldsNoPlan(t *testing.T) {
	tables, resolve, _ := golden.Tables(t)
	recorded := mistyped(t, tables, "Астана", "ru", "ru")
	if _, ok := core.PlanReplacement(recorded, []core.WordCorrection{{Lo: 0, Hi: len(recorded), TargetLayoutID: tables["kk"].SourceID}}, resolve); ok {
		t.Fatal("Russian letters render identically on the Kazakh layout; no edit expected")
	}
	if _, ok := core.PlanReplacement(nil, nil, resolve); ok {
		t.Fatal("empty input yields no plan")
	}
}

func TestInverseRestoresTheOriginal(t *testing.T) {
	plan := core.Replacement{DeletedText: "ghbdtn", InsertText: "привет"}
	undo := plan.Inverse()
	if undo.DeletedText != "привет" || undo.InsertText != "ghbdtn" || undo.Inverse() != plan {
		t.Fatal("inverse broken")
	}
}

func TestRewrittenTailAdoptsTheTargetLayout(t *testing.T) {
	tables, _, _ := golden.Tables(t)
	recorded := mistyped(t, tables, "привет", "ru", "abc")
	tail := core.RewrittenTail(recorded, []core.WordCorrection{{Lo: 0, Hi: len(recorded), TargetLayoutID: tables["ru"].SourceID}}, 0)
	if len(tail) != len(recorded) {
		t.Fatal("length changed")
	}
	for i := range tail {
		if tail[i].LayoutID != tables["ru"].SourceID || tail[i].Stroke != recorded[i].Stroke {
			t.Fatal("tail must keep the keys and adopt the target layout")
		}
	}
}

// MARK: - Model files

func TestModelSurvivesEncodeDecode(t *testing.T) {
	models := golden.Models(t)
	original := models.Model(core.LangKK)
	restored, err := core.DecodeTrigramModel(original.Encode())
	if err != nil {
		t.Fatal(err)
	}
	a, _ := original.Score("қазақ")
	b, _ := restored.Score("қазақ")
	if restored.Lang != core.LangKK || restored.Alphabet.Kind != core.AlphabetCyrillic || a != b {
		t.Fatal("round trip changed the model")
	}
	files := golden.ModelFiles(t)
	if !bytes.Equal(original.Encode(), files["kk.bin"]) {
		t.Fatal("re-encoding kk.bin must reproduce the file byte for byte")
	}
	morphology := models.KazakhMorphology()
	if !bytes.Equal(morphology.Encode(), files["kk-morph.bin"]) {
		t.Fatal("re-encoding kk-morph.bin must reproduce the file byte for byte")
	}
}

func TestGarbageIsRejected(t *testing.T) {
	if _, err := core.DecodeTrigramModel([]byte{1, 2, 3}); err == nil {
		t.Fatal("short data accepted")
	}
	if _, err := core.DecodeTrigramModel(make([]byte, 40)); err == nil {
		t.Fatal("zeroes accepted")
	}
	if _, err := core.DecodeMorphology(make([]byte, 40)); err == nil {
		t.Fatal("zeroes accepted as morphology")
	}
	if _, err := core.LoadModels(map[string][]byte{}); err == nil {
		t.Fatal("missing models accepted")
	}
}

func TestRealWordsBeatLayoutGarble(t *testing.T) {
	models := golden.Models(t)
	check := func(lang core.Lang, real, garble []string) {
		worstReal, bestGarble := -1.0, math.Inf(1)
		for _, word := range real {
			score, _ := models.Model(lang).Score(word)
			worstReal = math.Max(worstReal, score.PenalizedCost())
		}
		for _, word := range garble {
			score, _ := models.Model(lang).Score(word)
			bestGarble = math.Min(bestGarble, score.PenalizedCost())
		}
		if !(worstReal < bestGarble) {
			t.Errorf("%s: real words up to %.2f, garble from %.2f", lang, worstReal, bestGarble)
		}
	}
	check(core.LangEN, []string{"hello", "world", "keyboard", "language"}, []string{"ghbdtn", "hjccbz", "gjkyjcnm", "ktdjuj"})
	check(core.LangRU, []string{"привет", "клавиатура", "переключатель", "спасибо"}, []string{"руддщ", "цщкдв", "здефну", "ыцйксд"})
	check(core.LangKK, []string{"сәлеметсіз", "қазақстан", "рақмет", "мүмкіндік"}, []string{"руддщ", "цщкдв", "ыцйксд"})
}

// MARK: - Undo stack

func TestUndoStackIsBoundedAndFresh(t *testing.T) {
	stack := core.NewUndoStack(3)
	now := time.Now()
	for i := 0; i < 5; i++ {
		stack.Push(core.AppliedFix{OriginalWord: string(rune('a' + i)), Time: now})
	}
	if stack.Count() != 3 {
		t.Fatalf("count %d", stack.Count())
	}
	if last, _ := stack.Last(); last.OriginalWord != "e" {
		t.Fatal("newest must be last")
	}
	if !stack.LastIsFresh(core.DefaultFreshWindow, now.Add(5*time.Second)) ||
		stack.LastIsFresh(core.DefaultFreshWindow, now.Add(9*time.Second)) {
		t.Fatal("freshness window broken")
	}
	fix, _ := stack.PopLast()
	if fix.OriginalWord != "e" || stack.Count() != 2 {
		t.Fatal("pop broken")
	}
	stack.Clear()
	if _, ok := stack.PopLast(); ok {
		t.Fatal("clear broken")
	}
}

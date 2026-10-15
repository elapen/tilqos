package core_test

// The tests the detection rewrite added to DetectorTests.swift, and LearnerTests.swift,
// ported one for one on the macOS tables from the golden dump.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tilqos/internal/core"
)

// MARK: - Real words are never rewritten (DetectorTests)

// The reported bug: `еще` typed correctly became `tot`, because `tot` is English too.
func TestCorrectRussianWordIsNeverRewritten(t *testing.T) {
	f := newFixture(t)
	for _, word := range []string{"еще ", "ещё ", "где ", "тот ", "тут ", "рук "} {
		s := f.newSession()
		s.typeCorrectly(word, "ru")
		if action := f.evaluate(s, core.ScopeLastWord); action.Kind != core.ActionNone {
			t.Errorf("%q was typed correctly and must not change: %s", word, describeAction(action))
		}
	}
}

func TestCorrectEnglishWordIsNeverRewritten(t *testing.T) {
	f := newFixture(t)
	for _, word := range []string{"tot ", "her ", "the ", "afk ", "lol "} {
		s := f.newSession()
		s.typeCorrectly(word, "abc")
		if action := f.evaluate(s, core.ScopeLastWord); action.Kind != core.ActionNone {
			t.Errorf("%q was typed correctly and must not change: %s", word, describeAction(action))
		}
	}
}

// On ABC `,` `[` `;` are punctuation, but on the Russian layout they are б, х, ж — so a
// Russian word typed on ABC must not be cut at them.
func TestPunctuationKeysStayInsideTheWord(t *testing.T) {
	f := newFixture(t)
	for _, word := range []string{"было", "хорошо", "тебе", "тоже", "люблю", "ещё"} {
		s := f.newSession()
		s.typeOn(word, "ru", "abc")
		s.typeCorrectly(" ", "abc")
		action := f.evaluate(s, core.ScopeLastWord)
		if action.Outcome == nil || action.Outcome.After != word {
			t.Errorf("%s: %s", word, describeAction(action))
		}
	}
}

func TestOnlySeparatorsOfEveryLayoutEndAWord(t *testing.T) {
	f := newFixture(t)
	layouts := []*core.KeyMapTable{f.tables["abc"], f.tables["ru"], f.tables["kk"]}
	stroke := func(text, layout string) core.KeyStroke {
		s, ok := f.tables[layout].Stroke(text)
		if !ok {
			t.Fatalf("%s cannot type %q", layout, text)
		}
		return s
	}
	if !core.EndsWord(stroke(" ", "abc"), f.tables["abc"], layouts) {
		t.Error("space must end a word")
	}
	// б, х, ю on the Russian layout.
	for _, key := range []string{",", "[", "."} {
		if core.EndsWord(stroke(key, "abc"), f.tables["abc"], layouts) {
			t.Errorf("%q is a letter on the Russian layout and must not end a word", key)
		}
	}
	// The Russian comma is Shift+6, which nothing else reads as a letter.
	if !core.EndsWord(stroke(",", "ru"), f.tables["ru"], layouts) {
		t.Error("the Russian comma must end a word")
	}
}

// `^` is where the Russian comma lives: typed on the same wrong layout right after a garbled
// word, it was meant as a comma.
func TestGarbledPunctuationAfterAFixComesAlong(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("привет,", "ru", "abc")
	action := f.evaluate(s, core.ScopeLastWord)
	if action.Outcome == nil || action.Outcome.Replacement.InsertText != "привет," {
		t.Fatalf("got %s", describeAction(action))
	}
}

// A sure fix pulls in the short garble before it, which alone was too short to touch.
func TestConfidentFixPullsInShortNeighbours(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("я тебя", "ru", "abc")
	s.typeCorrectly(" ", "abc")
	action := f.evaluate(s, core.ScopeLastWord)
	if action.Outcome == nil || action.Outcome.After != "я тебя" {
		t.Fatalf("got %s", describeAction(action))
	}
	var typed []string
	for _, change := range action.Outcome.Changes {
		typed = append(typed, change.Typed)
	}
	if strings.Join(typed, "|") != "z|nt,z" {
		t.Fatalf("changes %q", typed)
	}
}

// A word that is a word on both layouts is left alone without context, and only offered
// with it.
func TestTwoSidedWordNeedsContext(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeOn("где", "ru", "abc")
	s.typeCorrectly(" ", "abc")
	if action := f.evaluate(s, core.ScopeLastWord); action.Kind != core.ActionNone {
		t.Fatalf("`ult` is an English word too; without context it must stay: %s", describeAction(action))
	}
	context := core.NewContext(s.recorded, []*core.KeyMapTable{f.tables["abc"], f.tables["ru"], f.tables["kk"]}, f.resolve)
	context.PrecedingText = "мы вчера говорили,"
	action := f.detector.Evaluate(context, core.ScopeLastWord)
	if action.Kind != core.ActionSuggest || action.Outcome.After != "где" {
		t.Fatalf("with Russian text before it, `где` should be offered: %s", describeAction(action))
	}
}

func TestBrandInsideRussianTextStays(t *testing.T) {
	f := newFixture(t)
	s := f.newSession()
	s.typeCorrectly("скачай ", "ru")
	s.typeCorrectly("Telegram ", "abc")
	if action := f.evaluate(s, core.ScopeLastWord); action.Kind != core.ActionNone {
		t.Fatalf("a known Latin word inside Russian text is deliberate: %s", describeAction(action))
	}
}

// taught is DetectorTests.Taught.
type taught struct {
	decisions map[string]core.LearnedDecision
	version   int
}

func (v *taught) Version() int                   { return v.version }
func (v *taught) IsKnown(string, core.Lang) bool { return false }
func (v *taught) Decision(typed string) (core.LearnedDecision, bool) {
	decision, ok := v.decisions[core.Lower(typed)]
	return decision, ok
}

func TestLearnedDecisionsOutrankInference(t *testing.T) {
	f := newFixture(t)
	layouts := []*core.KeyMapTable{f.tables["abc"], f.tables["ru"], f.tables["kk"]}
	learned := &taught{decisions: map[string]core.LearnedDecision{"ghbdtn": {Kind: core.DecisionKeep}}}

	s := f.newSession()
	s.typeOn("привет", "ru", "abc")
	s.typeCorrectly(" ", "abc")
	context := core.NewContext(s.recorded, layouts, f.resolve)
	context.Learned = learned
	if action := f.detector.Evaluate(context, core.ScopeLastWord); action.Kind != core.ActionNone {
		t.Fatalf("a word the user keeps must stay: %s", describeAction(action))
	}

	// Converting a real word only because the user asked for it before.
	tot := f.newSession()
	tot.typeCorrectly("tot ", "abc")
	totContext := core.NewContext(tot.recorded, layouts, f.resolve)
	totContext.Learned = learned
	learned.decisions = map[string]core.LearnedDecision{"tot": {Kind: core.DecisionConvert, Lang: core.LangRU}}
	learned.version++
	action := f.detector.Evaluate(totContext, core.ScopeLastWord)
	if action.Kind != core.ActionFix || action.Outcome.After != "еще" {
		t.Fatalf("an accepted conversion is applied next time: %s", describeAction(action))
	}
}

// MARK: - The learner store (LearnerTests)

func TestIgnoredTwiceMeansKeep(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Ignored("nen")
	if _, ok := learner.Decision("nen"); ok {
		t.Fatal("one ignored offer proves nothing")
	}
	learner.Ignored("nen")
	if decision, ok := learner.Decision("nen"); !ok || decision.Kind != core.DecisionKeep {
		t.Fatalf("decision %+v %v", decision, ok)
	}
}

func TestAcceptanceOutranksPassiveLearning(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Ignored("nen")
	learner.Ignored("nen")
	learner.Confirmed("nen", core.LangRU)
	if decision, _ := learner.Decision("nen"); decision != (core.LearnedDecision{Kind: core.DecisionConvert, Lang: core.LangRU}) {
		t.Fatalf("after confirming: %+v", decision)
	}
	learner.Rejected("nen")
	if decision, _ := learner.Decision("nen"); decision.Kind != core.DecisionKeep {
		t.Fatalf("after rejecting: %+v", decision)
	}
}

func TestVocabularyNeedsTwoSightings(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Kept("Айдар", core.LangRU)
	if learner.IsKnown("айдар", core.LangRU) {
		t.Fatal("one sighting is not enough")
	}
	learner.Kept("айдар", core.LangRU)
	if !learner.IsKnown("АЙДАР", core.LangRU) {
		t.Fatal("case and ё do not make a new word")
	}
	if learner.IsKnown("айдар", core.LangEN) {
		t.Fatal("a word of one language only")
	}
	if decision, _ := learner.Decision("айдар"); decision.Kind != core.DecisionKeep {
		t.Fatalf("a user word is kept: %+v", decision)
	}
}

func TestJunkIsNotLearnedAsVocabulary(t *testing.T) {
	learner := core.NewLearner(nil)
	for i := 0; i < 3; i++ {
		learner.Kept("ab", core.LangEN)
		learner.Kept("x1y2", core.LangEN)
	}
	if total := learner.Counts().Total(); total != 0 {
		t.Fatalf("learned %d entries", total)
	}
}

func TestLearnerPersistsWithoutPlaintext(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Confirmed("ghbdtn", core.LangRU)
	learner.Kept("Айдар", core.LangRU)
	learner.Kept("Айдар", core.LangRU)
	data, err := learner.Encode()
	if err != nil {
		t.Fatal(err)
	}
	dump := strings.ToLower(string(data))
	if strings.Contains(dump, "ghbdtn") || strings.Contains(dump, "айдар") {
		t.Fatalf("plaintext in the stored data: %s", data)
	}
	reloaded, err := core.DecodeLearner(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision, _ := reloaded.Decision("ghbdtn"); decision != (core.LearnedDecision{Kind: core.DecisionConvert, Lang: core.LangRU}) {
		t.Fatalf("reloaded decision %+v", decision)
	}
	if !reloaded.IsKnown("айдар", core.LangRU) {
		t.Fatal("reloaded vocabulary")
	}
	if counts := reloaded.Counts(); counts != (core.LearnerCounts{AlwaysConvert: 1, UserWords: 1}) {
		t.Fatalf("counts %+v", counts)
	}
}

func TestLearnerForgetting(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Confirmed("ghbdtn", core.LangRU)
	learner.Rejected("tot")
	if n := len(learner.Items()); n != 2 {
		t.Fatalf("%d items", n)
	}
	if !learner.ForgetWord("GHBDTN") {
		t.Fatal("forget by word")
	}
	if _, ok := learner.Decision("ghbdtn"); ok {
		t.Fatal("still decided")
	}
	if learner.ForgetWord("ghbdtn") {
		t.Fatal("forgot twice")
	}
	learner.Forget(learner.Items()[0].ID)
	if total := learner.Counts().Total(); total != 0 {
		t.Fatalf("%d left", total)
	}
	learner.Confirmed("ghbdtn", core.LangRU)
	before := learner.Hash("ghbdtn")
	learner.ForgetAll()
	if learner.Counts().Total() != 0 || learner.Hash("ghbdtn") == before {
		t.Fatal("a wipe clears everything and changes the salt")
	}
}

func TestDisabledLearnerNeitherLearnsNorAnswers(t *testing.T) {
	learner := core.NewLearner(nil)
	learner.Confirmed("ghbdtn", core.LangRU)
	learner.SetEnabled(false)
	if _, ok := learner.Decision("ghbdtn"); ok {
		t.Fatal("answers while off")
	}
	learner.Rejected("tot")
	learner.SetEnabled(true)
	if _, ok := learner.Decision("tot"); ok {
		t.Fatal("nothing is learned while switched off")
	}
	if decision, _ := learner.Decision("ghbdtn"); decision.Kind != core.DecisionConvert {
		t.Fatal("what was learned before survives")
	}
}

func TestLearnerCapacityEvictsTheOldest(t *testing.T) {
	day := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	learner := core.NewLearner(func() time.Time { return day })
	learner.Rejected("oldest")
	day = day.Add(10 * 24 * time.Hour)
	for i := 0; i < core.LearnerCapacity; i++ {
		learner.Rejected("word" + itoa(i))
	}
	if total := learner.Counts().Total(); total != core.LearnerCapacity {
		t.Fatalf("%d entries", total)
	}
	if _, ok := learner.Decision("oldest"); ok {
		t.Fatal("the oldest entry must go first")
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for ; i > 0; i /= 10 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
	}
	return string(digits)
}

// MARK: - Learning through the engine (LearnerTests)

func newLearningHarness(t *testing.T, f *fixture, learner *core.Learner) *harness {
	h := newHarness(f.models, f.tables, f.resolve)
	h.Learned = learner
	h.Engine.Learner = learner
	h.Engine.Resolve = f.resolve
	detector := h.Engine.Detector
	h.Engine.IsDictionaryWord = detector.IsDictionaryWord
	return h
}

func mustType(t *testing.T, h *harness, text, on string) {
	t.Helper()
	if !h.TypeText(text, on) {
		t.Fatalf("%s cannot type %q", on, text)
	}
}

// Fixed automatically, erased, and the original typed again: the fix was wrong, and it
// must not be made a second time — neither now nor later.
func TestErasingAFixAndRetypingTheOriginalIsARejection(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	h := newLearningHarness(t, f, learner)
	h.Active = f.tables["abc"]
	mustType(t, h, "привет ", "ru")
	if h.Screen() != "привет " {
		t.Fatalf("screen %q", h.Screen())
	}
	// Erase it, switch back by hand, type the original again.
	for i := 0; i < 7; i++ {
		h.Backspace()
	}
	h.Active = f.tables["abc"]
	mustType(t, h, "привет ", "ru")
	if h.Screen() != "ghbdtn " {
		t.Fatalf("the same fix must not be made twice: %q", h.Screen())
	}
	if decision, _ := learner.Decision("ghbdtn"); decision.Kind != core.DecisionKeep {
		t.Fatalf("decision %+v", decision)
	}
	later := newLearningHarness(t, f, learner)
	later.Active = f.tables["abc"]
	mustType(t, later, "привет ", "ru")
	if later.Screen() != "ghbdtn " {
		t.Fatalf("and in a later session too: %q", later.Screen())
	}
}

// Erasing a fix and typing something else is not a rejection.
func TestErasingAFixAndTypingSomethingElseTeachesNothing(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	h := newLearningHarness(t, f, learner)
	h.Active = f.tables["abc"]
	mustType(t, h, "привет ", "ru")
	for i := 0; i < 7; i++ {
		h.Backspace()
	}
	mustType(t, h, "здравствуйте ", "ru")
	if _, ok := learner.Decision("ghbdtn"); ok {
		t.Fatal("learned from something else")
	}
}

// Accepted once, applied without asking from then on.
func TestAcceptedConversionIsAppliedNextTime(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	before := newLearningHarness(t, f, learner)
	before.Active = f.tables["abc"]
	mustType(t, before, "тут ", "ru")
	if before.Screen() == "тут " {
		t.Fatal("on its own, too short to impose")
	}
	learner.Confirmed("nen", core.LangRU)
	after := newLearningHarness(t, f, learner)
	after.Active = f.tables["abc"]
	mustType(t, after, "тут ", "ru")
	if after.Screen() != "тут " {
		t.Fatalf("screen %q", after.Screen())
	}
}

// The user's own word, learned from keeping it, becomes something the detector can fix when
// it is typed on the wrong layout.
func TestLearnedVocabularyMakesTheUsersWordsFixable(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	unknown := "зюзяка"
	if f.models.Lexicon(core.LangRU).Contains(unknown) {
		t.Fatal("the test needs an unknown word")
	}
	before := newLearningHarness(t, f, learner)
	before.Active = f.tables["abc"]
	mustType(t, before, unknown+" ", "ru")
	if before.Screen() == unknown+" " {
		t.Fatal("typed on the wrong layout, an unknown word can at best be offered")
	}
	for i := 0; i < 2; i++ {
		session := newLearningHarness(t, f, learner)
		session.Active = f.tables["ru"]
		mustType(t, session, "мой "+unknown+" друг ", "ru")
	}
	if !learner.IsKnown(unknown, core.LangRU) {
		t.Fatal("kept in two sessions, it is a word")
	}
	after := newLearningHarness(t, f, learner)
	after.Active = f.tables["abc"]
	mustType(t, after, unknown+" ", "ru")
	if after.Screen() != unknown+" " {
		t.Fatalf("screen %q", after.Screen())
	}
}

// A layout mistake that slipped through and was left standing is not the user's word.
//
// Learned as vocabulary, it would never be corrected again — this is how «я написал так но
// потом» typed on ABC stopped being fixed at all.
func TestAMissedLayoutMistakeIsNeverLearnedAsVocabulary(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	for i := 0; i < 3; i++ {
		session := newLearningHarness(t, f, learner)
		// Nothing is fixed, as when the fix did not happen for whatever reason.
		session.Auto, session.Suggest = 2, 2
		session.Active = f.tables["abc"]
		mustType(t, session, "я написал так но потом всё ", "ru")
		if session.Screen() != `z yfgbcfk nfr yj gjnjv dc\ ` {
			t.Fatalf("screen %q", session.Screen())
		}
	}
	if items := learner.Items(); len(items) != 0 {
		t.Fatalf("every one of those is a Russian word on another layout: %+v", items)
	}
}

// Russian typed on the Kazakh layout is correct Russian, not Kazakh vocabulary to learn — but
// a word no dictionary has, like the slang spelling «кароче», is the user's own.
func TestRussianOnTheKazakhLayoutIsCheckedAgainstRussian(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	for i := 0; i < 2; i++ {
		session := newLearningHarness(t, f, learner)
		session.Active = f.tables["kk"]
		mustType(t, session, "ну короче вот так кароче сделаем ", "kk")
	}
	if learner.IsKnown("короче", core.LangKK) {
		t.Fatal("«короче» is Russian, not Kazakh vocabulary")
	}
	if learner.IsKnown("сделаем", core.LangKK) {
		t.Fatal("«сделаем» is Russian, not Kazakh vocabulary")
	}
	if !learner.IsKnown("кароче", core.LangKK) {
		t.Fatal("a word no dictionary has is the user's own")
	}
}

// What the first version learned from meaningless signals is dropped on upgrade; what the
// user accepted explicitly stays.
func TestUpgradeKeepsOnlyExplicitAcceptances(t *testing.T) {
	old := core.NewLearner(nil)
	old.Confirmed("rfhjxt", core.LangRU)
	old.Rejected("yfgbcfk")
	old.Ignored("ghbdtn")
	old.Kept("nfrjq", core.LangEN)
	old.Kept("nfrjq", core.LangEN)
	old.Kept("gjnjv", core.LangEN)
	stored := encodeWithSchema(t, old, 1)

	upgraded, err := core.DecodeLearner(stored, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision, _ := upgraded.Decision("rfhjxt"); decision != (core.LearnedDecision{Kind: core.DecisionConvert, Lang: core.LangRU}) {
		t.Fatalf("an explicit acceptance stays: %+v", decision)
	}
	if _, ok := upgraded.Decision("yfgbcfk"); ok {
		t.Fatal("a rejection learned under the old rules goes")
	}
	if upgraded.IsKnown("nfrjq", core.LangEN) {
		t.Fatal("vocabulary learned under the old rules goes")
	}
	if total := upgraded.Counts().Total(); total != 1 {
		t.Fatalf("%d entries left", total)
	}
	if upgrade, ok := upgraded.Upgraded(); !ok || upgrade != (core.LearnerUpgrade{FromSchema: 1, Before: 5, Kept: 1}) {
		t.Fatalf("upgrade %+v %v", upgrade, ok)
	}

	// Once only: later entries survive the next launch.
	upgraded.Rejected("tot")
	stored, err = upgraded.Encode()
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := core.DecodeLearner(stored, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision, _ := reloaded.Decision("tot"); decision.Kind != core.DecisionKeep {
		t.Fatalf("decision %+v", decision)
	}
	if _, again := reloaded.Upgraded(); again {
		t.Fatal("upgraded twice")
	}
}

// encodeWithSchema stores a learner as if under an older schema — what the Swift test does by
// setting `learningSchema` in its defaults. Schema 0 leaves the field out, as the first
// Windows version wrote learned.json.
func encodeWithSchema(t *testing.T, learner *core.Learner, schema int) []byte {
	t.Helper()
	data, err := learner.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if _, ok := file["schema"]; !ok {
		t.Fatal("the schema is stored")
	}
	if schema == 0 {
		delete(file, "schema")
	} else {
		file["schema"] = schema
	}
	data, err = json.Marshal(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A store from before schemas existed is upgraded too; one from a newer version is left as
// it is, as the Swift app leaves a newer `learningSchema` alone.
func TestLearnerSchemaIsStoredAndOnlyOlderStoresAreUpgraded(t *testing.T) {
	old := core.NewLearner(nil)
	old.Rejected("yfgbcfk")
	old.Confirmed("rfhjxt", core.LangRU)
	first, err := core.DecodeLearner(encodeWithSchema(t, old, 0), nil)
	if err != nil || first.Counts().Total() != 1 {
		t.Fatalf("a store without a schema is from the old rules: %v", err)
	}
	newer, err := core.DecodeLearner(encodeWithSchema(t, old, core.LearnerSchemaVersion+1), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, upgraded := newer.Upgraded(); upgraded || newer.Counts().Total() != 2 {
		t.Fatal("a newer store is not touched")
	}
	data, err := newer.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"schema": 3`) {
		t.Fatalf("a newer schema is kept as it is: %s", data)
	}
	fresh, err := core.NewLearner(nil).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(fresh), `"schema": 2`) {
		t.Fatalf("a new store is written under the current rules: %s", fresh)
	}
}

// Words the user keeps never turn into mistakes later, whatever the context.
func TestKeptWordIsNeverOffered(t *testing.T) {
	f := newFixture(t)
	learner := core.NewLearner(nil)
	learner.Rejected("tot")
	h := newLearningHarness(t, f, learner)
	h.Active = f.tables["ru"]
	mustType(t, h, "мне кажется ", "ru")
	h.Active = f.tables["abc"]
	mustType(t, h, "tot ", "abc")
	if h.Screen() != "мне кажется tot " || len(h.Events) != 0 {
		t.Fatalf("screen %q events %v", h.Screen(), h.Events)
	}
}

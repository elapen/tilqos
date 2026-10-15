package core_test

// The regression corpus (Tests/TilqosKitTests/Corpus.swift and CorpusTests.swift):
// every run the Swift CorpusTests make, replayed through the Go TypingEngine exactly the way
// the Swift HarnessEngine drives the Swift one (golden.CorpusHarness), compared run for run,
// and held to the same quality bar.

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"testing"
	"time"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

// harness is the corpus harness, for the tests that drive it by hand.
type harness = golden.CorpusHarness

func newHarness(models *core.Models, tables map[string]*core.KeyMapTable, resolve core.Resolver) *harness {
	return golden.NewCorpusHarness(models, tables, resolve)
}

// TestCorpusParity replays every CorpusTests run and compares the screen and every fix and
// offer with Swift; then it holds the CorpusTests quality bar and prints the report.
func TestCorpusParity(t *testing.T) {
	models := golden.Models(t)
	tables, resolve, _ := golden.Tables(t)
	file := golden.Load[golden.CorpusRun](t, "corpus.json")

	report := golden.NewCorpusReport()
	mismatches := 0
	for _, run := range file.Records {
		h := newHarness(models, tables, resolve)
		h.Auto, h.Suggest = golden.F(t, run.Auto), golden.F(t, run.Suggest)
		if run.Field != nil {
			h.Preceding = *run.Field
		}
		if !h.Simulate(run.Segs) {
			t.Errorf("cannot type %+v", run.Segs)
			continue
		}
		screen := h.Screen()
		if screen != run.Screen || fmt.Sprint(h.Events) != fmt.Sprint(run.Events) {
			mismatches++
			if mismatches <= 30 {
				t.Errorf("%s %q:\n  Go    %q %v\n  Swift %q %v", run.Group, joinSegs(run.Segs), screen, h.Events, run.Screen, run.Events)
			}
		}
		report.Add(run, screen, h.Events)
	}
	if mismatches > 0 {
		t.Errorf("%d of %d corpus runs differ from Swift", mismatches, len(file.Records))
	}
	t.Logf("corpus report (%d runs, %d differ from Swift):\n%s", len(file.Records), mismatches, report)
	for _, failure := range report.Violations(true) {
		t.Error(failure)
	}
	t.Log(report.Headline())
}

// learningRecord is one learning scenario from learning.json: text typed in several sessions
// with a learner attached, and what Swift saw and learned.
type learningRecord struct {
	Group    string           `json:"group"`
	Segs     []golden.Segment `json:"segs"`
	Sessions int              `json:"sessions"`
	Auto     string           `json:"auto"`
	Suggest  string           `json:"suggest"`
	Screens  []string         `json:"screens"`
	Events   [][][]string     `json:"events"`
	Items    [][]string       `json:"items"`
}

// TestLearningParity replays the learning scenarios — the corpus sentences and unknown words
// typed correctly, left standing on the wrong layouts, fixed; each in separate sessions with
// one learner, as LearnerTests drive it — and compares every screen, every fix and offer, and
// what the learner holds afterwards with Swift. The salt and the day are Swift's, so the
// entries compare hash for hash.
func TestLearningParity(t *testing.T) {
	f := newFixture(t)
	file := golden.Load[learningRecord](t, "learning.json")
	salt, err := hex.DecodeString(file.Header["salt"])
	if err != nil || len(salt) != 8 {
		t.Fatalf("salt %q: %v", file.Header["salt"], err)
	}
	seconds, err := strconv.Atoi(file.Header["day"])
	if err != nil {
		t.Fatalf("day %q: %v", file.Header["day"], err)
	}
	day := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(seconds) * time.Second)

	mismatches, learned := 0, 0
	for _, record := range file.Records {
		learner := core.NewLearner(func() time.Time { return day })
		learner.SetSaltForTesting(salt)
		var screens []string
		var events [][][]string
		for i := 0; i < record.Sessions; i++ {
			h := newLearningHarness(t, f, learner)
			h.Auto, h.Suggest = golden.F(t, record.Auto), golden.F(t, record.Suggest)
			if !h.Simulate(record.Segs) {
				t.Fatalf("cannot type %+v", record.Segs)
			}
			screens = append(screens, h.Screen())
			events = append(events, h.Events)
		}
		var items [][]string
		for _, item := range learner.Items() {
			items = append(items, []string{item.ID, string(item.Kind), string(item.Lang)})
		}
		learned += len(items)
		if fmt.Sprint(screens) != fmt.Sprint(record.Screens) || fmt.Sprint(events) != fmt.Sprint(record.Events) ||
			fmt.Sprint(items) != fmt.Sprint(record.Items) {
			mismatches++
			if mismatches <= 20 {
				t.Errorf("%s %q:\n  Go    %q %v %v\n  Swift %q %v %v", record.Group, joinSegs(record.Segs),
					screens, events, items, record.Screens, record.Events, record.Items)
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d of %d learning scenarios differ from Swift", mismatches, len(file.Records))
	}
	t.Logf("%d learning scenarios, %d entries learned, %d differ from Swift", len(file.Records), learned, mismatches)
}

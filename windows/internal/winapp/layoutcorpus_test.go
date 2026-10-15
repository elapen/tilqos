package winapp

// The regression corpus on Windows layouts. The parity test replays it on the macOS tables
// the Swift numbers were measured on; the Windows layouts put punctuation elsewhere (the
// Russian comma on Shift+/, the Kazakh comma and period on the number row, no digits on the
// Kazakh number row at all), so the same text becomes different keystrokes and different
// garble. This holds the corpus to the same bar on those layouts — above all, that no
// correctly typed word is ever rewritten — using the scan-code fixture tables of
// keyspace_test.go (hand-written after KBDUS, KBDRU and KBDKAZ; on Windows the real tables
// come from ToUnicodeEx).

import (
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

func windowsFixtureTables() map[string]*core.KeyMapTable {
	us := fixtureTable("04090409", core.LangEN, func(row int) []string { return fixtureRows[row].us }, ".")
	ru := fixtureTable("04190419", core.LangRU, func(row int) []string { return fixtureRows[row].ru }, ",")
	kk := fixtureTable("043F043F", core.LangKK, func(row int) []string {
		if fixtureRows[row].kk != nil {
			return fixtureRows[row].kk
		}
		return fixtureRows[row].ru
	}, ",")
	return map[string]*core.KeyMapTable{"abc": us, "ru": ru, "kk": kk}
}

func TestCorpusOnWindowsLayouts(t *testing.T) {
	models := golden.Models(t)
	tables := windowsFixtureTables()
	byID := map[string]*core.KeyMapTable{}
	for _, table := range tables {
		byID[table.SourceID] = table
	}
	resolve := func(id string) *core.KeyMapTable { return byID[id] }

	report := golden.NewCorpusReport()
	skipped := 0
	for _, run := range golden.Load[golden.CorpusRun](t, "corpus.json").Records {
		if !golden.Typeable(tables, run.Segs) {
			// «» and a few symbols have no key on these layouts.
			skipped++
			continue
		}
		h := golden.NewCorpusHarness(models, tables, resolve)
		h.Auto, h.Suggest = golden.F(t, run.Auto), golden.F(t, run.Suggest)
		if run.Field != nil {
			h.Preceding = *run.Field
		}
		h.Simulate(run.Segs)
		report.Add(run, h.Screen(), h.Events)
	}
	t.Logf("corpus on Windows layouts (%d runs not typeable there, skipped):\n%s", skipped, report)
	// The full CorpusTests bar, catch rates included: the Windows layouts meet it too.
	for _, failure := range report.Violations(true) {
		t.Error(failure)
	}
	t.Log(report.Headline())
}

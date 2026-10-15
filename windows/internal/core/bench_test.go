package core_test

// The Swift PerformanceTests, as Go benchmarks: the same buffer (a sentence of mistyped
// Cyrillic, 11 words) and the same two evaluations.
//
//	go test -bench . -run '^$' ./internal/core

import (
	"testing"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

func benchmarkContext(b *testing.B) (*core.Detector, core.Context) {
	tables, resolve, _ := golden.Tables(b)
	models := golden.Models(b)
	strokes, ok := tables["ru"].Strokes("привет как дела я сегодня работаю над клавиатурой и переключателем")
	if !ok {
		b.Fatal("cannot type the benchmark sentence")
	}
	recorded := make([]core.RecordedStroke, len(strokes))
	for i, stroke := range strokes {
		recorded[i] = core.RecordedStroke{Stroke: stroke, Time: float64(i) * 0.05, LayoutID: tables["abc"].SourceID}
	}
	context := core.NewContext(recorded, []*core.KeyMapTable{tables["abc"], tables["ru"], tables["kk"]}, resolve)
	return core.NewDetector(models), context
}

func BenchmarkLastWordEvaluation(b *testing.B) {
	detector, context := benchmarkContext(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Evaluate(context, core.ScopeLastWord)
	}
}

func BenchmarkTailScan(b *testing.B) {
	detector, context := benchmarkContext(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		detector.Evaluate(context, core.ScopeTail)
	}
}

func BenchmarkKeystrokeAppend(b *testing.B) {
	buffer := core.NewStrokeBuffer(256, 5)
	stroke := core.KeyStroke{Key: 4}
	for i := 0; i < b.N; i++ {
		buffer.Append(stroke, 1, "layout")
	}
}

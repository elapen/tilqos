package licenses_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tilqos/internal/licenses"
)

// TestEmbeddedLicensesMatchSource fails — never skips — when the embedded copies are missing
// or differ from the files at the repository root, the single source of truth.
func TestEmbeddedLicensesMatchSource(t *testing.T) {
	embedded, err := licenses.Files()
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, name := range licenses.Names {
		source, err := os.ReadFile(filepath.Join("..", "..", "..", name))
		if err != nil {
			t.Fatalf("the source file %s: %v", name, err)
		}
		if !bytes.Equal(source, embedded[name]) {
			t.Errorf("embedded %s differs from ../%s — run `go generate ./...`", name, name)
		}
	}
}

func TestEmbeddedLicensesLookRight(t *testing.T) {
	embedded, err := licenses.Files()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !strings.Contains(string(embedded["LICENSE"]), "MIT License") {
		t.Error("LICENSE does not look like the MIT License")
	}
	if len(embedded["THIRD_PARTY_NOTICES.md"]) < 500 {
		t.Error("THIRD_PARTY_NOTICES.md is suspiciously short")
	}
}

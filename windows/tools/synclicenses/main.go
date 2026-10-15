// Command synclicenses copies LICENSE and THIRD_PARTY_NOTICES.md from the repository root (the
// single source of truth, shared with the macOS app) into windows/internal/licenses/embedded
// for //go:embed.
//
//	go run ./tools/synclicenses             copy (only files that differ are rewritten)
//	go run ./tools/synclicenses -check      verify only; exit status 1 on drift
//
// It finds the module root by walking up from the working directory, so it works both from
// windows/ and through `go generate` in internal/licenses.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tilqos/internal/licenses"
)

// isThisModule checks a go.mod's module line, whatever its line endings.
func isThisModule(goMod []byte) bool {
	line, _, _ := strings.Cut(string(goMod), "\n")
	return strings.TrimSpace(line) == "module tilqos"
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil && isThisModule(data) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not inside the tilqos Go module")
		}
		dir = parent
	}
}

func main() {
	check := flag.Bool("check", false, "verify the copies instead of writing them")
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	repoRoot := filepath.Dir(root)
	targetDir := filepath.Join(root, "internal", "licenses", "embedded")

	drift := 0
	for _, name := range licenses.Names {
		source, err := os.ReadFile(filepath.Join(repoRoot, name))
		if err != nil {
			fail(err)
		}
		target := filepath.Join(targetDir, name)
		current, err := os.ReadFile(target)
		if err == nil && bytes.Equal(current, source) {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "synclicenses: %s is missing or stale\n", target)
			drift++
			continue
		}
		if err := os.WriteFile(target, source, 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("synclicenses: %s (%d bytes)\n", name, len(source))
	}
	if drift > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "synclicenses: %v\n", err)
	os.Exit(1)
}

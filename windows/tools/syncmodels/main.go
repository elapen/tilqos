// Command syncmodels copies the model files from Resources/Models (the single source of
// truth, shared with the macOS app) into windows/internal/models/embedded for //go:embed.
//
//	go run ./tools/syncmodels             copy (only files that differ are rewritten)
//	go run ./tools/syncmodels -check      verify only; exit status 1 on drift
//	go run ./tools/syncmodels -rev main   take files missing from Resources/Models from a
//	                                      git revision (also TILQOS_MODELS_REV)
//
// It finds the module root by walking up from the working directory, so it works both
// from windows/ and through `go generate` in internal/models.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tilqos/internal/core"
	"tilqos/internal/modelsrc"
)

// isThisModule checks a go.mod's module line, whatever its line endings (a Windows
// checkout with core.autocrlf turns them into CRLF).
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
		if data, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil &&
			isThisModule(data) {
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
	rev := flag.String("rev", "", "git revision to read model files missing from Resources/Models from")
	flag.Parse()

	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	repoRoot := filepath.Dir(root)
	targetDir := filepath.Join(root, "internal", "models", "embedded")

	drift := 0
	for _, name := range core.ModelFileNames {
		source, origin, err := modelsrc.Read(repoRoot, name, *rev)
		if err != nil {
			fail(err)
		}
		target := filepath.Join(targetDir, name)
		current, err := os.ReadFile(target)
		if err == nil && bytes.Equal(current, source) {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "syncmodels: %s is missing or stale\n", target)
			drift++
			continue
		}
		if err := os.WriteFile(target, source, 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("syncmodels: %s (%d bytes, from %s)\n", name, len(source), origin)
	}
	if drift > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "syncmodels: %v\n", err)
	os.Exit(1)
}

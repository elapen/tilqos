// Package models embeds the language models into the executable.
//
// The single source of truth is Resources/Models at the repository root (shared with the
// macOS app). //go:embed cannot reach outside the module, so tools/syncmodels copies the
// files into embedded/ — run by `go generate ./...`, build.sh and build.ps1 — and
// TestEmbeddedModelsMatchSource fails if the copies are missing or have drifted.
//
// embedded/README.txt is committed so the embed pattern always matches and the package always
// compiles; a missing model is reported by Load (and by the test), not by the compiler.
package models

//go:generate go run ../../tools/syncmodels

import (
	"embed"
	"fmt"

	"tilqos/internal/core"
)

//go:embed embedded
var data embed.FS

// Files returns the embedded model files by name.
func Files() (map[string][]byte, error) {
	files := make(map[string][]byte, len(core.ModelFileNames))
	for _, name := range core.ModelFileNames {
		content, err := data.ReadFile("embedded/" + name)
		if err != nil {
			return nil, fmt.Errorf("model %s is not embedded in this build — run `go generate ./...` "+
				"(or build.sh / build.ps1) to copy Resources/Models into internal/models/embedded: %w", name, err)
		}
		files[name] = content
	}
	return files, nil
}

// Load decodes the embedded models.
func Load() (*core.Models, error) {
	files, err := Files()
	if err != nil {
		return nil, err
	}
	return core.LoadModels(files)
}

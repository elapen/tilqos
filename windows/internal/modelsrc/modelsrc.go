// Package modelsrc reads the model files from their single source of truth, Resources/Models
// at the repository root (shared with the macOS app). tools/syncmodels, the golden parity
// tests and the embed check all go through it, so they can never disagree about where the
// models come from.
package modelsrc

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RevEnv names a git revision to read model files from when Resources/Models lacks them.
//
// For a checkout of this branch that predates a model file its code already needs — the
// word lexicons ({en,ru,kk}-lex.bin) arrived with the detection rewrite on main. Once the
// branch is merged the files are on disk and the variable is not needed. It never overrides
// a file that is present: Resources/Models always wins.
const RevEnv = "TILQOS_MODELS_REV"

// Dir is Resources/Models under the repository root.
func Dir(repoRoot string) string {
	return filepath.Join(repoRoot, "Resources", "Models")
}

// Read returns a model file and a description of where it came from. A missing file is an
// error unless TILQOS_MODELS_REV (or rev, when non-empty) names a revision that has it.
func Read(repoRoot, name, rev string) ([]byte, string, error) {
	path := filepath.Join(Dir(repoRoot), name)
	data, err := os.ReadFile(path)
	if err == nil {
		return data, path, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, path, err
	}
	if rev == "" {
		rev = strings.TrimSpace(os.Getenv(RevEnv))
	}
	if rev == "" {
		return nil, path, fmt.Errorf("%s is missing (Resources/Models is the source of truth; on a "+
			"branch that predates it, set %s to a revision that has it, e.g. %s=main)", path, RevEnv, RevEnv)
	}
	object := rev + ":Resources/Models/" + name
	command := exec.Command("git", "-C", repoRoot, "show", object)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	data, err = command.Output()
	if err != nil {
		return nil, object, fmt.Errorf("%s is missing and `git show %s` failed: %v %s",
			path, object, err, strings.TrimSpace(stderr.String()))
	}
	return data, "git " + object, nil
}

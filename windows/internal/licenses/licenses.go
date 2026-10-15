// Package licenses embeds the license texts into the executable: the project's own license
// and the notices for the third-party data and the Go runtime that the program is built from.
//
// The single source of truth is LICENSE and THIRD_PARTY_NOTICES.md at the repository root
// (shared with the macOS app). //go:embed cannot reach outside the module, so
// tools/synclicenses copies them into embedded/ — run by `go generate ./...`, build.sh and
// build.ps1 — and TestEmbeddedLicensesMatchSource fails if the copies are missing or stale.
// A lone .exe that is passed around without its zip still carries what the licenses require.
//
// embedded/README.txt is committed so the embed pattern always matches and the package always
// compiles; a missing text is reported by Files (and by the test), not by the compiler.
package licenses

//go:generate go run ../../tools/synclicenses

import (
	"embed"
	"fmt"
)

//go:embed embedded
var data embed.FS

// Names are the embedded files, as they are named at the repository root.
var Names = []string{"LICENSE", "THIRD_PARTY_NOTICES.md"}

// Files returns the embedded license texts by name.
func Files() (map[string][]byte, error) {
	files := make(map[string][]byte, len(Names))
	for _, name := range Names {
		content, err := data.ReadFile("embedded/" + name)
		if err != nil {
			return nil, fmt.Errorf("%s is not embedded in this build — run `go generate ./...` "+
				"(or build.sh / build.ps1) to copy it into internal/licenses/embedded: %w", name, err)
		}
		files[name] = content
	}
	return files, nil
}

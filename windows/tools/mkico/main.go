// Command mkico renders the Windows icons from Resources/icon-1024.png (the macOS app's
// icon, the single source for both):
//
//	internal/winapp/assets/app.ico            tray icon (embedded with //go:embed)
//	internal/winapp/assets/app-off.ico        tray icon while switched off
//	cmd/tilqos/rsrc_windows_amd64.syso  executable icon, linked by `go build`
//	cmd/tilqos/rsrc_windows_arm64.syso
//
//	go run ./tools/mkico           write (only files that differ are rewritten)
//	go run ./tools/mkico -check    verify only; exit status 1 on drift
//
// The outputs are committed and deterministic; iconkit's tests fail if they drift from the
// PNG.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"tilqos/internal/iconkit"
)

func main() {
	check := flag.Bool("check", false, "verify the generated files instead of writing them")
	flag.Parse()

	root, err := iconkit.ModuleRoot()
	if err != nil {
		fail(err)
	}
	outputs, err := iconkit.Generate(filepath.Join(filepath.Dir(root), "Resources", "icon-1024.png"))
	if err != nil {
		fail(err)
	}
	drift := 0
	for _, name := range iconkit.OutputNames {
		path := filepath.Join(root, filepath.FromSlash(name))
		current, err := os.ReadFile(path)
		if err == nil && bytes.Equal(current, outputs[name]) {
			continue
		}
		if *check {
			fmt.Fprintf(os.Stderr, "mkico: %s is missing or stale\n", name)
			drift++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fail(err)
		}
		if err := os.WriteFile(path, outputs[name], 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("mkico: %s (%d bytes)\n", name, len(outputs[name]))
	}
	if drift > 0 {
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "mkico: %v\n", err)
	os.Exit(1)
}

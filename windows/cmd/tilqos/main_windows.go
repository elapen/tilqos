//go:build windows && (amd64 || arm64)

// Command tilqos is Tilqos for Windows: it fixes text typed in the wrong
// keyboard layout (English, Russian, Kazakh) and lives in the notification area.
//
// Build with -ldflags "-H windowsgui" (build.ps1 / build.sh do) so no console appears.
package main

import (
	"os"

	"tilqos/internal/winapp"
)

func main() {
	os.Exit(winapp.Run(os.Args[1:]))
}

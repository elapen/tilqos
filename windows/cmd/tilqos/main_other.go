//go:build !windows || !(amd64 || arm64)

// Command tilqos is Tilqos for Windows. On any other target it only explains
// how to build it, so `go build ./...` and `go vet ./...` work everywhere.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "Tilqos for Windows runs on 64-bit Windows (amd64, arm64).")
	fmt.Fprintln(os.Stderr, "Build it with ./build.sh (macOS/Linux) or .\\build.ps1 (Windows).")
	os.Exit(1)
}

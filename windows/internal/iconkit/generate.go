package iconkit

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// OutputNames are the generated files, relative to the module root.
var OutputNames = []string{
	"internal/winapp/assets/app.ico",
	"internal/winapp/assets/app-off.ico",
	"cmd/tilqos/rsrc_windows_amd64.syso",
	"cmd/tilqos/rsrc_windows_arm64.syso",
}

// Generate renders every output from the source PNG.
func Generate(pngPath string) (map[string][]byte, error) {
	file, err := os.Open(pngPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	src, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", pngPath, err)
	}
	normal, err := EncodeImages(src, false)
	if err != nil {
		return nil, err
	}
	off, err := EncodeImages(src, true)
	if err != nil {
		return nil, err
	}
	amd64, err := BuildResourceObject(normal, MachineAMD64)
	if err != nil {
		return nil, err
	}
	arm64, err := BuildResourceObject(normal, MachineARM64)
	if err != nil {
		return nil, err
	}
	return map[string][]byte{
		OutputNames[0]: EncodeICO(normal),
		OutputNames[1]: EncodeICO(off),
		OutputNames[2]: amd64,
		OutputNames[3]: arm64,
	}, nil
}

// isThisModule checks a go.mod's module line, whatever its line endings.
func isThisModule(goMod []byte) bool {
	line, _, _ := strings.Cut(string(goMod), "\n")
	return strings.TrimSpace(line) == "module tilqos"
}

// ModuleRoot finds the windows/ module root above the working directory.
func ModuleRoot() (string, error) {
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

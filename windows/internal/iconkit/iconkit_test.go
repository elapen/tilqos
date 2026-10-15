package iconkit

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func sourcePNG(t *testing.T) string {
	root, err := ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(filepath.Dir(root), "Resources", "icon-1024.png")
}

// TestGeneratedFilesAreCurrent fails when the committed icons drift from the PNG.
func TestGeneratedFilesAreCurrent(t *testing.T) {
	outputs, err := Generate(sourcePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	root, _ := ModuleRoot()
	for _, name := range OutputNames {
		current, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(current, outputs[name]) {
			t.Errorf("%s is missing or stale — run `go generate ./...` (tools/mkico)", name)
		}
	}
}

func TestResampleKeepsFlatColoursAndCorners(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			c := color.NRGBA{R: 200, G: 40, B: 10, A: 255}
			if x >= 50 {
				c = color.NRGBA{} // transparent right half
			}
			src.SetNRGBA(x, y, c)
		}
	}
	out := Resample(src, 7) // a non-integer ratio
	left := out.NRGBAAt(0, 3)
	if left != (color.NRGBA{R: 200, G: 40, B: 10, A: 255}) {
		t.Fatalf("flat colour changed: %v", left)
	}
	// The straddling column is half covered; premultiplication keeps its colour clean.
	mid := out.NRGBAAt(3, 3)
	if mid.A < 120 || mid.A > 135 || mid.R != 200 || mid.G != 40 {
		t.Fatalf("edge pixel %v", mid)
	}
	if out.NRGBAAt(6, 3).A != 0 {
		t.Fatal("transparent area must stay transparent")
	}
}

func TestICORoundTrip(t *testing.T) {
	images := []Image{{Size: 16, Data: []byte("sixteen")}, {Size: 256, Data: []byte("two-five-six")}}
	ico := EncodeICO(images)
	if binary.LittleEndian.Uint16(ico[2:]) != 1 || binary.LittleEndian.Uint16(ico[4:]) != 2 {
		t.Fatal("bad header")
	}
	if ico[6] != 16 || ico[6+16] != 0 {
		t.Fatal("256 must be stored as 0")
	}
	offset := binary.LittleEndian.Uint32(ico[6+16+12:])
	if string(ico[offset:offset+12]) != "two-five-six" {
		t.Fatal("image data misplaced")
	}
}

func TestIconImagesDecode(t *testing.T) {
	file, err := os.Open(sourcePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	src, err := png.Decode(file)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	images, err := EncodeImages(src, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, img := range images {
		if img.Size >= pngThreshold {
			decoded, err := png.Decode(bytes.NewReader(img.Data))
			if err != nil || decoded.Bounds().Dx() != img.Size {
				t.Fatalf("%d px PNG does not decode: %v", img.Size, err)
			}
			continue
		}
		// DIB: BITMAPINFOHEADER with a double height (XOR + AND mask) and 32 bpp.
		if binary.LittleEndian.Uint32(img.Data) != 40 ||
			int32(binary.LittleEndian.Uint32(img.Data[8:])) != int32(2*img.Size) ||
			binary.LittleEndian.Uint16(img.Data[14:]) != 32 {
			t.Fatalf("%d px DIB header is wrong", img.Size)
		}
		maskStride := ((img.Size + 31) / 32) * 4
		if len(img.Data) != 40+img.Size*img.Size*4+maskStride*img.Size {
			t.Fatalf("%d px DIB has the wrong length", img.Size)
		}
	}
}

// TestResourceObjectLinksIntoWindowsExecutables links a real Windows executable per
// architecture with the generated .syso and walks the PE resource tree the way the Windows
// loader and Explorer do: RT_GROUP_ICON 1 → its entries → RT_ICON by ID → bytes at the RVA.
// This proves the relocations are resolved by the Go linker and the tree is well formed.
func TestResourceObjectLinksIntoWindowsExecutables(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		goTool = filepath.Join(runtime.GOROOT(), "bin", "go")
	}
	images := []Image{
		{Size: 16, Data: bytes.Repeat([]byte{0xA1}, 1000)},
		{Size: 32, Data: bytes.Repeat([]byte{0xB2}, 777)},
		{Size: 256, Data: append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{0xC3}, 3001)...)},
	}
	for _, arch := range []struct {
		name    string
		machine uint16
	}{{"amd64", MachineAMD64}, {"arm64", MachineARM64}} {
		t.Run(arch.name, func(t *testing.T) {
			object, err := BuildResourceObject(images, arch.machine)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			write := func(name, content string) {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			write("go.mod", "module rsrctest\n\ngo 1.25\n")
			write("main.go", "package main\n\nfunc main() {}\n")
			write("rsrc_windows_"+arch.name+".syso", string(object))
			exe := filepath.Join(dir, "out.exe")
			cmd := exec.Command(goTool, "build", "-o", exe, ".")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+arch.name, "CGO_ENABLED=0", "GOFLAGS=")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("linking failed: %v\n%s", err, output)
			}
			checkResources(t, exe, images)
		})
	}
}

func checkResources(t *testing.T, exe string, images []Image) {
	file, err := pe.Open(exe)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	section := file.Section(".rsrc")
	if section == nil {
		t.Fatal("no .rsrc section in the executable")
	}
	data, err := section.Data()
	if err != nil {
		t.Fatal(err)
	}
	// The data directory must point at the section, or the loader will not find it.
	var dirRVA uint32
	switch header := file.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		dirRVA = header.DataDirectory[pe.IMAGE_DIRECTORY_ENTRY_RESOURCE].VirtualAddress
	default:
		t.Fatalf("unexpected optional header %T", header)
	}
	if dirRVA != section.VirtualAddress {
		t.Fatalf("resource data directory %#x, .rsrc at %#x", dirRVA, section.VirtualAddress)
	}

	type entry struct {
		id     uint32
		offset uint32
		subdir bool
	}
	readDir := func(offset uint32) []entry {
		named := binary.LittleEndian.Uint16(data[offset+12:])
		ids := binary.LittleEndian.Uint16(data[offset+14:])
		var out []entry
		for i := uint32(0); i < uint32(named)+uint32(ids); i++ {
			at := offset + 16 + 8*i
			value := binary.LittleEndian.Uint32(data[at+4:])
			out = append(out, entry{binary.LittleEndian.Uint32(data[at:]), value &^ 0x80000000, value&0x80000000 != 0})
		}
		return out
	}
	// type → id → bytes, resolving RVAs through the section's virtual address.
	resources := map[uint32]map[uint32][]byte{}
	for _, typ := range readDir(0) {
		if !typ.subdir {
			t.Fatal("type entry must point at a directory")
		}
		resources[typ.id] = map[uint32][]byte{}
		for _, name := range readDir(typ.offset) {
			languages := readDir(name.offset)
			if len(languages) != 1 || languages[0].subdir || languages[0].id != langEnUS {
				t.Fatalf("resource %d/%d: bad language level %+v", typ.id, name.id, languages)
			}
			leaf := languages[0].offset
			rva := binary.LittleEndian.Uint32(data[leaf:])
			size := binary.LittleEndian.Uint32(data[leaf+4:])
			start := rva - section.VirtualAddress
			if rva < section.VirtualAddress || start+size > uint32(len(data)) {
				t.Fatalf("resource %d/%d: RVA %#x outside .rsrc [%#x, +%#x)", typ.id, name.id, rva, section.VirtualAddress, len(data))
			}
			resources[typ.id][name.id] = data[start : start+size]
		}
	}
	group := resources[rtGroupIcon][1]
	if len(group) != 6+14*len(images) || binary.LittleEndian.Uint16(group[4:]) != uint16(len(images)) {
		t.Fatalf("group icon directory has %d bytes", len(group))
	}
	for i, image := range images {
		at := 6 + 14*i
		id := uint32(binary.LittleEndian.Uint16(group[at+12:]))
		size := binary.LittleEndian.Uint32(group[at+8:])
		stored := resources[rtIcon][id]
		if !bytes.Equal(stored, image.Data) || size != uint32(len(image.Data)) {
			t.Fatalf("icon %d (%d px): %s", id, image.Size, fmt.Sprintf("stored %d bytes, want %d", len(stored), len(image.Data)))
		}
		if want := dimension(image.Size); group[at] != want {
			t.Fatalf("icon %d width byte %d, want %d", id, group[at], want)
		}
	}
}

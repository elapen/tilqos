// Package iconkit renders the Windows icons from Resources/icon-1024.png and packs them as
// an .ico file and as a COFF resource object (.syso) that the Go linker merges into the
// executable, which is where Explorer reads the file icon from.
//
// Standard library only. The .syso layout follows the PE/COFF specification: one .rsrc
// section holding the resource directory tree (RT_ICON images plus one RT_GROUP_ICON), the
// data entries' RVAs expressed as ADDR32NB relocations against the section symbol, which
// the Go linker resolves when it places .rsrc. iconkit_test.go links real Windows
// executables for amd64 and arm64 and walks their resource tree to prove it.
package iconkit

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
)

// Sizes rendered into the icons: the small-icon sizes of 100–250 % DPI, plus Explorer's.
var Sizes = []int{16, 20, 24, 32, 40, 48, 64, 128, 256}

// pngThreshold: images at least this large are stored as PNG, smaller ones as DIBs, which
// every Windows icon consumer understands.
const pngThreshold = 128

// Resample scales src to a size×size image by exact area averaging in premultiplied
// alpha — the right filter for large downscales, with no ringing and no dark fringes.
func Resample(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	// Premultiplied float planes.
	plane := make([][4]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(src.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			a := float64(c.A) / 255
			plane[y*w+x] = [4]float64{float64(c.R) * a, float64(c.G) * a, float64(c.B) * a, a}
		}
	}
	horizontal := resampleAxis(plane, w, h, size, true)
	both := resampleAxis(horizontal, size, h, size, false)

	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for i, p := range both {
		a := p[3]
		var r, g, bl float64
		if a > 0 {
			r, g, bl = p[0]/a, p[1]/a, p[2]/a
		}
		out.Pix[i*4+0] = clamp8(r)
		out.Pix[i*4+1] = clamp8(g)
		out.Pix[i*4+2] = clamp8(bl)
		out.Pix[i*4+3] = clamp8(a * 255)
	}
	return out
}

// resampleAxis averages along one axis: each destination cell covers [i*s, (i+1)*s) of the
// source, and each source cell contributes in proportion to its overlap.
func resampleAxis(in [][4]float64, w, h, size int, horizontal bool) [][4]float64 {
	srcLen, lines := h, w
	if horizontal {
		srcLen, lines = w, h
	}
	scale := float64(srcLen) / float64(size)
	var out [][4]float64
	if horizontal {
		out = make([][4]float64, size*h)
	} else {
		out = make([][4]float64, w*size)
	}
	for line := 0; line < lines; line++ {
		for i := 0; i < size; i++ {
			start, end := float64(i)*scale, float64(i+1)*scale
			var acc [4]float64
			for j := int(math.Floor(start)); j < int(math.Ceil(end)) && j < srcLen; j++ {
				weight := math.Min(end, float64(j+1)) - math.Max(start, float64(j))
				if weight <= 0 {
					continue
				}
				var p [4]float64
				if horizontal {
					p = in[line*w+j]
				} else {
					p = in[j*w+line]
				}
				for k := range acc {
					acc[k] += p[k] * weight
				}
			}
			for k := range acc {
				acc[k] /= scale
			}
			if horizontal {
				out[line*size+i] = acc
			} else {
				out[i*w+line] = acc
			}
		}
	}
	return out
}

func clamp8(v float64) uint8 {
	v = math.Round(v)
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// Disabled turns an icon into its "switched off" look: grayscale, partly transparent.
func Disabled(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	for i := 0; i < len(src.Pix); i += 4 {
		r, g, b, a := float64(src.Pix[i]), float64(src.Pix[i+1]), float64(src.Pix[i+2]), float64(src.Pix[i+3])
		luma := clamp8(0.299*r + 0.587*g + 0.114*b)
		out.Pix[i], out.Pix[i+1], out.Pix[i+2] = luma, luma, luma
		out.Pix[i+3] = clamp8(a * 0.55)
	}
	return out
}

// encodeImage stores one icon image: PNG for large sizes, a 32-bit DIB otherwise.
func encodeImage(img *image.NRGBA) ([]byte, error) {
	size := img.Bounds().Dx()
	if size >= pngThreshold {
		var buf bytes.Buffer
		encoder := png.Encoder{CompressionLevel: png.BestCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	// BITMAPINFOHEADER, then bottom-up BGRA rows, then the 1-bpp AND mask (all zero: the
	// alpha channel carries the transparency), rows padded to 32 bits.
	maskStride := ((size + 31) / 32) * 4
	var buf bytes.Buffer
	header := struct {
		Size          uint32
		Width, Height int32
		Planes        uint16
		BitCount      uint16
		Compression   uint32
		SizeImage     uint32
		XPelsPerMeter int32
		YPelsPerMeter int32
		ClrUsed       uint32
		ClrImportant  uint32
	}{Size: 40, Width: int32(size), Height: int32(2 * size), Planes: 1, BitCount: 32,
		SizeImage: uint32(size*size*4 + maskStride*size)}
	binary.Write(&buf, binary.LittleEndian, header)
	for y := size - 1; y >= 0; y-- {
		for x := 0; x < size; x++ {
			i := img.PixOffset(x, y)
			buf.Write([]byte{img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]})
		}
	}
	buf.Write(make([]byte, maskStride*size))
	return buf.Bytes(), nil
}

// Image is one encoded icon image.
type Image struct {
	Size int
	Data []byte
}

// CropToContent trims the transparent margin macOS icons carry (the artwork sits inside a
// padded canvas), keeping a square centred on the opaque content. Windows icons are
// expected to fill their square; at 16 px the margin would cost a fifth of the icon.
func CropToContent(src image.Image) image.Image {
	b := src.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X-1, b.Min.Y-1
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a > 0x0800 {
				minX, maxX = min(minX, x), max(maxX, x)
				minY, maxY = min(minY, y), max(maxY, y)
			}
		}
	}
	if maxX < minX {
		return src
	}
	side := max(maxX-minX+1, maxY-minY+1)
	cx, cy := (minX+maxX+1)/2, (minY+maxY+1)/2
	rect := image.Rect(cx-side/2, cy-side/2, cx-side/2+side, cy-side/2+side).Intersect(b)
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	offsetX, offsetY := (side-rect.Dx())/2, (side-rect.Dy())/2
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			out.Set(x-rect.Min.X+offsetX, y-rect.Min.Y+offsetY, src.At(x, y))
		}
	}
	return out
}

// EncodeImages renders src at every size.
func EncodeImages(src image.Image, disabled bool) ([]Image, error) {
	src = CropToContent(src)
	var images []Image
	for _, size := range Sizes {
		img := Resample(src, size)
		if disabled {
			img = Disabled(img)
		}
		data, err := encodeImage(img)
		if err != nil {
			return nil, err
		}
		images = append(images, Image{Size: size, Data: data})
	}
	return images, nil
}

func dimension(size int) byte {
	if size >= 256 {
		return 0 // 0 means 256 in icon directories
	}
	return byte(size)
}

// EncodeICO builds an .ico file.
func EncodeICO(images []Image) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	offset := 6 + 16*len(images)
	for _, image := range images {
		buf.Write([]byte{dimension(image.Size), dimension(image.Size), 0, 0})
		binary.Write(&buf, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&buf, binary.LittleEndian, [2]uint32{uint32(len(image.Data)), uint32(offset)})
		offset += len(image.Data)
	}
	for _, image := range images {
		buf.Write(image.Data)
	}
	return buf.Bytes()
}

// MARK: - COFF resource object

// Machine types and the image-relative 32-bit relocation of each.
const (
	MachineAMD64 uint16 = 0x8664
	MachineARM64 uint16 = 0xAA64

	relocAMD64Addr32NB uint16 = 0x0003 // IMAGE_REL_AMD64_ADDR32NB
	relocARM64Addr32NB uint16 = 0x0002 // IMAGE_REL_ARM64_ADDR32NB

	rtIcon      = 3
	rtGroupIcon = 14
	langEnUS    = 0x0409
)

type resource struct {
	typ, id uint32
	data    []byte
}

// BuildResourceObject packs icon images into a COFF object with one .rsrc section:
// RT_ICON 1…n and RT_GROUP_ICON 1, in language en-US.
func BuildResourceObject(images []Image, machine uint16) ([]byte, error) {
	var relocType uint16
	switch machine {
	case MachineAMD64:
		relocType = relocAMD64Addr32NB
	case MachineARM64:
		relocType = relocARM64Addr32NB
	default:
		return nil, fmt.Errorf("unsupported machine %#x", machine)
	}

	var resources []resource
	var group bytes.Buffer
	binary.Write(&group, binary.LittleEndian, [3]uint16{0, 1, uint16(len(images))})
	for i, image := range images {
		id := uint32(i + 1)
		resources = append(resources, resource{rtIcon, id, image.Data})
		group.Write([]byte{dimension(image.Size), dimension(image.Size), 0, 0})
		binary.Write(&group, binary.LittleEndian, [2]uint16{1, 32})
		binary.Write(&group, binary.LittleEndian, uint32(len(image.Data)))
		binary.Write(&group, binary.LittleEndian, uint16(id))
	}
	resources = append(resources, resource{rtGroupIcon, 1, group.Bytes()})
	sort.SliceStable(resources, func(i, j int) bool {
		if resources[i].typ != resources[j].typ {
			return resources[i].typ < resources[j].typ
		}
		return resources[i].id < resources[j].id
	})

	// Group by type, keeping ID order.
	var types []uint32
	byType := map[uint32][]resource{}
	for _, r := range resources {
		if _, seen := byType[r.typ]; !seen {
			types = append(types, r.typ)
		}
		byType[r.typ] = append(byType[r.typ], r)
	}

	const dirSize, entrySize, dataEntrySize = 16, 8, 16
	// Offsets of every part of the tree, then of the data.
	rootSize := dirSize + entrySize*len(types)
	typeDirOffset := map[uint32]int{}
	offset := rootSize
	for _, typ := range types {
		typeDirOffset[typ] = offset
		offset += dirSize + entrySize*len(byType[typ])
	}
	langDirOffset := make([]int, len(resources))
	for i := range resources {
		langDirOffset[i] = offset
		offset += dirSize + entrySize
	}
	dataEntryOffset := make([]int, len(resources))
	for i := range resources {
		dataEntryOffset[i] = offset
		offset += dataEntrySize
	}
	dataOffset := make([]int, len(resources))
	for i, r := range resources {
		offset = (offset + 7) &^ 7
		dataOffset[i] = offset
		offset += len(r.data)
	}
	section := make([]byte, (offset+7)&^7)

	putDir := func(at, idEntries int) {
		// Characteristics, TimeDateStamp, Major/MinorVersion, NumberOfNamedEntries = 0.
		binary.LittleEndian.PutUint16(section[at+14:], uint16(idEntries))
	}
	putEntry := func(at int, id uint32, target int, subdirectory bool) {
		binary.LittleEndian.PutUint32(section[at:], id)
		value := uint32(target)
		if subdirectory {
			value |= 0x80000000
		}
		binary.LittleEndian.PutUint32(section[at+4:], value)
	}

	putDir(0, len(types))
	for i, typ := range types {
		putEntry(dirSize+entrySize*i, typ, typeDirOffset[typ], true)
	}
	index := 0
	for _, typ := range types {
		putDir(typeDirOffset[typ], len(byType[typ]))
		for j, r := range byType[typ] {
			putEntry(typeDirOffset[typ]+dirSize+entrySize*j, r.id, langDirOffset[index], true)
			index++
		}
	}
	var relocations []int
	for i, r := range resources {
		putDir(langDirOffset[i], 1)
		putEntry(langDirOffset[i]+dirSize, langEnUS, dataEntryOffset[i], false)
		// IMAGE_RESOURCE_DATA_ENTRY: the RVA field holds the offset within the section;
		// the relocation adds the section's RVA at link time.
		binary.LittleEndian.PutUint32(section[dataEntryOffset[i]:], uint32(dataOffset[i]))
		binary.LittleEndian.PutUint32(section[dataEntryOffset[i]+4:], uint32(len(r.data)))
		relocations = append(relocations, dataEntryOffset[i])
		copy(section[dataOffset[i]:], r.data)
	}

	// COFF: file header, one section header, raw data, relocations, symbol table, string
	// table.
	const fileHeaderSize, sectionHeaderSize, relocationSize = 20, 40, 10
	rawPointer := fileHeaderSize + sectionHeaderSize
	relocPointer := rawPointer + len(section)
	symbolPointer := relocPointer + relocationSize*len(relocations)

	var out bytes.Buffer
	binary.Write(&out, binary.LittleEndian, struct {
		Machine              uint16
		NumberOfSections     uint16
		TimeDateStamp        uint32
		PointerToSymbolTable uint32
		NumberOfSymbols      uint32
		SizeOfOptionalHeader uint16
		Characteristics      uint16
	}{machine, 1, 0, uint32(symbolPointer), 1, 0, 0})
	binary.Write(&out, binary.LittleEndian, struct {
		Name                 [8]byte
		VirtualSize          uint32
		VirtualAddress       uint32
		SizeOfRawData        uint32
		PointerToRawData     uint32
		PointerToRelocations uint32
		PointerToLinenumbers uint32
		NumberOfRelocations  uint16
		NumberOfLinenumbers  uint16
		Characteristics      uint32
	}{
		Name:                 [8]byte{'.', 'r', 's', 'r', 'c'},
		SizeOfRawData:        uint32(len(section)),
		PointerToRawData:     uint32(rawPointer),
		PointerToRelocations: uint32(relocPointer),
		NumberOfRelocations:  uint16(len(relocations)),
		// IMAGE_SCN_CNT_INITIALIZED_DATA | IMAGE_SCN_MEM_READ
		Characteristics: 0x40000040,
	})
	out.Write(section)
	for _, at := range relocations {
		binary.Write(&out, binary.LittleEndian, uint32(at)) // VirtualAddress (section offset)
		binary.Write(&out, binary.LittleEndian, uint32(0))  // symbol 0: the section symbol
		binary.Write(&out, binary.LittleEndian, relocType)
	}
	// The section symbol: ".rsrc", value 0, section 1, type 0, IMAGE_SYM_CLASS_STATIC.
	out.Write([]byte{'.', 'r', 's', 'r', 'c', 0, 0, 0})
	binary.Write(&out, binary.LittleEndian, uint32(0))
	binary.Write(&out, binary.LittleEndian, int16(1))
	binary.Write(&out, binary.LittleEndian, uint16(0))
	out.Write([]byte{3, 0})
	// Empty string table: just its own size.
	binary.Write(&out, binary.LittleEndian, uint32(4))
	return out.Bytes(), nil
}

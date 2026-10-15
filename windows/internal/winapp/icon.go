package winapp

import (
	"embed"
	"encoding/binary"
	"fmt"
)

// The tray icon, generated from Resources/icon-1024.png by tools/mkico (go generate).
//
//go:generate go run ../../tools/mkico
//go:embed assets/app.ico assets/app-off.ico
var assets embed.FS

// IconImage is one image of an .ico file.
type IconImage struct {
	Width, Height int
	Data          []byte // PNG or DIB, as CreateIconFromResourceEx takes it
}

// ParseICO reads the ICONDIR of an .ico file.
func ParseICO(data []byte) ([]IconImage, error) {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:]) != 0 || binary.LittleEndian.Uint16(data[2:]) != 1 {
		return nil, fmt.Errorf("not an .ico file")
	}
	count := int(binary.LittleEndian.Uint16(data[4:]))
	if len(data) < 6+16*count {
		return nil, fmt.Errorf("truncated .ico directory")
	}
	images := make([]IconImage, 0, count)
	for i := 0; i < count; i++ {
		entry := data[6+16*i:]
		width, height := int(entry[0]), int(entry[1])
		if width == 0 {
			width = 256
		}
		if height == 0 {
			height = 256
		}
		size := int(binary.LittleEndian.Uint32(entry[8:]))
		offset := int(binary.LittleEndian.Uint32(entry[12:]))
		if size <= 0 || offset < 0 || offset+size > len(data) {
			return nil, fmt.Errorf("image %d lies outside the file", i)
		}
		images = append(images, IconImage{Width: width, Height: height, Data: data[offset : offset+size]})
	}
	return images, nil
}

// BestIcon picks the smallest image at least size pixels wide, or the largest one.
func BestIcon(images []IconImage, size int) (IconImage, bool) {
	var best IconImage
	found := false
	for _, image := range images {
		switch {
		case !found:
			best, found = image, true
		case best.Width < size && image.Width > best.Width:
			best = image
		case image.Width >= size && image.Width < best.Width:
			best = image
		}
	}
	return best, found
}

// IconAsset returns an embedded .ico by name ("app.ico", "app-off.ico").
func IconAsset(name string) ([]byte, error) { return assets.ReadFile("assets/" + name) }

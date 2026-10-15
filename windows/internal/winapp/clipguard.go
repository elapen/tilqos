package winapp

// The clipboard rules, apart from the Win32 calls that feed them, so they run (and are tested)
// on any OS. clipboard_windows.go implements clipboardSource over the real clipboard; the
// tests implement it in memory.
//
// The rules (see core/clipboardpolicy.go): content flagged as not to be monitored, not for the
// clipboard history or not for the cloud clipboard is never read, converted or overwritten;
// text over core.MaxClipboardText characters is ignored; what was on the clipboard before is
// saved in full and put back, or the clipboard is not borrowed at all.

import (
	"strings"
	"unicode/utf16"

	"tilqos/internal/core"
)

// Standard clipboard formats the rules need to know by number.
const (
	cfBitmap          = 2
	cfMetafilePict    = 3
	cfPalette         = 9
	cfUnicodeText     = 13
	cfEnhMetafile     = 14
	cfOwnerDisplay    = 0x0080
	cfDspBitmap       = 0x0082
	cfDspMetafilePict = 0x0083
	cfDspEnhMetafile  = 0x008E
)

// clipFormat is one saved clipboard format.
type clipFormat struct {
	format uint32
	data   []byte
}

// clipboardSaveLimit caps what is kept for restoring. A clipboard bigger than this is not
// borrowed at all: it could not be put back faithfully.
const clipboardSaveLimit = 32 << 20

// historyExclusions keep a temporary clipboard entry (the paste of a conversion) out of
// Windows clipboard history (Win+V), the cloud clipboard and clipboard monitors: the same
// formats the rules look for on content that is not ours.
var historyExclusions = core.ClipboardFlagFormats

// clipFormatInfo names one format on the clipboard.
type clipFormatInfo struct {
	id uint32
	// name is set for registered formats (id >= 0xC000) only.
	name string
}

// clipboardSource is the part of an open clipboard the rules need.
type clipboardSource interface {
	// formats lists the formats in enumeration order.
	formats() []clipFormatInfo
	// data returns the bytes of a memory-block format.
	data(id uint32) ([]byte, bool)
}

// skipFormat excludes formats that are not plain memory blocks — GDI handles, metafile
// pictures, owner-display, private per-process handles — and OLE formats whose data points
// at objects that die with the source application.
func skipFormat(format uint32, name string) bool {
	switch format {
	case cfBitmap, cfMetafilePict, cfPalette, cfEnhMetafile,
		cfOwnerDisplay, cfDspBitmap, cfDspMetafilePict, cfDspEnhMetafile:
		return true
	}
	if format >= 0x0200 && format <= 0x03FF { // CF_PRIVATEFIRST…CF_GDIOBJLAST
		return true
	}
	switch name {
	case "DataObject", "Ole Private Data", "Object Descriptor", "Link Source Descriptor",
		"Embed Source", "Link Source", "Embedded Object", "OwnerLink", "ObjectLink", "Native":
		return true
	}
	return false
}

// clipboardSensitive reports whether whoever put the content on the clipboard flagged it as
// not to be monitored, kept in the history or synced. Only the three small flag formats are
// read; the content itself is not.
func clipboardSensitive(src clipboardSource) bool {
	var flags []core.ClipboardFlag
	for _, format := range src.formats() {
		for _, name := range core.ClipboardFlagFormats {
			if strings.EqualFold(format.name, name) {
				data, _ := src.data(format.id)
				flags = append(flags, core.ClipboardFlag{Name: format.name, Data: data})
			}
		}
	}
	return core.ClipboardSensitive(flags)
}

// clipReading is the outcome of asking for the clipboard's text.
type clipReading int

const (
	// clipText: there is text and it may be used.
	clipText clipReading = iota
	// clipEmpty: no text on the clipboard.
	clipEmpty
	// clipSensitive: flagged; nothing was read.
	clipSensitive
	// clipTooLong: more than core.MaxClipboardText characters; the text was dropped.
	clipTooLong
)

// readGuardedText returns the clipboard's text if the rules allow it.
func readGuardedText(src clipboardSource) (string, clipReading) {
	if clipboardSensitive(src) {
		return "", clipSensitive
	}
	data, ok := src.data(cfUnicodeText)
	if !ok {
		return "", clipEmpty
	}
	text := decodeUTF16Text(data)
	if text == "" {
		return "", clipEmpty
	}
	if core.ExceedsClipboardLimit(text) {
		return "", clipTooLong
	}
	return text, clipText
}

// saveGuarded copies every restorable format so the clipboard can be put back. ok is false —
// and nothing was copied — when the content is flagged as sensitive or too big to restore.
func saveGuarded(src clipboardSource, limit int) (saved []clipFormat, ok bool) {
	if clipboardSensitive(src) {
		return nil, false
	}
	total := 0
	for _, format := range src.formats() {
		if skipFormat(format.id, format.name) {
			continue
		}
		data, found := src.data(format.id)
		if !found || data == nil {
			continue
		}
		total += len(data)
		if total > limit {
			return nil, false
		}
		saved = append(saved, clipFormat{format: format.id, data: data})
	}
	return saved, true
}

// decodeUTF16Text reads a NUL-terminated UTF-16 string out of clipboard bytes.
func decodeUTF16Text(data []byte) string {
	units := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		unit := uint16(data[i]) | uint16(data[i+1])<<8
		if unit == 0 {
			break
		}
		units = append(units, unit)
	}
	return string(utf16.Decode(units))
}

package core

// The rules for touching the clipboard (the counterpart of Apply/ClipboardBoard.swift's
// ClipboardPolicy). The Windows side reads the clipboard; these functions decide what may be
// done with what it found, and are tested on any OS.
//
// The app uses the clipboard only when the user asks for a conversion that needs it (convert
// clipboard, transliterate, or fixing a selection that cannot be read directly). Then:
//   - Content flagged by whoever put it there as not to be monitored, not for the clipboard
//     history and not for the cloud clipboard (password managers, one-time codes) is never read
//     and never converted, and is not touched either.
//   - Text longer than MaxClipboardText characters is ignored: not converted, not kept.
//   - What was on the clipboard before is saved in full and put back.

import (
	"encoding/binary"
	"strings"
	"unicode/utf8"
)

// MaxClipboardText is the longest text, in Unicode characters, that is read for conversion.
const MaxClipboardText = 10_000

// The registered clipboard formats with which an application says that its clipboard content
// is sensitive (Microsoft's "Clipboard formats for opting out of history and sync").
const (
	// FormatExcludeFromMonitor: if the format is present at all, clipboard monitors ignore
	// the content.
	FormatExcludeFromMonitor = "ExcludeClipboardContentFromMonitorProcessing"
	// FormatCanIncludeInHistory: a DWORD; 0 keeps the content out of the clipboard history.
	FormatCanIncludeInHistory = "CanIncludeInClipboardHistory"
	// FormatCanUploadToCloud: a DWORD; 0 keeps the content out of the cloud clipboard.
	FormatCanUploadToCloud = "CanUploadToCloudClipboard"
)

// ClipboardFlagFormats lists the formats whose presence and value ClipboardSensitive wants.
var ClipboardFlagFormats = []string{
	FormatExcludeFromMonitor, FormatCanIncludeInHistory, FormatCanUploadToCloud,
}

// ClipboardFlag is one of the flag formats found on the clipboard, with its data.
type ClipboardFlag struct {
	Name string
	Data []byte
}

// ClipboardSensitive reports whether the clipboard content is flagged as one that must not be
// read, converted or overwritten.
func ClipboardSensitive(flags []ClipboardFlag) bool {
	for _, flag := range flags {
		switch {
		case strings.EqualFold(flag.Name, FormatExcludeFromMonitor):
			return true // present is enough, whatever it holds
		case strings.EqualFold(flag.Name, FormatCanIncludeInHistory),
			strings.EqualFold(flag.Name, FormatCanUploadToCloud):
			if dwordIsZero(flag.Data) {
				return true
			}
		}
	}
	return false
}

// dwordIsZero: a flag that cannot be read as a DWORD counts as 0, the cautious reading.
func dwordIsZero(data []byte) bool {
	if len(data) < 4 {
		return true
	}
	return binary.LittleEndian.Uint32(data) == 0
}

// ExceedsClipboardLimit reports whether text is longer than MaxClipboardText characters.
// Cheap for huge strings: four bytes bound a character.
func ExceedsClipboardLimit(text string) bool {
	if len(text) > 4*MaxClipboardText {
		return true
	}
	return utf8.RuneCountInString(text) > MaxClipboardText
}

package winapp

import (
	"fmt"

	"tilqos/internal/core"
)

// Primary language IDs (the low 10 bits of a LANGID).
const (
	langChinese  = 0x04
	langEnglish  = 0x09
	langJapanese = 0x11
	langKorean   = 0x12
	langRussian  = 0x19
	langKazakh   = 0x3F
)

// LayoutID is the stable name of an input locale: the 32 meaningful bits of its HKL in
// hex ("04190419" for Russian, "F0020409" for a US variant). 64-bit HKLs of variants are
// sign-extended, so only the low half identifies them.
func LayoutID(hkl uintptr) string { return fmt.Sprintf("%08X", uint32(hkl)) }

// LangForHKL maps an input locale onto a modelled language by the input language (the low
// word of the HKL): English 0x09, Russian 0x19, Kazakh 0x3F — whatever the region.
func LangForHKL(hkl uintptr) core.Lang {
	switch uint16(hkl) & 0x3FF {
	case langEnglish:
		return core.LangEN
	case langRussian:
		return core.LangRU
	case langKazakh:
		return core.LangKK
	}
	return core.LangNone
}

// IsIMELayout reports input locales whose text comes from an input method rather than
// straight from the keys (Chinese, Japanese, Korean, legacy IMM IMEs). Keys typed there
// cannot be re-rendered, so the buffer is not kept for them.
func IsIMELayout(hkl uintptr) bool {
	switch uint16(hkl) & 0x3FF {
	case langChinese, langJapanese, langKorean:
		return true
	}
	return (uint32(hkl)>>28)&0xF == 0xE
}

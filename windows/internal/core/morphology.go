package core

// Port of Detect/Morphology.swift: the kk-morph.bin format, the FNV-1a stem hash and the
// suffix-stripping decomposition.

import (
	"encoding/binary"
	"fmt"
	"sort"
	"strings"
)

// Morphology checks whether a Kazakh word is a well-formed inflected form.
//
// Kazakh is agglutinative: `десеңіздер` is `десе + ңіз + дер`, three morphemes deep. A
// character model cannot express that. Used as evidence, never as a filter: a word that
// fails to decompose is merely unsupported, not wrong.
type Morphology struct {
	// stems holds hashes of the base word list, sorted for binary search.
	stems []uint32
	// suffixes maps a suffix to the letters it puts back on the stem (usually none).
	suffixes        map[string][]string
	maxSuffixLength int
}

const (
	morphologyMagic      uint32 = 0x4C53_4D4F // "LSMO"
	morphologyVersion    uint16 = 1
	morphologyHeaderSize        = 20
	// DefaultMorphologyDepth: how many suffixes may be peeled off. `десе + ңіз + дер` is
	// already at the limit.
	DefaultMorphologyDepth = 3
)

// NewMorphology sorts the stems and measures the longest suffix, like the Swift init.
func NewMorphology(stems []uint32, suffixes map[string][]string) *Morphology {
	sorted := append([]uint32(nil), stems...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	normalized := make(map[string][]string, len(suffixes))
	maxLength := 0
	for affix, strippings := range suffixes {
		normalized[Canon(affix)] = strippings
		if n := CharCount(affix); n > maxLength {
			maxLength = n
		}
	}
	return &Morphology{stems: sorted, suffixes: normalized, maxSuffixLength: maxLength}
}

func (m *Morphology) StemCount() int   { return len(m.stems) }
func (m *Morphology) SuffixCount() int { return len(m.suffixes) }

// MorphologyHash is FNV-1a over the lowercased UTF-8 bytes.
func MorphologyHash(word string) uint32 {
	value := uint32(0x811C_9DC5)
	for _, b := range []byte(Lower(word)) {
		value ^= uint32(b)
		value *= 0x0100_0193
	}
	return value
}

// IsStem looks the word's hash up in the stem list.
func (m *Morphology) IsStem(word string) bool {
	target := MorphologyHash(word)
	low, high := 0, len(m.stems)-1
	for low <= high {
		mid := (low + high) / 2
		if m.stems[mid] == target {
			return true
		}
		if m.stems[mid] < target {
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return false
}

// Decomposes is true when the word is a stem, or a stem plus up to depth suffixes.
func (m *Morphology) Decomposes(word string, depth int) bool {
	_, _, ok := m.decompose(Graphemes(Lower(word)), depth)
	return ok
}

// Decomposition returns the stem and the suffixes peeled off it, outermost last.
func (m *Morphology) Decomposition(word string, depth int) (stem string, suffixes []string, ok bool) {
	return m.decompose(Graphemes(Lower(word)), depth)
}

func (m *Morphology) decompose(characters []string, depth int) (string, []string, bool) {
	word := strings.Join(characters, "")
	if m.IsStem(word) {
		return word, []string{}, true
	}
	if depth <= 0 || len(characters) <= 1 {
		return "", nil, false
	}

	// Longest suffix first: a longer match is a more specific rule, and it cuts the search.
	longest := m.maxSuffixLength
	if len(characters)-1 < longest {
		longest = len(characters) - 1
	}
	for length := longest; length >= 1; length-- {
		tail := strings.Join(characters[len(characters)-length:], "")
		strippings, found := m.suffixes[Canon(tail)]
		if !found {
			continue
		}
		head := strings.Join(characters[:len(characters)-length], "")
		for _, stripping := range strippings {
			base := head + stripping
			if base == "" {
				continue
			}
			if stem, suffixes, ok := m.decompose(Graphemes(base), depth-1); ok {
				return stem, append(suffixes, tail), true
			}
		}
	}
	return "", nil, false
}

// Encode serialises in the Swift format.
func (m *Morphology) Encode() []byte {
	data := make([]byte, morphologyHeaderSize, morphologyHeaderSize+4*len(m.stems))
	binary.LittleEndian.PutUint32(data[0:], morphologyMagic)
	binary.LittleEndian.PutUint16(data[4:], morphologyVersion)
	binary.LittleEndian.PutUint32(data[8:], uint32(len(m.stems)))
	binary.LittleEndian.PutUint32(data[12:], uint32(len(m.suffixes)))
	binary.LittleEndian.PutUint16(data[16:], uint16(m.maxSuffixLength))
	var word [4]byte
	for _, stem := range m.stems {
		binary.LittleEndian.PutUint32(word[:], stem)
		data = append(data, word[:]...)
	}
	affixes := make([]string, 0, len(m.suffixes))
	for affix := range m.suffixes {
		affixes = append(affixes, affix)
	}
	sort.Strings(affixes)
	for _, affix := range affixes {
		data = append(data, byte(len(affix)))
		data = append(data, affix...)
		data = append(data, byte(len(m.suffixes[affix])))
		for _, stripping := range m.suffixes[affix] {
			data = append(data, byte(len(stripping)))
			data = append(data, stripping...)
		}
	}
	return data
}

// DecodeMorphology reads kk-morph.bin.
func DecodeMorphology(data []byte) (*Morphology, error) {
	if len(data) < morphologyHeaderSize {
		return nil, fmt.Errorf("morphology file is shorter than its header")
	}
	if binary.LittleEndian.Uint32(data[0:]) != morphologyMagic {
		return nil, fmt.Errorf("not a Tilqos morphology file")
	}
	if version := binary.LittleEndian.Uint16(data[4:]); version != morphologyVersion {
		return nil, fmt.Errorf("unsupported morphology version %d", version)
	}
	stemCount := int(binary.LittleEndian.Uint32(data[8:]))
	suffixCount := int(binary.LittleEndian.Uint32(data[12:]))

	truncated := fmt.Errorf("morphology file ends mid-record")
	offset := morphologyHeaderSize
	if stemCount < 0 || len(data) < offset+stemCount*4 {
		return nil, truncated
	}
	stems := make([]uint32, stemCount)
	for i := range stems {
		stems[i] = binary.LittleEndian.Uint32(data[offset:])
		offset += 4
	}

	suffixes := make(map[string][]string, suffixCount)
	for i := 0; i < suffixCount; i++ {
		if offset >= len(data) {
			return nil, truncated
		}
		affixLength := int(data[offset])
		offset++
		if offset+affixLength > len(data) {
			return nil, truncated
		}
		affix := string(data[offset : offset+affixLength])
		offset += affixLength

		if offset >= len(data) {
			return nil, truncated
		}
		strippingCount := int(data[offset])
		offset++
		strippings := make([]string, 0, strippingCount)
		for j := 0; j < strippingCount; j++ {
			if offset >= len(data) {
				return nil, truncated
			}
			length := int(data[offset])
			offset++
			if offset+length > len(data) {
				return nil, truncated
			}
			strippings = append(strippings, string(data[offset:offset+length]))
			offset += length
		}
		suffixes[affix] = strippings
	}
	return NewMorphology(stems, suffixes), nil
}

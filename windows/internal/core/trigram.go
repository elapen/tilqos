package core

// Port of Detect/TrigramModel.swift. Same binary format, same quantisation, same scoring.

import (
	"encoding/binary"
	"fmt"
)

// TrigramScore is how well a word fits one language, judged on character trigrams.
type TrigramScore struct {
	// MeanCost is the average cost in bits per trigram. Lower fits better.
	MeanCost float64
	// InteriorUnknownCount counts off-alphabet characters with a letter of this script on
	// *both* sides.
	//
	// This, not the raw count, is the signal. A digit wedged between Cyrillic letters
	// (`с2лем`) means Kazakh was typed on the Russian layout. Digits hanging off the end of
	// a word (`iPhone15`) mean nothing at all.
	InteriorUnknownCount int
	// UnknownCount is every character that is not a letter of this script.
	UnknownCount int
	Length       int
}

// PenalizedCost is the cost with interior off-alphabet characters charged for. Priced well
// above any improbability the trigrams alone could express, because it is a structural
// fact rather than a statistical hunch.
func (s TrigramScore) PenalizedCost() float64 {
	return s.MeanCost + float64(s.InteriorUnknownCount)*UnknownCharacterCost
}

const (
	trigramMagic   uint32 = 0x4C53_5447 // "LSTG"
	trigramVersion uint16 = 1
	// CostScale is the quantisation step: stored value = cost in bits × 12, clamped to a byte.
	CostScale = 12.0
	// UnknownCharacterCost is charged once per character that is not a letter of the
	// model's script.
	UnknownCharacterCost = 6.0
	// MinimumScorableLength: words shorter than this carry too little structure to judge.
	MinimumScorableLength = 2
	// TrigramHeaderSize: magic(4) version(2) kind(1) reserved(1) lang(4, NUL-padded)
	// symbolCount(2) reserved(2) tableSize(4), little-endian, then tableSize cost bytes.
	TrigramHeaderSize = 20
)

// TrigramModel is a character trigram language model.
type TrigramModel struct {
	Lang     Lang
	Alphabet *Alphabet
	// costs is the flat cube, (a*symbolCount + b)*symbolCount + c.
	costs []byte
}

// NewTrigramModel checks the table size like the Swift precondition does.
func NewTrigramModel(lang Lang, alphabet *Alphabet, costs []byte) (*TrigramModel, error) {
	size := alphabet.SymbolCount()
	if len(costs) != size*size*size {
		return nil, fmt.Errorf("cost table must be symbolCount cubed (%d), got %d", size*size*size, len(costs))
	}
	return &TrigramModel{Lang: lang, Alphabet: alphabet, costs: costs}, nil
}

// Score judges a word; ok is false for words too short to judge.
func (m *TrigramModel) Score(word string) (TrigramScore, bool) {
	lowered := Lower(word)
	length := CharCount(lowered)
	if length < MinimumScorableLength {
		return TrigramScore{}, false
	}

	symbols, unknown := m.Alphabet.Encode(lowered)
	boundary := m.Alphabet.Boundary()
	size := m.Alphabet.SymbolCount()

	// Two leading boundaries so the very first letter is conditioned on "start of word",
	// and one trailing so word endings are modelled too.
	padded := make([]int, 0, len(symbols)+3)
	padded = append(padded, boundary, boundary)
	padded = append(padded, symbols...)
	padded = append(padded, boundary)

	total := 0
	count := 0
	for index := 2; index < len(padded); index++ {
		a, b, c := padded[index-2], padded[index-1], padded[index]
		total += int(m.costs[(a*size+b)*size+c])
		count++
	}
	if count == 0 {
		return TrigramScore{}, false
	}
	return TrigramScore{
		MeanCost:             float64(total) / float64(count) / CostScale,
		InteriorUnknownCount: m.interiorUnknownCount(lowered),
		UnknownCount:         unknown,
		Length:               length,
	}, true
}

// Symbol returns the symbol index for a Character, or false when it is not a letter of
// this script.
func (m *TrigramModel) Symbol(c string) (int, bool) { return m.Alphabet.Index(c) }

func (m *TrigramModel) BoundarySymbol() int { return m.Alphabet.Boundary() }

// Cost is the cost in bits of c appearing after a, b. Exposed so a decoder can score one
// letter at a time.
func (m *TrigramModel) Cost(a, b, c int) float64 {
	size := m.Alphabet.SymbolCount()
	if a >= size || b >= size || c >= size {
		return UnknownCharacterCost
	}
	return float64(m.costs[(a*size+b)*size+c]) / CostScale
}

// interiorUnknownCount counts off-alphabet characters that have a letter of this script on
// both sides.
func (m *TrigramModel) interiorUnknownCount(lowered string) int {
	characters := Graphemes(lowered)
	count := 0
	for position, c := range characters {
		if _, ok := m.Alphabet.Index(c); ok {
			continue
		}
		letterBefore := false
		if position > 0 {
			_, letterBefore = m.Alphabet.Index(characters[position-1])
		}
		letterAfter := false
		if position+1 < len(characters) {
			_, letterAfter = m.Alphabet.Index(characters[position+1])
		}
		if letterBefore && letterAfter {
			count++
		}
	}
	return count
}

// Encode serialises the model in the Swift format.
func (m *TrigramModel) Encode() []byte {
	data := make([]byte, TrigramHeaderSize, TrigramHeaderSize+len(m.costs))
	binary.LittleEndian.PutUint32(data[0:], trigramMagic)
	binary.LittleEndian.PutUint16(data[4:], trigramVersion)
	data[6] = byte(m.Alphabet.Kind)
	copy(data[8:12], []byte(m.Lang))
	binary.LittleEndian.PutUint16(data[12:], uint16(m.Alphabet.SymbolCount()))
	binary.LittleEndian.PutUint32(data[16:], uint32(len(m.costs)))
	return append(data, m.costs...)
}

// DecodeTrigramModel reads the Swift format, rejecting anything malformed.
func DecodeTrigramModel(data []byte) (*TrigramModel, error) {
	if len(data) < TrigramHeaderSize {
		return nil, fmt.Errorf("model file is shorter than its header")
	}
	if binary.LittleEndian.Uint32(data[0:]) != trigramMagic {
		return nil, fmt.Errorf("not a Tilqos trigram model")
	}
	if version := binary.LittleEndian.Uint16(data[4:]); version != trigramVersion {
		return nil, fmt.Errorf("unsupported model version %d", version)
	}
	if data[6] != byte(AlphabetLatin) && data[6] != byte(AlphabetCyrillic) {
		return nil, fmt.Errorf("not a Tilqos trigram model")
	}
	alphabet := AlphabetForKind(AlphabetKind(data[6]))

	codeBytes := data[8:12]
	for i, b := range codeBytes {
		if b == 0 {
			codeBytes = codeBytes[:i]
			break
		}
	}
	code := string(codeBytes)
	lang := Lang(code)
	if lang != LangEN && lang != LangRU && lang != LangKK {
		return nil, fmt.Errorf("unknown language code '%s'", code)
	}

	tableSize := int(binary.LittleEndian.Uint32(data[16:]))
	expected := alphabet.SymbolCount() * alphabet.SymbolCount() * alphabet.SymbolCount()
	if tableSize != expected || len(data) != TrigramHeaderSize+tableSize {
		return nil, fmt.Errorf("expected %d cost bytes, found %d", expected, len(data)-TrigramHeaderSize)
	}
	costs := make([]byte, tableSize)
	copy(costs, data[TrigramHeaderSize:])
	return NewTrigramModel(lang, alphabet, costs)
}

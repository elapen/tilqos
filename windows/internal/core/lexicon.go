package core

// Port of Detect/Lexicon.swift: decoding and lookup. The builder (hunspell .dic/.aff to
// .bin) stays in the Swift GenModel tool; the golden data carries lexicons it encoded.

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Lexicon is which words exist in a language, every inflected form included.
//
// Stored the way hunspell stores it — stems with affix flags, plus the affix rules — rather
// than as expanded forms. Lookup reverses the rules: strip a candidate ending, restore what
// the rule removed, check the condition, and ask whether that stem exists and carries the
// rule's flag.
//
// Stems are 32-bit FNV-1a hashes, so the word list cannot be read back out of the file. A
// collision can only make an unknown string look known, which costs a missed correction —
// the safe direction. Immutable once decoded; safe to share between goroutines.
type Lexicon struct {
	Lang Lang
	// flagSets are affix-flag bitmasks, interned.
	flagSets []uint64
	// stemHashes is sorted, for binary search; stemFlagSets is parallel to it.
	stemHashes   []uint32
	stemFlagSets []uint16
	suffixes     []affixRule
	prefixes     []affixRule
	// core is the language's most frequent words, hashed and sorted.
	core []uint32

	suffixIndex     map[uint32][]int
	prefixIndex     map[uint32][]int
	maxSuffixLength int
	maxPrefixLength int
}

const (
	lexiconMagic      uint32 = 0x4C534C58 // "LSLX"
	lexiconVersion    uint16 = 1
	lexiconHeaderSize        = 36
	fnvOffset32       uint32 = 0x811C9DC5
	fnvPrime32        uint32 = 0x01000193
)

func newLexicon(lang Lang, flagSets []uint64, stemHashes []uint32, stemFlagSets []uint16,
	suffixes, prefixes []affixRule, core []uint32) *Lexicon {
	l := &Lexicon{
		Lang: lang, flagSets: flagSets, stemHashes: stemHashes, stemFlagSets: stemFlagSets,
		suffixes: suffixes, prefixes: prefixes, core: core,
		suffixIndex: map[uint32][]int{}, prefixIndex: map[uint32][]int{},
	}
	for index, rule := range suffixes {
		key := lexiconHash(rule.affix)
		l.suffixIndex[key] = append(l.suffixIndex[key], index)
		l.maxSuffixLength = max(l.maxSuffixLength, len(rule.affix))
	}
	for index, rule := range prefixes {
		key := lexiconHash(rule.affix)
		l.prefixIndex[key] = append(l.prefixIndex[key], index)
		l.maxPrefixLength = max(l.maxPrefixLength, len(rule.affix))
	}
	return l
}

func (l *Lexicon) StemCount() int { return len(l.stemHashes) }
func (l *Lexicon) RuleCount() int { return len(l.suffixes) + len(l.prefixes) }
func (l *Lexicon) CoreCount() int { return len(l.core) }

// MARK: - Normalization and hashing

// NormalizeWord is Lexicon.normalize: lowercase, ё folded into е, the typographic
// apostrophe folded into ASCII. People type `еще` for `ещё` far more often than not, so the
// two are one word here.
func NormalizeWord(word string) []rune {
	lowered := Lower(word)
	result := make([]rune, 0, len(lowered))
	for _, r := range lowered {
		switch r {
		case 'ё':
			result = append(result, 'е')
		case '’':
			result = append(result, '\'')
		default:
			result = append(result, r)
		}
	}
	return result
}

// LexiconHash is FNV-1a over the UTF-8 encoding of the scalars — the hash the lexicon files
// store.
func LexiconHash(scalars []rune) uint32 { return lexiconHash(scalars) }

func lexiconHash(scalars []rune) uint32 {
	value := fnvOffset32
	for _, scalar := range scalars {
		value = fnvStep(value, uint32(scalar))
	}
	return value
}

// fnvStep feeds one scalar's UTF-8 bytes into a running FNV-1a hash, so a hash can be
// extended without building the string it describes.
func fnvStep(value, code uint32) uint32 {
	mix := func(b uint32) { value = (value ^ b) * fnvPrime32 }
	switch {
	case code < 0x80:
		mix(code)
	case code < 0x800:
		mix(0xC0 | code>>6)
		mix(0x80 | code&0x3F)
	case code < 0x10000:
		mix(0xE0 | code>>12)
		mix(0x80 | code>>6&0x3F)
		mix(0x80 | code&0x3F)
	default:
		mix(0xF0 | code>>18)
		mix(0x80 | code>>12&0x3F)
		mix(0x80 | code>>6&0x3F)
		mix(0x80 | code&0x3F)
	}
	return value
}

// MARK: - Lookup

// Contains: the word is a dictionary form, or one the affix rules derive from one.
func (l *Lexicon) Contains(word string) bool {
	known, _ := l.Lookup(word)
	return known
}

// IsCore: the word is among the language's most frequent.
func (l *Lexicon) IsCore(word string) bool {
	_, found := binarySearch32(l.core, lexiconHash(NormalizeWord(word)))
	return found
}

// Lookup gives both answers from one normalization: whether the word exists, and whether it
// is among the most frequent. The detector always wants both.
func (l *Lexicon) Lookup(word string) (known, core bool) {
	normalized := NormalizeWord(word)
	if len(normalized) == 0 {
		return false, false
	}
	wordHash := lexiconHash(normalized)
	_, isCore := binarySearch32(l.core, wordHash)
	if _, isStem := l.flags(wordHash); isCore || isStem {
		return true, isCore
	}
	return l.containsNormalized(normalized), false
}

func (l *Lexicon) containsNormalized(word []rune) bool {
	if len(word) == 0 {
		return false
	}
	if _, ok := l.flags(lexiconHash(word)); ok {
		return true
	}
	if l.matchesSuffix(word, -1) {
		return true
	}
	return l.matchesPrefix(word)
}

func (l *Lexicon) flags(stemHash uint32) (uint64, bool) {
	index, ok := binarySearch32(l.stemHashes, stemHash)
	if !ok {
		return 0, false
	}
	return l.flagSets[l.stemFlagSets[index]], true
}

// matchesSuffix reverses one suffix rule. With prefixFlag >= 0, only cross-product rules
// count and the stem must also carry the prefix flag — the word had a prefix stripped
// already.
//
// The candidate stem is never built: its hash is the running hash of the word's first part
// continued over the rule's strip string, and the condition is checked across the same
// virtual concatenation.
func (l *Lexicon) matchesSuffix(word []rune, prefixFlag int) bool {
	count := len(word)
	if count == 0 {
		return false
	}
	prefixHashes := make([]uint32, count+1)
	prefixHashes[0] = fnvOffset32
	for index := 0; index < count; index++ {
		prefixHashes[index+1] = fnvStep(prefixHashes[index], uint32(word[index]))
	}

	for length := 0; length <= min(l.maxSuffixLength, count); length++ {
		base := count - length
		endingHash := fnvOffset32
		for index := base; index < count; index++ {
			endingHash = fnvStep(endingHash, uint32(word[index]))
		}
		candidates, ok := l.suffixIndex[endingHash]
		if !ok {
			continue
		}
	candidateLoop:
		for _, index := range candidates {
			rule := &l.suffixes[index]
			if prefixFlag >= 0 && !rule.cross {
				continue
			}
			if len(rule.affix) != length {
				continue
			}
			for offset := 0; offset < length; offset++ {
				if rule.affix[offset] != word[base+offset] {
					continue candidateLoop
				}
			}
			strip := rule.strip
			if base+len(strip) <= 0 || !rule.matchesEnd(word, base, strip) {
				continue
			}
			stemHash := prefixHashes[base]
			for _, value := range strip {
				stemHash = fnvStep(stemHash, uint32(value))
			}
			mask, found := l.flags(stemHash)
			if !found || mask&(1<<uint64(rule.flag)) == 0 {
				continue
			}
			if prefixFlag >= 0 && mask&(1<<uint64(prefixFlag)) == 0 {
				continue
			}
			return true
		}
	}
	return false
}

// matchesPrefix handles prefix rules — English only, and far fewer of them, so this path
// builds stems plainly.
func (l *Lexicon) matchesPrefix(word []rune) bool {
	if l.maxPrefixLength <= 0 {
		return false
	}
	for length := 1; length <= min(l.maxPrefixLength, len(word)); length++ {
		start := word[:length]
		candidates, ok := l.prefixIndex[lexiconHash(start)]
		if !ok {
			continue
		}
		for _, index := range candidates {
			rule := &l.prefixes[index]
			if !runesEqual(rule.affix, start) {
				continue
			}
			stem := make([]rune, 0, len(rule.strip)+len(word)-length)
			stem = append(stem, rule.strip...)
			stem = append(stem, word[length:]...)
			if len(stem) == 0 || !rule.matchesStart(stem) {
				continue
			}
			if mask, found := l.flags(lexiconHash(stem)); found && mask&(1<<uint64(rule.flag)) != 0 {
				return true
			}
			if rule.cross && l.matchesSuffix(stem, int(rule.flag)) {
				return true
			}
		}
	}
	return false
}

func runesEqual(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// binarySearch32 is Lexicon.binarySearch.
func binarySearch32(sorted []uint32, target uint32) (int, bool) {
	low, high := 0, len(sorted)-1
	for low <= high {
		mid := (low + high) >> 1
		value := sorted[mid]
		if value == target {
			return mid, true
		}
		if value < target {
			low = mid + 1
		} else {
			high = mid - 1
		}
	}
	return 0, false
}

// MARK: - Affix rules

// affixRule is one hunspell SFX/PFX line: remove strip from the stem, add affix, provided
// the stem matches the condition (at its end for suffixes, its start for prefixes).
type affixRule struct {
	// flag is the bit index of the flag in the stems' masks.
	flag uint8
	// cross: the rule combines with rules of the other kind (hunspell's cross product).
	cross     bool
	strip     []rune
	affix     []rune
	condition string
	pattern   []charClass
}

func newAffixRule(flag uint8, cross bool, strip, affix []rune, condition string) affixRule {
	return affixRule{flag: flag, cross: cross, strip: strip, affix: affix, condition: condition,
		pattern: compileCondition(condition)}
}

// matchesEnd checks the condition against the end of word[:base] + strip without
// concatenating them.
func (r *affixRule) matchesEnd(word []rune, base int, strip []rune) bool {
	total := base + len(strip)
	if len(r.pattern) > total {
		return false
	}
	offset := total - len(r.pattern)
	for index := range r.pattern {
		position := offset + index
		var value rune
		if position < base {
			value = word[position]
		} else {
			value = strip[position-base]
		}
		if !r.pattern[index].matches(value) {
			return false
		}
	}
	return true
}

func (r *affixRule) matchesStart(stem []rune) bool {
	if len(r.pattern) > len(stem) {
		return len(r.pattern) == 0
	}
	for index := range r.pattern {
		if !r.pattern[index].matches(stem[index]) {
			return false
		}
	}
	return true
}

type charClassKind uint8

const (
	classAny charClassKind = iota
	classOne
	classOneOf
	classNoneOf
)

// charClass is one position of a hunspell condition: `.`, a letter, `[abc]` or `[^abc]`.
type charClass struct {
	kind charClassKind
	one  rune
	set  []rune
}

func (c charClass) matches(value rune) bool {
	switch c.kind {
	case classAny:
		return true
	case classOne:
		return value == c.one
	case classOneOf:
		for _, member := range c.set {
			if member == value {
				return true
			}
		}
		return false
	default:
		for _, member := range c.set {
			if member == value {
				return false
			}
		}
		return true
	}
}

// compileCondition: `.` alone means "no condition" and compiles to nothing.
func compileCondition(condition string) []charClass {
	if condition == "." {
		return nil
	}
	scalars := NormalizeWord(condition)
	var result []charClass
	for index := 0; index < len(scalars); index++ {
		scalar := scalars[index]
		switch scalar {
		case '[':
			negated := false
			var members []rune
			index++
			if index < len(scalars) && scalars[index] == '^' {
				negated = true
				index++
			}
			for index < len(scalars) && scalars[index] != ']' {
				members = append(members, scalars[index])
				index++
			}
			if negated {
				result = append(result, charClass{kind: classNoneOf, set: members})
			} else {
				result = append(result, charClass{kind: classOneOf, set: members})
			}
		case '.':
			result = append(result, charClass{kind: classAny})
		default:
			result = append(result, charClass{kind: classOne, one: scalar})
		}
	}
	return result
}

// MARK: - Decoding

// Lexicon decoding errors.
var (
	ErrLexiconTooShort  = errors.New("lexicon file is shorter than its header")
	ErrLexiconBadMagic  = errors.New("not a Tilqos lexicon")
	ErrLexiconTruncated = errors.New("lexicon file ends mid-record")
)

// DecodeLexicon reads the file GenModel writes. Layout, little-endian:
//
//	magic(4) version(2) reserved(2) lang(4, NUL-padded) stemCount(4) flagSetCount(4)
//	suffixCount(4) prefixCount(4) coreCount(4) reserved(4)
//
// then flagSets (u64 as low and high u32), stemHashes (u32 each), stemFlagSets (u16 each),
// suffix rules, prefix rules, core hashes (u32 each). A rule is flag(1) cross(1) and three
// length-prefixed UTF-8 strings: strip, affix, condition.
func DecodeLexicon(data []byte) (*Lexicon, error) {
	if len(data) < lexiconHeaderSize {
		return nil, ErrLexiconTooShort
	}
	r := &byteReader{bytes: data}
	magic, _ := r.u32()
	if magic != lexiconMagic {
		return nil, ErrLexiconBadMagic
	}
	version, _ := r.u16()
	if version != lexiconVersion {
		return nil, fmt.Errorf("unsupported lexicon version %d", version)
	}
	r.u16()
	codeBytes, _ := r.take(4)
	code := codeBytes
	for i, b := range codeBytes {
		if b == 0 {
			code = codeBytes[:i]
			break
		}
	}
	lang := Lang(string(code))
	if !isKnownLang(lang) {
		return nil, fmt.Errorf("unknown language code '%s'", string(code))
	}
	stemCount, _ := r.u32()
	flagSetCount, _ := r.u32()
	suffixCount, _ := r.u32()
	prefixCount, _ := r.u32()
	coreCount, _ := r.u32()
	r.u32()

	// Every count is bounded by the bytes left, so a corrupt header cannot make us allocate
	// gigabytes before failing.
	if uint64(flagSetCount)*8+uint64(stemCount)*6+uint64(coreCount)*4 > uint64(len(data)) {
		return nil, ErrLexiconTruncated
	}
	flagSets := make([]uint64, flagSetCount)
	for i := range flagSets {
		low, err := r.u32()
		if err != nil {
			return nil, err
		}
		high, err := r.u32()
		if err != nil {
			return nil, err
		}
		flagSets[i] = uint64(low) | uint64(high)<<32
	}
	stemHashes := make([]uint32, stemCount)
	for i := range stemHashes {
		value, err := r.u32()
		if err != nil {
			return nil, err
		}
		stemHashes[i] = value
	}
	stemFlagSets := make([]uint16, stemCount)
	for i := range stemFlagSets {
		index, err := r.u16()
		if err != nil {
			return nil, err
		}
		if uint32(index) >= flagSetCount {
			return nil, ErrLexiconTruncated
		}
		stemFlagSets[i] = index
	}
	rule := func() (affixRule, error) {
		flag, err := r.u8()
		if err != nil {
			return affixRule{}, err
		}
		cross, err := r.u8()
		if err != nil {
			return affixRule{}, err
		}
		strip, err := r.str()
		if err != nil {
			return affixRule{}, err
		}
		affix, err := r.str()
		if err != nil {
			return affixRule{}, err
		}
		condition, err := r.str()
		if err != nil {
			return affixRule{}, err
		}
		return newAffixRule(flag, cross != 0, []rune(strip), []rune(affix), condition), nil
	}
	var suffixes, prefixes []affixRule
	for i := uint32(0); i < suffixCount; i++ {
		parsed, err := rule()
		if err != nil {
			return nil, err
		}
		suffixes = append(suffixes, parsed)
	}
	for i := uint32(0); i < prefixCount; i++ {
		parsed, err := rule()
		if err != nil {
			return nil, err
		}
		prefixes = append(prefixes, parsed)
	}
	core := make([]uint32, coreCount)
	for i := range core {
		value, err := r.u32()
		if err != nil {
			return nil, err
		}
		core[i] = value
	}
	return newLexicon(lang, flagSets, stemHashes, stemFlagSets, suffixes, prefixes, core), nil
}

func isKnownLang(lang Lang) bool {
	for _, known := range AllLangs {
		if lang == known {
			return true
		}
	}
	return false
}

// byteReader is ByteReader: bounds-checked little-endian reads.
type byteReader struct {
	bytes  []byte
	offset int
}

func (r *byteReader) take(count int) ([]byte, error) {
	if count < 0 || r.offset+count > len(r.bytes) {
		return nil, ErrLexiconTruncated
	}
	out := r.bytes[r.offset : r.offset+count]
	r.offset += count
	return out, nil
}

func (r *byteReader) u8() (uint8, error) {
	b, err := r.take(1)
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

func (r *byteReader) u16() (uint16, error) {
	b, err := r.take(2)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b), nil
}

func (r *byteReader) u32() (uint32, error) {
	b, err := r.take(4)
	if err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b), nil
}

func (r *byteReader) str() (string, error) {
	length, err := r.u8()
	if err != nil {
		return "", err
	}
	b, err := r.take(int(length))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

package core

// Port of Core/Learner.swift. Persistence is left to the caller (internal/config keeps it in
// learned.json next to config.json); this type is the state machine and its encoding.

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Learner is what the app learns from the user, and the only place it is kept.
//
// Four signals, each read the way a person would read it:
//
//   - Rejection. An automatic fix undone, or erased and the original typed again — that word
//     is never converted again.
//   - Ignored offers. A suggestion dismissed without accepting it, twice for the same word —
//     stop offering it.
//   - Acceptance. The accept key on a suggestion, or the fix-last-word hotkey on a word —
//     convert it without asking next time.
//   - Vocabulary. A word no dictionary has, typed and kept in two separate sessions — it is
//     a word of that language (names, slang, jargon). That both protects it and, typed on
//     the wrong layout, makes it recognisable.
//
// Privacy: words are stored only as salted 64-bit FNV-1a hashes, never as text, and only
// words that took part in one of those signals. The salt is random per installation and is
// stored in the same file as the hashes. FNV-1a is a fast non-cryptographic hash, so this is
// obfuscation and not protection: someone who can read the file can hash a list of candidate
// words with the salt and see which of them are present (short, common words are easily
// found). That is why the store is small (3,000 entries at most), local, switchable off, and
// can be wiped with one call (ResetLearnedVocabulary). Nothing here ever leaves the machine or
// reaches the log.
//
// Safe for concurrent use: the engine asks and teaches on its goroutine, the tray reads
// counts and wipes on the UI thread.
type Learner struct {
	mu       sync.Mutex
	enabled  bool
	salt     []byte
	entries  map[uint64]learnEntry
	version  int
	now      func() time.Time
	onChange func()
	// schema is the version of the rules the entries were learned under (see
	// LearnerSchemaVersion); upgrade is what loading them under older rules dropped.
	schema  int
	upgrade *LearnerUpgrade
}

// LearnerSchemaVersion is the version of the rules stored entries were learned under.
// Entries from older, looser rules are dropped when loaded — see migrate.
const LearnerSchemaVersion = 2

const (
	// LearnerIgnoreLimit is how many ignored offers before a word stops being offered.
	LearnerIgnoreLimit = 2
	// LearnerVocabularySightings is how many sessions a dictionary-less word must be kept in
	// before it counts as a word.
	LearnerVocabularySightings = 2
	// LearnerCapacity is the most entries kept. Past this the least recently used go first.
	LearnerCapacity = 3000
)

// learnKind values are the stored integers (Swift's encode).
type learnKind int

const (
	learnKeep learnKind = iota
	learnConvert
	learnIgnored
	learnSighted
	learnKnown
)

// learnState is Swift's Learner.State.
type learnState struct {
	kind  learnKind
	lang  Lang
	count int
}

type learnEntry struct {
	state learnState
	// lastUsed is days since 2001-01-01 — precise enough for eviction, too coarse to be a
	// record of when anything was typed.
	lastUsed int
}

// NewLearner starts empty, enabled, with a fresh random salt. now may be nil (time.Now).
func NewLearner(now func() time.Time) *Learner {
	if now == nil {
		now = time.Now
	}
	return &Learner{enabled: true, salt: makeSalt(), entries: map[uint64]learnEntry{}, now: now,
		schema: LearnerSchemaVersion}
}

func makeSalt() []byte {
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand does not fail on supported platforms; a fixed salt would still work.
		panic(fmt.Sprintf("learner salt: %v", err))
	}
	return salt
}

// OnChange registers a callback run after every change (outside the lock), for saving.
func (l *Learner) OnChange(fn func()) {
	l.mu.Lock()
	l.onChange = fn
	l.mu.Unlock()
}

// Enabled: off means nothing new is learned and nothing learned is used.
func (l *Learner) Enabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enabled
}

func (l *Learner) SetEnabled(enabled bool) {
	l.mu.Lock()
	l.enabled = enabled
	l.version++
	callback := l.onChange
	l.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// Version changes whenever anything learned changes.
func (l *Learner) Version() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.version
}

// MARK: - What the detector asks

// IsKnown: the user's own word in this language.
func (l *Learner) IsKnown(word string, lang Lang) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled {
		return false
	}
	entry, ok := l.entries[l.hash(word)]
	return ok && entry.state.kind == learnKnown && entry.state.lang == lang
}

// Decision is the standing decision about converting this exact on-screen word.
func (l *Learner) Decision(typed string) (LearnedDecision, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.enabled {
		return LearnedDecision{}, false
	}
	entry, ok := l.entries[l.hash(typed)]
	if !ok {
		return LearnedDecision{}, false
	}
	switch entry.state.kind {
	case learnKeep, learnKnown:
		return LearnedDecision{Kind: DecisionKeep}, true
	case learnConvert:
		return LearnedDecision{Kind: DecisionConvert, Lang: entry.state.lang}, true
	}
	return LearnedDecision{}, false
}

// MARK: - Signals

// Rejected: a fix was undone or erased and retyped — leave this word alone from now on.
func (l *Learner) Rejected(typed string) {
	l.update(typed, func(*learnState) learnState { return learnState{kind: learnKeep} })
}

// Confirmed: the user accepted a conversion or asked for it. Explicit, so it outranks
// anything learned passively.
func (l *Learner) Confirmed(typed string, lang Lang) {
	l.update(typed, func(*learnState) learnState { return learnState{kind: learnConvert, lang: lang} })
}

// Ignored: an offer went by without being taken.
func (l *Learner) Ignored(typed string) {
	l.update(typed, func(state *learnState) learnState {
		switch {
		case state != nil && state.kind == learnIgnored:
			if state.count+1 >= LearnerIgnoreLimit {
				return learnState{kind: learnKeep}
			}
			return learnState{kind: learnIgnored, count: state.count + 1}
		case state == nil || state.kind == learnSighted:
			if LearnerIgnoreLimit <= 1 {
				return learnState{kind: learnKeep}
			}
			return learnState{kind: learnIgnored, count: 1}
		}
		return *state
	})
}

// Kept: a word the dictionaries do not have was typed and left in place.
func (l *Learner) Kept(typed string, lang Lang) {
	if CharCount(typed) < 3 || !AllChars(typed, func(c string) bool { return IsLetterChar(c) || c == "-" || c == "'" }) {
		return
	}
	l.update(typed, func(state *learnState) learnState {
		switch {
		case state != nil && state.kind == learnSighted && state.lang == lang:
			if state.count+1 >= LearnerVocabularySightings {
				return learnState{kind: learnKnown, lang: lang}
			}
			return learnState{kind: learnSighted, lang: lang, count: state.count + 1}
		case state == nil || state.kind == learnSighted:
			if LearnerVocabularySightings <= 1 {
				return learnState{kind: learnKnown, lang: lang}
			}
			return learnState{kind: learnSighted, lang: lang, count: 1}
		}
		return *state
	})
}

func (l *Learner) update(typed string, transition func(*learnState) learnState) {
	l.mu.Lock()
	if !l.enabled || typed == "" {
		l.mu.Unlock()
		return
	}
	key := l.hash(typed)
	var current *learnState
	if entry, ok := l.entries[key]; ok {
		state := entry.state
		current = &state
	}
	entry := learnEntry{state: transition(current), lastUsed: l.today()}
	if old, ok := l.entries[key]; ok && old == entry {
		l.mu.Unlock()
		return
	}
	l.entries[key] = entry
	l.evictIfNeeded()
	callback := l.changedLocked()
	l.mu.Unlock()
	if callback != nil {
		callback()
	}
}

func (l *Learner) changedLocked() func() {
	l.version++
	return l.onChange
}

// MARK: - For the settings

// LearnedKind is what was learned about one word.
type LearnedKind string

const (
	LearnedNeverConvert  LearnedKind = "neverConvert"
	LearnedAlwaysConvert LearnedKind = "alwaysConvert"
	LearnedUserWord      LearnedKind = "userWord"
	LearnedPending       LearnedKind = "pending"
)

// LearnedItem is one learned word. The word itself is not stored, so it cannot be shown —
// only what was learned about it.
type LearnedItem struct {
	ID       string
	Kind     LearnedKind
	Lang     Lang
	LastUsed time.Time
}

// LearnerCounts are the entries by kind.
type LearnerCounts struct {
	NeverConvert, AlwaysConvert, UserWords, Pending int
}

func (c LearnerCounts) Total() int {
	return c.NeverConvert + c.AlwaysConvert + c.UserWords + c.Pending
}

func kindOf(state learnState) LearnedKind {
	switch state.kind {
	case learnKeep:
		return LearnedNeverConvert
	case learnConvert:
		return LearnedAlwaysConvert
	case learnKnown:
		return LearnedUserWord
	}
	return LearnedPending
}

func (l *Learner) Counts() LearnerCounts {
	l.mu.Lock()
	defer l.mu.Unlock()
	var counts LearnerCounts
	for _, entry := range l.entries {
		switch kindOf(entry.state) {
		case LearnedNeverConvert:
			counts.NeverConvert++
		case LearnedAlwaysConvert:
			counts.AlwaysConvert++
		case LearnedUserWord:
			counts.UserWords++
		default:
			counts.Pending++
		}
	}
	return counts
}

// Items lists what was learned, newest first.
func (l *Learner) Items() []LearnedItem {
	l.mu.Lock()
	defer l.mu.Unlock()
	items := make([]LearnedItem, 0, len(l.entries))
	for key, entry := range l.entries {
		item := LearnedItem{ID: fmt.Sprintf("%016x", key), Kind: kindOf(entry.state),
			LastUsed: referenceDate.Add(time.Duration(entry.lastUsed) * 24 * time.Hour)}
		switch entry.state.kind {
		case learnConvert, learnKnown, learnSighted:
			item.Lang = entry.state.lang
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if !items[i].LastUsed.Equal(items[j].LastUsed) {
			return items[i].LastUsed.After(items[j].LastUsed)
		}
		return items[i].ID < items[j].ID
	})
	return items
}

// Forget removes one entry by its ID (the hex hash).
func (l *Learner) Forget(id string) {
	key, err := strconv.ParseUint(id, 16, 64)
	if err != nil {
		return
	}
	l.mu.Lock()
	if _, ok := l.entries[key]; !ok {
		l.mu.Unlock()
		return
	}
	delete(l.entries, key)
	callback := l.changedLocked()
	l.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// ForgetWord forgets whatever was learned about a word, as typed or as converted.
func (l *Learner) ForgetWord(word string) bool {
	l.mu.Lock()
	key := l.hash(word)
	if _, ok := l.entries[key]; !ok {
		l.mu.Unlock()
		return false
	}
	delete(l.entries, key)
	callback := l.changedLocked()
	l.mu.Unlock()
	if callback != nil {
		callback()
	}
	return true
}

// ForgetAll is the older name of ResetLearnedVocabulary.
func (l *Learner) ForgetAll() { l.ResetLearnedVocabulary() }

// ResetLearnedVocabulary deletes everything learned: every entry, and the salt, which is
// replaced so that no hash computed before the wipe matches anything learned after it. The
// change callback fires, and LearnerStore writes the empty store at once.
func (l *Learner) ResetLearnedVocabulary() {
	l.mu.Lock()
	l.entries = map[uint64]learnEntry{}
	l.salt = makeSalt()
	callback := l.changedLocked()
	l.mu.Unlock()
	if callback != nil {
		callback()
	}
}

// MARK: - Hashing

// Hash is the salted FNV-1a over the normalized word: the same word is the same entry
// whatever its case or ё, and no entry is stored as text. (Guessable all the same; see the
// type's documentation.)
func (l *Learner) Hash(word string) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.hash(word)
}

func (l *Learner) hash(word string) uint64 {
	const prime = 0x00000100000001B3
	value := uint64(0xCBF29CE484222325)
	for _, b := range l.salt {
		value = (value ^ uint64(b)) * prime
	}
	var buffer [4]byte
	for _, scalar := range NormalizeWord(word) {
		n := encodeRune(buffer[:], scalar)
		for _, b := range buffer[:n] {
			value = (value ^ uint64(b)) * prime
		}
	}
	return value
}

// encodeRune is UTF8.encode for a Unicode scalar.
func encodeRune(buffer []byte, r rune) int {
	code := uint32(r)
	switch {
	case code < 0x80:
		buffer[0] = byte(code)
		return 1
	case code < 0x800:
		buffer[0] = byte(0xC0 | code>>6)
		buffer[1] = byte(0x80 | code&0x3F)
		return 2
	case code < 0x10000:
		buffer[0] = byte(0xE0 | code>>12)
		buffer[1] = byte(0x80 | code>>6&0x3F)
		buffer[2] = byte(0x80 | code&0x3F)
		return 3
	default:
		buffer[0] = byte(0xF0 | code>>18)
		buffer[1] = byte(0x80 | code>>12&0x3F)
		buffer[2] = byte(0x80 | code>>6&0x3F)
		buffer[3] = byte(0x80 | code&0x3F)
		return 4
	}
}

// referenceDate is Foundation's reference date, 2001-01-01 UTC.
var referenceDate = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

func (l *Learner) today() int {
	seconds := float64(l.now().Sub(referenceDate)) / float64(time.Second)
	return int(seconds / 86400)
}

func (l *Learner) evictIfNeeded() {
	if len(l.entries) <= LearnerCapacity {
		return
	}
	type aged struct {
		key      uint64
		lastUsed int
	}
	all := make([]aged, 0, len(l.entries))
	for key, entry := range l.entries {
		all = append(all, aged{key, entry.lastUsed})
	}
	// Oldest first; the key breaks ties so eviction is deterministic.
	sort.Slice(all, func(i, j int) bool {
		if all[i].lastUsed != all[j].lastUsed {
			return all[i].lastUsed < all[j].lastUsed
		}
		return all[i].key < all[j].key
	})
	for _, entry := range all[:len(l.entries)-LearnerCapacity] {
		delete(l.entries, entry.key)
	}
}

// MARK: - Persistence

// learnerFile is the stored form: integers only, keyed by hex hash, as `[kind, language,
// count, day]` — the same shape the Swift app keeps in UserDefaults. Version is the file
// format; Schema is the Swift app's `learningSchema`, the version of the rules the entries
// were learned under (absent, so 0, in files written before it existed).
type learnerFile struct {
	Version int              `json:"version"`
	Schema  int              `json:"schema"`
	Enabled bool             `json:"enabled"`
	Salt    string           `json:"salt"`
	Entries map[string][]int `json:"entries"`
}

func langIndex(lang Lang) int {
	for index, candidate := range AllLangs {
		if candidate == lang {
			return index
		}
	}
	return -1
}

// Encode serializes the learner. Nothing readable: hashes and small integers.
func (l *Learner) Encode() ([]byte, error) {
	l.mu.Lock()
	file := learnerFile{Version: 1, Schema: l.schema, Enabled: l.enabled, Salt: hex.EncodeToString(l.salt),
		Entries: make(map[string][]int, len(l.entries))}
	for key, entry := range l.entries {
		var lang, count int
		switch entry.state.kind {
		case learnKeep:
			lang, count = -1, 0
		case learnConvert, learnKnown:
			lang = langIndex(entry.state.lang)
		case learnIgnored:
			lang, count = -1, entry.state.count
		case learnSighted:
			lang, count = langIndex(entry.state.lang), entry.state.count
		}
		file.Entries[fmt.Sprintf("%016x", key)] = []int{int(entry.state.kind), lang, count, entry.lastUsed}
	}
	l.mu.Unlock()
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// DecodeLearner restores a learner. Unreadable entries are skipped, as the Swift loader
// skips them; a missing or malformed salt is replaced, which leaves old entries unreachable
// (they age out). Entries learned under older rules are dropped (see migrate and Upgraded):
// the caller saves the result straight away.
func DecodeLearner(data []byte, now func() time.Time) (*Learner, error) {
	learner := NewLearner(now)
	var file learnerFile
	if err := json.Unmarshal(data, &file); err != nil {
		return learner, fmt.Errorf("learned words are not valid JSON: %w", err)
	}
	if file.Version != 1 {
		return learner, errors.New("unsupported learned-words version")
	}
	learner.enabled = file.Enabled
	salt, err := hex.DecodeString(file.Salt)
	if err != nil || len(salt) != 8 {
		return learner, errors.New("learned words have no valid salt")
	}
	learner.salt = salt
	for text, fields := range file.Entries {
		key, err := strconv.ParseUint(text, 16, 64)
		if err != nil || len(fields) != 4 {
			continue
		}
		state, ok := decodeState(fields[0], fields[1], fields[2])
		if !ok {
			continue
		}
		learner.entries[key] = learnEntry{state: state, lastUsed: fields[3]}
	}
	// A newer schema is kept as it is, as the Swift app leaves a newer `learningSchema` alone.
	learner.schema = file.Schema
	if file.Schema < LearnerSchemaVersion {
		learner.migrate()
	}
	return learner, nil
}

// LearnerUpgrade is what loading a store written under older rules dropped.
type LearnerUpgrade struct {
	// FromSchema is the schema the store was written under (0: before schemas existed).
	FromSchema int
	// Before and Kept count the entries before and after the old ones were dropped.
	Before, Kept int
}

// migrate drops what the first version learned from signals that meant nothing.
//
// It learned "never convert" from offers the user merely typed past (or moved away from, or
// let time out), and learned garbled words as the user's own vocabulary whenever a fix had
// not happened — Russian typed on the Kazakh layout included. None of that is evidence. What
// the user accepted explicitly — the accept key on an offer, the fix-last-word hotkey — is,
// and it stays.
func (l *Learner) migrate() {
	before := len(l.entries)
	for key, entry := range l.entries {
		if entry.state.kind != learnConvert {
			delete(l.entries, key)
		}
	}
	l.upgrade = &LearnerUpgrade{FromSchema: l.schema, Before: before, Kept: len(l.entries)}
	l.schema = LearnerSchemaVersion
}

// Upgraded reports whether DecodeLearner dropped entries learned under older rules. The
// caller saves the learner at once when it did, as the Swift app stores the new schema as
// soon as it has migrated, and says so in the log.
func (l *Learner) Upgraded() (LearnerUpgrade, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.upgrade == nil {
		return LearnerUpgrade{}, false
	}
	return *l.upgrade, true
}

func decodeState(kind, lang, count int) (learnState, bool) {
	language := LangNone
	if lang >= 0 && lang < len(AllLangs) {
		language = AllLangs[lang]
	}
	switch learnKind(kind) {
	case learnKeep:
		return learnState{kind: learnKeep}, true
	case learnConvert:
		return learnState{kind: learnConvert, lang: language}, language != LangNone
	case learnIgnored:
		return learnState{kind: learnIgnored, count: count}, true
	case learnSighted:
		return learnState{kind: learnSighted, lang: language, count: count}, language != LangNone
	case learnKnown:
		return learnState{kind: learnKnown, lang: language}, language != LangNone
	}
	return learnState{}, false
}

// SetSaltForTesting fixes the salt (golden hashes).
func (l *Learner) SetSaltForTesting(salt []byte) {
	l.mu.Lock()
	l.salt = append([]byte(nil), salt...)
	l.mu.Unlock()
}

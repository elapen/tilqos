// Package config holds the user's settings, exceptions and learned data, persisted as
// %LOCALAPPDATA%\Tilqos\config.json (the port of Core/Prefs.swift and Core/AppRules.swift).
//
// Platform-neutral: the Windows app supplies the path. Writes are atomic (temporary file
// plus rename), and a file that fails to parse is set aside as config.json.bad rather than
// silently overwritten.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"tilqos/internal/core"
	"tilqos/internal/i18n"
)

// FixMode is what the app is allowed to do in a given context.
type FixMode string

const (
	// ModeOff: watch nothing, change nothing.
	ModeOff FixMode = "off"
	// ModeSuggest: only offer a correction the user accepts explicitly.
	ModeSuggest FixMode = "suggest"
	// ModeAuto: apply confident corrections immediately, offer the rest.
	ModeAuto FixMode = "auto"
)

func (m FixMode) Valid() bool { return m == ModeOff || m == ModeSuggest || m == ModeAuto }

func (m FixMode) DisplayName() string {
	switch m {
	case ModeOff:
		return i18n.T(i18n.ModeOff)
	case ModeSuggest:
		return i18n.T(i18n.ModeSuggest)
	}
	return i18n.T(i18n.ModeAuto)
}

// AcceptKey is the key that accepts a pending suggestion.
type AcceptKey string

const (
	AcceptTab AcceptKey = "tab"
	// AcceptInsert is the fallback in apps where Tab is load-bearing (terminals, editors,
	// Excel). macOS uses ⌥Space there; on Windows Alt+Space opens the window menu, while
	// Insert is almost never pressed on purpose in those apps.
	AcceptInsert AcceptKey = "insert"
)

// CurrentVersion is the config schema version.
const CurrentVersion = 1

// Config is everything that is persisted. JSON field names are part of the user-facing
// format (people edit this file by hand), so they are stable.
type Config struct {
	Version int `json:"version"`

	// Enabled is the master switch.
	Enabled bool `json:"enabled"`
	// SmartMode: when false, only the manual hotkeys work — no automatic detection.
	SmartMode      bool    `json:"smartMode"`
	DefaultFixMode FixMode `json:"defaultFixMode"`

	// AutoThreshold: confidence at or above which a correction is applied without asking.
	AutoThreshold float64 `json:"autoThreshold"`
	// SuggestThreshold: confidence at or above which a correction is offered.
	SuggestThreshold float64 `json:"suggestThreshold"`
	// MinWordLength: shorter words are never touched.
	MinWordLength    int      `json:"minWordLength"`
	EnabledLanguages []string `json:"enabledLanguages"`

	AcceptKey AcceptKey `json:"acceptKey"`
	// SuggestionTimeout is how long a suggestion stays on screen, in seconds.
	SuggestionTimeout float64 `json:"suggestionTimeoutSeconds"`
	// BufferIdleReset is the idle time after which the keystroke buffer is discarded, by a
	// timer, in seconds. Between core.MinIdleReset and core.MaxIdleReset (5): the app promises
	// that typed text does not outlive five seconds of silence, so a longer value is replaced by
	// the default.
	BufferIdleReset float64 `json:"bufferIdleResetSeconds"`

	RememberLayoutPerApp bool `json:"rememberLayoutPerApp"`
	// CyrillicPreference decides between the Russian and Kazakh layouts when the letters
	// alone cannot tell them apart.
	CyrillicPreference CyrillicPreference `json:"cyrillicPreference"`
	PlaySound          bool               `json:"playSound"`
	// Diagnostics turns the log file on (%LOCALAPPDATA%\Tilqos\tilqos.log); off by default.
	// What is written is lengths, languages and decisions, never typed text, clipboard content
	// or window titles.
	Diagnostics bool `json:"diagnostics"`

	// WordExceptions are words never to correct, stored lowercased.
	WordExceptions []string `json:"wordExceptions"`
	// AppRules maps a lowercased executable name to a FixMode.
	AppRules map[string]FixMode `json:"appRules"`
	// AppCyrillicMemory maps a lowercased executable name to the Cyrillic layout last
	// used there — the tie-breaker when letters cannot tell Russian from Kazakh.
	AppCyrillicMemory map[string]string `json:"appCyrillicMemory"`

	// Hotkeys maps an action name to a key spec such as "Shift+Pause"; "" disables it.
	Hotkeys map[string]string `json:"hotkeys"`

	TranslitScheme       string            `json:"translitScheme"`
	TranslitOverrides    map[string]string `json:"translitOverrides"`
	TranslitNeverConvert []string          `json:"translitNeverConvert"`

	WelcomeShown bool `json:"welcomeShown"`

	// UILanguage is the language of the menus and messages: "system" (follow Windows), "en",
	// "ru" or "kk". Anything else is read as "system".
	UILanguage string `json:"uiLanguage"`
}

// Default is the configuration a fresh install starts with (the Swift defaults).
func Default() Config {
	return Config{
		Version:              CurrentVersion,
		Enabled:              true,
		SmartMode:            true,
		DefaultFixMode:       ModeAuto,
		AutoThreshold:        core.DefaultAutoThreshold,
		SuggestThreshold:     core.DefaultSuggestThreshold,
		MinWordLength:        core.DefaultMinWordLength,
		EnabledLanguages:     []string{"en", "ru", "kk"},
		AcceptKey:            AcceptTab,
		SuggestionTimeout:    4,
		BufferIdleReset:      5,
		RememberLayoutPerApp: true,
		CyrillicPreference:   CyrillicAutomatic,
		UILanguage:           string(i18n.System),
		WordExceptions:       []string{},
		AppRules:             map[string]FixMode{},
		AppCyrillicMemory:    map[string]string{},
		Hotkeys:              DefaultHotkeys(),
		TranslitScheme:       "practical",
		TranslitOverrides:    map[string]string{},
		TranslitNeverConvert: []string{},
	}
}

// Normalize repairs anything a hand edit could break: clamps numbers, fills nil maps,
// lowercases and de-duplicates word lists, drops unknown values.
func (c *Config) Normalize() {
	d := Default()
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if !c.DefaultFixMode.Valid() {
		c.DefaultFixMode = d.DefaultFixMode
	}
	if !(c.AutoThreshold >= 0 && c.AutoThreshold <= 1) {
		c.AutoThreshold = d.AutoThreshold
	}
	if !(c.SuggestThreshold >= 0 && c.SuggestThreshold <= 1) {
		c.SuggestThreshold = d.SuggestThreshold
	}
	// Like Prefs.minWordLength: never below 2.
	if c.MinWordLength < 2 {
		c.MinWordLength = 2
	}
	if !c.CyrillicPreference.Valid() {
		c.CyrillicPreference = CyrillicAutomatic
	}
	if c.AcceptKey != AcceptTab && c.AcceptKey != AcceptInsert {
		c.AcceptKey = AcceptTab
	}
	if !(c.SuggestionTimeout >= 1 && c.SuggestionTimeout <= 60) {
		c.SuggestionTimeout = d.SuggestionTimeout
	}
	if !(c.BufferIdleReset >= core.MinIdleReset && c.BufferIdleReset <= core.MaxIdleReset) {
		c.BufferIdleReset = d.BufferIdleReset
	}
	var langs []string
	for _, code := range c.EnabledLanguages {
		if lang := core.LangFromCode(code); lang != core.LangNone && !contains(langs, string(lang)) {
			langs = append(langs, string(lang))
		}
	}
	if len(langs) == 0 {
		// Like Prefs.enabledLanguages: an empty selection means all of them.
		langs = d.EnabledLanguages
	}
	c.EnabledLanguages = langs
	c.WordExceptions = lowerUnique(c.WordExceptions)
	c.TranslitNeverConvert = lowerUnique(c.TranslitNeverConvert)

	rules := map[string]FixMode{}
	for app, mode := range c.AppRules {
		if mode.Valid() && app != "" {
			rules[strings.ToLower(app)] = mode
		}
	}
	c.AppRules = rules
	memory := map[string]string{}
	for app, layout := range c.AppCyrillicMemory {
		if app != "" && layout != "" {
			memory[strings.ToLower(app)] = layout
		}
	}
	c.AppCyrillicMemory = memory

	hotkeys := DefaultHotkeys()
	for action, spec := range c.Hotkeys {
		if _, known := hotkeys[action]; !known {
			continue
		}
		if spec == "" {
			hotkeys[action] = ""
			continue
		}
		if hotkey, err := ParseHotkey(spec); err == nil && hotkey.Valid() {
			hotkeys[action] = hotkey.String()
		}
	}
	c.Hotkeys = hotkeys

	if c.TranslitScheme != "official2021" && c.TranslitScheme != "practical" {
		c.TranslitScheme = d.TranslitScheme
	}
	if !i18n.Setting(c.UILanguage).Valid() {
		c.UILanguage = d.UILanguage
	}
	overrides := map[string]string{}
	for latin, cyrillic := range c.TranslitOverrides {
		if latin != "" && cyrillic != "" {
			overrides[core.Lower(latin)] = cyrillic
		}
	}
	c.TranslitOverrides = overrides
}

func contains(list []string, item string) bool {
	for _, candidate := range list {
		if candidate == item {
			return true
		}
	}
	return false
}

func lowerUnique(words []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, word := range words {
		word = strings.TrimSpace(core.Lower(word))
		if word == "" || seen[word] {
			continue
		}
		seen[word] = true
		out = append(out, word)
	}
	sort.Strings(out)
	return out
}

// Clone returns a deep copy.
func (c Config) Clone() Config {
	out := c
	out.EnabledLanguages = append([]string(nil), c.EnabledLanguages...)
	out.WordExceptions = append([]string(nil), c.WordExceptions...)
	out.TranslitNeverConvert = append([]string(nil), c.TranslitNeverConvert...)
	out.AppRules = make(map[string]FixMode, len(c.AppRules))
	for k, v := range c.AppRules {
		out.AppRules[k] = v
	}
	out.AppCyrillicMemory = make(map[string]string, len(c.AppCyrillicMemory))
	for k, v := range c.AppCyrillicMemory {
		out.AppCyrillicMemory[k] = v
	}
	out.Hotkeys = make(map[string]string, len(c.Hotkeys))
	for k, v := range c.Hotkeys {
		out.Hotkeys[k] = v
	}
	out.TranslitOverrides = make(map[string]string, len(c.TranslitOverrides))
	for k, v := range c.TranslitOverrides {
		out.TranslitOverrides[k] = v
	}
	return out
}

// Languages returns the enabled languages.
func (c Config) Languages() map[core.Lang]bool {
	out := map[core.Lang]bool{}
	for _, code := range c.EnabledLanguages {
		out[core.LangFromCode(code)] = true
	}
	return out
}

// ClearExceptionWords deletes every word the user added to the exceptions, including the ones
// the app added when a correction was undone. They are stored as plain text.
func (c *Config) ClearExceptionWords() { c.WordExceptions = []string{} }

// ClearNeverConvertWords deletes the list of Latin words to leave alone (plain text).
func (c *Config) ClearNeverConvertWords() { c.TranslitNeverConvert = []string{} }

// ClearTranslitOverrides deletes the pinned transliteration spellings (plain text).
func (c *Config) ClearTranslitOverrides() { c.TranslitOverrides = map[string]string{} }

// ClearAppRules deletes the per-application modes the user chose.
func (c *Config) ClearAppRules() { c.AppRules = map[string]FixMode{} }

// ClearRememberedLayouts deletes the memory of which Cyrillic layout was last used in which
// application.
func (c *Config) ClearRememberedLayouts() { c.AppCyrillicMemory = map[string]string{} }

// AddWordException adds a word (lowercased) to the exceptions.
func (c *Config) AddWordException(word string) {
	c.WordExceptions = lowerUnique(append(c.WordExceptions, word))
}

// AddNeverConvert adds rejected transliteration words.
func (c *Config) AddNeverConvert(words []string) {
	c.TranslitNeverConvert = lowerUnique(append(c.TranslitNeverConvert, words...))
}

// MARK: - Persistence

// Load reads a config file. A missing file yields the defaults; an unreadable one yields
// the defaults plus an error, after moving the broken file aside so it is not lost.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		backup := path + ".bad"
		_ = os.Rename(path, backup)
		return Default(), fmt.Errorf("config is not valid JSON (moved to %s): %w", filepath.Base(backup), err)
	}
	cfg.Normalize()
	return cfg, nil
}

// Save writes atomically: a temporary file in the same directory, then a rename.
func Save(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, append(data, '\n'))
}

// WriteFileAtomic writes a temporary file in the same directory, then renames it over the
// target, so a crash never leaves half a file.
func WriteFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// Store is the live, thread-safe configuration: every change is normalised, saved and
// announced to subscribers.
type Store struct {
	mu        sync.Mutex
	path      string
	cfg       Config
	listeners []func(Config)
	lastErr   error
	// forceDiagnostics turns diagnostics on for this run without saving that choice.
	forceDiagnostics bool
}

// NewStore loads path (or starts from the defaults). The returned error is informational:
// the store is always usable.
func NewStore(path string) (*Store, error) {
	cfg, err := Load(path)
	return &Store{path: path, cfg: cfg}, err
}

// NewMemoryStore is a store that never touches the disk (tests).
func NewMemoryStore(cfg Config) *Store {
	cfg.Normalize()
	return &Store{cfg: cfg}
}

// Path is the file the store saves to ("" for memory stores).
func (s *Store) Path() string { return s.path }

// Get returns a deep copy of the current configuration.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view(s.cfg)
}

// view is cfg as the rest of the app sees it: a copy, with the diagnostics override applied.
func (s *Store) view(cfg Config) Config {
	out := cfg.Clone()
	if s.forceDiagnostics {
		out.Diagnostics = true
	}
	return out
}

// ForceDiagnostics turns diagnostics on for this run only (the TILQOS_DEBUG environment
// variable): the setting reads as on, and is never written to the file.
func (s *Store) ForceDiagnostics(on bool) {
	s.mu.Lock()
	s.forceDiagnostics = on
	s.mu.Unlock()
}

// Update applies fn, normalises, saves and notifies. Listeners run on the caller's
// goroutine, outside the lock.
func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	next := s.cfg.Clone()
	fn(&next)
	next.Normalize()
	s.cfg = next
	var err error
	if s.path != "" {
		err = Save(s.path, next)
		s.lastErr = err
	}
	listeners := append([]func(Config){}, s.listeners...)
	snapshot := s.view(next)
	s.mu.Unlock()
	for _, listener := range listeners {
		listener(snapshot.Clone())
	}
	return err
}

// The wipe functions, one per store. Each saves at once and announces the change; the
// settings window calls them from its "clear" buttons.

// ClearExceptionWords deletes every user-added exception word.
func (s *Store) ClearExceptionWords() error { return s.Update((*Config).ClearExceptionWords) }

// ClearNeverConvertWords deletes the words to leave in Latin.
func (s *Store) ClearNeverConvertWords() error { return s.Update((*Config).ClearNeverConvertWords) }

// ClearTranslitOverrides deletes the pinned transliteration spellings.
func (s *Store) ClearTranslitOverrides() error { return s.Update((*Config).ClearTranslitOverrides) }

// ClearAppRules deletes the per-application modes.
func (s *Store) ClearAppRules() error { return s.Update((*Config).ClearAppRules) }

// ClearRememberedLayouts deletes the per-application Cyrillic layout memory.
func (s *Store) ClearRememberedLayouts() error { return s.Update((*Config).ClearRememberedLayouts) }

// Subscribe registers a change listener.
func (s *Store) Subscribe(fn func(Config)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, fn)
}

// LastSaveError is the error of the most recent save, if any.
func (s *Store) LastSaveError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultsSurviveARoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Tilqos", "config.json")
	cfg := Default()
	cfg.AddWordException("Ghbdtn")
	cfg.SetMode("Chrome.EXE", ModeOff, true)
	cfg.RememberCyrillic("0000043F", "telegram.exe")
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Normalize()
	if !reflect.DeepEqual(loaded, cfg) {
		t.Fatalf("round trip changed the config:\n%+v\n%+v", loaded, cfg)
	}
	if loaded.WordExceptions[0] != "ghbdtn" || loaded.AppRules["chrome.exe"] != ModeOff ||
		loaded.RememberedCyrillic("Telegram.exe") != "0000043F" {
		t.Fatalf("normalisation lost data: %+v", loaded)
	}
}

func TestMissingFileGivesDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}

func TestBrokenFileIsSetAsideNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err == nil || !reflect.DeepEqual(cfg, Default()) {
		t.Fatalf("expected defaults and an error, got %+v %v", cfg, err)
	}
	if data, err := os.ReadFile(path + ".bad"); err != nil || string(data) != "{ not json" {
		t.Fatal("the broken file must be kept as config.json.bad")
	}
}

func TestHandEditsAreRepaired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	edited := `{
		"autoThreshold": 7,
		"suggestThreshold": -1,
		"minWordLength": 0,
		"enabledLanguages": ["RU", "xx", "ru"],
		"acceptKey": "space",
		"defaultFixMode": "sometimes",
		"appRules": {"Code.EXE": "suggest", "bad.exe": "maybe"},
		"hotkeys": {"fixLastWord": "shift+pause", "undoLastFix": "", "transliterate": "Q", "bogus": "F1"},
		"wordExceptions": ["B", "a", "b", " "]
	}`
	if err := os.WriteFile(path, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	d := Default()
	switch {
	case cfg.AutoThreshold != d.AutoThreshold, cfg.SuggestThreshold != d.SuggestThreshold:
		t.Fatalf("thresholds not clamped: %v %v", cfg.AutoThreshold, cfg.SuggestThreshold)
	case cfg.MinWordLength != 2:
		t.Fatalf("minWordLength %d", cfg.MinWordLength)
	case !reflect.DeepEqual(cfg.EnabledLanguages, []string{"ru"}):
		t.Fatalf("languages %v", cfg.EnabledLanguages)
	case cfg.AcceptKey != AcceptTab, cfg.DefaultFixMode != ModeAuto:
		t.Fatalf("enums not repaired")
	case !reflect.DeepEqual(cfg.AppRules, map[string]FixMode{"code.exe": ModeSuggest}):
		t.Fatalf("rules %v", cfg.AppRules)
	case cfg.Hotkeys["fixLastWord"] != "Shift+Pause", cfg.Hotkeys["undoLastFix"] != "",
		cfg.Hotkeys["transliterate"] != "Alt+ScrollLock", cfg.Hotkeys["bogus"] != "":
		// A bare letter is not a valid hotkey, so the default stays.
		t.Fatalf("hotkeys %v", cfg.Hotkeys)
	case !reflect.DeepEqual(cfg.WordExceptions, []string{"a", "b"}):
		t.Fatalf("exceptions %v", cfg.WordExceptions)
	}
}

func TestStoreSavesAndNotifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	var seen []bool
	store.Subscribe(func(c Config) { seen = append(seen, c.Enabled) })
	if err := store.Update(func(c *Config) { c.Enabled = false }); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] {
		t.Fatalf("listener saw %v", seen)
	}
	reloaded, _ := Load(path)
	if reloaded.Enabled {
		t.Fatal("change not persisted")
	}
	// Get returns a copy: mutating it must not leak into the store.
	copy := store.Get()
	copy.AppRules["x.exe"] = ModeOff
	if _, leaked := store.Get().AppRules["x.exe"]; leaked {
		t.Fatal("Get must return a deep copy")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatal("temporary file left behind")
		}
	}
}

func TestAppRules(t *testing.T) {
	cfg := Default()
	cases := map[string]FixMode{
		"":                    ModeAuto,
		"notepad.exe":         ModeAuto,
		"KeePassXC.exe":       ModeOff,
		"mstsc.exe":           ModeOff,
		"WindowsTerminal.exe": ModeSuggest,
	}
	for app, want := range cases {
		if got := cfg.ModeFor(app); got != want {
			t.Errorf("ModeFor(%q) = %s, want %s", app, got, want)
		}
	}
	cfg.SetMode("keepassxc.exe", ModeAuto, true)
	if cfg.ModeFor("keepassxc.exe") != ModeOff || !IsLocked("KEEPASSXC.EXE") {
		t.Fatal("password managers must stay off")
	}
	cfg.SetMode("mstsc.exe", ModeAuto, true)
	if cfg.ModeFor("mstsc.exe") != ModeAuto {
		t.Fatal("a user rule overrides the remote-desktop default")
	}
	cfg.SetMode("mstsc.exe", "", false)
	if cfg.ModeFor("mstsc.exe") != ModeOff {
		t.Fatal("clearing the rule restores the default")
	}
	if !IsTabCritical("Code.exe") || IsTabCritical("notepad.exe") {
		t.Fatal("tab-critical list")
	}
	if !cfg.RememberCyrillic("A", "app.exe") || cfg.RememberCyrillic("A", "app.exe") {
		t.Fatal("RememberCyrillic must report changes only")
	}
	cfg.RememberLayoutPerApp = false
	if cfg.RememberedCyrillic("app.exe") != "" {
		t.Fatal("memory must be ignored when disabled")
	}
}

func TestHotkeyParsing(t *testing.T) {
	cases := map[string]Hotkey{
		"Pause":          {VK: VKPause},
		"shift+pause":    {VK: VKPause, Mods: ModShift},
		"Alt+ScrollLock": {VK: VKScrollLock, Mods: ModAlt},
		"Ctrl+Alt+Z":     {VK: 'Z', Mods: ModCtrl | ModAlt},
		"Win+F9":         {VK: VKF1 + 8, Mods: ModWin},
		"Ctrl+Shift+1":   {VK: '1', Mods: ModCtrl | ModShift},
		"Break":          {VK: VKPause},
		"Ctrl+`":         {VK: 0xC0, Mods: ModCtrl},
	}
	for spec, want := range cases {
		got, err := ParseHotkey(spec)
		if err != nil || got != want {
			t.Errorf("ParseHotkey(%q) = %+v, %v; want %+v", spec, got, err, want)
			continue
		}
		// Formatting parses back to the same hotkey.
		if again, err := ParseHotkey(got.String()); err != nil || again != got {
			t.Errorf("%q → %q does not round-trip", spec, got.String())
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Shift", "Pause+Shift", "Ctrl+Nope", "F25"} {
		if _, err := ParseHotkey(bad); err == nil {
			t.Errorf("ParseHotkey(%q) should fail", bad)
		}
	}
	valid := map[string]bool{"Pause": true, "F5": true, "Q": false, "Space": false, "Shift+Q": true}
	for spec, want := range valid {
		hotkey, _ := ParseHotkey(spec)
		if hotkey.Valid() != want {
			t.Errorf("%q valid = %v", spec, hotkey.Valid())
		}
	}
	bindings := Bindings(DefaultHotkeys())
	if len(bindings) != 4 || bindings[0].Action != ActionFixLastWord {
		t.Fatalf("default bindings %+v", bindings)
	}
}

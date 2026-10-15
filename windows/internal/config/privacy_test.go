package config

// P5 and P4's file locations: what is stored, the functions that wipe each store, and where
// the files live. Everything runs in temporary directories.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tilqos/internal/core"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestClearExceptionWordsRemovesThemFromTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, _ := NewStore(path)
	store.Update(func(c *Config) { c.AddWordException("Kaspi"); c.AddWordException("zhmyh") })
	if !strings.Contains(readFile(t, path), "zhmyh") {
		t.Fatal("they are stored as plain text, as documented")
	}
	notified := 0
	store.Subscribe(func(Config) { notified++ })

	if err := store.ClearExceptionWords(); err != nil {
		t.Fatal(err)
	}
	if got := store.Get().WordExceptions; len(got) != 0 {
		t.Fatalf("exceptions remain: %v", got)
	}
	if text := readFile(t, path); strings.Contains(text, "zhmyh") || strings.Contains(text, "kaspi") {
		t.Fatalf("the words are still in the file:\n%s", text)
	}
	if notified != 1 {
		t.Fatal("subscribers must hear about it (the engine refreshes its snapshot)")
	}
}

func TestEachStoreIsWipedOnItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, _ := NewStore(path)
	store.Update(func(c *Config) {
		c.AddWordException("exceptionword")
		c.AddNeverConvert([]string{"instagram", "zoom"})
		c.TranslitOverrides["jatqanyn"] = "жатқанын"
		c.SetMode("code.exe", ModeOff, true)
		c.RememberCyrillic("00000419", "notepad.exe")
		c.AutoThreshold = 0.7
	})

	store.ClearNeverConvertWords()
	if cfg := store.Get(); len(cfg.TranslitNeverConvert) != 0 || len(cfg.WordExceptions) != 1 {
		t.Fatalf("%+v", cfg)
	}
	store.ClearTranslitOverrides()
	if cfg := store.Get(); len(cfg.TranslitOverrides) != 0 || len(cfg.AppRules) != 1 {
		t.Fatalf("%+v", cfg)
	}
	store.ClearRememberedLayouts()
	if cfg := store.Get(); len(cfg.AppCyrillicMemory) != 0 || len(cfg.AppRules) != 1 {
		t.Fatalf("%+v", cfg)
	}
	store.ClearAppRules()
	if cfg := store.Get(); len(cfg.AppRules) != 0 {
		t.Fatalf("%+v", cfg)
	}
	store.ClearExceptionWords()

	text := readFile(t, path)
	for _, gone := range []string{"exceptionword", "instagram", "жатқанын", "code.exe", "notepad.exe", "00000419"} {
		if strings.Contains(text, gone) {
			t.Errorf("%q survived the wipes", gone)
		}
	}
	if store.Get().AutoThreshold != 0.7 {
		t.Fatal("wiping lists must leave the settings alone")
	}
}

func TestBufferIdleResetIsLimitedToFiveSeconds(t *testing.T) {
	for in, want := range map[float64]float64{5: 5, 3: 3, 1: 1, 6: 5, 600: 5, 0: 5, 0.5: 5, -2: 5} {
		cfg := Default()
		cfg.BufferIdleReset = in
		cfg.Normalize()
		if cfg.BufferIdleReset != want {
			t.Errorf("%v -> %v, want %v", in, cfg.BufferIdleReset, want)
		}
		if cfg.BufferIdleReset > core.MaxIdleReset {
			t.Errorf("%v: beyond the promise", in)
		}
	}
}

func TestDiagnosticsAreOffByDefault(t *testing.T) {
	if Default().Diagnostics {
		t.Fatal("the log is off by default")
	}
}

func TestForcedDiagnosticsAreNeverWrittenToTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store, _ := NewStore(path)
	store.ForceDiagnostics(true)
	if !store.Get().Diagnostics {
		t.Fatal("forced on for this run")
	}
	store.Update(func(c *Config) { c.PlaySound = true })
	if !store.Get().Diagnostics {
		t.Fatal("still on after an update")
	}
	reloaded, _ := NewStore(path)
	if reloaded.Get().Diagnostics || strings.Contains(readFile(t, path), `"diagnostics": true`) {
		t.Fatal("TILQOS_DEBUG must not turn the log on for ever")
	}
}

func TestLearnerStoreResetWritesTheEmptyStoreAtOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learned.json")
	store, err := OpenLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	learner := store.Learner()
	learner.Confirmed("ghbdtn", core.LangRU)
	learner.Kept("Aidar", core.LangEN)
	learner.Kept("Aidar", core.LangEN)
	if err := store.Flush(); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, path)
	hashOfWord := learner.Hash("ghbdtn")

	if err := store.ResetLearnedVocabulary(); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, path)
	if after == before || learner.Counts().Total() != 0 {
		t.Fatal("not written")
	}
	if strings.Contains(after, hex16(hashOfWord)) {
		t.Fatal("the old hash is still in the file")
	}
	reopened, err := OpenLearner(path)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Learner().Counts().Total() != 0 {
		t.Fatal("a restart sees the old entries")
	}
	store.Close()
	reopened.Close()
}

func hex16(v uint64) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 16)
	for i := 15; i >= 0; i-- {
		out[i] = digits[v&0xF]
		v >>= 4
	}
	return string(out)
}

// MARK: - where the files live

func TestDataDirIsLocalAppDataNotTheRoamingProfile(t *testing.T) {
	env := map[string]string{"LOCALAPPDATA": `C:\Users\user\AppData\Local`, "APPDATA": `C:\Users\user\AppData\Roaming`}
	getenv := func(key string) string { return env[key] }
	cache := func() (string, error) { return "/cache", nil }
	got := DataDir(getenv, cache, func() string { return "/tmp" })
	if got != filepath.Join(`C:\Users\user\AppData\Local`, "Tilqos") {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "Roaming") {
		t.Fatal("the roaming profile follows the user to other computers")
	}
	delete(env, "LOCALAPPDATA")
	if got := DataDir(getenv, cache, func() string { return "/tmp" }); got != filepath.Join("/cache", "Tilqos") {
		t.Fatalf("fallback %q", got)
	}
	failing := func() (string, error) { return "", os.ErrNotExist }
	if got := DataDir(getenv, failing, func() string { return "/tmp" }); got != filepath.Join("/tmp", "Tilqos") {
		t.Fatalf("last resort %q", got)
	}
}

func TestMigrateDataMovesFilesAndDeletesTheOldLog(t *testing.T) {
	root := t.TempDir()
	oldDir, newDir := filepath.Join(root, "Roaming", "Tilqos"), filepath.Join(root, "Local", "Tilqos")
	os.MkdirAll(oldDir, 0o755)
	write := func(dir, name, text string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(oldDir, "config.json", "{old}")
	write(oldDir, "learned.json", "{learned}")
	write(oldDir, "tilqos.log", "notepad.exe: fixed")
	write(oldDir, "tilqos.log.old", "x")
	write(oldDir, ".config-123.tmp", "half")

	if err := MigrateData(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(newDir, "config.json")) != "{old}" || readFile(t, filepath.Join(newDir, "learned.json")) != "{learned}" {
		t.Fatal("settings and learned words must move")
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("nothing may be left behind in the roaming profile, the folder included")
	}
	for _, name := range []string{"tilqos.log", "tilqos.log.old", ".config-123.tmp"} {
		if _, err := os.Stat(filepath.Join(newDir, name)); !os.IsNotExist(err) {
			t.Errorf("%s must be deleted, not moved", name)
		}
	}

	// A second run, with nothing to do, is fine.
	if err := MigrateData(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
}

func TestMigrateDataNeverOverwritesNewerFiles(t *testing.T) {
	root := t.TempDir()
	oldDir, newDir := filepath.Join(root, "a"), filepath.Join(root, "b")
	os.MkdirAll(oldDir, 0o755)
	os.MkdirAll(newDir, 0o755)
	os.WriteFile(filepath.Join(oldDir, "config.json"), []byte("stale"), 0o644)
	os.WriteFile(filepath.Join(newDir, "config.json"), []byte("current"), 0o644)
	if err := MigrateData(oldDir, newDir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(newDir, "config.json")) != "current" {
		t.Fatal("the file in the new place wins")
	}
	if _, err := os.Stat(filepath.Join(oldDir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("and the stale copy does not stay behind")
	}
}

func TestMigrateDataIgnoresSamePathAndMissingDirectories(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte("x"), 0o644)
	if err := MigrateData(dir, dir); err != nil {
		t.Fatal(err)
	}
	if readFile(t, filepath.Join(dir, "config.json")) != "x" {
		t.Fatal("moved onto itself")
	}
	if err := MigrateData(filepath.Join(dir, "nope"), filepath.Join(dir, "new")); err != nil {
		t.Fatal(err)
	}
	if err := MigrateData("", dir); err != nil {
		t.Fatal(err)
	}
}

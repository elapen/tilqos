package winapp

// The privacy promises that can be checked on any OS: the log (P4), the clipboard rules (P3),
// and audits of the source for stray output (P4) and network use (P6). What needs Windows —
// the hook thread, the clipboard APIs, the window messages for lock and sleep — is compiled
// by `GOOS=windows go vet` but cannot run here; see PRIVACY_IMPLEMENTATION_NOTES.md.

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf16"

	"tilqos/internal/core"
	"tilqos/internal/golden"
)

// MARK: - The log

func TestLoggerIsOffByDefaultAndCreatesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Tilqos", "tilqos.log")
	log := NewLogger(path, 1<<20)
	if log.Enabled() {
		t.Fatal("must start off")
	}
	log.Printf("fixed %d chars", 7)
	log.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("a disabled logger must not create a file")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("nor its folder")
	}
}

func TestLoggerWritesOnlyWhileEnabledAndKeepsTheFileSmall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tilqos.log")
	log := NewLogger(path, 300)
	log.SetEnabled(true)
	for i := 0; i < 50; i++ {
		log.Printf("line %d with padding to make it long enough", i)
	}
	log.SetEnabled(false)
	log.Printf("written while off")
	info, err := os.Stat(path)
	if err != nil || info.Size() > 300 {
		t.Fatalf("size %v %v", info, err)
	}
	old, err := os.Stat(path + ".old")
	if err != nil || old.Size() > 300 {
		t.Fatal("one rotated copy, equally small")
	}
	current, _ := os.ReadFile(path)
	if strings.Contains(string(current), "written while off") {
		t.Fatal("a line was written after the log was switched off")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 2 {
		t.Fatalf("exactly the log and its rotated copy, found %d files", len(entries))
	}
}

func TestLoggerClearDeletesBothFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tilqos.log")
	log := NewLogger(path, 100)
	log.SetEnabled(true)
	for i := 0; i < 20; i++ {
		log.Printf("line %d padding padding", i)
	}
	if err := log.Clear(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path + ".old"} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("%s survived Clear", name)
		}
	}
	log.Printf("and it still works afterwards")
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "still works") {
		t.Fatal("logging after Clear")
	}
	var nilLogger *Logger
	nilLogger.SetEnabled(true)
	if err := nilLogger.Clear(); err != nil || nilLogger.Enabled() || nilLogger.Path() != "" {
		t.Fatal("nil logger")
	}
}

// MARK: - The clipboard rules

type memClip struct {
	list  []clipFormatInfo
	blobs map[uint32][]byte
	reads map[uint32]int
}

func newMemClip() *memClip { return &memClip{blobs: map[uint32][]byte{}, reads: map[uint32]int{}} }

func (m *memClip) add(id uint32, name string, data []byte) *memClip {
	m.list = append(m.list, clipFormatInfo{id: id, name: name})
	m.blobs[id] = data
	return m
}

func (m *memClip) text(s string) *memClip {
	units := append(utf16.Encode([]rune(s)), 0)
	data := make([]byte, 2*len(units))
	for i, u := range units {
		data[2*i], data[2*i+1] = byte(u), byte(u>>8)
	}
	return m.add(cfUnicodeText, "", data)
}

func (m *memClip) formats() []clipFormatInfo { return m.list }
func (m *memClip) data(id uint32) ([]byte, bool) {
	m.reads[id]++
	data, ok := m.blobs[id]
	return data, ok
}

var (
	dwordZero = []byte{0, 0, 0, 0}
	dwordOne  = []byte{1, 0, 0, 0}
)

func TestFlaggedClipboardIsNeverReadConvertedOrOverwritten(t *testing.T) {
	cases := map[string]*memClip{
		"exclude from monitor": newMemClip().text("hunter2").add(0xC001, core.FormatExcludeFromMonitor, dwordOne),
		"history 0":            newMemClip().text("hunter2").add(0xC002, core.FormatCanIncludeInHistory, dwordZero),
		"cloud 0":              newMemClip().text("hunter2").add(0xC003, core.FormatCanUploadToCloud, dwordZero),
	}
	for name, clip := range cases {
		text, reading := readGuardedText(clip)
		if reading != clipSensitive || text != "" {
			t.Errorf("%s: read %q (%v)", name, text, reading)
		}
		if clip.reads[cfUnicodeText] != 0 {
			t.Errorf("%s: the text was read", name)
		}
		if saved, ok := saveGuarded(clip, clipboardSaveLimit); ok || saved != nil {
			t.Errorf("%s: a flagged clipboard must not be borrowed", name)
		}
		if clip.reads[cfUnicodeText] != 0 {
			t.Errorf("%s: the text was copied for a restore", name)
		}
	}
}

func TestAllowingFlagsDoNotMakeTheClipboardOffLimits(t *testing.T) {
	clip := newMemClip().text("ghbdtn").
		add(0xC002, core.FormatCanIncludeInHistory, dwordOne).
		add(0xC003, core.FormatCanUploadToCloud, dwordOne)
	if text, reading := readGuardedText(clip); reading != clipText || text != "ghbdtn" {
		t.Fatalf("got %q %v", text, reading)
	}
}

func TestOnlyTheFlagFormatsAreReadToJudgeSensitivity(t *testing.T) {
	clip := newMemClip().text("ghbdtn").add(0xC010, "HTML Format", []byte("<b>x</b>")).
		add(0xC002, core.FormatCanIncludeInHistory, dwordOne)
	clipboardSensitive(clip)
	if clip.reads[cfUnicodeText] != 0 || clip.reads[0xC010] != 0 {
		t.Fatal("judging sensitivity reads the three flags and nothing else")
	}
}

func TestTextOverTenThousandCharactersIsIgnored(t *testing.T) {
	long := strings.Repeat("q", core.MaxClipboardText+1)
	if text, reading := readGuardedText(newMemClip().text(long)); reading != clipTooLong || text != "" {
		t.Fatalf("got %d chars, %v", len(text), reading)
	}
	ok := strings.Repeat("q", core.MaxClipboardText)
	if text, reading := readGuardedText(newMemClip().text(ok)); reading != clipText || text != ok {
		t.Fatalf("exactly the limit must pass: %v", reading)
	}
	if _, reading := readGuardedText(newMemClip()); reading != clipEmpty {
		t.Fatal("nothing on the clipboard")
	}
}

func TestEveryRestorableFormatIsSavedAndBitmapsAreNotPretended(t *testing.T) {
	clip := newMemClip().text("hello").
		add(0xC010, "HTML Format", []byte("<b>x</b>")).
		add(0xC011, "Rich Text Format", []byte("{\\rtf1}")).
		add(cfBitmap, "", []byte("handle")).
		add(0xC012, "Ole Private Data", []byte("ole")).
		add(0x0205, "", []byte("private"))
	saved, ok := saveGuarded(clip, clipboardSaveLimit)
	if !ok {
		t.Fatal("refused")
	}
	var ids []int
	for _, f := range saved {
		ids = append(ids, int(f.format))
	}
	sort.Ints(ids)
	if len(ids) != 3 || ids[0] != cfUnicodeText || ids[1] != 0xC010 || ids[2] != 0xC011 {
		t.Fatalf("saved formats %v: text, HTML and RTF are restorable; GDI handles, OLE and private formats are not", ids)
	}
}

func TestClipboardTooBigToPutBackIsNotBorrowed(t *testing.T) {
	clip := newMemClip().text("x").add(0xC010, "Big", make([]byte, 100))
	if saved, ok := saveGuarded(clip, 50); ok || saved != nil {
		t.Fatal("partial restores lose data: refuse instead")
	}
	if _, ok := saveGuarded(clip, 1000); !ok {
		t.Fatal("small enough")
	}
}

func TestTheTemporaryPasteIsMarkedWithTheSameFormats(t *testing.T) {
	// historyExclusions is what writeClipboardText sets on the pasted text; it must be exactly
	// what the rules look for on other applications' content.
	if len(historyExclusions) != 3 {
		t.Fatalf("%v", historyExclusions)
	}
	for i, name := range core.ClipboardFlagFormats {
		if historyExclusions[i] != name {
			t.Fatalf("%q != %q", historyExclusions[i], name)
		}
	}
	// ... and they all read as "do not keep" with the zero value the writer stores.
	flags := []core.ClipboardFlag{{Name: historyExclusions[1], Data: dwordZero}}
	if !core.ClipboardSensitive(flags) {
		t.Fatal("our own marker must read as sensitive")
	}
}

func TestDecodeUTF16Text(t *testing.T) {
	data := []byte{'h', 0, 0x4F, 0x04, 0, 0, 'x', 0} // "hя" then NUL, then junk after it
	if got := decodeUTF16Text(data); got != "hя" {
		t.Fatalf("got %q", got)
	}
}

// MARK: - Audits of the source

func internalGoFiles(t *testing.T, includeTools bool) []string {
	t.Helper()
	root := golden.ModuleRoot(t)
	var files []string
	for _, dir := range []string{"internal", "cmd", "tools"} {
		if dir == "tools" && !includeTools {
			continue
		}
		filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

func render(fset *token.FileSet, node ast.Node) string {
	var b bytes.Buffer
	printer.Fprint(&b, fset, node)
	return b.String()
}

// reviewedLogArguments are the values that may be passed to a log call after the format string:
// numbers, flags, language and layout names, fixed phrases, errors of the app itself, and
// engine.LogPrivate. Never typed text, clipboard content, window titles, or the name of the
// application in front (that is LogPrivate). A new argument fails the test until it is reviewed
// and added here.
var reviewedLogArguments = map[string]bool{
	"LogPrivate": true, "engine.LogPrivate": true, "Version": true, "configErr": true, "dpiMode": true, "dropped": true,
	"err": true, "errModifiersHeld": true, "errTypingContinued": true, "how": true,
	"id": true, "kind": true, "learnErr": true, "migrateErr": true, "reason": true,
	"runtime.GOARCH": true, "upgrade.Before": true, "upgrade.Kept": true,
	"outcome.Confidence": true, "outcome.Reason": true, "outcome.TargetLang": true,
	"o.Confidence": true, "o.TargetLang": true, "result.Strategy": true,
	"e.typing.Buffer.Count()": true, "r.DeleteCount()": true,
	"len(h.eventHooks)": true, "len(kept)": true, "len(list)": true, "len(o.Changes)": true,
	"len(rejected)": true, "len(tables)": true,
	"core.CharCount(o.After)": true, "core.CharCount(o.Before)": true,
	"core.CharCount(outcome.After)": true, "core.CharCount(outcome.Before)": true,
	"core.CharCount(plan.DeletedText)": true, "core.CharCount(result.Inserted)": true,
}

func TestEveryLoggedValueHasBeenReviewed(t *testing.T) {
	var unreviewed []string
	checked := 0
	for _, path := range internalGoFiles(t, false) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Printf" || !strings.HasSuffix(render(fset, sel.X), "log") {
				return true
			}
			checked++
			if len(call.Args) == 0 {
				return true
			}
			if _, literal := call.Args[0].(*ast.BasicLit); !literal {
				unreviewed = append(unreviewed, filepath.Base(path)+": the format is not a literal")
			}
			for _, arg := range call.Args[1:] {
				if text := render(fset, arg); !reviewedLogArguments[text] {
					unreviewed = append(unreviewed, filepath.Base(path)+": "+text)
				}
			}
			return true
		})
	}
	if checked < 30 {
		t.Fatalf("the audit found only %d log calls; the scanner is broken", checked)
	}
	if len(unreviewed) > 0 {
		t.Fatalf("log calls pass values nobody has reviewed. They may never be typed text, clipboard "+
			"text, window titles or the application's name (use engine.LogPrivate). Review them, then "+
			"add them to reviewedLogArguments:\n%s", strings.Join(unreviewed, "\n"))
	}
}

// engine.nothing logs its argument: every call must pass a literal, so the reason is a fixed
// phrase.
func TestNothingIsAlwaysGivenAFixedPhrase(t *testing.T) {
	path := filepath.Join(golden.ModuleRoot(t), "internal", "engine", "engine.go")
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "nothing" && render(fset, sel.X) == "e" {
			calls++
			if _, literal := call.Args[0].(*ast.BasicLit); !literal {
				t.Errorf("e.nothing(%s): not a fixed phrase", render(fset, call.Args[0]))
			}
		}
		return true
	})
	if calls < 5 {
		t.Fatalf("found %d calls", calls)
	}
}

// Everything is logged through Logger.Printf; nothing prints.
func TestNoStrayOutput(t *testing.T) {
	var offenders []string
	for _, path := range internalGoFiles(t, false) {
		if strings.HasSuffix(path, filepath.Join("cmd", "tilqos", "main_other.go")) {
			continue // the message of the non-Windows stub: two fixed lines
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		// A variable named log (the app's Logger) is fine; the standard package is not.
		importsLog := false
		for _, spec := range file.Imports {
			importsLog = importsLog || spec.Path.Value == `"log"`
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.CallExpr:
				text := render(fset, n.Fun)
				if strings.HasPrefix(text, "fmt.Print") || strings.HasPrefix(text, "fmt.Fprint") && len(n.Args) > 0 &&
					(strings.HasPrefix(render(fset, n.Args[0]), "os.Std")) ||
					importsLog && strings.HasPrefix(text, "log.") || text == "println" || text == "print" {
					offenders = append(offenders, filepath.Base(path)+": "+text)
				}
			case *ast.SelectorExpr:
				if text := render(fset, n); text == "os.Stderr" || text == "os.Stdout" {
					offenders = append(offenders, filepath.Base(path)+": "+text)
				}
			}
			return true
		})
	}
	if len(offenders) > 0 {
		t.Fatalf("output outside the Logger:\n%s", strings.Join(offenders, "\n"))
	}
}

// P6: nothing in the program imports a networking package, loads a networking DLL, or starts a
// process.
func TestNoNetworkCode(t *testing.T) {
	forbiddenImports := []string{"net", "net/http", "net/rpc", "net/smtp", "net/textproto", "crypto/tls", "os/exec", "plugin"}
	for _, path := range internalGoFiles(t, false) {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			imported := strings.Trim(spec.Path.Value, `"`)
			for _, bad := range forbiddenImports {
				// os/exec is allowed in the model-fetching helper that only the build tools use.
				if imported == bad && !strings.Contains(path, filepath.Join("internal", "modelsrc")) {
					t.Errorf("%s imports %s", filepath.Base(path), imported)
				}
			}
		}
	}
	root := golden.ModuleRoot(t)
	dlls := []string{"ws2_32", "winhttp", "wininet", "mswsock", "dnsapi", "iphlpapi", "urlmon", "wsock32", "httpapi"}
	filepath.Walk(filepath.Join(root, "internal", "win32"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, dll := range dlls {
			if strings.Contains(strings.ToLower(string(data)), dll) {
				t.Errorf("%s mentions %s.dll", filepath.Base(path), dll)
			}
		}
		return nil
	})
	if len(readGoMod(t, root)) != 0 {
		t.Errorf("go.mod requires modules: %v", readGoMod(t, root))
	}
}

func readGoMod(t *testing.T, root string) []string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	var requires []string
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "require") {
			requires = append(requires, line)
		}
	}
	return requires
}

// The linked program, not only the sources: the full import closure of the Windows executable
// has no networking package. (Needs the go tool; skipped without it.)
func TestWindowsExecutableLinksNoNetworkPackage(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go tool on PATH")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		cmd := exec.Command(goTool, "list", "-deps", "-f", "{{.ImportPath}}", "./cmd/tilqos")
		cmd.Dir = golden.ModuleRoot(t)
		cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+arch)
		out, err := cmd.Output()
		if err != nil {
			t.Skipf("go list failed: %v", err)
		}
		deps := strings.Fields(string(out))
		if len(deps) < 20 {
			t.Fatalf("suspiciously few dependencies: %d", len(deps))
		}
		for _, dep := range deps {
			if dep == "net" || strings.HasPrefix(dep, "net/") || dep == "crypto/tls" || dep == "os/exec" {
				t.Errorf("windows/%s links %s", arch, dep)
			}
		}
	}
}

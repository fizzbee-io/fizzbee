package modelchecker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

func imp(line int32, path, alias string, names ...string) *ast.Import {
	i := &ast.Import{
		SourceInfo: &ast.SourceInfo{Start: &ast.Position{Line: line}},
		Path:       path,
		Alias:      alias,
	}
	for _, n := range names {
		name, as, _ := strings.Cut(n, " as ")
		i.Names = append(i.Names, &ast.ImportName{Name: name, Alias: as})
	}
	return i
}

func writeTestFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func resolveFile(t *testing.T, dir string, file *ast.File) (starlark.StringDict, []string, error) {
	t.Helper()
	var warnings []string
	loader := newModuleLoader(dir)
	loader.resolver.warn = func(msg string) { warnings = append(warnings, msg) }
	if file.SourceInfo == nil {
		file.SourceInfo = &ast.SourceInfo{FileName: "Spec.fizz"}
	}
	bound, err := resolveImports(file, loader)
	return bound, warnings, err
}

func resolveQuiet(t *testing.T, dir string, imports ...*ast.Import) (starlark.StringDict, []string, error) {
	t.Helper()
	return resolveFile(t, dir, &ast.File{Imports: imports})
}

func TestResolveBuiltinImports(t *testing.T) {
	bound, warnings, err := resolveQuiet(t, t.TempDir(),
		imp(1, "symmetry", ""),
		imp(2, "math", "m"),
		imp(3, "math", "", "floor", "ceil as up"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v", warnings)
	}
	if bound["symmetry"] != lib.ImportableModules["symmetry"] {
		t.Errorf("symmetry not bound to the builtin module")
	}
	if bound["m"] != lib.ImportableModules["math"] {
		t.Errorf("m not bound to the math module")
	}
	for _, name := range []string{"floor", "up"} {
		if _, ok := bound[name].(*starlark.Builtin); !ok {
			t.Errorf("%s = %v, want a builtin", name, bound[name])
		}
	}
	if len(bound) != 4 {
		t.Errorf("bound %d names, want 4: %v", len(bound), bound.Keys())
	}
}

func TestResolveUserModules(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "util.star", "def double(x):\n    return 2 * x\nITEMS = [1, 2]\n")
	writeTestFile(t, dir, "lib/queue.star", "CAP = 3\n")
	bound, _, err := resolveQuiet(t, dir,
		imp(1, "util", ""),
		imp(2, "lib.queue", "q"),
		imp(3, "lib.queue", "", "CAP as cap"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if bound["q"] == nil || bound["cap"] != starlark.MakeInt(3) {
		t.Errorf("bound = %v", bound)
	}
	items, _ := moduleAttr(bound["util"], "ITEMS")
	if err := items.(*starlark.List).Append(starlark.MakeInt(3)); err == nil {
		t.Errorf("a module's list can be changed; modules must be frozen")
	}
}

func TestModuleLoadedOnce(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "util.star", "X = []\n")
	bound, _, err := resolveQuiet(t, dir, imp(1, "util", "a"), imp(2, "util", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if bound["a"] != bound["b"] {
		t.Errorf("one module imported twice gave two values")
	}
}

func TestLocalFileHidesBuiltin(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "math.star", "X = 1\n")
	bound, warnings, err := resolveQuiet(t, dir, imp(1, "math", ""))
	if err != nil {
		t.Fatal(err)
	}
	if bound["math"] == lib.ImportableModules["math"] {
		t.Errorf("the builtin math module hid a local math.star; local files must win")
	}
	if v, _ := moduleAttr(bound["math"], "X"); v != starlark.MakeInt(1) {
		t.Errorf("math.X = %v, want the local file's 1", v)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "hides the builtin module math") {
		t.Errorf("warnings = %v, want one saying the builtin is hidden", warnings)
	}
}

func TestOneFileTwoPathsLoadsOnce(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "lib/util.star", "X = []\n")
	if err := os.Symlink(filepath.Join(dir, "lib", "util.star"), filepath.Join(dir, "util.star")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	bound, _, err := resolveQuiet(t, dir, imp(1, "util", ""), imp(2, "lib.util", "u"))
	if err != nil {
		t.Fatal(err)
	}
	if bound["util"] != bound["u"] {
		t.Errorf("one file reached through a symlink and a dotted path was loaded twice")
	}
}

func TestNestedValuesAreFrozen(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "util.star", "DATA = {\"items\": [1, 2, 3]}\n")
	bound, _, err := resolveQuiet(t, dir, imp(1, "util", "", "DATA"))
	if err != nil {
		t.Fatal(err)
	}
	items, _, _ := bound["DATA"].(*starlark.Dict).Get(starlark.String("items"))
	if err := items.(*starlark.List).Append(starlark.MakeInt(4)); err == nil {
		t.Errorf("a list nested in a module dict can be changed")
	}
	if err := bound["DATA"].(*starlark.Dict).SetKey(starlark.String("k"), starlark.None); err == nil {
		t.Errorf("a module dict imported with from can be changed")
	}
}

func TestBrokenModuleErrorIsShared(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "bad.star", "def f(:\n")
	loader := newModuleLoader(dir)
	_, err1 := loader.load("bad")
	_, err2 := loader.load("bad")
	if err1 == nil || err1 != err2 {
		t.Errorf("errors = %v, %v; want the same cached error twice", err1, err2)
	}
}

func TestImportClashesWithDeclarations(t *testing.T) {
	tests := []struct {
		name string
		file *ast.File
		want string
	}{
		{"function", &ast.File{Functions: []*ast.Function{{Name: "math"}}}, "also the name of a function"},
		{"role", &ast.File{Roles: []*ast.Role{{Name: "math"}}}, "also the name of a role"},
		{"action", &ast.File{Actions: []*ast.Action{{Name: "math"}}}, "also the name of an action"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.file.Imports = []*ast.Import{imp(1, "math", "")}
			_, _, err := resolveFile(t, t.TempDir(), tt.file)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
	// An alias avoids the clash.
	file := &ast.File{Functions: []*ast.Function{{Name: "math"}}, Imports: []*ast.Import{imp(1, "math", "m")}}
	if _, _, err := resolveFile(t, t.TempDir(), file); err != nil {
		t.Errorf("aliased import: %v", err)
	}
}

func TestImportErrors(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "bad.star", "def f(:\n")
	writeTestFile(t, dir, "spec.fizz", "action Init:\n    x = 0\n")
	writeTestFile(t, dir, "ok.star", "A = 1\n_hidden = 2\n")
	tests := []struct {
		name    string
		imports []*ast.Import
		want    string
	}{
		{"missing", []*ast.Import{imp(4, "nosuch", "")}, "Spec.fizz:4: module \"nosuch\" not found: looked for /"},
		{"no member", []*ast.Import{imp(2, "ok", "", "B")}, "has no member \"B\" (it has: A)"},
		{"parse error", []*ast.Import{imp(1, "bad", "")}, "failed to load"},
		{"fizz files are not modules", []*ast.Import{imp(1, "spec", "")}, "module \"spec\" not found"},
		{"private member", []*ast.Import{imp(1, "ok", "", "_hidden")}, "names starting with _ are private"},
		{"twice", []*ast.Import{imp(1, "ok", "x"), imp(2, "math", "x")}, "Spec.fizz:2: \"x\" is already imported on line 1"},
		{"member clash", []*ast.Import{imp(1, "ok", "A"), imp(2, "ok", "", "A")}, "\"A\" is already imported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := resolveQuiet(t, dir, tt.imports...)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestNoBaseDirAllowsOnlyBuiltins(t *testing.T) {
	if _, _, err := resolveQuiet(t, "", imp(1, "threads_not_registered", "")); err == nil {
		t.Errorf("want an error for an unknown module without a base directory")
	}
	if _, _, err := resolveQuiet(t, "", imp(1, "math", "")); err != nil {
		t.Errorf("builtin import without a base directory: %v", err)
	}
}

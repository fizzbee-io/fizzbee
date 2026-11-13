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

func resolveQuiet(t *testing.T, dir string, imports ...*ast.Import) (starlark.StringDict, []string, error) {
	t.Helper()
	var warnings []string
	loader := newModuleLoader(dir)
	loader.Warn = func(msg string) { warnings = append(warnings, msg) }
	file := &ast.File{SourceInfo: &ast.SourceInfo{FileName: "Spec.fizz"}, Imports: imports}
	bound, err := resolveImports(file, loader)
	return bound, warnings, err
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

func TestBuiltinHidesLocalFile(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "math.star", "X = 1\n")
	bound, warnings, err := resolveQuiet(t, dir, imp(1, "math", ""))
	if err != nil {
		t.Fatal(err)
	}
	if bound["math"] != lib.ImportableModules["math"] {
		t.Errorf("a local math.star replaced the builtin module")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "math.star") {
		t.Errorf("warnings = %v, want one naming math.star", warnings)
	}
}

func TestImportErrors(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "bad.star", "def f(:\n")
	writeTestFile(t, dir, "spec.fizz", "action Init:\n    x = 0\n")
	writeTestFile(t, dir, "ok.star", "A = 1\n")
	tests := []struct {
		name    string
		imports []*ast.Import
		want    string
	}{
		{"missing", []*ast.Import{imp(4, "nosuch", "")}, "Spec.fizz:4: module \"nosuch\" not found"},
		{"no member", []*ast.Import{imp(2, "ok", "", "B")}, "has no member \"B\" (it has: A)"},
		{"parse error", []*ast.Import{imp(1, "bad", "")}, "module \"bad\" failed to load"},
		{"fizz module", []*ast.Import{imp(1, "spec", "")}, "importing .fizz specs is not supported yet"},
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

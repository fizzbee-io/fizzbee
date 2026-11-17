package modelchecker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// Import contract (see the design doc):
//   - Imports come before every other top-level statement in a file (the
//     parser enforces this) and are resolved and loaded eagerly, before any
//     top-level statement runs. A broken import fails the run even if the
//     imported name is never used.
//   - `import a.b` is resolved to <spec dir>/a/b.star first, then to a
//     builtin module (lib.ImportableModules). A local file wins, so adding
//     a builtin in a later release never changes what an existing import
//     means. .fizz files are not looked up.
//   - Imported names are a namespace of their own: not state, not hashed,
//     visible everywhere in the file. A top-level variable, state variable,
//     function, role or action with the same name is a load-time error; a
//     local or parameter may shadow an import inside its scope.
//   - User modules are frozen deeply after loading, and `from m import _x`
//     is an error, as with Starlark's load().

// ImportError is a problem with one import statement, reported at load
// time with the line it is on.
type ImportError struct {
	File string
	Line int32
	Msg  string
}

func (e *ImportError) Error() string {
	switch {
	case e.Line == 0 && e.File == "":
		return e.Msg
	case e.Line == 0:
		return fmt.Sprintf("%s: %s", e.File, e.Msg)
	case e.File == "":
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

// displayPath shows a path relative to the working directory when it is
// under it, else absolute, so messages are short and stable across machines.
func displayPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return abs
}

// moduleSource says where a module path resolved to. key identifies the
// module: "builtin:<name>" for builtins and the canonical file path for
// user modules, so one file reached two ways is loaded once.
type moduleSource struct {
	key     string
	builtin starlark.Value
	file    string
}

// moduleResolver maps an import path to a module source. It does not load
// anything.
type moduleResolver struct {
	// baseDir is the directory of the spec. Empty means only builtin
	// modules can be imported.
	baseDir string
	warn    func(string)
}

func moduleFile(baseDir, path string) string {
	return filepath.Join(baseDir, filepath.Join(strings.Split(path, ".")...)+".star")
}

func isModuleFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// canonicalPath returns an absolute path with symlinks resolved.
func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

func (r *moduleResolver) resolve(path string) (moduleSource, error) {
	builtin, isBuiltin := lib.ImportableModules[path]
	if r.baseDir != "" {
		file := moduleFile(r.baseDir, path)
		if isModuleFile(file) {
			canonical, err := canonicalPath(file)
			if err != nil {
				return moduleSource{}, fmt.Errorf("module %q: %v", path, err)
			}
			if isBuiltin {
				r.warn(fmt.Sprintf("import %s uses %s, which hides the builtin module %s", path, displayPath(canonical), path))
			}
			return moduleSource{key: canonical, file: canonical}, nil
		}
	}
	if isBuiltin {
		return moduleSource{key: "builtin:" + path, builtin: builtin}, nil
	}
	if r.baseDir == "" {
		return moduleSource{}, fmt.Errorf("module %q not found: there is no builtin module with that name", path)
	}
	searched := displayPath(moduleFile(r.baseDir, path))
	return moduleSource{}, fmt.Errorf("module %q not found: looked for %s and for a builtin module", path, searched)
}

// moduleLoader loads, freezes and caches modules for one run. Loading
// happens once, on one goroutine, before the model checker starts, so the
// cache needs no locking.
type moduleLoader struct {
	resolver moduleResolver
	cache    map[string]starlark.Value
	// errs caches failures too, so every importer of a broken module gets
	// the same error without loading it again.
	errs map[string]error
}

func newModuleLoader(baseDir string) *moduleLoader {
	return &moduleLoader{
		resolver: moduleResolver{
			baseDir: baseDir,
			warn:    func(msg string) { fmt.Fprintln(os.Stderr, "WARNING:", msg) },
		},
		cache: map[string]starlark.Value{},
		errs:  map[string]error{},
	}
}

func (l *moduleLoader) load(path string) (starlark.Value, error) {
	src, err := l.resolver.resolve(path)
	if err != nil {
		return nil, err
	}
	if m, ok := l.cache[src.key]; ok {
		return m, nil
	}
	if err, ok := l.errs[src.key]; ok {
		return nil, err
	}
	if src.builtin != nil {
		l.cache[src.key] = src.builtin
		return src.builtin, nil
	}
	members, err := LoadModule(displayPath(src.file))
	if err != nil {
		err = fmt.Errorf("module %q (%s) failed to load: %v", path, displayPath(src.file), err)
		l.errs[src.key] = err
		return nil, err
	}
	// Freeze is deep: lists and dicts nested in module values are frozen
	// too, so a module cannot become hidden mutable state.
	members.Freeze()
	name := strings.TrimSuffix(filepath.Base(src.file), ".star")
	m := &starlarkstruct.Module{Name: name, Members: members}
	l.cache[src.key] = m
	return m, nil
}

// moduleAttr returns a member of a module value.
func moduleAttr(m starlark.Value, name string) (starlark.Value, bool) {
	if mod, ok := m.(*starlarkstruct.Module); ok {
		v, ok := mod.Members[name]
		return v, ok
	}
	if ha, ok := m.(starlark.HasAttrs); ok {
		v, err := ha.Attr(name)
		return v, err == nil && v != nil
	}
	return nil, false
}

func moduleAttrNames(m starlark.Value) []string {
	var names []string
	if mod, ok := m.(*starlarkstruct.Module); ok {
		for k := range mod.Members {
			names = append(names, k)
		}
	} else if ha, ok := m.(starlark.HasAttrs); ok {
		names = ha.AttrNames()
	}
	public := names[:0]
	for _, n := range names {
		if !strings.HasPrefix(n, "_") {
			public = append(public, n)
		}
	}
	sort.Strings(public)
	return public
}

// ResolveImports binds the names a file's import statements introduce.
// `import a as b` binds b to module a; `from a import x as y` binds y to
// member x of a.
func ResolveImports(file *ast.File, baseDir string) (starlark.StringDict, error) {
	return resolveImports(file, newModuleLoader(baseDir))
}

// reservedNames are language-level names an import may not bind. Role
// constructors are covered by the role check in declaredNames. Ordinary
// builtins such as len or math may be shadowed by an import.
var reservedNames = map[string]bool{
	"Channel":      true,
	"deepcopy":     true,
	"resolve_role": true,
	"self":         true,
}

// declaredNames maps each name the file declares at the top level, other
// than variables, to what declares it.
func declaredNames(file *ast.File) map[string]string {
	names := map[string]string{}
	for name := range reservedNames {
		names[name] = "a reserved FizzBee name"
	}
	for _, f := range file.GetFunctions() {
		names[f.GetName()] = "the name of a function in this file"
	}
	for _, r := range file.GetRoles() {
		names[r.GetName()] = "the name of a role in this file"
	}
	for _, a := range file.GetActions() {
		names[a.GetName()] = "the name of an action in this file"
	}
	return names
}

func resolveImports(file *ast.File, loader *moduleLoader) (starlark.StringDict, error) {
	bound := starlark.StringDict{}
	boundAt := map[string]int32{}
	declared := declaredNames(file)
	fileName := file.GetSourceInfo().GetFileName()
	bind := func(imp *ast.Import, name string, v starlark.Value) error {
		line := imp.GetSourceInfo().GetStart().GetLine()
		if prev, ok := boundAt[name]; ok {
			return &ImportError{fileName, line, fmt.Sprintf("%q is already imported on line %d", name, prev)}
		}
		if kind, ok := declared[name]; ok {
			return &ImportError{fileName, line, fmt.Sprintf("import binds %q, which is already %s; use 'as' to rename the import", name, kind)}
		}
		bound[name] = v
		boundAt[name] = line
		return nil
	}
	for _, imp := range file.GetImports() {
		line := imp.GetSourceInfo().GetStart().GetLine()
		m, err := loader.load(imp.GetPath())
		if err != nil {
			return nil, &ImportError{fileName, line, err.Error()}
		}
		if len(imp.GetNames()) == 0 {
			name := imp.GetAlias()
			if name == "" {
				if strings.Contains(imp.GetPath(), ".") {
					return nil, &ImportError{fileName, line, fmt.Sprintf("import %s needs an alias", imp.GetPath())}
				}
				name = imp.GetPath()
			}
			if err := bind(imp, name, m); err != nil {
				return nil, err
			}
			continue
		}
		for _, n := range imp.GetNames() {
			if strings.HasPrefix(n.GetName(), "_") {
				return nil, &ImportError{fileName, line, fmt.Sprintf("cannot import %q from %q: names starting with _ are private to their module", n.GetName(), imp.GetPath())}
			}
			v, ok := moduleAttr(m, n.GetName())
			if !ok {
				return nil, &ImportError{fileName, line, fmt.Sprintf("module %q has no member %q (it has: %s)", imp.GetPath(), n.GetName(), strings.Join(moduleAttrNames(m), ", "))}
			}
			name := n.GetAlias()
			if name == "" {
				name = n.GetName()
			}
			if err := bind(imp, name, v); err != nil {
				return nil, err
			}
		}
	}
	return bound, nil
}

// unimportedModules lists the .star files in dir whose names the spec's
// imports do not bind.
func unimportedModules(dir string, imports starlark.StringDict) map[string]bool {
	files, _ := filepath.Glob(filepath.Join(dir, "*.star"))
	names := map[string]bool{}
	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".star")
		if _, ok := imports[name]; !ok {
			names[name] = true
		}
	}
	return names
}

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

// ImportError is a problem with one import statement, reported at load
// time with the line it is on.
type ImportError struct {
	File string
	Line int32
	Msg  string
}

func (e *ImportError) Error() string {
	if e.File == "" {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
}

// moduleLoader resolves module paths for one run and loads each user
// module at most once.
type moduleLoader struct {
	// baseDir is the directory of the spec. Empty means only builtin
	// modules can be imported.
	baseDir string
	cache   map[string]starlark.Value
	// Warn receives non-fatal messages, such as a builtin module hiding a
	// user module file of the same name.
	Warn func(string)
}

func newModuleLoader(baseDir string) *moduleLoader {
	return &moduleLoader{
		baseDir: baseDir,
		cache:   map[string]starlark.Value{},
		Warn:    func(msg string) { fmt.Fprintln(os.Stderr, "WARNING:", msg) },
	}
}

// moduleFiles returns the candidate files for a dotted module path, in
// the order they are tried: m.fizz, then m.star.
func moduleFiles(baseDir, path string) []string {
	rel := filepath.Join(strings.Split(path, ".")...)
	return []string{
		filepath.Join(baseDir, rel+".fizz"),
		filepath.Join(baseDir, rel+".star"),
	}
}

func isModuleFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// load resolves path to a module value: a builtin module first, then a
// file under the spec's directory.
func (l *moduleLoader) load(path string) (starlark.Value, error) {
	if m, ok := l.cache[path]; ok {
		return m, nil
	}
	var files []string
	if l.baseDir != "" {
		files = moduleFiles(l.baseDir, path)
	}
	if m, ok := lib.ImportableModules[path]; ok {
		for _, f := range files {
			if isModuleFile(f) {
				l.Warn(fmt.Sprintf("import %s uses the builtin module; %s is ignored. Move it into a directory and import it with a dotted path to use it.", path, f))
				break
			}
		}
		l.cache[path] = m
		return m, nil
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no builtin module named %q", path)
	}
	fizzFile, starFile := files[0], files[1]
	if isModuleFile(fizzFile) {
		if isModuleFile(starFile) {
			return nil, fmt.Errorf("module %q: importing .fizz specs is not supported yet, and %s would hide %s", path, fizzFile, starFile)
		}
		return nil, fmt.Errorf("module %q: importing .fizz specs is not supported yet (%s)", path, fizzFile)
	}
	if !isModuleFile(starFile) {
		return nil, fmt.Errorf("module %q not found: no builtin module and no %s or %s", path, fizzFile, starFile)
	}
	members, err := LoadModule(starFile)
	if err != nil {
		return nil, fmt.Errorf("module %q failed to load: %v", path, err)
	}
	members.Freeze()
	name := path[strings.LastIndex(path, ".")+1:]
	m := &starlarkstruct.Module{Name: name, Members: members}
	l.cache[path] = m
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
	if mod, ok := m.(*starlarkstruct.Module); ok {
		names := make([]string, 0, len(mod.Members))
		for k := range mod.Members {
			names = append(names, k)
		}
		sort.Strings(names)
		return names
	}
	if ha, ok := m.(starlark.HasAttrs); ok {
		return ha.AttrNames()
	}
	return nil
}

// ResolveImports binds the names a file's import statements introduce.
// `import a as b` binds b to module a; `from a import x as y` binds y to
// member x of a. Binding a name twice is an error.
func ResolveImports(file *ast.File, baseDir string) (starlark.StringDict, error) {
	return resolveImports(file, newModuleLoader(baseDir))
}

func resolveImports(file *ast.File, loader *moduleLoader) (starlark.StringDict, error) {
	bound := starlark.StringDict{}
	boundAt := map[string]int32{}
	fileName := file.GetSourceInfo().GetFileName()
	bind := func(imp *ast.Import, name string, v starlark.Value) error {
		line := imp.GetSourceInfo().GetStart().GetLine()
		if prev, ok := boundAt[name]; ok {
			return &ImportError{fileName, line, fmt.Sprintf("%q is already imported on line %d", name, prev)}
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

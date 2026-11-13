package lib

import (
	"go.starlark.net/lib/math"
	"go.starlark.net/starlark"
)

// ImportableModules are the builtin modules a spec can name in an import
// statement, as in `import threads` or `from symmetry import nominal`.
// A builtin module wins over a user module file of the same name.
//
// The first four are also predeclared in Builtins, so older specs that use
// them without importing keep working. New modules are import-only and
// are added here, not to Builtins.
var ImportableModules = map[string]starlark.Value{
	"math":        math.Module,
	"itertools":   ItertoolsModule,
	"symmetry":    SymmetryModule,
	"cardinality": CardinalityModule,
}

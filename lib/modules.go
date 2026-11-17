package lib

import (
	"go.starlark.net/lib/math"
	"go.starlark.net/starlark"
)

// ImportableModules are the builtin modules a spec can name in an import
// statement, as in `import threads` or `from symmetry import nominal`.
// A .star file next to the spec with the same name wins over a builtin
// module, so adding a builtin never changes what an existing import means.
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

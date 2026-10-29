package lib

import "sync/atomic"

// symmetryUsed records that some symmetric value has been created in this
// process: by the symmetric_values builtin, a symmetry domain constructor,
// or a symmetric role. Those are the only origins; everything else derives
// values from existing ones. The checker consults it to decide whether a
// state can need symmetry canonicalization at all.
var symmetryUsed atomic.Bool

// MarkSymmetryUsed records that a symmetric value origin has run.
func MarkSymmetryUsed() { symmetryUsed.Store(true) }

// SymmetryUsed reports whether any symmetric value origin has run.
func SymmetryUsed() bool { return symmetryUsed.Load() }

// ResetSymmetryUsed clears the record, for the start of a fresh run (the
// companion of ClearRoleRefs).
func ResetSymmetryUsed() { symmetryUsed.Store(false) }

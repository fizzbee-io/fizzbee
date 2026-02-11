package modelchecker

import (
	"fmt"
	"math"

	"github.com/fizzbee-io/fizzbee/lib"
)

// CheckSymmetryConstraints checks post-transition symmetry constraints.
// 1. Limit check (interval & ordinal): the number of distinct used values
//    must not exceed the domain's Limit. Arithmetic on symmetric values can
//    create values beyond what fresh() would allow; this catches that.
// 2. Divergence check (interval only): the spread (max - min) of used values
//    must not exceed the domain's Divergence.
// Returns (false, error) if a constraint is violated; the caller should prune the path.
func CheckSymmetryConstraints(process *Process) (bool, error) {
	// Fast path: check if any domains need constraint checking before
	// doing the expensive state visitor walk.
	needsCheck := false
	for _, val := range process.Heap.globals {
		if domain, ok := val.(*lib.SymmetryDomain); ok {
			if domain.Kind == lib.SymmetryKindInterval || domain.Kind == lib.SymmetryKindOrdinal {
				needsCheck = true
				break
			}
		}
	}
	if !needsCheck {
		return true, nil
	}

	visitor := NewUsedSymmetricValuesCollector()
	process.AcceptVisitor(visitor)
	usedIDs := visitor.GetAllUsedIDs()

	for _, val := range process.Heap.globals {
		domain, ok := val.(*lib.SymmetryDomain)
		if !ok {
			continue
		}
		if domain.Kind != lib.SymmetryKindInterval && domain.Kind != lib.SymmetryKindOrdinal {
			continue
		}

		ids, found := usedIDs[domain.Name]
		if !found || len(ids) == 0 {
			continue
		}

		// Limit check: arithmetic can create values beyond the domain limit.
		if len(ids) > domain.Limit {
			return false, fmt.Errorf("symmetry limit exceeded for domain %q (limit %d, used %d)", domain.Name, domain.Limit, len(ids))
		}

		// Divergence check (interval only).
		if domain.Kind == lib.SymmetryKindInterval && domain.Divergence > 0 {
			minID := int64(math.MaxInt64)
			maxID := int64(math.MinInt64)
			for _, id := range ids {
				if id < minID {
					minID = id
				}
				if id > maxID {
					maxID = id
				}
			}

			spread := maxID - minID
			if spread > int64(domain.Divergence) {
				return false, fmt.Errorf("symmetry divergence exceeded for domain %q (divergence %d, spread %d)", domain.Name, domain.Divergence, spread)
			}
		}
	}

	return true, nil
}

package modelchecker

import (
	"fmt"
	"math"

	"github.com/fizzbee-io/fizzbee/lib"
)

// CheckSymmetryConstraints checks post-transition symmetry constraints.
// Currently checks the divergence constraint for interval symmetry domains:
// the spread (max - min) of used values must not exceed the domain's divergence.
// Returns (false, error) if a constraint is violated; the caller should prune the path.
func CheckSymmetryConstraints(process *Process) (bool, error) {
	// Fast path: check if any interval domains with divergence exist before
	// doing the expensive state visitor walk.
	hasIntervalDivergence := false
	for _, val := range process.Heap.globals {
		if domain, ok := val.(*lib.SymmetryDomain); ok {
			if domain.Kind == lib.SymmetryKindInterval && domain.Divergence > 0 {
				hasIntervalDivergence = true
				break
			}
		}
	}
	if !hasIntervalDivergence {
		return true, nil
	}

	visitor := NewUsedSymmetricValuesCollector()
	process.AcceptVisitor(visitor)
	usedIDs := visitor.GetAllUsedIDs()

	for _, val := range process.Heap.globals {
		domain, ok := val.(*lib.SymmetryDomain)
		if !ok || domain.Kind != lib.SymmetryKindInterval || domain.Divergence <= 0 {
			continue
		}

		ids, found := usedIDs[domain.Name]
		if !found || len(ids) == 0 {
			continue
		}

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

	return true, nil
}

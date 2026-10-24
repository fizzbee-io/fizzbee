package modelchecker

import (
	"slices"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

// respaceOrdinalIDs rewrites the raw IDs of ordinal symmetric values in the
// process state, when a domain's IDs have become too close together for
// midpoint allocation to keep inserting. Each domain that needs it gets its
// live IDs spread evenly by rank. Only the order of ordinal values is
// observable, and canonical hashing already rank-squashes them, so the
// respaced state hashes and behaves the same as the original.
//
// Returns true if the state was rewritten.
func (p *Process) respaceOrdinalIDs() bool {
	ordinalDomains := make(map[string]bool)
	for _, val := range p.Heap.globals {
		if domain, ok := val.(*lib.SymmetryDomain); ok {
			// Materialized domains hold the fixed IDs 0..limit-1 and never
			// allocate by midpoint.
			if domain.Kind == lib.SymmetryKindOrdinal && !domain.Materialize {
				ordinalDomains[domain.Name] = true
			}
		}
	}
	if len(ordinalDomains) == 0 {
		return false
	}

	collector := &ordinalIDCollector{
		UsedSymmetricValuesCollector: NewUsedSymmetricValuesCollector(),
		segmentIDs:                   make(map[string]map[int64]bool),
	}
	p.AcceptVisitor(collector)
	newIDs := make(map[string]map[int64]int64)
	for name := range ordinalDomains {
		ids := collector.GetUsedIds(name)
		// A held segment can outlive the values at its ends. Respace its
		// endpoints along with the live values so it keeps its place.
		for id := range collector.segmentIDs[name] {
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		if len(ids) == 0 || !lib.OrdinalNeedsRespace(ids) {
			continue
		}
		respaced := lib.OrdinalRespacedIDs(len(ids))
		mapping := make(map[int64]int64, len(ids))
		for i, id := range ids {
			mapping[id] = respaced[i]
		}
		newIDs[name] = mapping
	}
	if len(newIDs) == 0 {
		return false
	}

	// Clone once to collect every symmetric value pointer in the state, then
	// clone that copy with a mapping for each pointer. The second clone also
	// remaps segment endpoints held in the state. Values of other domains
	// map to plain copies.
	p2, refs, _ := p.cloneAndGetSymmetricValuePointers()
	permutations := make(map[*lib.SymmetricValue][]*lib.SymmetricValue)
	remap := func(sv *lib.SymmetricValue) {
		id := sv.GetId()
		if mapping, ok := newIDs[sv.GetPrefix()]; ok {
			id = mapping[id]
		}
		newVal := lib.NewSymmetricValueWithKind(sv.GetPrefix(), id, sv.GetKind())
		if sv.Kind == lib.SymmetryKindRotational {
			newVal = lib.NewRotationalSymmetricValue(sv.GetPrefix(), id, sv.Limit)
		}
		permutations[sv] = []*lib.SymmetricValue{newVal}
	}
	for _, val := range refs {
		if sv, ok := val.(*lib.SymmetricValue); ok {
			remap(sv)
		}
	}
	// Returns are not cloned with the state, so values that appear only
	// there are not in refs. Add them so the clone of Returns below can
	// find them by value.
	for _, byID := range collector.pointers {
		for _, sv := range byID {
			remap(sv)
		}
	}
	// Endpoints of held segments whose values are gone from the state. The
	// clone maps segment endpoints by value, so a stand-in key is enough.
	for name, ids := range collector.segmentIDs {
		mapping, ok := newIDs[name]
		if !ok {
			continue
		}
		for id := range ids {
			if collector.usedValues[name][id] {
				continue
			}
			key := lib.NewSymmetricValueWithKind(name, id, lib.SymmetryKindOrdinal)
			permutations[key] = []*lib.SymmetricValue{lib.NewSymmetricValueWithKind(name, mapping[id], lib.SymmetryKindOrdinal)}
		}
	}
	p3refs := make(map[starlark.Value]starlark.Value)
	p3 := p2.CloneWithRefs(permutations, 0, p3refs)
	returns := make(starlark.StringDict, len(p.Returns))
	for k, v := range p.Returns {
		cloned, err := deepCloneStarlarkValueWithPermutations(v, p3refs, permutations, 0)
		PanicOnError(err)
		returns[k] = cloned
	}

	// Swap the respaced state into p, keeping p's identity and bookkeeping
	// (parent, children, stats, labels).
	p.Heap = p3.Heap
	for _, thread := range p3.Threads {
		if thread != nil {
			thread.Process = p
		}
	}
	p.Threads = p3.Threads
	p.Roles = p3.Roles
	p.ChannelMessages = p3.ChannelMessages
	p.Returns = returns
	return true
}

// ordinalIDCollector collects the symmetric values in a state, and the
// endpoints of the ordinal segments held in it.
type ordinalIDCollector struct {
	*UsedSymmetricValuesCollector
	segmentIDs map[string]map[int64]bool // domain -> endpoint IDs
}

func (c *ordinalIDCollector) VisitSegment(s *lib.Segment) {
	name := s.Domain.Name
	if c.segmentIDs[name] == nil {
		c.segmentIDs[name] = make(map[int64]bool)
	}
	if !s.IsHead {
		c.segmentIDs[name][s.Left] = true
	}
	if !s.IsTail {
		c.segmentIDs[name][s.Right] = true
	}
}

package modelchecker

import (
	"fmt"
	"slices"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

// StateVisitor defines the interface for visiting state elements during traversal
type StateVisitor interface {
	VisitSymmetricValue(sv *lib.SymmetricValue)
}

// UsedSymmetricValuesCollector collects all SymmetricValue instances in the current state
type UsedSymmetricValuesCollector struct {
	usedValues map[string]map[int64]bool                // prefix -> set of IDs
	limits     map[string]int                           // prefix -> Limit (for rotational domains)
	pointers   map[string]map[int64]*lib.SymmetricValue // prefix -> ID -> actual pointer from state
}

// NewUsedSymmetricValuesCollector creates a new collector
func NewUsedSymmetricValuesCollector() *UsedSymmetricValuesCollector {
	return &UsedSymmetricValuesCollector{
		usedValues: make(map[string]map[int64]bool),
		limits:     make(map[string]int),
		pointers:   make(map[string]map[int64]*lib.SymmetricValue),
	}
}

// VisitSymmetricValue records that a symmetric value is used in the state
func (c *UsedSymmetricValuesCollector) VisitSymmetricValue(sv *lib.SymmetricValue) {
	prefix := sv.GetPrefix()
	id := sv.GetId()

	if c.usedValues[prefix] == nil {
		c.usedValues[prefix] = make(map[int64]bool)
		c.pointers[prefix] = make(map[int64]*lib.SymmetricValue)
	}
	c.usedValues[prefix][id] = true
	c.pointers[prefix][id] = sv // Store the actual pointer
	if sv.Limit > 0 {
		c.limits[prefix] = sv.Limit
	}
}

// GetUsedIds returns a sorted list of IDs that are used for a given prefix
func (c *UsedSymmetricValuesCollector) GetUsedIds(prefix string) []int64 {
	if c.usedValues[prefix] == nil {
		return []int64{}
	}

	ids := make([]int64, 0, len(c.usedValues[prefix]))
	for id := range c.usedValues[prefix] {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// GetUsedSymmetricValues returns the actual SymmetricValue pointers for a given prefix
func (c *UsedSymmetricValuesCollector) GetUsedSymmetricValues(prefix string, kind lib.SymmetryKind) []*lib.SymmetricValue {
	ids := c.GetUsedIds(prefix)
	values := make([]*lib.SymmetricValue, len(ids))
	for i, id := range ids {
		// Return the actual pointers from the state
		values[i] = c.pointers[prefix][id]
	}
	return values
}

// HasUsedValues returns true if any symmetric values with the given prefix are used
func (c *UsedSymmetricValuesCollector) HasUsedValues(prefix string) bool {
	return len(c.usedValues[prefix]) > 0
}

// GetAllUsedIDs returns all used IDs organized by prefix/domain name.
// This is used by the symmetry module to initialize its cache.
func (c *UsedSymmetricValuesCollector) GetAllUsedIDs() map[string][]int64 {
	result := make(map[string][]int64)
	for prefix, idSet := range c.usedValues {
		ids := make([]int64, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		result[prefix] = ids
	}
	return result
}

// visitStarlarkValue recursively traverses a starlark value and calls visitor for each SymmetricValue found
func visitStarlarkValue(value starlark.Value, visitor StateVisitor, visited map[starlark.Value]bool) {
	if value == nil {
		return
	}

	// Check if already visited to avoid infinite loops
	if isPointerType(value) {
		if visited[value] {
			return
		}
		visited[value] = true
	}

	switch value.Type() {
	case "NoneType", "int", "float", "bool", "string", "bytes", "function",
		"builtin_function_or_method", "range", "struct", "symmetric_values",
		"model_value", "Channel":
		return

	case "symmetric_value":
		sv := value.(*lib.SymmetricValue)
		visitor.VisitSymmetricValue(sv)

	case "list":
		list := value.(*starlark.List)
		iter := list.Iterate()
		defer iter.Done()
		var elem starlark.Value
		for iter.Next(&elem) {
			visitStarlarkValue(elem, visitor, visited)
		}

	case "tuple":
		tuple := value.(starlark.Tuple)
		for i := 0; i < tuple.Len(); i++ {
			visitStarlarkValue(tuple.Index(i), visitor, visited)
		}

	case "set":
		set := value.(*starlark.Set)
		iter := set.Iterate()
		defer iter.Done()
		var elem starlark.Value
		for iter.Next(&elem) {
			visitStarlarkValue(elem, visitor, visited)
		}

	case "dict":
		dict := value.(*starlark.Dict)
		for _, item := range dict.Items() {
			key, val := item[0], item[1]
			visitStarlarkValue(key, visitor, visited)
			visitStarlarkValue(val, visitor, visited)
		}

	case "bag":
		bag := value.(*lib.Bag)
		iter := bag.Iterate()
		defer iter.Done()
		var elem starlark.Value
		for iter.Next(&elem) {
			visitStarlarkValue(elem, visitor, visited)
		}

	case "genericset":
		gset := value.(*lib.GenericSet)
		iter := gset.Iterate()
		defer iter.Done()
		var elem starlark.Value
		for iter.Next(&elem) {
			visitStarlarkValue(elem, visitor, visited)
		}

	case "genericmap":
		gmap := value.(*lib.GenericMap)
		for _, item := range gmap.Items() {
			key, val := item[0], item[1]
			visitStarlarkValue(key, visitor, visited)
			visitStarlarkValue(val, visitor, visited)
		}

	case "record":
		record := value.(*lib.Struct)
		dict := starlark.StringDict{}
		record.ToStringDict(dict)
		for _, val := range dict {
			visitStarlarkValue(val, visitor, visited)
		}

	case "role":
		role := value.(*lib.Role)
		if role.IsSymmetric() {
			roleId := lib.NewSymmetricValue(role.Name, role.GetRef())
			visitor.VisitSymmetricValue(roleId) // roleId is already a pointer now
		}
		visitStarlarkValue(role.Fields, visitor, visited)
		visitStarlarkValue(role.Params, visitor, visited)

	case "RoleStub":
		stub := value.(*lib.RoleStub)
		visitStarlarkValue(stub.Role, visitor, visited)
	}
}

// isPointerType returns true if the value is a pointer type that could cause cycles
func isPointerType(value starlark.Value) bool {
	switch value.Type() {
	case "list", "set", "dict", "bag", "genericset", "genericmap", "record", "role", "RoleStub":
		return true
	default:
		return false
	}
}

// visitStringDict visits all values in a starlark.StringDict
func visitStringDict(dict starlark.StringDict, visitor StateVisitor, visited map[starlark.Value]bool) {
	for _, value := range dict {
		visitStarlarkValue(value, visitor, visited)
	}
}

// AcceptVisitor traverses the entire process state calling visitor for each SymmetricValue
func (p *Process) AcceptVisitor(visitor StateVisitor) {
	visited := make(map[starlark.Value]bool)

	visitStringDict(p.Heap.state, visitor, visited)

	for _, thread := range p.Threads {
		if thread == nil {
			continue
		}
		frames := thread.Stack.RawArray()
		for _, frame := range frames {
			visitStringDict(frame.vars, visitor, visited)
			if frame.obj != nil {
				visitStarlarkValue(frame.obj, visitor, visited)
			}
			visitScope(frame.scope, visitor, visited)
		}
	}

	for _, role := range p.Roles {
		if role != nil {
			visitStarlarkValue(role, visitor, visited)
		}
	}

	for _, messages := range p.ChannelMessages {
		for _, msg := range messages {
			if msg != nil {
				visitStringDict(msg.params, visitor, visited)
			}
		}
	}

	visitStringDict(p.Returns, visitor, visited)
}

func visitScope(scope *Scope, visitor StateVisitor, visited map[starlark.Value]bool) {
	if scope == nil {
		return
	}
	visitStringDict(scope.vars, visitor, visited)
	for _, value := range scope.loopRange {
		visitStarlarkValue(value, visitor, visited)
	}
	if scope.parent != nil {
		visitScope(scope.parent, visitor, visited)
	}
}

// getUsedSymmetricValues returns the symmetric values that are actually used in the process state
func (p *Process) getUsedSymmetricValues() [][]*lib.SymmetricValue {
	collector := NewUsedSymmetricValuesCollector()
	p.AcceptVisitor(collector)

	// Build a map of materialized rotational domains from globals
	materializedDomains := make(map[string]*lib.SymmetryDomain)
	for _, val := range p.Heap.globals {
		if domain, ok := val.(*lib.SymmetryDomain); ok {
			if domain.Materialize && domain.Kind == lib.SymmetryKindRotational {
				materializedDomains[domain.Name] = domain
			}
		}
	}

	// Get all symmetric value definitions
	defs := p.Heap.GetSymmetryDefs()

	result := make([][]*lib.SymmetricValue, 0)
	for _, def := range defs {
		if def.Len() == 0 {
			continue
		}

		// Get the prefix from the first element
		prefix := def.Index(0).GetPrefix()
		kind := def.Index(0).GetKind()

		// For materialized rotational domains, return the full set [0..limit-1]
		if domain, ok := materializedDomains[prefix]; ok {
			fullSet := make([]*lib.SymmetricValue, domain.Limit)
			for i := 0; i < domain.Limit; i++ {
				fullSet[i] = lib.NewRotationalSymmetricValue(prefix, int64(i), domain.Limit)
			}
			result = append(result, fullSet)
			continue
		}

		// Get the actually used values
		usedValues := collector.GetUsedSymmetricValues(prefix, kind)

		if len(usedValues) > 0 {
			result = append(result, usedValues)
		}
	}

	return result
}

// getUsedSymmetricValuesFromRefs extracts SymmetricValue pointers from refs map after cloning
// This is an alternative to using StateVisitor - the refs map keys ARE the original pointers
func (p *Process) getUsedSymmetricValuesFromRefs() [][]*lib.SymmetricValue {
	// Clone with nil permutations just to populate refs
	refs := make(map[starlark.Value]starlark.Value)
	_ = p.CloneWithRefs(nil, 0, refs)

	// Extract SymmetricValues from refs keys, grouped by prefix
	byPrefix := make(map[string]map[int64]*lib.SymmetricValue)
	for key := range refs {
		if sv, ok := key.(*lib.SymmetricValue); ok {
			prefix := sv.GetPrefix()
			if byPrefix[prefix] == nil {
				byPrefix[prefix] = make(map[int64]*lib.SymmetricValue)
			}
			byPrefix[prefix][sv.GetId()] = sv
		}
	}

	// Build a map of materialized rotational domains from globals
	materializedDomains := make(map[string]*lib.SymmetryDomain)
	for _, val := range p.Heap.globals {
		if domain, ok := val.(*lib.SymmetryDomain); ok {
			if domain.Materialize && domain.Kind == lib.SymmetryKindRotational {
				materializedDomains[domain.Name] = domain
			}
		}
	}

	// Get all symmetric value definitions to maintain ordering
	defs := p.Heap.GetSymmetryDefs()

	result := make([][]*lib.SymmetricValue, 0)
	for _, def := range defs {
		if def.Len() == 0 {
			continue
		}

		prefix := def.Index(0).GetPrefix()

		// For materialized rotational domains, return the full set
		if domain, ok := materializedDomains[prefix]; ok {
			fullSet := make([]*lib.SymmetricValue, domain.Limit)
			for i := 0; i < domain.Limit; i++ {
				fullSet[i] = lib.NewRotationalSymmetricValue(prefix, int64(i), domain.Limit)
			}
			result = append(result, fullSet)
			continue
		}

		// Get values from refs for this prefix
		if prefixMap, ok := byPrefix[prefix]; ok && len(prefixMap) > 0 {
			// Sort by ID
			ids := make([]int64, 0, len(prefixMap))
			for id := range prefixMap {
				ids = append(ids, id)
			}
			slices.Sort(ids)

			values := make([]*lib.SymmetricValue, len(ids))
			for i, id := range ids {
				values[i] = prefixMap[id]
			}
			result = append(result, values)
		}
	}

	return result
}

// CompareSymmetricValueMethods compares StateVisitor vs CloneWithRefs approaches
// Returns true if they produce the same pointers, false otherwise with details
func (p *Process) CompareSymmetricValueMethods() (bool, string) {
	visitorResult := p.getUsedSymmetricValues()
	refsResult := p.getUsedSymmetricValuesFromRefs()

	if len(visitorResult) != len(refsResult) {
		return false, fmt.Sprintf("different number of groups: visitor=%d, refs=%d", len(visitorResult), len(refsResult))
	}

	for i := range visitorResult {
		if len(visitorResult[i]) != len(refsResult[i]) {
			return false, fmt.Sprintf("group %d has different sizes: visitor=%d, refs=%d", i, len(visitorResult[i]), len(refsResult[i]))
		}

		for j := range visitorResult[i] {
			vPtr := visitorResult[i][j]
			rPtr := refsResult[i][j]

			// Check if same pointer
			if vPtr != rPtr {
				// Check if same values but different pointers
				if vPtr.GetPrefix() == rPtr.GetPrefix() && vPtr.GetId() == rPtr.GetId() {
					return false, fmt.Sprintf("group %d, index %d: same value but DIFFERENT pointers - visitor=%p, refs=%p (prefix=%s, id=%d)",
						i, j, vPtr, rPtr, vPtr.GetPrefix(), vPtr.GetId())
				}
				return false, fmt.Sprintf("group %d, index %d: different values - visitor=%v, refs=%v", i, j, vPtr, rPtr)
			}
		}
	}

	return true, "all pointers match"
}

// getActualSymmetricValuePointers returns a map from prefix to the actual SymmetricValue pointers
// found during cloning. These are the real pointers that will be encountered when cloning
// with permutations, so they should be used as map keys for efficient O(1) lookup.
func (p *Process) getActualSymmetricValuePointers() map[string][]*lib.SymmetricValue {
	_, _, result := p.cloneAndGetSymmetricValuePointers()
	return result
}

// cloneAndGetSymmetricValuePointers does a preliminary clone and returns both:
// 1. The map from prefix to actual SymmetricValue pointers (for use as permutation map keys)
// 2. These pointers are the REAL pointers from the state, not newly created ones
// This replaces both getUsedSymmetricValues() and GetSymmetryRoles() with a single clone operation.
func (p *Process) cloneAndGetSymmetricValuePointers() (*Process, map[starlark.Value]starlark.Value, map[string][]*lib.SymmetricValue) {
	// Clone with nil permutations to populate refs with actual pointers
	refs := make(map[starlark.Value]starlark.Value)
	p2 := p.CloneWithRefs(nil, 0, refs)

	// Count total SymmetricValue pointers vs unique (prefix, id) pairs
	totalSVPointers := 0
	totalRolePointers := 0
	uniquePairs := make(map[string]bool)
	rolesByName := make(map[string]int)
	for _, val := range refs {
		if sv, ok := val.(*lib.SymmetricValue); ok {
			totalSVPointers++
			key := fmt.Sprintf("%s:%d", sv.GetPrefix(), sv.GetId())
			uniquePairs[key] = true
		}
		if role, ok := val.(*lib.Role); ok {
			totalRolePointers++
			rolesByName[role.Name]++
		}
	}
	if totalSVPointers != len(uniquePairs) {
		// fmt.Printf("DEBUG: Found %d SymmetricValue pointers but only %d unique (prefix,id) pairs\n", totalSVPointers, len(uniquePairs))
	}
	if totalRolePointers > 0 {
		// fmt.Printf("DEBUG: Found %d Role pointers: %v\n", totalRolePointers, rolesByName)
	}

	// Extract SymmetricValues from refs - collect ALL pointers, not just unique (prefix, id)
	// Group by (prefix, id) to track multiple pointers with same logical value
	type svKey struct {
		prefix string
		id     int64
	}
	allPointersByKey := make(map[svKey][]*lib.SymmetricValue)
	for _, val := range refs {
		if sv, ok := val.(*lib.SymmetricValue); ok {
			key := svKey{prefix: sv.GetPrefix(), id: sv.GetId()}
			allPointersByKey[key] = append(allPointersByKey[key], sv)
		}
	}

	// For the result, we just need one representative pointer per (prefix, id) for permutation generation
	// But we'll need all pointers for mutation - store them in p2 for later use
	byPrefix := make(map[string]map[int64]*lib.SymmetricValue)
	for key, ptrs := range allPointersByKey {
		if byPrefix[key.prefix] == nil {
			byPrefix[key.prefix] = make(map[int64]*lib.SymmetricValue)
		}
		// Use the first pointer as representative
		byPrefix[key.prefix][key.id] = ptrs[0]
	}

	// Convert to sorted slices
	result := make(map[string][]*lib.SymmetricValue)
	for prefix, idMap := range byPrefix {
		ids := make([]int64, 0, len(idMap))
		for id := range idMap {
			ids = append(ids, id)
		}
		slices.Sort(ids)

		values := make([]*lib.SymmetricValue, len(ids))
		for i, id := range ids {
			values[i] = idMap[id]
		}
		result[prefix] = values
	}

	return p2, refs, result
}

// getUsedSymmetricValuesFromClone returns the symmetric values grouped for permutation generation.
// This replaces getUsedSymmetricValues() by using actual pointers from cloning.
// Returns [][]*lib.SymmetricValue in the same format as getUsedSymmetricValues.
// Also returns the refs map which contains ALL SymmetricValue pointers (for mutation during hashing).
func (p *Process) getUsedSymmetricValuesFromClone() (*Process, map[starlark.Value]starlark.Value, [][]*lib.SymmetricValue) {
	p2, refs, byPrefix := p.cloneAndGetSymmetricValuePointers()

	// Build a map of materialized rotational domains from globals
	materializedDomains := make(map[string]*lib.SymmetryDomain)
	for _, val := range p.Heap.globals {
		if domain, ok := val.(*lib.SymmetryDomain); ok {
			if domain.Materialize && domain.Kind == lib.SymmetryKindRotational {
				materializedDomains[domain.Name] = domain
			}
		}
	}

	// Get all symmetric value definitions to maintain ordering
	defs := p.Heap.GetSymmetryDefs()

	// Track which prefixes we've already added
	addedPrefixes := make(map[string]bool)

	result := make([][]*lib.SymmetricValue, 0)

	// First, add values from definitions (to maintain ordering)
	for _, def := range defs {
		if def.Len() == 0 {
			continue
		}

		prefix := def.Index(0).GetPrefix()
		addedPrefixes[prefix] = true

		// For materialized rotational domains, return the full set
		if domain, ok := materializedDomains[prefix]; ok {
			fullSet := make([]*lib.SymmetricValue, domain.Limit)
			for i := 0; i < domain.Limit; i++ {
				fullSet[i] = lib.NewRotationalSymmetricValue(prefix, int64(i), domain.Limit)
			}
			result = append(result, fullSet)
			continue
		}

		// Get values from refs for this prefix
		if values, ok := byPrefix[prefix]; ok && len(values) > 0 {
			result = append(result, values)
		}
	}

	// Then, add any remaining prefixes (e.g., symmetric roles not in definitions)
	for prefix, values := range byPrefix {
		if !addedPrefixes[prefix] && len(values) > 0 {
			result = append(result, values)
		}
	}

	return p2, refs, result
}

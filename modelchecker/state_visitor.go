package modelchecker

import (
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"sort"
)

// StateVisitor defines the interface for visiting state elements during traversal
type StateVisitor interface {
	VisitSymmetricValue(sv lib.SymmetricValue)
}

// UsedSymmetricValuesCollector collects all SymmetricValue instances in the current state
type UsedSymmetricValuesCollector struct {
	usedValues map[string]map[int]bool // prefix -> set of IDs
}

// NewUsedSymmetricValuesCollector creates a new collector
func NewUsedSymmetricValuesCollector() *UsedSymmetricValuesCollector {
	return &UsedSymmetricValuesCollector{
		usedValues: make(map[string]map[int]bool),
	}
}

// VisitSymmetricValue records that a symmetric value is used in the state
func (c *UsedSymmetricValuesCollector) VisitSymmetricValue(sv lib.SymmetricValue) {
	prefix := sv.GetPrefix()
	id := sv.GetId()

	if c.usedValues[prefix] == nil {
		c.usedValues[prefix] = make(map[int]bool)
	}
	c.usedValues[prefix][id] = true
}

// GetUsedIds returns a sorted list of IDs that are used for a given prefix
func (c *UsedSymmetricValuesCollector) GetUsedIds(prefix string) []int {
	if c.usedValues[prefix] == nil {
		return []int{}
	}

	ids := make([]int, 0, len(c.usedValues[prefix]))
	for id := range c.usedValues[prefix] {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

// GetUsedSymmetricValues returns SymmetricValues for a given prefix
func (c *UsedSymmetricValuesCollector) GetUsedSymmetricValues(prefix string) []lib.SymmetricValue {
	ids := c.GetUsedIds(prefix)
	values := make([]lib.SymmetricValue, len(ids))
	for i, id := range ids {
		values[i] = lib.NewSymmetricValue(prefix, id)
	}
	return values
}

// HasUsedValues returns true if any symmetric values with the given prefix are used
func (c *UsedSymmetricValuesCollector) HasUsedValues(prefix string) bool {
	return len(c.usedValues[prefix]) > 0
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
		sv := value.(lib.SymmetricValue)
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
			roleId := lib.NewSymmetricValue(role.Name, role.Ref)
			visitor.VisitSymmetricValue(roleId)
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
func (p *Process) getUsedSymmetricValues() [][]lib.SymmetricValue {
	collector := NewUsedSymmetricValuesCollector()
	p.AcceptVisitor(collector)

	// Get all symmetric value definitions
	defs := p.Heap.GetSymmetryDefs()

	result := make([][]lib.SymmetricValue, 0)
	for _, def := range defs {
		if def.Len() == 0 {
			continue
		}

		// Get the prefix from the first element
		prefix := def.Index(0).GetPrefix()

		// Get the actually used values
		usedValues := collector.GetUsedSymmetricValues(prefix)

		if len(usedValues) > 0 {
			result = append(result, usedValues)
		}
	}

	return result
}

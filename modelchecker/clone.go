package modelchecker

import (
	"fmt"
	"reflect"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

func deepCloneStarlarkValue(value starlark.Value, refs map[starlark.Value]starlark.Value) (starlark.Value, error) {
	return deepCloneStarlarkValueWithPermutations(value, refs, nil, 0)
}

func deepCloneStarlarkValueWithPermutations(value starlark.Value, refs map[starlark.Value]starlark.Value, permutations map[*lib.SymmetricValue][]*lib.SymmetricValue, alt int) (starlark.Value, error) {
	if refs == nil {
		refs = make(map[starlark.Value]starlark.Value)
	} else if reflect.TypeOf(value).Kind() != reflect.Ptr {
		// value (non pointer) types are not cached.
	} else if cached, ok := refs[value]; ok {
		// Cache the new reference before recursively cloning the elements
		return cached, nil
	}
	// starlark has other types as well "string.elems", "string.codepoints", "function"
	// "builtin_function_or_method".
	switch value.Type() {

	case "NoneType", "int", "float", "bool", "string", "bytes", "function", "range", "symmetric_values", "Channel", "symmetry_domain":
		//case starlark.Bool, starlark.String, starlark.Int:
		// For simple values, just return a copy
		// Also starlark struct is immutable
		// symmetry_domain is stateless configuration (Name, Limit, DomainType)
		return value, nil

	case "struct":
		// A struct is immutable, but its fields may hold symmetric values or
		// mutable containers, which must be cloned (and permuted) like any
		// other. A struct with nothing to clone is shared as is.
		//
		// The struct can't be registered in refs before its fields are cloned,
		// so a cycle through it (a list holding the struct that holds the
		// list) is cut at the container, which is registered first. The inner
		// reference then gets its own copy of the struct, equal in value.
		v, ok := value.(*starlarkstruct.Struct)
		if !ok {
			return value, nil
		}
		fields := make(starlark.StringDict)
		changed := false
		for _, name := range v.AttrNames() {
			field, err := v.Attr(name)
			if err != nil {
				return nil, err
			}
			cloned, err := deepCloneStarlarkValueWithPermutations(field, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			fields[name] = cloned
			switch field.Type() {
			case "NoneType", "int", "float", "bool", "string", "bytes", "function", "range", "symmetric_values", "Channel", "symmetry_domain":
			default:
				changed = true
			}
		}
		if !changed {
			return value, nil
		}
		newVal := starlarkstruct.FromStringDict(v.Constructor(), fields)
		refs[value] = newVal
		return newVal, nil

	case "model_value":
		// ModelValue is now a pointer type - clone it as a reference
		v := value.(*lib.ModelValue)
		newVal := lib.NewModelValue(v.GetPrefix(), v.GetId())
		refs[value] = newVal
		return newVal, nil

	case "symmetric_value":
		// SymmetricValue is now a pointer type - clone it as a reference
		v := value.(*lib.SymmetricValue)
		if permutations != nil {
			// First try direct pointer lookup
			if other, ok := permutations[v]; ok {
				// Return the permuted value - also add to refs for consistency
				refs[value] = other[alt]
				return other[alt], nil
			}
			// Fall back to value-based lookup (for cases where SymmetricValues are created on-the-fly)
			for k, other := range permutations {
				if k.GetPrefix() == v.GetPrefix() && k.GetId() == v.GetId() {
					refs[value] = other[alt]
					return other[alt], nil
				}
			}
			panic(fmt.Sprintf("symmetric_value %#v (Kind: %d) should be in %v and alt %v. Keys: %+v", v, v.Kind, permutations, alt, func() []string {
				keys := make([]string, 0, len(permutations))
				for k := range permutations {
					keys = append(keys, fmt.Sprintf("%#v (Kind: %d)", k, k.Kind))
				}
				return keys
			}()))
		}
		// Clone the symmetric value - now it's a pointer type
		newVal := lib.NewSymmetricValueWithKind(v.GetPrefix(), v.GetId(), v.GetKind())
		if v.Kind == lib.SymmetryKindRotational {
			newVal = lib.NewRotationalSymmetricValue(v.GetPrefix(), v.GetId(), v.Limit)
		}
		refs[value] = newVal
		return newVal, nil
	case "list":
		newVal := starlark.NewList(make([]starlark.Value, 0))
		refs[value] = newVal

		// For lists, recursively clone each element
		iterable := value.(starlark.Iterable)
		newList, err := deepCloneIterableToList(iterable, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		for _, elem := range newList {
			_ = newVal.Append(elem)
		}
		return newVal, nil
	case "set":
		newSet := starlark.NewSet(0)
		refs[value] = newSet
		// For sets, recursively clone each element
		iterable := value.(starlark.Iterable)
		newList, err := deepCloneIterableToList(iterable, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		for _, v := range newList {
			err := newSet.Insert(v)
			if err != nil {
				return nil, err
			}
		}
		return newSet, nil
	case "tuple":
		newTuple := starlark.Tuple{}
		// Tuple is a value type (not a pointer), so refs caching won't work
		// For tuples, recursively clone each element
		iterable := value.(starlark.Iterable)
		newList, err := deepCloneIterableToList(iterable, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		for _, v := range newList {
			newTuple = append(newTuple, v)
		}
		return newTuple, nil
	case "dict":
		v := value.(*starlark.Dict)
		// For dictionaries, recursively clone each key-value pair
		newDict, err := deepCloneStringDict(v, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		return newDict, nil
	case "record":
		v := value.(*lib.Struct)
		// Copy the fields in place rather than via a StringDict round
		// trip: the source is already sorted by name, so the clone needs
		// no sort and no intermediate map.
		newVal := lib.NewEmptyStruct(v.Constructor(), v.Len())
		refs[value] = newVal
		var fieldErr error
		v.EachField(func(name string, fv starlark.Value) {
			if fieldErr != nil {
				return
			}
			cloned, err := deepCloneStarlarkValueWithPermutations(fv, refs, permutations, alt)
			if err != nil {
				fieldErr = err
				return
			}
			newVal.AppendSortedField(name, cloned)
		})
		if fieldErr != nil {
			return nil, fieldErr
		}
		return newVal, nil
	case "RoleStub":
		// TODO(jp): Should this also reuse cached refs?
		r := value.(*lib.RoleStub)
		role, err := deepCloneStarlarkValueWithPermutations(r.Role, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		return lib.NewRoleStub(role.(*lib.Role), r.Channel), nil
	case "symmetry_segment":
		s := value.(*lib.Segment)
		newLeft := s.Left
		newRight := s.Right

		if permutations != nil {
			// Map Left ID - look up by pointer in permutations
			for oldSV, newSVs := range permutations {
				if oldSV.GetPrefix() == s.Domain.Name && oldSV.GetId() == s.Left {
					newLeft = newSVs[alt].GetId()
					break
				}
			}

			// Map Right ID - look up by pointer in permutations
			for oldSV, newSVs := range permutations {
				if oldSV.GetPrefix() == s.Domain.Name && oldSV.GetId() == s.Right {
					newRight = newSVs[alt].GetId()
					break
				}
			}
		}

		newResolved := s.Resolved
		newResolvedID := s.ResolvedID
		if permutations != nil && s.Resolved {
			// The memoized value only has a counterpart in the permuted state
			// while the value itself is still in the state. Otherwise drop the
			// memo: a later value() allocates afresh in the (remapped) gap,
			// which is equivalent under ordinal symmetry.
			newResolved = false
			newResolvedID = 0
			for oldSV, newSVs := range permutations {
				if oldSV.GetPrefix() == s.Domain.Name && oldSV.GetId() == s.ResolvedID {
					newResolved = true
					newResolvedID = newSVs[alt].GetId()
					break
				}
			}
		}

		newSegment := &lib.Segment{
			Domain:     s.Domain,
			Left:       newLeft,
			Right:      newRight,
			IsHead:     s.IsHead,
			IsTail:     s.IsTail,
			IsPoint:    s.IsPoint,
			Resolved:   newResolved,
			ResolvedID: newResolvedID,
		}
		// value() memoizes into the segment, so aliases must stay shared.
		refs[value] = newSegment
		return newSegment, nil
	case "genericset":
		// The source keys are distinct and a permutation renames IDs
		// bijectively, so the clones are distinct too: append them
		// without the per-insert equality scan (which made this
		// quadratic in the set size).
		s := value.(*lib.GenericSet)
		newSet := lib.GenericSetFromDistinct(nil)
		refs[value] = newSet
		keys, err := deepCloneIterableToList(s, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		*newSet = *lib.GenericSetFromDistinct(keys)
		return newSet, nil
	case "genericmap":
		m := value.(*lib.GenericMap)
		newMap := lib.GenericMapFromEntries(nil)
		refs[value] = newMap
		items := m.Items()
		entries := make([]starlark.Tuple, 0, len(items))
		for _, tuple := range items {
			key, value := tuple[0], tuple[1]
			clonedKey, err := deepCloneStarlarkValueWithPermutations(key, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			clonedValue, err := deepCloneStarlarkValueWithPermutations(value, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			entries = append(entries, starlark.Tuple{clonedKey, clonedValue})
		}
		*newMap = *lib.GenericMapFromEntries(entries)
		return newMap, nil
	case "bag":
		b := value.(*lib.Bag)
		newBag := lib.NewBag(nil)
		refs[value] = newBag
		elems, err := deepCloneIterableToList(b, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		if permutations == nil {
			// Cloning keeps each element's string form, so the source's
			// sorted order carries over: skip the per-insert sort.
			*newBag = *lib.BagFromSortedSlice(elems)
		} else {
			*newBag = *lib.NewBag(elems)
		}
		return newBag, nil
	case "role":
		r := value.(*lib.Role)
		prefix := r.Name
		id := r.GetId() // Get the ID object directly
		if r.IsSymmetric() {
			// Cloning the ID object checks permutations
			newVal, err := deepCloneStarlarkValueWithPermutations(r.ID, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			id = newVal // Use the new (permuted) ID object

			// If we need prefix from the new ID (though it shouldn't change for roles usually)
			if sv, ok := id.(*lib.SymmetricValue); ok {
				prefix = sv.GetPrefix()
			}
		} else {
			// For non-symmetric roles, we still want to clone the ID model value
			newVal, err := deepCloneStarlarkValueWithPermutations(r.ID, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			id = newVal
		}

		if cached, ok := refs[value]; ok {
			return cached, nil
		} else {
			params, err := deepCloneStarlarkValueWithPermutations(r.Params, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			fields, err := deepCloneStarlarkValueWithPermutations(r.Fields, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			newRole := &lib.Role{
				ID:          id,
				Name:        prefix,
				Symmetric:   r.IsSymmetric(),
				Params:      params.(*lib.Struct),
				Fields:      fields.(*lib.Struct),
				Methods:     r.Methods,
				RoleMethods: r.RoleMethods,
				InitValues:  r.InitValues,
			}
			refs[value] = newRole
			return newRole, nil
		}
	default:
		return nil, fmt.Errorf("unsupported type: %T, %s", value, value.Type())
	}
}

func deepCloneStringDict(v *starlark.Dict, refs map[starlark.Value]starlark.Value,
	src map[*lib.SymmetricValue][]*lib.SymmetricValue, alt int) (*starlark.Dict, error) {

	newDict := &starlark.Dict{}
	refs[v] = newDict
	for _, item := range v.Items() {
		k, v := item[0], item[1]
		clonedKey, err := deepCloneStarlarkValueWithPermutations(k, refs, src, alt)
		if err != nil {
			return nil, err
		}
		clonedValue, err := deepCloneStarlarkValueWithPermutations(v, refs, src, alt)
		if err != nil {
			return nil, err
		}
		newDict.SetKey(clonedKey, clonedValue)
	}
	return newDict, nil
}

func deepCloneIterableToList(iterable starlark.Iterable, refs map[starlark.Value]starlark.Value, permutations map[*lib.SymmetricValue][]*lib.SymmetricValue, alt int) ([]starlark.Value, error) {
	var newList []starlark.Value
	iter := iterable.Iterate()
	defer iter.Done()
	var x starlark.Value
	for iter.Next(&x) {
		clonedElem, err := deepCloneStarlarkValueWithPermutations(x, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		PanicOnError(err)
		newList = append(newList, clonedElem)
	}
	return newList, nil
}

func DeepCopyBuiltIn(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected: only a single arg, but had %d args", len(args))
	}
	value, err := deepCloneStarlarkValue(args[0], nil)
	return value, err
}

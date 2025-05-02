package modelchecker

import (
	"fmt"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"reflect"
)

func deepCloneStarlarkValue(value starlark.Value, refs map[starlark.Value]starlark.Value) (starlark.Value, error) {
	return deepCloneStarlarkValueWithPermutations(value, refs, nil, 0)
}

func deepCloneStarlarkValueWithPermutations(value starlark.Value, refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) (starlark.Value, error) {
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

	case "NoneType", "int", "float", "bool", "string", "bytes", "function", "range", "struct", "symmetric_values", "model_value", "Channel":
		//case starlark.Bool, starlark.String, starlark.Int:
		// For simple values, just return a copy
		// Also starlark struct is immutable
		return value, nil

	case "symmetric_value":
		if permutations != nil && alt > 0 {
			v := value.(lib.SymmetricValue)
			if other, ok := permutations[v]; ok {
				return other[alt], nil
			}
			panic(fmt.Sprintf("symmetric_value %v should have src %v and alt %v", v, permutations, alt))
		}
		return value, nil
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
		// Tuple is a value type, so not adding to refs
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
		dict := starlark.StringDict{}
		newVal := lib.FromStringDict(v.Constructor(), dict)
		refs[value] = newVal
		v.ToStringDict(dict)
		newDict := CloneDict(dict, refs, permutations, alt)
		newVal.ReplaceEntriesFromStringDict(newDict)
		return newVal, nil
	case "RoleStub":
		// TODO(jp): Should this also reuse cached refs?
		r := value.(*lib.RoleStub)
		role, err := deepCloneStarlarkValueWithPermutations(r.Role, refs, permutations, alt)
		if err != nil {
			return nil, err
		}
		return lib.NewRoleStub(role.(*lib.Role), r.Channel), nil
	case "genericset":
		s := value.(*lib.GenericSet)
		newSet := lib.NewGenericSet()
		refs[value] = newSet
		iter := s.Iterate()
		defer iter.Done()
		var x starlark.Value
		for iter.Next(&x) {
			clonedElem, err := deepCloneStarlarkValueWithPermutations(x, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			PanicOnError(err)
			newSet.Insert(clonedElem)
		}
		return newSet, nil
	case "genericmap":
		m := value.(*lib.GenericMap)
		newMap := lib.NewGenericMap()
		refs[value] = newMap
		for _, tuple := range m.Items() {
			key, value := tuple[0], tuple[1]
			clonedKey, err := deepCloneStarlarkValueWithPermutations(key, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			clonedValue, err := deepCloneStarlarkValueWithPermutations(value, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			newMap.SetKey(clonedKey, clonedValue)
		}
		return newMap, nil
	case "bag":
		b := value.(*lib.Bag)
		newBag := lib.NewBag(nil)
		refs[value] = newBag
		iter := b.Iterate()
		defer iter.Done()
		var x starlark.Value
		for iter.Next(&x) {
			clonedElem, err := deepCloneStarlarkValueWithPermutations(x, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			PanicOnError(err)
			newBag.Insert(clonedElem)
		}
		return newBag, nil
	case "role":
		r := value.(*lib.Role)
		prefix := r.Name
		id := r.Ref
		if r.IsSymmetric() {
			oldSymVal := lib.NewSymmetricValue(r.Name, r.Ref)
			newVal, err := deepCloneStarlarkValueWithPermutations(oldSymVal, refs, permutations, alt)
			if err != nil {
				return nil, err
			}
			newRoleId := newVal.(lib.SymmetricValue)
			prefix = newRoleId.GetPrefix()
			id = newRoleId.GetId()
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
				Ref:         id,
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
	src map[lib.SymmetricValue][]lib.SymmetricValue, alt int) (*starlark.Dict, error) {

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

func deepCloneIterableToList(iterable starlark.Iterable, refs map[starlark.Value]starlark.Value, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) ([]starlark.Value, error) {
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

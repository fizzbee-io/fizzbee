package modelchecker

import (
    "fmt"
    "github.com/fizzbee-io/fizzbee/lib"
    "go.starlark.net/starlark"
)

func deepCloneStarlarkValue(value starlark.Value, refs map[string]*Role) (starlark.Value, error) {
    return deepCloneStarlarkValueWithPermutations(value, refs, nil, 0)
}

func deepCloneStarlarkValueWithPermutations(value starlark.Value, refs map[string]*Role, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) (starlark.Value, error) {
    // starlark has other types as well "string.elems", "string.codepoints", "function"
    // "builtin_function_or_method".
    switch value.Type() {

    case "NoneType", "int", "float", "bool", "string", "bytes", "range", "struct", "symmetric_values", "model_value":
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
        // For lists, recursively clone each element
        iterable := value.(starlark.Iterable)
        newList, err := deepCloneIterableToList(iterable, refs, permutations, alt)
        if err != nil {
            return nil, err
        }

        return starlark.NewList(newList), nil
    case "set":
        // For lists, recursively clone each element
        iterable := value.(starlark.Iterable)
        newList, err := deepCloneIterableToList(iterable, refs, permutations, alt)
        if err != nil {
            return nil, err
        }
        newSet := starlark.NewSet(len(newList))
        for _, v := range newList {
            err := newSet.Insert(v)
            if err != nil {
                return nil, err
            }
        }
        return newSet, nil
    case "tuple":
        // For lists, recursively clone each element
        iterable := value.(starlark.Iterable)
        newList, err := deepCloneIterableToList(iterable, refs, permutations, alt)
        if err != nil {
            return nil, err
        }
        newTuple := starlark.Tuple{}
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
        v.ToStringDict(dict)
        newDict := CloneDict(dict, refs, permutations, alt)
        return lib.FromStringDict(v.Constructor(), newDict), nil

    case "genericset":
        s := value.(*lib.GenericSet)
        newSet := lib.NewGenericSet()
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
        r := value.(*Role)
        prefix := r.Name
        id := r.ref
        if r.IsSymmetric() {
            oldSymVal := lib.NewSymmetricValue(r.Name, r.ref)
            newVal, err := deepCloneStarlarkValueWithPermutations(oldSymVal, refs, permutations, alt)
            if err != nil {
                return nil, err
            }
            newRoleId := newVal.(lib.SymmetricValue)
            prefix = newRoleId.GetPrefix()
            id = newRoleId.GetId()
        }

        newRefString := GenerateRefString(prefix, id)

        if cached, ok := refs[newRefString]; ok {
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
            newRole := &Role{
                ref:       id,
                Name:      prefix,
                Symmetric: r.IsSymmetric(),
                Params:    params.(*lib.Struct),
                Fields:    fields.(*lib.Struct),
            }
            refs[newRole.RefString()] = newRole
            return newRole, nil
        }
    default:
        return nil, fmt.Errorf("unsupported type: %T, %s", value, value.Type())
    }
}

func deepCloneStringDict(v *starlark.Dict, refs map[string]*Role, src map[lib.SymmetricValue][]lib.SymmetricValue, alt int) (*starlark.Dict, error) {
    newDict := &starlark.Dict{}
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

func deepCloneIterableToList(iterable starlark.Iterable, refs map[string]*Role, permutations map[lib.SymmetricValue][]lib.SymmetricValue, alt int) ([]starlark.Value, error) {
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

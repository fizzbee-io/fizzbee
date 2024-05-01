package modelchecker

import (
    "fmt"
    "github.com/fizzbee-io/fizzbee/lib"
    "go.starlark.net/starlark"
)

func deepCloneStarlarkValue(value starlark.Value, refs map[string]*Role) (starlark.Value, error) {
    // starlark has other types as well "string.elems", "string.codepoints", "function"
    // "builtin_function_or_method".
    switch value.Type() {

    case "NoneType", "int", "float", "bool", "string", "bytes", "range", "struct":
    //case starlark.Bool, starlark.String, starlark.Int:
        // For simple values, just return a copy
        // Also starlark struct is immutable
        return value, nil

    case "list":
        // For lists, recursively clone each element
        iterable := value.(starlark.Iterable)
        newList, err := deepCloneIterableToList(iterable, refs)
        if err != nil {
            return nil, err
        }

        return starlark.NewList(newList), nil
    case "set":
        // For lists, recursively clone each element
        iterable := value.(starlark.Iterable)
        newList, err := deepCloneIterableToList(iterable, refs)
        if err != nil {
            return nil, err
        }
        newSet := starlark.NewSet(10)
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
        newList, err := deepCloneIterableToList(iterable, refs)
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
        newDict, err := deepCloneStringDict(v, refs)
        if err != nil {
            return nil, err
        }
        return newDict, nil
    case "record":
        v := value.(*lib.Struct)
        dict := starlark.StringDict{}
        v.ToStringDict(dict)
        newDict := CloneDict(dict, refs)
        return lib.FromStringDict(lib.Default, newDict), nil

    case "genericset":
        s := value.(*lib.GenericSet)
        newSet := lib.NewGenericSet()
        iter := s.Iterate()
        defer iter.Done()
        var x starlark.Value
        for iter.Next(&x) {
            clonedElem, err := deepCloneStarlarkValue(x, refs)
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
            clonedKey, err := deepCloneStarlarkValue(key, refs)
            if err != nil {
                return nil, err
            }
            clonedValue, err := deepCloneStarlarkValue(value, refs)
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
            clonedElem, err := deepCloneStarlarkValue(x, refs)
            if err != nil {
                return nil, err
            }
            PanicOnError(err)
            newBag.Insert(clonedElem)
        }
        return newBag, nil
    case "role":
        r := value.(*Role)
        if cached, ok := refs[r.RefString()]; ok {
            return cached, nil
        } else {
            fields, err := deepCloneStarlarkValue(r.Fields, refs)
            if err != nil {
                return nil, err
            }
            newRole := &Role{
                ref: r.ref,
                Name: r.Name,
                Params: r.Params,
                Fields: fields.(*lib.Struct),
            }
            refs[r.RefString()] = newRole
            return newRole, nil
        }
    default:
        return nil, fmt.Errorf("unsupported type: %T, %s", value, value.Type())
    }
}

func deepCloneStringDict(v *starlark.Dict, refs map[string]*Role) (*starlark.Dict, error) {
    newDict := &starlark.Dict{}
    for _, item := range v.Items() {
        k, v := item[0], item[1]
        clonedKey, err := deepCloneStarlarkValue(k, refs)
        if err != nil {
            return nil, err
        }
        clonedValue, err := deepCloneStarlarkValue(v, refs)
        if err != nil {
            return nil, err
        }
        newDict.SetKey(clonedKey, clonedValue)
    }
    return newDict, nil
}

func deepCloneIterableToList(iterable starlark.Iterable, refs map[string]*Role) ([]starlark.Value, error) {
    var newList []starlark.Value
    iter := iterable.Iterate()
    var x starlark.Value
    for iter.Next(&x) {
        clonedElem, err := deepCloneStarlarkValue(x, refs)
        if err != nil {
            return nil, err
        }
        PanicOnError(err)
        newList = append(newList, clonedElem)
    }
    return newList, nil
}

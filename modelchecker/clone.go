package modelchecker

import (
    "fmt"
    "github.com/fizzbee-io/fizzbee/lib"
    "go.starlark.net/starlark"
)

func deepCloneStarlarkValue(value starlark.Value) (starlark.Value, error) {
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
        newList, err := deepCloneIterableToList(iterable)
        if err != nil {
            return nil, err
        }

        return starlark.NewList(newList), nil
    case "set":
        // For lists, recursively clone each element
        iterable := value.(starlark.Iterable)
        newList, err := deepCloneIterableToList(iterable)
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
        newList, err := deepCloneIterableToList(iterable)
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
        newDict, err := deepCloneStringDict(v)
        if err != nil {
            return nil, err
        }
        return newDict, nil
    case "record":
        v := value.(*lib.Struct)
        dict := starlark.StringDict{}
        v.ToStringDict(dict)
        newDict := CloneDict(dict)
        return lib.FromStringDict(lib.Default, newDict), nil

    case "genericset":
        s := value.(*lib.GenericSet)
        newSet := lib.NewGenericSet()
        iter := s.Iterate()
        defer iter.Done()
        var x starlark.Value
        for iter.Next(&x) {
            clonedElem, err := deepCloneStarlarkValue(x)
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
            clonedKey, err := deepCloneStarlarkValue(key)
            if err != nil {
                return nil, err
            }
            clonedValue, err := deepCloneStarlarkValue(value)
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
            clonedElem, err := deepCloneStarlarkValue(x)
            if err != nil {
                return nil, err
            }
            PanicOnError(err)
            newBag.Insert(clonedElem)
        }
        return newBag, nil
    case "role":
        return value, nil
    default:
        return nil, fmt.Errorf("unsupported type: %T, %s", value, value.Type())
    }
}

func deepCloneStringDict(v *starlark.Dict) (*starlark.Dict, error) {
    newDict := &starlark.Dict{}
    for _, item := range v.Items() {
        k, v := item[0], item[1]
        clonedKey, err := deepCloneStarlarkValue(k)
        if err != nil {
            return nil, err
        }
        clonedValue, err := deepCloneStarlarkValue(v)
        if err != nil {
            return nil, err
        }
        newDict.SetKey(clonedKey, clonedValue)
    }
    return newDict, nil
}

func deepCloneIterableToList(iterable starlark.Iterable) ([]starlark.Value, error) {
    var newList []starlark.Value
    iter := iterable.Iterate()
    var x starlark.Value
    for iter.Next(&x) {
        clonedElem, err := deepCloneStarlarkValue(x)
        if err != nil {
            return nil, err
        }
        PanicOnError(err)
        newList = append(newList, clonedElem)
    }
    return newList, nil
}

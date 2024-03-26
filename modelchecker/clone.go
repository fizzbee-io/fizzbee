package modelchecker

import (
    "fmt"
    "go.starlark.net/starlark"
)

func deepCloneStarlarkValue(value starlark.Value) (starlark.Value, error) {
    // starlark has other types as well "string.elems", "string.codepoints", "function"
    // "builtin_function_or_method".
    switch value.Type() {

    case "NoneType", "int", "float", "bool", "string", "bytes", "range":
    //case starlark.Bool, starlark.String, starlark.Int:
        // For simple values, just return a copy
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

    default:
        return nil, fmt.Errorf("unsupported type: %T, %s", value, value.Type())
    }
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

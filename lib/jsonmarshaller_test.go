package lib

import (
    "fmt"
    "github.com/stretchr/testify/assert"
    "go.starlark.net/starlark"
    "testing"
)

func TestMarshalJSONStarlarkValue(t *testing.T) {
    fmt.Println("TestMarshalJSONStarlarkValue")
    listValues := []starlark.Value{starlark.MakeInt(1),  starlark.String(`hello "world"`), starlark.MakeInt(3)}
    list := starlark.NewList(listValues)
    set := starlark.NewSet(len(listValues))
    for _, v := range listValues {
        set.Insert(v)
    }
    dict := starlark.NewDict(3)
    dict.SetKey(starlark.String("one"), starlark.MakeInt(1))
    dict.SetKey(starlark.String("list"), list)
    dict.SetKey(starlark.String("str"), starlark.String("hello"))
    tests := []struct {
        name string
        m starlark.Value
        want string
    }{
        {
            name: "NoneType",
            m: starlark.None,
            want: "null",
        },
        {
            name: "Int",
            m: starlark.MakeInt(-10),
            want: "-10",
        },
        {
            name: "Float",
            m: starlark.Float(-10.12),
            want: "-10.12",
        },
        {
            name: "Bool-true",
            m: starlark.Bool(true),
            want: "true",
        },
        {
            name: "Bool-false",
            m: starlark.Bool(false),
            want: "false",
        },
        {
            name: "string",
            m: starlark.String(`hello "world"`),
            want: `"hello \"world\""`,
        },
        {
            name: "list",
            m: list,
            want: `[1,"hello \"world\"",3]`,
        },
        {
            name: "list",
            m: set,
            want: `[1,"hello \"world\"",3]`,
        },
        {
            name: "dict",
            m: dict,
            want: `{"one":1,"list":[1,"hello \"world\"",3],"str":"hello"}`,
        },

    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := MarshalJSONStarlarkValue(tt.m, 0)
            assert.Nil(t, err)
            assert.Equal(t, tt.want, string(got))
        })
    }
}

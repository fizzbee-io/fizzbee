package lib

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
)

// marshalFunctionJSON encodes a Starlark function (a lambda or a nested def)
// held in state. Two functions are the same value when they come from the
// same source position and have the same default values and the same
// captured values, so all three go into the encoding; the state hash uses
// it too. A spec statement runs as a function whose parameters are the
// names in scope, so a lambda's outside names are captured values, fixed
// as of the statement that created it.
//
//	{"function":"lambda@12:9","defaults":[null,1],"captured":{"x":3}}
func marshalFunctionJSON(fn *starlark.Function, depth int) ([]byte, error) {
	var buf strings.Builder
	pos := fn.Position()
	name, err := json.Marshal(fmt.Sprintf("%s@%d:%d", fn.Name(), pos.Line, pos.Col))
	if err != nil {
		return nil, err
	}
	buf.WriteString(`{"function":`)
	buf.Write(name)

	hasDefaults := false
	for i := 0; i < fn.NumParams(); i++ {
		if fn.ParamDefault(i) != nil {
			hasDefaults = true
			break
		}
	}
	if hasDefaults {
		buf.WriteString(`,"defaults":[`)
		for i := 0; i < fn.NumParams(); i++ {
			if i > 0 {
				buf.WriteString(",")
			}
			if err := writeFunctionValue(&buf, fn.ParamDefault(i), depth); err != nil {
				return nil, err
			}
		}
		buf.WriteString("]")
	}

	if n := fn.NumFreeVars(); n > 0 {
		buf.WriteString(`,"captured":{`)
		for i := 0; i < n; i++ {
			binding, v := fn.FreeVar(i)
			if i > 0 {
				buf.WriteString(",")
			}
			key, err := json.Marshal(binding.Name)
			if err != nil {
				return nil, err
			}
			buf.Write(key)
			buf.WriteString(":")
			if err := writeFunctionValue(&buf, v, depth); err != nil {
				return nil, err
			}
		}
		buf.WriteString("}")
	}
	buf.WriteString("}")
	return []byte(buf.String()), nil
}

// writeFunctionValue writes a default or captured value; nil (no default,
// or a captured name not bound yet) is written as null.
func writeFunctionValue(buf *strings.Builder, v starlark.Value, depth int) error {
	if v == nil {
		buf.WriteString("null")
		return nil
	}
	b, err := MarshalJSONStarlarkValue(v, depth+1)
	if err != nil {
		return err
	}
	buf.Write(b)
	return nil
}

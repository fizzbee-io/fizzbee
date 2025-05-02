package lib

import (
	"encoding/json"
	"fmt"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"strings"
)

func MarshalJSON(obj interface{}) ([]byte, error) {
	if obj == nil {
		return json.Marshal(obj)
	}
	if m, ok := obj.(json.Marshaler); ok {
		return m.MarshalJSON()
	}
	if m, ok := obj.(starlark.Value); ok {
		return MarshalJSONStarlarkValue(m, 0)
	}
	if m, ok := obj.(starlark.StringDict); ok {
		buf := strings.Builder{}
		buf.WriteString("{")
		first := true
		for k, v := range m {
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			buf.WriteString("\"")
			buf.WriteString(k)
			buf.WriteString("\":")
			b, err := MarshalJSONStarlarkValue(v, 0)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("}")
		return []byte(buf.String()), nil
	}
	return json.Marshal(obj)
}

func MarshalJSONStarlarkValue(m starlark.Value, depth int) ([]byte, error) {
	// TODO: using depth to limit the depth of recursion for json export to handle circular references.
	// Ideally, this should be dealt with by managing the visited nodes in the graph.
	// But for now, we just limit the depth.
	if depth > 20 {
		return []byte("\"TOO_DEEP\""), nil
	}
	switch m.Type() {
	case "NoneType":
		return []byte("null"), nil
	case "bool":
		return json.Marshal(m.Truth())
	case "string":
		return json.Marshal(m.(starlark.String).GoString())
	case "bytes":
		return json.Marshal(string(m.(starlark.Bytes)))
	case "int", "float":
		return []byte(m.String()), nil
	case "list", "set", "range", "tuple", "genericset", "bag":
		iter := m.(starlark.Iterable).Iterate()
		defer iter.Done()
		var x starlark.Value
		buf := strings.Builder{}
		buf.WriteString("[")
		first := true
		for iter.Next(&x) {
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			b, err := MarshalJSONStarlarkValue(x, depth+1)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("]")
		return []byte(buf.String()), nil
	case "genericmap", "dict":
		items := m.(starlark.IterableMapping).Items()
		buf := strings.Builder{}
		buf.WriteString("{")
		first := true
		for _, item := range items {
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			k, v := item[0], item[1]
			if k.Type() != "string" {
				k = starlark.String(k.String())
			}
			kb, err := MarshalJSONStarlarkValue(k, depth+1)
			if err != nil {
				return nil, err
			}
			vb, err := MarshalJSONStarlarkValue(v, depth+1)
			if err != nil {
				return nil, err
			}
			buf.Write(kb)
			buf.WriteString(":")
			buf.Write(vb)
		}
		buf.WriteString("}")
		return []byte(buf.String()), nil
	case "struct":
		st := m.(*starlarkstruct.Struct)
		buf := strings.Builder{}
		buf.WriteString("{")
		first := true
		for _, attrName := range st.AttrNames() {
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			buf.WriteString("\"")
			buf.WriteString(attrName)
			buf.WriteString("\":")
			v, err := st.Attr(attrName)
			if err != nil {
				return nil, err
			}
			b, err := MarshalJSONStarlarkValue(v, depth+1)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("}")
		return []byte(buf.String()), nil
	case "record":
		st := m.(*Struct)
		buf := strings.Builder{}
		buf.WriteString("{")
		first := true
		for _, attrName := range st.AttrNames() {
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			buf.WriteString("\"")
			buf.WriteString(attrName)
			buf.WriteString("\":")
			v, err := st.Attr(attrName)
			if err != nil {
				return nil, err
			}
			b, err := MarshalJSONStarlarkValue(v, depth+1)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("}")
		return []byte(buf.String()), nil
	case "role":
		role := m.(*Role)
		return json.Marshal(role.RefString())
	case "RoleStub":
		buf := strings.Builder{}
		//buf.WriteString("{")
		stub := m.(*RoleStub)
		buf.WriteString("{")
		buf.WriteString("\"Role\":")
		b, err := json.Marshal(stub.Role.RefStringShort())
		if err != nil {
			return nil, err
		}
		buf.Write(b)
		buf.WriteString(",")
		buf.WriteString("\"Channel\":")
		b, err = json.Marshal(stub.Channel.RefStringShort())
		if err != nil {
			return nil, err
		}
		buf.Write(b)
		buf.WriteString("}")
		return []byte(buf.String()), nil
	case "model_value", "symmetric_value", "Channel":
		return json.Marshal(m)
	default:
		fmt.Println("Warn: unknown type: ", m.Type(), " value: ", m.String(), " using default json.Marshal")
		return json.Marshal(m)
	}
}

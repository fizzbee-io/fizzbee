package lib

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
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
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		first := true
		for _, k := range keys {
			v := m[k]
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
	if m, ok := obj.(map[string]interface{}); ok {
		buf := strings.Builder{}
		buf.WriteString("{")
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		first := true
		for _, k := range keys {
			v := m[k]
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			buf.WriteString("\"")
			buf.WriteString(k)
			buf.WriteString("\":")
			b, err := MarshalJSON(v)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("}")
		return []byte(buf.String()), nil
	}
	if s, ok := obj.([]interface{}); ok {
		buf := strings.Builder{}
		buf.WriteString("[")
		for i, v := range s {
			if i > 0 {
				buf.WriteString(",")
			}
			b, err := MarshalJSON(v)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("]")
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
	case "list", "range", "tuple":
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
	case "set", "genericset", "bag":
		iter := m.(starlark.Iterable).Iterate()
		defer iter.Done()
		var x starlark.Value
		var elements []string
		for iter.Next(&x) {
			b, err := MarshalJSONStarlarkValue(x, depth+1)
			if err != nil {
				return nil, err
			}
			elements = append(elements, string(b))
		}
		sort.Strings(elements)
		buf := strings.Builder{}
		buf.WriteString("[")
		for i, el := range elements {
			if i > 0 {
				buf.WriteString(",")
			}
			buf.WriteString(el)
		}
		buf.WriteString("]")
		return []byte(buf.String()), nil
	case "genericmap", "dict":
		items := m.(starlark.IterableMapping).Items()
		// Sort items by key to have deterministic output
		sort.Slice(items, func(i, j int) bool {
			// Marshal keys to JSON and compare strings for more robust sorting
			// Using direct String() comparison might not match JSON output sort order
			// but for now String() on starlark values is usually sufficient?
			// Actually, StringDictToMap was using String().
			// However, we should be consistent.
			// If we use MarshalJSONStarlarkValue(items[i][0]) it might be better.
			// But let's stick to String() for now as it is cheaper?
			// NO, `String()` on a dict returns "{...}" which might depend on internal order.
			// But here items[i][0] are keys. Keys are usually immutable (strings, ints, tuples).
			// Tuples containing dicts are invalid keys.
			// So String() comparison is mostly safe for keys.
			return items[i][0].String() < items[j][0].String()
		})
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
			// If the key is a json.Marshaler, use its MarshalJSON method
			if marshaler, ok := k.(json.Marshaler); ok {
				kb, err := marshaler.MarshalJSON()
				if err != nil {
					return nil, err
				}
				// keys in json must be strings. If the key is not a string (e.g. number, bool, object), quote it.
				if len(kb) > 0 && kb[0] != '"' {
					kb = []byte(strconv.Quote(string(kb)))
				}
				vb, err := MarshalJSONStarlarkValue(v, depth+1)
				if err != nil {
					return nil, err
				}
				buf.Write(kb)
				buf.WriteString(":")
				buf.Write(vb)
				continue
			}

			if k.Type() != "string" {
				k = starlark.String(k.String())
			}
			kb, err := MarshalJSONStarlarkValue(k, depth+1)
			if err != nil {
				return nil, err
			}
			// keys in json must be strings. If the key is not a string (e.g. number, bool, object), quote it.
			if len(kb) > 0 && kb[0] != '"' {
				kb = []byte(strconv.Quote(string(kb)))
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
	case "model_value", "symmetric_value", "Channel", "symmetry_segment":
		return json.Marshal(m)
	default:
		fmt.Println("Warn: unknown type: ", m.Type(), " value: ", m.String(), " using default json.Marshal")
		return json.Marshal(m)
	}
}

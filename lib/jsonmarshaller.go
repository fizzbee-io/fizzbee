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
		return MarshalJSONStarlarkValue(m)
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
			b, err := MarshalJSONStarlarkValue(v)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		}
		buf.WriteString("}")
		return []byte(buf.String()), nil
	}
	if msg, ok := obj.(*ChannelMessage); ok && msg != nil {
		buf := strings.Builder{}
		buf.WriteString("{")
		buf.WriteString("\"receiver\":")
		fmt.Println("Message: ", msg, msg == nil)
		fmt.Println("Receiver: ", msg.receiver)
		b, err := json.Marshal(msg.receiver)
		if err != nil {
			return nil, err
		}
		buf.Write(b)
		buf.WriteString(",")
		if msg.frame == nil {
			buf.WriteString("\"args\":")
			b, err = MarshalJSON(msg.args)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
			buf.WriteString(",")
			buf.WriteString("\"kwargs\":")
			b, err = MarshalJSON(msg.kwargs)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
		} else {
			buf.WriteString("\"name\":")
			b, err = MarshalJSON(msg.function)
			if err != nil {
				return nil, err
			}
			buf.Write(b)
			buf.WriteString(",")
			buf.WriteString("\"params\":")
			b, err = MarshalJSON(msg.params)
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

func MarshalJSONStarlarkValue(m starlark.Value) ([]byte, error) {
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
			b, err := MarshalJSONStarlarkValue(x)
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
			kb, err := MarshalJSONStarlarkValue(k)
			if err != nil {
				return nil, err
			}
			vb, err := MarshalJSONStarlarkValue(v)
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
			b, err := MarshalJSONStarlarkValue(v)
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
	case "record", "model_value", "symmetric_value":
		return json.Marshal(m)
	case "Channel":
		buf := strings.Builder{}
		//buf.WriteString("{")
		channel := m.(*Channel)
		//buf.WriteString("\"Messages\":")
		buf.WriteString("[")
		first := true
		for _, message := range channel.Messages {
			if !first {
				buf.WriteString(",")
			} else {
				first = false
			}
			msgBytes, err := MarshalJSON(message)
			if err != nil {
				return nil, err
			}
			buf.Write(msgBytes)
		}
		buf.WriteString("]")
		//buf.WriteString("}")
		str := buf.String()
		fmt.Println("Channel JSON: ", str)
		return []byte(str), nil
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
	default:
		fmt.Println("Warn: unknown type: ", m.Type(), " value: ", m.String(), " using default json.Marshal")
		return json.Marshal(m)
	}
}

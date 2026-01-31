package lib

import (
	ast "fizz/proto"
	"fmt"
	"go.starlark.net/starlark"
	"strings"
)

var (
	roleMethods = map[string]*starlark.Builtin{
		//"clear": starlark.NewBuiltin("clear", dict_clear),
	}
)

var (
	roleRefs = map[string]uint64{}
)

func ClearRoleRefs() {
	roleRefs = map[string]uint64{}
}

type Role struct {
	Ref         uint64
	Name        string
	Symmetric   bool
	Params      *Struct
	Fields      *Struct
	Methods     map[string]*starlark.Function
	RoleMethods map[string]*starlark.Builtin
	InitValues  *Struct
}

func (r *Role) AddMethod(name string, val starlark.Value) error {
	if val.Type() != "function" {
		return fmt.Errorf("value must be a function. got %s", val.Type())
	}
	r.Methods[name] = val.(*starlark.Function)
	return nil
}

func AddSelfParamBuiltin(role *Role, val *starlark.Function) func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		newArgs := make([]starlark.Value, 0, len(args)+1)
		newArgs = append(newArgs, role)
		newArgs = append(newArgs, args...)
		return val.CallInternal(thread, newArgs, kwargs)
	}
}

func (r *Role) SetField(name string, val starlark.Value) error {
	// If name is found in Params or BuiltinAttrNames, then fail
	// Otherwise call Fields.SetField
	if _, err := r.Params.Attr(name); err == nil {
		return fmt.Errorf("cannot set immutable field %s on role %s", name, r.Name)
	} else if _, ok := err.(starlark.NoSuchAttrError); !ok {
		return err
	} else if v, _ := BuiltinAttr(r, name, roleMethods); v != nil {
		return fmt.Errorf("cannot override builtins %s on role %s", name, r.Name)
	}
	return r.Fields.SetField(name, val)
}

func (r *Role) Attr(name string) (starlark.Value, error) {
	if name == "__id__" {
		return r.GetId(), nil
	}
	if v, err := r.Fields.Attr(name); err == nil {
		return v, nil
	} else if _, ok := err.(starlark.NoSuchAttrError); !ok {
		return v, err
	} else if v, err := r.Params.Attr(name); err == nil {
		return v, nil
	} else if _, ok := err.(starlark.NoSuchAttrError); !ok {
		return v, err
	} else if v, ok := r.Methods[name]; ok {
		return starlark.NewBuiltin(name, AddSelfParamBuiltin(r, v)), nil
	}
	return BuiltinAttr(r, name, r.RoleMethods)
}

func (r *Role) GetId() starlark.Value {
	if r.Symmetric {
		return NewSymmetricValue(r.Name, r.Ref)
	}
	return NewModelValue(r.Name, r.Ref)
}

func (r *Role) AttrNames() []string {
	return BuiltinAttrNames(r.RoleMethods)
}

func (r *Role) String() string {
	b := strings.Builder{}
	b.WriteString(fmt.Sprintf("role %s#%d (", r.Name, r.Ref))
	if len(r.Params.AttrNames()) > 0 {
		b.WriteString(r.Params.String())
		b.WriteString(",")
	}
	b.WriteString(r.Fields.String())
	b.WriteString(")")
	return b.String()
}

func (r *Role) MarshalJSON() ([]byte, error) {
	b := strings.Builder{}
	b.WriteString("{")
	b.WriteString(fmt.Sprintf("\"name\": \"%s\",", r.Name))
	b.WriteString(fmt.Sprintf("\"ref\": %d,", r.Ref))
	b.WriteString(fmt.Sprintf("\"ref_string\": \"%s\",", r.RefStringShort()))
	b.WriteString("\"params\": ")
	params, err := r.Params.MarshalJSON()
	if err != nil {
		return nil, err
	}
	b.Write(params)
	b.WriteString(",")
	b.WriteString("\"fields\": ")
	fields, err := r.Fields.MarshalJSON()
	if err != nil {
		return nil, err
	}
	b.Write(fields)
	b.WriteString("}")
	s := b.String()
	return []byte(s), nil
}

func (r *Role) RefString() string {
	return GenerateRefString(r.Name, r.Ref)
}

func GenerateRefString(name string, ref uint64) string {
	return fmt.Sprintf("role %s#%d", name, ref)
}

func (r *Role) RefStringShort() string {
	return fmt.Sprintf("%s#%d", r.Name, r.Ref)
}

func (r *Role) Type() string {
	return "role"
}

func (r *Role) Freeze() {

}

func (r *Role) Truth() starlark.Bool {
	return true
}

func (r *Role) Hash() (uint32, error) {
	hash, _ := starlark.String(r.Name).Hash()
	return hash + uint32(r.Ref), nil
}

func (r *Role) IsSymmetric() bool {
	return r.Symmetric
}

var _ starlark.HasAttrs = (*Role)(nil)
var _ starlark.HasSetField = (*Role)(nil)
var _ starlark.Value = (*Role)(nil)

func CreateRoleBuiltin(astRole *ast.Role, symmetric bool, roles *[]*Role) *starlark.Builtin {
	name := astRole.Name
	return starlark.NewBuiltin(name, func(t *starlark.Thread, b *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		params := FromKeywords(starlark.String("params"), kwargs)
		nextRef := roleRefs[name]
		if roleRefs[name] > 0 {
			roleRefs[name]++
		} else {
			roleRefs[name] = 1
		}
		fields := FromStringDict(starlark.String("fields"), starlark.StringDict{})
		initValues := FromStringDict(starlark.String("init_values"), starlark.StringDict{})
		roleMethods := make(map[string]*starlark.Builtin)
		for _, function := range astRole.Functions {
			roleMethods[function.Name] = starlark.NewBuiltin(function.Name, fizz_func_always_error)
		}
		r := &Role{Ref: nextRef, Name: name, Symmetric: symmetric, Params: params, Fields: fields, Methods: map[string]*starlark.Function{}, RoleMethods: roleMethods, InitValues: initValues}
		*roles = append(*roles, r)
		return r, nil
	})
}

func fizz_func_always_error(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

	return starlark.None, fmt.Errorf("%s: %v", b.Name(), "Currently, fizz functions can be called only in the following ways. \n See https://fizzbee.io/tutorials/limitations/#fizz-functions-can-be-called-only-in-a-limited-ways for more details and workaround.")
}

// CreateResolveRoleBuiltIn returns a builtin resolve_role, when called with a role's __id__ (GetId()), returns
// the Role object in the roles array with the given id.
func CreateResolveRoleBuiltIn(roles *[]*Role) *starlark.Builtin {
	return starlark.NewBuiltin("resolve_role", func(t *starlark.Thread, b *starlark.Builtin,
		args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("resolve_role expects exactly one argument")
		}
		idValue := args[0]
		if idValue.Type() != ModelValueType && idValue.Type() != SymmetricValueType {
			return nil, fmt.Errorf("resolve_role expects a ModelValue or SymmetricValue, got %s", idValue.Type())
		}
		for _, role := range *roles {
			if role.GetId().String() == idValue.String() {
				return role, nil
			}
		}
		return nil, fmt.Errorf("role with id %d not found", idValue)
	})
}

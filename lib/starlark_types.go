package lib

import (
    "cmp"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "go.starlark.net/lib/math"
    "go.starlark.net/starlark"
    "go.starlark.net/starlarkstruct"
    "go.starlark.net/syntax"
    "math/rand"
    "slices"
    "sort"
    "strings"
)
const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"


func RandString(n int) string {
    b := make([]byte, n)
    dest := make([]byte, base64.URLEncoding.EncodedLen(n))
    rand.Read(b)
    base64.URLEncoding.Encode(dest, b)
    return string(dest)
}

var (
    SymmetryPrefix = RandString(9) + "-" // Replace this with a UUID

    Builtins = starlark.StringDict{
        "struct": starlark.NewBuiltin("struct", starlarkstruct.Make),
        "record": starlark.NewBuiltin("record", Make),
        "enum":   starlark.NewBuiltin("enum", MakeEnum),
        "genericmap": starlark.NewBuiltin("genericmap", MakeGenericMap),
        "genericset": starlark.NewBuiltin("genericset", MakeGenericSet),
        "symmetric_values": starlark.NewBuiltin("symmetric_values", MakeSymmetricValues),
        "bag": starlark.NewBuiltin("bag", MakeBag),
        "math": math.Module,
    }

    StarlarkPtrTypes = []starlark.Value{
		&starlark.Set{},
		&starlark.List{},
		&starlark.Dict{},
		&starlarkstruct.Struct{},
		&starlark.Tuple{},
		&Struct{},
		&Bag{},
		&GenericSet{},
		&GenericMap{},
		&Role{},
    }

    mapMethods = map[string]*starlark.Builtin{
        "clear":      starlark.NewBuiltin("clear", dict_clear),
        "get":        starlark.NewBuiltin("get", dict_get),
        "items":      starlark.NewBuiltin("items", dict_items),
        "keys":       starlark.NewBuiltin("keys", dict_keys),
        "pop":        starlark.NewBuiltin("pop", dict_pop),
        "popitem":    starlark.NewBuiltin("popitem", dict_popitem),
        "setdefault": starlark.NewBuiltin("setdefault", dict_setdefault),
        "update":     starlark.NewBuiltin("update", dict_update),
        "values":     starlark.NewBuiltin("values", dict_values),
    }

    setMethods = map[string]*starlark.Builtin{
        "add":                  starlark.NewBuiltin("add", set_add),
        "clear":                starlark.NewBuiltin("clear", set_clear),
        "difference":           starlark.NewBuiltin("difference", set_difference),
        "discard":              starlark.NewBuiltin("discard", set_discard),
        "intersection":         starlark.NewBuiltin("intersection", set_intersection),
        "issubset":             starlark.NewBuiltin("issubset", set_issubset),
        "issuperset":           starlark.NewBuiltin("issuperset", set_issuperset),
        "pop":                  starlark.NewBuiltin("pop", set_pop),
        "remove":               starlark.NewBuiltin("remove", set_remove),
        "symmetric_difference": starlark.NewBuiltin("symmetric_difference", set_symmetric_difference),
        "union":                starlark.NewBuiltin("union", set_union),
    }

    bagMethods = map[string]*starlark.Builtin{
        "add":                  starlark.NewBuiltin("add", bag_add),
        "add_all":              starlark.NewBuiltin("add_all", bag_add_all),
        "clear":                starlark.NewBuiltin("clear", bag_clear),
        "discard":              starlark.NewBuiltin("discard", bag_discard),
        "pop":                  starlark.NewBuiltin("pop", bag_pop),
        "remove":               starlark.NewBuiltin("remove", bag_remove),
    }
)

type SymmetricValues struct {
    starlark.Tuple
}
func (s SymmetricValues) Index(i int) SymmetricValue { return s.Tuple.Index(i).(SymmetricValue) }
func (s SymmetricValues) Type() string   { return "symmetric_values" }

func MakeSymmetricValues(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    prefix := ""
    var countValue starlark.Value
    if len(args) == 1 {
        countValue = args[0]
    }
    if len(args) == 2 {
        if s, ok := args[0].(starlark.String); ok {
            prefix = s.GoString()
        } else {
            return nil, fmt.Errorf("symmetric_values(int) or symmetric_values(string, int) expected. Unexpected: %v", args[0])
        }
        countValue = args[1]
    }
    count, err := starlark.AsInt32(countValue)
    if err != nil {
        return nil, fmt.Errorf("symmetric_values(int) or symmetric_values(string, int) expected. %v", err)
    }
    values := SymmetricValues{}
    for i := 0; i < count; i++ {
        values.Tuple = append(values.Tuple, NewSymmetricValue(prefix, i))
    }
    return values, nil
}

func NewSymmetricValues(values []SymmetricValue) *SymmetricValues {
    sv := &SymmetricValues{}
    for _, v := range values {
        sv.Tuple = append(sv.Tuple, v)
    }
    return sv
}

func MakeEnum(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    for _, arg := range args {
        kwargs = append(kwargs, starlark.Tuple{arg, arg})
    }
    return starlarkstruct.FromKeywords(starlark.String("enum"), kwargs), nil
}

func BuiltinAttr(recv starlark.Value, name string, methods map[string]*starlark.Builtin) (starlark.Value, error) {
    b := methods[name]
    if b == nil {
        return nil, nil // no such method
    }
    return b.BindReceiver(recv), nil
}

func BuiltinAttrNames(methods map[string]*starlark.Builtin) []string {
    names := make([]string, 0, len(methods))
    for name := range methods {
        names = append(names, name)
    }
    sort.Strings(names)
    return names
}


func MakeGenericMap(thread *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (
    starlark.Value, error) {
    if len(args) > 1 {
        return nil, fmt.Errorf("genericmap: got %d arguments, want at most 1", len(args))
    }
    if len(args) > 0 {
        return nil, fmt.Errorf("genericmap: genericmap with another map not implemented yet")
    }
    dict := new(GenericMap)
    err := updateDict(dict, args, kwargs)
    if err != nil {
        return nil, fmt.Errorf("genericmap: %v", err)
    }
    return dict, nil
}

func NewGenericMap() *GenericMap {
    return &GenericMap{}
}

func updateDict(dict *GenericMap, args starlark.Tuple, kwargs []starlark.Tuple) error {
    for _, kwarg := range kwargs {
        dict.entries = append(dict.entries, kwarg)
    }
    return nil
}

type GenericMap struct {
    // Should this simply be a slice of values instead?
    entries []starlark.Tuple
}

func (g *GenericMap) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
    y := y_.(*GenericMap)
    switch op {
    case syntax.EQL:
        ok, err := dictsEqual(g, y, depth)
        return ok, err
    case syntax.NEQ:
        ok, err := dictsEqual(g, y, depth)
        return !ok, err
    default:
        return false, fmt.Errorf("%s %s %s not implemented", g.Type(), op, y.Type())
    }
}

func dictsEqual(x, y *GenericMap, depth int) (bool, error) {
    if x.Len() != y.Len() {
        return false, nil
    }
    for _, entry := range x.entries {
        key, xval := entry[0], entry[1]
        yval, found, _ := y.Get(key)
        if !found {
            return false, nil
        }
        if eq, err := starlark.EqualDepth(xval, yval, depth-1); err != nil {
            return false, err
        } else if !eq {
            return false, nil
        }
    }

    return true, nil
}

func (g *GenericMap) Len() int {
    return len(g.entries)
}

func (g *GenericMap) Items() []starlark.Tuple {
    c := make([]starlark.Tuple, len(g.entries))
    copy(c, g.entries)
    return c
}

type keyIterator struct {
    entries []starlark.Tuple
    i       int
}

func (it *keyIterator) Next(k *starlark.Value) bool {
    if it.i >= len(it.entries) {
        return false
    }
    *k = it.entries[it.i][0]
    it.i++
    return true
}

func (it *keyIterator) Done() {}

func (g *GenericMap) Iterate() starlark.Iterator {
    return &keyIterator{entries: g.entries}
}

func (g *GenericMap) Get(key starlark.Value) (v starlark.Value, found bool, err error) {
    for _, entry := range g.entries {
        if eq, err := starlark.Equal(entry[0], key); err != nil {
            return nil, false, fmt.Errorf("%s: %v", "genericmap", err)
        } else if eq {
            return entry[1], true, nil
        }
    }
    return starlark.None, false, nil
}

func (g *GenericMap) SetKey(key, value starlark.Value) error {
    for _, entry := range g.entries {
        if eq, err := starlark.Equal(entry[0], key); err != nil {
            return fmt.Errorf("%s: %v", "genericmap", err)
        } else if eq {
            entry[1] = value
            return nil
        }
    }
    g.entries = append(g.entries, starlark.Tuple{key, value})
    return nil
}

func (g *GenericMap) Attr(name string) (starlark.Value, error) {
    return BuiltinAttr(g, name, mapMethods)
}

func (g *GenericMap) AttrNames() []string {
    return BuiltinAttrNames(mapMethods)
}

func sortByKeys(entries []starlark.Tuple) []starlark.Tuple {
    c := make([]starlark.Tuple, len(entries))
    copy(c, entries)
    sort.SliceStable(c, func(i, j int) bool {
        return c[i][0].String() < c[j][0].String()
    })
    return c
}

func (g *GenericMap) String() string {
    buf := new(strings.Builder)
    buf.WriteByte('{')
    // Handle case where a map contains itself
    sorted := sortByKeys(g.entries)
    sep := ""
    for _, entry := range sorted {
        buf.WriteString(sep)
        sep = ", "
        buf.WriteString(entry[0].String())
        buf.WriteString(": ")
        buf.WriteString(entry[1].String())
    }
    buf.WriteByte('}')
    return buf.String()
}

func (g *GenericMap) Type() string {
    return "genericmap"
}

func (g *GenericMap) Freeze() {
    for _, entry := range g.entries {
        entry.Freeze()
    }
}

func (g *GenericMap) Truth() starlark.Bool {
    return g.Len() > 0
}

func (g *GenericMap) Hash() (uint32, error) {
    return 0, fmt.Errorf("unhashable type: genericmap")
}

func (g *GenericMap) Clear() error {
    g.entries = nil
    return nil
}

func (g *GenericMap) Delete(k starlark.Value) (v starlark.Value, found bool, err error) {
    for i, entry := range g.entries {
        if eq, err := starlark.Equal(entry[0], k); err != nil {
            return nil, false, fmt.Errorf("%s: %v", "genericmap", err)
        } else if eq {
            g.entries = append(g.entries[:i], g.entries[i+1:]...)
            return entry[1], true, nil
        }
    }
    return starlark.None, false, nil
}

func (g *GenericMap) Keys() []starlark.Value {
    keys := make([]starlark.Value, len(g.entries))
    for i, entry := range g.entries {
        keys[i] = entry[0]
    }
    return keys
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·get
func dict_get(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var key, dflt starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &key, &dflt); err != nil {
        return nil, err
    }
    if v, ok, err := b.Receiver().(*GenericMap).Get(key); err != nil {
        return nil, nameErr(b, err)
    } else if ok {
        return v, nil
    } else if dflt != nil {
        return dflt, nil
    }
    return starlark.None, nil
}

func dict_clear(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    return starlark.None, b.Receiver().(*GenericMap).Clear()
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·items
func dict_items(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (
    starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    items := b.Receiver().(*GenericMap).Items()
    res := make([]starlark.Value, len(items))
    for i, item := range items {
        res[i] = item // convert [2]Value to Value
    }
    return starlark.NewList(res), nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·keys
func dict_keys(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    return starlark.NewList(b.Receiver().(*GenericMap).Keys()), nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·pop
func dict_pop(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var k, d starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &k, &d); err != nil {
        return nil, err
    }
    if v, found, err := b.Receiver().(*GenericMap).Delete(k); err != nil {
        return nil, nameErr(b, err) // dict is frozen or key is unhashable
    } else if found {
        return v, nil
    } else if d != nil {
        return d, nil
    }
    return nil, nameErr(b, "missing key")
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·popitem
func dict_popitem(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    recv := b.Receiver().(*GenericMap)
    recv.Len()

    if recv.Len() == 0 {
        return nil, nameErr(b, "empty dict")
    }
    lastIndex := len(recv.entries) - 1
    last := recv.entries[lastIndex]
    recv.entries = recv.entries[:lastIndex]
    return last, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·setdefault
func dict_setdefault(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var key, dflt starlark.Value = nil, starlark.None
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &key, &dflt); err != nil {
        return nil, err
    }
    dict := b.Receiver().(*GenericMap)
    if v, ok, err := dict.Get(key); err != nil {
        return nil, nameErr(b, err)
    } else if ok {
        return v, nil
    } else if err := dict.SetKey(key, dflt); err != nil {
        return nil, nameErr(b, err)
    } else {
        return dflt, nil
    }
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·update
func dict_update(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if len(args) > 1 {
        return nil, fmt.Errorf("update: got %d arguments, want at most 1", len(args))
    }
    if len(args) > 0 {
        return nil, fmt.Errorf("genericmap: update with another map not implemented yet")
    }
    if err := updateDict(b.Receiver().(*GenericMap), args, kwargs); err != nil {
        return nil, fmt.Errorf("update: %v", err)
    }
    return starlark.None, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#dict·update
func dict_values(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    items := b.Receiver().(*GenericMap).Items()
    res := make([]starlark.Value, len(items))
    for i, item := range items {
        res[i] = item[1]
    }
    return starlark.NewList(res), nil
}

// nameErr returns an error message of the form "name: msg"
// where name is b.Name() and msg is a string or error.
func nameErr(b *starlark.Builtin, msg interface{}) error {
    return fmt.Errorf("%s: %v", b.Name(), msg)
}

// Assert that GenericMap implements the starlark.Value interface.
var _ starlark.Comparable = (*GenericMap)(nil)
var _ starlark.HasAttrs = (*GenericMap)(nil)
var _ starlark.HasSetKey = (*GenericMap)(nil)
var _ starlark.Iterable = (*GenericMap)(nil)
var _ starlark.IterableMapping = (*GenericMap)(nil)
var _ starlark.Mapping = (*GenericMap)(nil)
var _ starlark.Sequence = (*GenericMap)(nil)
var _ starlark.Value = (*GenericMap)(nil)


func MakeGenericSet(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (
    starlark.Value, error) {
    var iterable starlark.Iterable
    if err := starlark.UnpackPositionalArgs("genericset", args, kwargs, 0, &iterable); err != nil {
        return nil, err
    }
    set := NewGenericSet()
    if iterable != nil {
        iter := iterable.Iterate()
        defer iter.Done()
        var x starlark.Value
        for iter.Next(&x) {
            if err := set.Insert(x); err != nil {
                return nil, nameErr(b, err)
            }
        }
    }
    return set, nil
}

func NewGenericSet() *GenericSet {
    return &GenericSet{dict: &GenericMap{}}
}

type GenericSet struct {
    dict *GenericMap
}

func (g *GenericSet) Get(value starlark.Value) (v starlark.Value, found bool, err error) {
    return g.dict.Get(value)
}

func (g *GenericSet) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
    x:=g
    y := y_.(*GenericSet)
    switch op {
    case syntax.EQL:
        ok, err := setsEqual(x, y, depth)
        return ok, err
    case syntax.NEQ:
        ok, err := setsEqual(x, y, depth)
        return !ok, err
    case syntax.GE: // superset
       if x.Len() < y.Len() {
           return false, nil
       }
       iter := y.Iterate()
       defer iter.Done()
       return x.IsSuperset(iter)
    case syntax.LE: // subset
       gt, err := y.CompareSameType(syntax.GT, x, depth)
       return !gt, err
    case syntax.GT: // proper superset
       if x.Len() <= y.Len() {
           return false, nil
       }
       iter := y.Iterate()
       defer iter.Done()
       return x.IsSuperset(iter)
    case syntax.LT: // proper subset
        ge, err := y.CompareSameType(syntax.GE, x, depth)
        return !ge, err
    default:
        return false, fmt.Errorf("%s %s %s not implemented", x.Type(), op, y.Type())
    }
}

func setsEqual(x, y *GenericSet, depth int) (bool, error) {
    if x.Len() != y.Len() {
        return false, nil
    }
    for _, entry := range x.dict.entries {
        key := entry[0]
        found, _ := y.Has(key)
        if !found {
            return false, nil
        }
    }
    return true, nil
}

func (s *GenericSet) IsSuperset(other starlark.Iterator) (bool, error) {
   var x starlark.Value
   for other.Next(&x) {
       found, err := s.Has(x)
       if err != nil {
           return false, err
       }
       if !found {
           return false, nil
       }
   }
   return true, nil
}

func (g *GenericSet) IsSubset(other *GenericSet) (bool, error) {
    var x starlark.Value
    it := g.Iterate()
    for it.Next(&x) {
        found, err := other.Has(x)
        if err != nil {
            return false, err
        }
        if !found {
            return false, nil
        }
    }
    return true, nil
}

func (g *GenericSet) Attr(name string) (starlark.Value, error) {
    return BuiltinAttr(g, name, setMethods)
}

func (g *GenericSet) AttrNames() []string {
    return BuiltinAttrNames(setMethods)
}

func (g *GenericSet) Iterate() starlark.Iterator {
    return g.dict.Iterate()
}

func (g *GenericSet) Len() int {
    return g.dict.Len()
}

func (g *GenericSet) String() string {
    buf := new(strings.Builder)
    buf.WriteByte('{')
    // TODO: Handle case where a set contains itself

    sorted := sortByKeys(g.dict.entries)

    sep := ""
    for _, entry := range sorted {
        buf.WriteString(sep)
        sep = ", "
        buf.WriteString(entry[0].String())
    }
    buf.WriteByte('}')
    return buf.String()
}

func (g *GenericSet) Type() string {
    return "genericset"
}

func (g *GenericSet) Freeze() {
    g.dict.Freeze()
}

func (g *GenericSet) Truth() starlark.Bool {
    return g.Len() > 0
}

func (g *GenericSet) Hash() (uint32, error) {
    return 0, fmt.Errorf("unhashable type: genericset")
}

func (g *GenericSet) Delete(k starlark.Value) (found bool, err error) {
    _, found, err = g.dict.Delete(k)
    return
}

func (g *GenericSet) Clear() error {
    return g.dict.Clear()
}
func (g *GenericSet) Has(k starlark.Value) (found bool, err error) {
    _, found, err = g.dict.Get(k); return
}
func (g *GenericSet) Insert(k starlark.Value) error {
    return g.dict.SetKey(k, starlark.None)
}


func (g *GenericSet) clone() *GenericSet {
    set := &GenericSet{dict: &GenericMap{}}
    for _, e := range g.dict.entries{
        _ = set.Insert(e[0]) // can't fail
    }
    return set
}

func (g *GenericSet) Union(iter starlark.Iterator) (starlark.Value, error) {
    set := g.clone()
    var x starlark.Value
    for iter.Next(&x) {
        if err := set.Insert(x); err != nil {
            return nil, err
        }
    }
    return set, nil
}

func (g *GenericSet) Difference(other starlark.Iterator) (starlark.Value, error) {
    diff := g.clone()
    var x starlark.Value
    for other.Next(&x) {
        if _, err := diff.Delete(x); err != nil {
            return nil, err
        }
    }
    return diff, nil
}

func (s *GenericSet) Intersection(other starlark.Iterator) (starlark.Value, error) {
    intersect := &GenericSet{dict: &GenericMap{}}
    var x starlark.Value
    for other.Next(&x) {
        found, err := s.Has(x)
        if err != nil {
            return nil, err
        }
        if found {
            err = intersect.Insert(x)
            if err != nil {
                return nil, err
            }
        }
    }
    return intersect, nil
}

func (s *GenericSet) SymmetricDifference(other starlark.Iterator) (starlark.Value, error) {
    diff := s.clone()
    var x starlark.Value
    for other.Next(&x) {
        found, err := diff.Delete(x)
        if err != nil {
            return nil, err
        }
        if !found {
            diff.Insert(x)
        }
    }
    return diff, nil
}


// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·add.
func set_add(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var elem starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &elem); err != nil {
        return nil, err
    }
    if found, err := b.Receiver().(*GenericSet).Has(elem); err != nil {
        return nil, nameErr(b, err)
    } else if found {
        return starlark.None, nil
    }
    err := b.Receiver().(*GenericSet).Insert(elem)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return starlark.None, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·clear.
func set_clear(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    if b.Receiver().(*GenericSet).Len() > 0 {
        if err := b.Receiver().(*GenericSet).Clear(); err != nil {
            return nil, nameErr(b, err)
        }
    }
    return starlark.None, nil
}


// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·difference.
func set_difference(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    // TODO: support multiple others: s.difference(*others)
    var other starlark.Iterable
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0, &other); err != nil {
        return nil, err
    }
    iter := other.Iterate()
    defer iter.Done()
    diff, err := b.Receiver().(*GenericSet).Difference(iter)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return diff, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set_intersection.
func set_intersection(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    // TODO: support multiple others: s.difference(*others)
    var other starlark.Iterable
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0, &other); err != nil {
        return nil, err
    }
    iter := other.Iterate()
    defer iter.Done()
    diff, err := b.Receiver().(*GenericSet).Intersection(iter)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return diff, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set_issubset.
func set_issubset(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {

    other, err := MakeGenericSet(thread, b, args, kwargs)
    if err != nil {
        return nil, err
    }
    diff, err := b.Receiver().(*GenericSet).IsSubset(other.(*GenericSet))
    return starlark.Bool(diff), nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set_issuperset.
func set_issuperset(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
   var other starlark.Iterable
   if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0, &other); err != nil {
       return nil, err
   }
   iter := other.Iterate()
   defer iter.Done()
   diff, err := b.Receiver().(*GenericSet).IsSuperset(iter)
   if err != nil {
       return nil, nameErr(b, err)
   }
   return starlark.Bool(diff), nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·discard.
func set_discard(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var k starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &k); err != nil {
        return nil, err
    }
    if found, err := b.Receiver().(*GenericSet).Has(k); err != nil {
        return nil, nameErr(b, err)
    } else if !found {
        return starlark.None, nil
    }
    if _, err := b.Receiver().(*GenericSet).Delete(k); err != nil {
        return nil, nameErr(b, err) // set is frozen
    }
    return starlark.None, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·pop.
func set_pop(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    recv := b.Receiver().(*GenericSet)
    if recv.Len() <= 0 {
        return nil, nameErr(b, "empty set")
    }
    lastIndex := len(recv.dict.entries) - 1
    last := recv.dict.entries[lastIndex]
    recv.dict.entries = recv.dict.entries[:lastIndex]
    return last[0], nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·remove.
func set_remove(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var k starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &k); err != nil {
        return nil, err
    }
    if found, err := b.Receiver().(*GenericSet).Delete(k); err != nil {
        return nil, nameErr(b, err) // dict is frozen or key is unhashable
    } else if found {
        return starlark.None, nil
    }
    return nil, nameErr(b, "missing key")
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·symmetric_difference.
func set_symmetric_difference(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var other starlark.Iterable
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0, &other); err != nil {
        return nil, err
    }
    iter := other.Iterate()
    defer iter.Done()
    diff, err := b.Receiver().(*GenericSet).SymmetricDifference(iter)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return diff, nil
}

// https://github.com/google/starlark-go/blob/master/doc/spec.md#set·union.
func set_union(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var iterable starlark.Iterable
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0, &iterable); err != nil {
        return nil, err
    }
    iter := iterable.Iterate()
    defer iter.Done()
    union, err := b.Receiver().(*GenericSet).Union(iter)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return union, nil
}

// Assert that GenericMap implements the starlark interfaces.
var _ starlark.Comparable = (*GenericSet)(nil)
var _ starlark.HasAttrs = (*GenericSet)(nil)
var _ starlark.Iterable = (*GenericSet)(nil)
var _ starlark.Sequence = (*GenericSet)(nil)
var _ starlark.Value = (*GenericSet)(nil)
// There is a limit in the API, this is required to get 'in' keyword to work
var _ starlark.Mapping = (*GenericSet)(nil)


func MakeBag(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (
    starlark.Value, error) {
    var iterable starlark.Iterable
    if err := starlark.UnpackPositionalArgs("genericset", args, kwargs, 0, &iterable); err != nil {
        return nil, err
    }
    bag := NewBag(nil)
    if iterable != nil {
        iter := iterable.Iterate()
        defer iter.Done()
        var x starlark.Value
        for iter.Next(&x) {
            if err := bag.Insert(x); err != nil {
                return nil, nameErr(b, err)
            }
        }
    }
    return bag, nil
}

func NewBag(elems []starlark.Value) *Bag {
    return &Bag{elems: sortAsString(elems)}
}

type Bag struct {
    elems []starlark.Value
}

func (b *Bag) Get(value starlark.Value) (v starlark.Value, found bool, err error) {
    index, err := b.Find(value)
    if err != nil || index < 0 {
        return nil, false, err
    }
    return b.elems[index], true, nil
}

func (b *Bag) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
    y := y_.(*Bag)
    // It's tempting to check x == y as an optimization here,
    // but wrong because a list containing NaN is not equal to itself.
    switch op {
    case syntax.EQL:
        return bagEqual(b.elems, y.elems, depth)
    case syntax.NEQ:
        eq, err := bagEqual(b.elems, y.elems, depth)
        return !eq, err
    default:
        return false, fmt.Errorf("%s %s %s not implemented", b.Type(), op, y.Type())
    }
}

func bagEqual(elems []starlark.Value, elems2 []starlark.Value, depth int) (bool, error) {
    if len(elems) != len(elems2) {
        return false, nil
    }
    for i, elem := range elems {
        eq, err := starlark.EqualDepth(elem, elems2[i], depth)
        if err != nil {
            return false, err
        }
        if !eq {
            return false, nil
        }
    }
    return true, nil
    
}

func (b *Bag) Attr(name string) (starlark.Value, error) {
    return BuiltinAttr(b, name, bagMethods)
}

func (b *Bag) AttrNames() []string {
    return BuiltinAttrNames(bagMethods)
}


type listIterator struct {
    entries []starlark.Value
    i       int
}

func (it *listIterator) Next(k *starlark.Value) bool {
    if it.i >= len(it.entries) {
        return false
    }
    *k = it.entries[it.i]
    it.i++
    return true
}

func (it *listIterator) Done() {}

func (b *Bag) Iterate() starlark.Iterator {
    return &listIterator{entries: b.elems}
}

func (b *Bag) Len() int {
    return len(b.elems)
}

func (b *Bag) String() string {
    buf := new(strings.Builder)
    buf.WriteByte('[')
    sep := ""
    for _, elem := range b.elems {
        buf.WriteString(sep)
        sep = ", "
        buf.WriteString(elem.String())
    }
    buf.WriteByte(']')
    return buf.String()
}

func (b *Bag) Type() string {
    return "bag"
}

func (b *Bag) Freeze() {

}

func (b *Bag) Truth() starlark.Bool {
    return b.Len() > 0
}

func (b *Bag) Hash() (uint32, error) {
    return 0, fmt.Errorf("unhashable type: bag")
}

func sortAsString(entries []starlark.Value) []starlark.Value {
    c := make([]starlark.Value, len(entries))
    copy(c, entries)
    sort.SliceStable(c, func(i, j int) bool {
        return c[i].String() < c[j].String()
    })
    return c
}

func (b *Bag) Insert(val starlark.Value) error {
    pos, _ := slices.BinarySearchFunc(b.elems, val, func(x, y starlark.Value) int {
        return cmp.Compare(x.String(), y.String())
    })
    b.elems = slices.Insert(b.elems, pos, val)
    return nil
}

func (b *Bag) InsertAll(v starlark.Iterable) error {
    iter := v.Iterate()
    defer iter.Done()
    var x starlark.Value
    for iter.Next(&x) {
        if err := b.Insert(x); err != nil {
            return err
        }
    }
    return nil
}

func (b *Bag) Delete(k starlark.Value) (bool, error) {
    index, err := b.Find(k)
    if err != nil || index < 0 {
        return index >= 0, err
    }
    b.elems = append(b.elems[:index], b.elems[index+1:]...)

    return true, nil
}

func (b *Bag) Has(x starlark.Value) (found bool, err error) {
    index, err := b.Find(x)
    return index >= 0, err
}

func (b *Bag) Find(k starlark.Value) (int, error) {
    for i, elem := range b.elems {
        if eq, err := starlark.Equal(elem, k); err != nil {
            return -1, err
        } else if eq {
            return i, nil
        }
    }
    return -1, nil
}

func (b *Bag) Clear() error {
    b.elems = nil
    return nil
}

func bag_add(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var elem starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &elem); err != nil {
        return nil, err
    }

    err := b.Receiver().(*Bag).Insert(elem)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return starlark.None, nil
}

func bag_add_all(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var other starlark.Iterable
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &other); err != nil {
        return nil, err
    }

    err := b.Receiver().(*Bag).InsertAll(other)
    if err != nil {
        return nil, nameErr(b, err)
    }
    return starlark.None, nil
}

func bag_clear(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    if b.Receiver().(*Bag).Len() > 0 {
        if err := b.Receiver().(*Bag).Clear(); err != nil {
            return nil, nameErr(b, err)
        }
    }
    return starlark.None, nil
}

func bag_discard(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var k starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &k); err != nil {
        return nil, err
    }
    if found, err := b.Receiver().(*Bag).Has(k); err != nil {
        return nil, nameErr(b, err)
    } else if !found {
        return starlark.None, nil
    }
    if _, err := b.Receiver().(*Bag).Delete(k); err != nil {
        return nil, nameErr(b, err) // set is frozen
    }
    return starlark.None, nil
}

func bag_pop(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
        return nil, err
    }
    recv := b.Receiver().(*Bag)
    if recv.Len() <= 0 {
        return nil, nameErr(b, "empty set")
    }
    lastIndex := len(recv.elems) - 1
    last := recv.elems[lastIndex]
    recv.elems = recv.elems[:lastIndex]
    return last, nil
}

func bag_remove(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
    var k starlark.Value
    if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 1, &k); err != nil {
        return nil, err
    }
    if found, err := b.Receiver().(*Bag).Delete(k); err != nil {
        return nil, nameErr(b, err) // dict is frozen or key is unhashable
    } else if found {
        return starlark.None, nil
    }
    return nil, nameErr(b, "missing key")
}


// Assert that GenericMap implements the starlark interfaces.
var _ starlark.Comparable = (*Bag)(nil)
var _ starlark.HasAttrs = (*Bag)(nil)
var _ starlark.Iterable = (*Bag)(nil)
var _ starlark.Sequence = (*Bag)(nil)
var _ starlark.Value = (*Bag)(nil)
// There is a limit in the API, this is required to get 'in' keyword to work
var _ starlark.Mapping = (*Bag)(nil)

func NewModelValue(prefix string, i int) ModelValue {
    return ModelValue{
        prefix: prefix,
        id:     i,
    }
}

type ModelValue struct {
    prefix string
    id int
}

func (m ModelValue) GetPrefix() string {
    return m.prefix
}

func (m ModelValue) GetId() int {
    return m.id
}

func (m ModelValue) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
    y := y_.(ModelValue)
    switch op {
    case syntax.EQL:
        return modelValueEqual(m, y), nil
    case syntax.NEQ:
        return !modelValueEqual(m, y), nil
    default:
        return false, fmt.Errorf("%s %s %s not implemented", m.Type(), op, y.Type())
    }
}

func modelValueEqual(s ModelValue, y ModelValue) bool {
    return s.prefix == y.prefix && s.id == y.id
}

func (m ModelValue) String() string {
    return fmt.Sprintf("%s%s%d", SymmetryPrefix, m.prefix, m.id)
}

func (m ModelValue) FullString() string {
    return fmt.Sprintf("%s%s%d", SymmetryPrefix, m.prefix, m.id)
}

func (m ModelValue) ShortString() string {
    return fmt.Sprintf("%s%d", m.prefix, m.id)
}

func (m ModelValue) MarshalJSON() ([]byte, error) {
    return json.Marshal(m.FullString())
}

func (m ModelValue) Type() string {
    return "model_value"
}

func (m ModelValue) Freeze() {}

func (m ModelValue) Truth() starlark.Bool {
    return true
}

func (m ModelValue) Hash() (uint32, error) {
    return starlark.String(m.FullString()).Hash()
}

var _ starlark.Value = ModelValue{}
var _ starlark.Comparable = ModelValue{}


func NewSymmetricValue(prefix string, i int) SymmetricValue {
    return SymmetricValue{ModelValue{prefix: prefix, id: i}}
}

type SymmetricValue struct {
    ModelValue
}

func (s SymmetricValue) CompareSameType(op syntax.Token, y_ starlark.Value, depth int) (bool, error) {
    y := y_.(SymmetricValue)
    switch op {
    case syntax.EQL:
        return modelValueEqual(s.ModelValue, y.ModelValue), nil
    case syntax.NEQ:
        return !modelValueEqual(s.ModelValue, y.ModelValue), nil
    default:
        return false, fmt.Errorf("%s %s %s not implemented", s.Type(), op, y.Type())
    }
}

func (s SymmetricValue) Type() string {
    return "symmetric_value"
}

var _ starlark.Value = SymmetricValue{}
var _ starlark.Comparable = SymmetricValue{}

func CompareStringer[E fmt.Stringer](a, b E) int {
    return strings.Compare(a.String(), b.String())
}

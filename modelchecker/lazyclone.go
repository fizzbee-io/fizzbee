package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
)

// Lazy cloning (--experimental_lazy_clone).
//
// A fork normally deep-copies the whole state: heap, every frame of every
// thread, roles and pending messages. Most of a step touches a few names,
// so in this mode a fork copies only the maps that bind names and shares
// every value with the process it was forked from. The source is frozen
// when it is first forked: nothing executes on it again. A value is then
// either old (reachable from the frozen source, never mutated by anyone)
// or new (created by or for this process).
//
// Before a statement or expression runs, the names it mentions are owned
// (ownNames). Owning an old value deep-clones the whole root it belongs to
// (the top-level binding or role field it was reached from) and every
// other root that reaches it, so two names that shared a value before the
// fork share its clone after, and no shared container is left pointing at
// a value this process has since changed. One clone map per process keeps
// that consistent. New values are left alone.
//
// Roles are entities: containers may hold any older copy of a role, and
// every consumer resolves a role to this process's current copy by its
// name and reference (see roleResolver, currentRole). A fork copies each
// reachable role shallowly (its field values stay shared, each field a
// root of its own) and a field is owned when code names it (self.field).
//
// Which values are old, which roots reach each of them, which hold roles
// and which roles are reachable is computed once per source by reachLazy
// and shared by all its forks; the reachable roles are what an eager fork
// keeps.

// roleKey identifies a role across its copies.
type roleKey struct {
	name string
	ref  int64
}

func keyOfRole(r *lib.Role) roleKey {
	return roleKey{name: r.Name, ref: r.GetRef()}
}

// fieldRoot is the root for values held by a role field: the source
// role, the struct (fields or params), the field name and its old value.
type fieldRoot struct {
	role   *lib.Role
	params bool
	name   string
	value  starlark.Value
}

func (f *fieldRoot) String() string        { return "field_root" }
func (f *fieldRoot) Type() string          { return "field_root" }
func (f *fieldRoot) Freeze()               {}
func (f *fieldRoot) Truth() starlark.Bool  { return starlark.False }
func (f *fieldRoot) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: field_root") }

// lazyReach is the per-source result of the reachability pass.
type lazyReach struct {
	// rootsOf maps every old pointer value to the roots it is reachable
	// from. A root is a top-level binding's value (heap, frame and scope
	// variables, loop ranges, message parameters) or a *fieldRoot.
	rootsOf map[starlark.Value][]starlark.Value
	// hasRole marks old pointer values that hold a role somewhere inside.
	hasRole map[starlark.Value]bool
	// roleFields are the role fields whose value is or holds a role.
	roleFields []*fieldRoot
	// roles are the roles reachable from the state, in p.Roles order.
	roles []*lib.Role
}

// reachLazy computes (once) what forks of this process share.
func (p *Process) reachLazy() *lazyReach {
	if p.lazyReach != nil {
		return p.lazyReach
	}
	r := &lazyReach{
		rootsOf: make(map[starlark.Value][]starlark.Value, 64),
		hasRole: make(map[starlark.Value]bool),
	}
	roleRefs := map[roleKey]bool{}
	// visit returns whether v holds a role.
	var visit func(v starlark.Value, root starlark.Value) bool
	visit = func(v starlark.Value, root starlark.Value) bool {
		switch x := v.(type) {
		case starlark.Tuple:
			has := false
			for _, e := range x {
				if visit(e, root) {
					has = true
				}
			}
			return has
		case *lib.Role:
			roleRefs[keyOfRole(x)] = true
			return true
		case *lib.RoleStub:
			roleRefs[keyOfRole(x.Role)] = true
			return true
		}
		if !isPointerType(v) {
			return false
		}
		roots, seen := r.rootsOf[v]
		if seen {
			if !slices.Contains(roots, root) {
				r.rootsOf[v] = append(roots, root)
			}
			return r.hasRole[v]
		}
		r.rootsOf[v] = []starlark.Value{root}
		has := false
		forEachChild(v, func(c starlark.Value) {
			if visit(c, root) {
				has = true
			}
		})
		if has {
			r.hasRole[v] = true
		}
		return has
	}
	var asRoot func(v starlark.Value)
	asRoot = func(v starlark.Value) {
		if t, ok := v.(starlark.Tuple); ok {
			for _, e := range t {
				asRoot(e)
			}
			return
		}
		visit(v, v)
	}
	rootDict := func(d starlark.StringDict) {
		for _, v := range d {
			asRoot(v)
		}
	}
	rootFrame := func(f *CallFrame) {
		rootDict(f.vars)
		if f.obj != nil {
			roleRefs[keyOfRole(f.obj)] = true
		}
		seen := map[*Scope]bool{}
		for s := f.scope; s != nil; s = s.parent {
			if !seen[s] {
				seen[s] = true
				rootDict(s.vars)
				for _, v := range s.loopRange {
					asRoot(v)
				}
			}
		}
	}
	rootDict(p.Heap.state)
	for _, thread := range p.Threads {
		if thread == nil {
			continue
		}
		for _, frame := range thread.Stack.RawArray() {
			rootFrame(frame)
		}
	}
	for _, msgs := range p.ChannelMessages {
		for _, msg := range msgs {
			if msg == nil {
				continue
			}
			rootDict(msg.params)
			if msg.frame != nil {
				rootFrame(msg.frame)
			}
		}
	}
	// Roles are reached by identity; each field is a root of its own, and
	// only the roles that are reachable at all count (a role reachable
	// only through another role's fields counts too, so iterate until no
	// new role turns up).
	kept := map[*lib.Role]bool{}
	fieldRoots := func(role *lib.Role, s *lib.Struct, params bool) {
		if s == nil {
			return
		}
		s.EachField(func(name string, fv starlark.Value) {
			if !needsOwn(fv) {
				return
			}
			fr := &fieldRoot{role: role, params: params, name: name, value: fv}
			if visit(fv, fr) {
				r.roleFields = append(r.roleFields, fr)
			}
		})
	}
	for {
		added := false
		for _, role := range p.Roles {
			if role == nil || kept[role] || !roleRefs[keyOfRole(role)] {
				continue
			}
			kept[role] = true
			added = true
			fieldRoots(role, role.Fields, false)
			fieldRoots(role, role.Params, true)
		}
		if !added {
			break
		}
	}
	r.roles = make([]*lib.Role, 0, len(kept))
	for _, role := range p.Roles {
		if kept[role] {
			r.roles = append(r.roles, role)
		}
	}
	p.lazyReach = r
	return r
}

// forEachChild calls fn on every value directly held by a container.
func forEachChild(v starlark.Value, fn func(starlark.Value)) {
	switch x := v.(type) {
	case *starlark.List:
		for i := 0; i < x.Len(); i++ {
			fn(x.Index(i))
		}
	case starlark.Tuple:
		for _, e := range x {
			fn(e)
		}
	case *starlark.Dict:
		for _, item := range x.Items() {
			fn(item[0])
			fn(item[1])
		}
	case *lib.Struct:
		x.EachField(func(_ string, fv starlark.Value) { fn(fv) })
	case *lib.GenericMap:
		for _, item := range x.Items() {
			fn(item[0])
			fn(item[1])
		}
	case starlark.Iterable: // set, bag, genericset
		iter := x.Iterate()
		var e starlark.Value
		for iter.Next(&e) {
			fn(e)
		}
		iter.Done()
	}
}

// dropLazyCache releases the reachability result once every fork of this
// process has been made; forks hold their own reference to it.
func (p *Process) dropLazyCache() {
	p.lazyReach = nil
}

// forkLazy is Fork for a process in lazy mode.
func (p *Process) forkLazy() *Process {
	reach := p.reachLazy()
	p.frozen = true
	p.cloneRefs = nil

	p2 := &Process{
		Name:      p.Name,
		Heap:      &Heap{state: shallowCopyState(p.Heap.state), globals: p.Heap.globals},
		Current:   p.Current,
		Parent:    p,
		Evaluator: p.Evaluator,

		durabilitySpec: p.durabilitySpec,
		guidedTrace:    p.guidedTrace,

		Children:    []*Process{},
		Files:       p.Files,
		Returns:     make(starlark.StringDict),
		SymbolTable: p.SymbolTable,
		Modules:     p.Modules,
		Labels:      make([]string, 0),
		Messages:    make([]*ast.Message, 0),
		Stats:       p.Stats.Clone(),

		assertionBindings: p.assertionBindings,

		lazyClone: true,
		reach:     reach,
		cloneRefs: make(map[starlark.Value]starlark.Value),
		roleByRef: make(map[roleKey]*lib.Role, len(reach.roles)),
	}
	p2.cloneRefs[roleResolverKey] = &roleResolver{p: p2}

	if p.Stats.TotalActions <= 1 {
		p2.topLevelVars = slices.Clone(p.topLevelVars)
	}

	p2.Witness = make([][]bool, len(p.Files))
	for i, file := range p.Files {
		p2.Witness[i] = make([]bool, len(file.Invariants))
	}

	clonedThreads := make([]*Thread, len(p.Threads))
	for i, thread := range p.Threads {
		if thread == nil {
			continue
		}
		clonedThreads[i] = thread.shallowClone()
		clonedThreads[i].Process = p2
	}
	p2.Threads = clonedThreads

	p2.Roles = make([]*lib.Role, len(reach.roles))
	for i, r := range reach.roles {
		nr := &lib.Role{
			ID:          r.ID,
			Name:        r.Name,
			Symmetric:   r.Symmetric,
			Params:      r.Params.ShallowCopy(),
			Fields:      r.Fields.ShallowCopy(),
			Methods:     r.Methods,
			RoleMethods: r.RoleMethods,
			InitValues:  r.InitValues,
		}
		p2.Roles[i] = nr
		p2.roleByRef[keyOfRole(nr)] = nr
	}
	p2.Channels = make(map[int]*lib.Channel)
	p2.ChannelMessages = make(map[int][]*ChannelMessage, len(p.ChannelMessages))
	for i, msgs := range p.ChannelMessages {
		copied := make([]*ChannelMessage, len(msgs))
		for j, msg := range msgs {
			copied[j] = msg.shallowClone()
		}
		p2.ChannelMessages[i] = copied
	}
	if p.RotationalLastAllocated != nil {
		p2.RotationalLastAllocated = maps.Clone(p.RotationalLastAllocated)
	}
	p2.resolveRoles()
	return p2
}

// resolveRoles points every binding that is or holds a role at this
// process's role copies, as an eager fork would: a role read through an
// older copy would show the fields as they were at the fork. Containers
// holding roles are owned (cloned) for that; everything else stays
// shared. After this, no reachable value refers to another process's
// copy of a role, so reads that bypass ownNames (hashing, liveness
// formulas) are safe.
func (p *Process) resolveRoles() {
	fix := func(d starlark.StringDict) {
		for k, v := range d {
			if !p.holdsRole(v) {
				continue
			}
			if nv, ch := p.ownValue(v); ch {
				d[k] = nv
			}
		}
	}
	seenScopes := map[*Scope]bool{}
	frame := func(f *CallFrame) {
		f.obj = p.currentRole(f.obj)
		fix(f.vars)
		for s := f.scope; s != nil; s = s.parent {
			if seenScopes[s] {
				continue
			}
			seenScopes[s] = true
			fix(s.vars)
			for i, v := range s.loopRange {
				if !p.holdsRole(v) {
					continue
				}
				if nv, ch := p.ownValue(v); ch {
					s.loopRange[i] = nv
				}
			}
		}
	}
	fix(p.Heap.state)
	for _, thread := range p.Threads {
		if thread == nil {
			continue
		}
		for _, f := range thread.Stack.RawArray() {
			frame(f)
		}
	}
	for _, msgs := range p.ChannelMessages {
		for _, msg := range msgs {
			if msg == nil {
				continue
			}
			fix(msg.params)
			if msg.frame != nil {
				frame(msg.frame)
			}
		}
	}
	for _, fr := range p.reach.roleFields {
		switch fr.value.(type) {
		case *lib.Role, *lib.RoleStub:
			cur := p.currentRole(fr.role)
			s := cur.Fields
			if fr.params {
				s = cur.Params
			}
			if nv, ch := p.ownValue(fr.value); ch && s != nil {
				if v, err := s.Attr(fr.name); err == nil && sameValue(v, fr.value) {
					s.ReplaceField(fr.name, nv)
				}
			}
		default:
			p.ownFieldRoot(fr)
		}
	}
}

// shallowCopyState copies the name bindings of a state dict, sharing the
// values. Builtins and modules are left out, as CopyDict leaves them out.
func shallowCopyState(from starlark.StringDict) starlark.StringDict {
	to := make(starlark.StringDict, len(from))
	for k, v := range from {
		switch v.Type() {
		case "builtin_function_or_method", "module", "failed_module":
			continue
		}
		to[k] = v
	}
	return to
}

func (t *Thread) shallowClone() *Thread {
	stack := NewCallStack()
	for _, frame := range t.Stack.RawArray() {
		stack.Push(frame.shallowClone())
	}
	return &Thread{Id: t.Id, Process: t.Process, Files: t.Files, Stack: stack, Fairness: t.Fairness}
}

func (c *CallFrame) shallowClone() *CallFrame {
	return &CallFrame{
		FileIndex:            c.FileIndex,
		pc:                   c.pc,
		Name:                 c.Name,
		scope:                c.scope.shallowClone(),
		vars:                 shallowCopyState(c.vars),
		callerAssignVarNames: c.callerAssignVarNames,
		obj:                  c.obj,
	}
}

// shallowClone copies the scope chain and its name bindings; values are
// shared. Child scopes share their parent's vars map, as in Clone.
func (s *Scope) shallowClone() *Scope {
	if s == nil {
		return nil
	}
	newScope := &Scope{
		parent:    s.parent.shallowClone(),
		flow:      s.flow,
		skipstmts: slices.Clone(s.skipstmts),
		loopVars:  slices.Clone(s.loopVars),
		loopRange: slices.Clone(s.loopRange),
	}
	if s.parent == nil {
		newScope.vars = shallowCopyState(s.vars)
	} else {
		newScope.vars = newScope.parent.vars
	}
	return newScope
}

func (cm *ChannelMessage) shallowClone() *ChannelMessage {
	var frame *CallFrame
	if cm.frame != nil {
		frame = cm.frame.shallowClone()
	}
	return &ChannelMessage{
		receiver: cm.receiver,
		frame:    frame,
		function: cm.function,
		params:   shallowCopyState(cm.params),
	}
}

// roleResolver, stored in a clone's refs map under roleResolverKey, maps a
// role of any age to the process's current copy. In the process's own
// clone map the copy is handed out as is; in a refs map of an eager clone
// (CloneWithRefs) the current copy is what gets cloned.
type roleResolver struct {
	p     *Process
	eager bool
}

func (res *roleResolver) String() string        { return "role_resolver" }
func (res *roleResolver) Type() string          { return "role_resolver" }
func (res *roleResolver) Freeze()               {}
func (res *roleResolver) Truth() starlark.Bool  { return starlark.False }
func (res *roleResolver) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: role_resolver") }

type sentinelKey struct{ name string }

func (k *sentinelKey) String() string        { return k.name }
func (k *sentinelKey) Type() string          { return "sentinel" }
func (k *sentinelKey) Freeze()               {}
func (k *sentinelKey) Truth() starlark.Bool  { return starlark.False }
func (k *sentinelKey) Hash() (uint32, error) { return 0, fmt.Errorf("unhashable: %s", k.name) }

var roleResolverKey starlark.Value = &sentinelKey{name: "role_resolver"}

// resolve returns the role the clone should use for r and whether it must
// still be cloned.
func (res *roleResolver) resolve(r *lib.Role) (*lib.Role, bool) {
	return res.p.currentRole(r), res.eager
}

// currentRole returns this process's copy of a role of any age.
func (p *Process) currentRole(r *lib.Role) *lib.Role {
	if r == nil || p.roleByRef == nil {
		return r
	}
	if cur, ok := p.roleByRef[keyOfRole(r)]; ok {
		return cur
	}
	return r
}

// needsOwn reports whether a value can be mutated in place, so that a
// shared one must be owned before code that names it runs.
func needsOwn(v starlark.Value) bool {
	switch v.Type() {
	case "list", "set", "dict", "bag", "genericset", "genericmap", "record", "role", "RoleStub", "tuple":
		return true
	}
	return false
}

func (p *Process) isOld(v starlark.Value) bool {
	_, ok := p.reach.rootsOf[v]
	return ok
}

// ownValue returns a value this process may mutate: v itself when it is
// new, otherwise this process's clone of it. changed reports whether the
// result differs from v. A role is resolved to the current copy; its
// fields are owned separately (ownField).
func (p *Process) ownValue(v starlark.Value) (starlark.Value, bool) {
	if p.reach == nil || v == nil {
		return v, false
	}
	switch x := v.(type) {
	case starlark.Tuple:
		if !p.tupleIsOld(x) {
			return v, false
		}
		p.ownTupleRoots(x)
		cloned, err := deepCloneStarlarkValue(v, p.cloneRefs)
		PanicOnError(err)
		return cloned, true
	case *lib.Role:
		cur := p.currentRole(x)
		return cur, cur != x
	case *lib.RoleStub:
		cur := p.currentRole(x.Role)
		if cur == x.Role {
			return v, false
		}
		return lib.NewRoleStub(cur, x.Channel), true
	}
	if !isPointerType(v) || !p.isOld(v) {
		return v, false
	}
	return p.ownObject(v), true
}

func (p *Process) tupleIsOld(t starlark.Tuple) bool {
	for _, e := range t {
		if inner, ok := e.(starlark.Tuple); ok {
			if p.tupleIsOld(inner) {
				return true
			}
			continue
		}
		if isPointerType(e) {
			return p.isOld(e)
		}
	}
	return false
}

func (p *Process) tupleHasRole(t starlark.Tuple) bool {
	for _, e := range t {
		switch x := e.(type) {
		case starlark.Tuple:
			if p.tupleHasRole(x) {
				return true
			}
		case *lib.Role, *lib.RoleStub:
			return true
		default:
			if isPointerType(e) && p.reach.hasRole[e] {
				return true
			}
		}
	}
	return false
}

func (p *Process) ownTupleRoots(t starlark.Tuple) {
	for _, e := range t {
		switch x := e.(type) {
		case starlark.Tuple:
			p.ownTupleRoots(x)
		case *lib.Role, *lib.RoleStub:
		default:
			if isPointerType(e) && p.isOld(e) {
				p.ownObject(e)
			}
		}
	}
}

// ownObject returns the clone of an old pointer value, cloning every root
// that reaches it.
func (p *Process) ownObject(v starlark.Value) starlark.Value {
	if c, ok := p.cloneRefs[v]; ok {
		return c
	}
	for _, root := range p.reach.rootsOf[v] {
		p.ownRoot(root)
	}
	if c, ok := p.cloneRefs[v]; ok {
		return c
	}
	panic("lazy clone: value not reached from any root")
}

// ownRoot deep-clones a root and rebinds every name or field that held it.
func (p *Process) ownRoot(root starlark.Value) {
	if fr, ok := root.(*fieldRoot); ok {
		p.ownFieldRoot(fr)
		return
	}
	if _, ok := p.cloneRefs[root]; ok {
		return
	}
	cloned, err := deepCloneStarlarkValue(root, p.cloneRefs)
	PanicOnError(err)
	p.rebindAll(root, cloned)
	p.ownCoRoots(root)
}

// ownFieldRoot owns the value of a role field and points the current
// copy's field at the clone, unless the field has since been reassigned.
func (p *Process) ownFieldRoot(fr *fieldRoot) {
	var cloned starlark.Value
	had := false
	if t, ok := fr.value.(starlark.Tuple); ok {
		// Cloning the tuple clones its old elements in place; their other
		// roots are owned by ownCoRoots below.
		had = !p.tupleNeedsClone(t)
	} else {
		cloned, had = p.cloneRefs[fr.value]
	}
	if cloned == nil {
		var err error
		cloned, err = deepCloneStarlarkValue(fr.value, p.cloneRefs)
		PanicOnError(err)
	}
	cur := p.currentRole(fr.role)
	s := cur.Fields
	if fr.params {
		s = cur.Params
	}
	if s != nil {
		if v, err := s.Attr(fr.name); err == nil && sameValue(v, fr.value) {
			s.ReplaceField(fr.name, cloned)
		}
	}
	if !had {
		p.rebindAll(fr.value, cloned)
		p.ownCoRoots(fr.value)
	}
}

// tupleNeedsClone reports whether some old pointer value inside the tuple
// has not been cloned yet.
func (p *Process) tupleNeedsClone(t starlark.Tuple) bool {
	for _, e := range t {
		switch x := e.(type) {
		case starlark.Tuple:
			if p.tupleNeedsClone(x) {
				return true
			}
		case *lib.Role, *lib.RoleStub:
		default:
			if isPointerType(e) && p.isOld(e) {
				if _, ok := p.cloneRefs[e]; !ok {
					return true
				}
			}
		}
	}
	return false
}

// sameValue reports whether two values are the same object; tuples are
// compared by their backing array, since slices are not comparable.
func sameValue(a, b starlark.Value) bool {
	ta, aok := a.(starlark.Tuple)
	tb, bok := b.(starlark.Tuple)
	if aok || bok {
		if !aok || !bok || len(ta) != len(tb) {
			return false
		}
		if len(ta) == 0 {
			return true
		}
		return reflect.ValueOf(ta).Pointer() == reflect.ValueOf(tb).Pointer()
	}
	return a == b
}

// ownCoRoots owns every other root that reaches a value under root.
func (p *Process) ownCoRoots(root starlark.Value) {
	visited := map[starlark.Value]bool{}
	var walk func(v starlark.Value)
	walk = func(v starlark.Value) {
		switch v.(type) {
		case *lib.Role, *lib.RoleStub:
			return
		}
		if t, ok := v.(starlark.Tuple); ok {
			for _, e := range t {
				walk(e)
			}
			return
		}
		if !isPointerType(v) || visited[v] {
			return
		}
		visited[v] = true
		for _, other := range p.reach.rootsOf[v] {
			if other != root {
				p.ownRoot(other)
			}
		}
		forEachChild(v, walk)
	}
	walk(root)
}

// rebindAll replaces old by nv in every name binding of this process,
// cached frame environments included. Role fields are rebound through
// their own roots (ownFieldRoot).
func (p *Process) rebindAll(old, nv starlark.Value) {
	fix := func(d starlark.StringDict) {
		for k, v := range d {
			if r, ok := p.remap(v, old, nv); ok {
				d[k] = r
			}
		}
	}
	fix(p.Heap.state)
	for _, thread := range p.Threads {
		if thread == nil {
			continue
		}
		for _, frame := range thread.Stack.RawArray() {
			p.rebindFrame(frame, old, nv, fix)
		}
	}
	for _, msgs := range p.ChannelMessages {
		for _, msg := range msgs {
			if msg == nil {
				continue
			}
			fix(msg.params)
			if msg.frame != nil {
				p.rebindFrame(msg.frame, old, nv, fix)
			}
		}
	}
}

func (p *Process) rebindFrame(frame *CallFrame, old, nv starlark.Value, fix func(starlark.StringDict)) {
	fix(frame.vars)
	if frame.env != nil {
		fix(frame.env)
	}
	seen := map[*Scope]bool{}
	for s := frame.scope; s != nil; s = s.parent {
		if seen[s] {
			continue
		}
		seen[s] = true
		fix(s.vars)
		for i, v := range s.loopRange {
			if r, ok := p.remap(v, old, nv); ok {
				s.loopRange[i] = r
			}
		}
	}
}

// remap returns nv when v is old, or a rebuilt tuple when v is a tuple
// that holds old somewhere.
func (p *Process) remap(v, old, nv starlark.Value) (starlark.Value, bool) {
	if sameValue(v, old) {
		return nv, true
	}
	t, ok := v.(starlark.Tuple)
	if !ok {
		return v, false
	}
	changed := false
	out := make(starlark.Tuple, len(t))
	for i, e := range t {
		r, ch := p.remap(e, old, nv)
		out[i] = r
		changed = changed || ch
	}
	if !changed {
		return v, false
	}
	return out, true
}

// ownField owns the value of one field of a role of this process.
func (p *Process) ownField(role *lib.Role, name string) {
	for _, s := range []*lib.Struct{role.Fields, role.Params} {
		if s == nil {
			continue
		}
		v, err := s.Attr(name)
		if err != nil || !needsOwn(v) {
			continue
		}
		if nv, changed := p.ownValue(v); changed {
			s.ReplaceField(name, nv)
		}
	}
}

// ownNames prepares the environment for code that references refs and
// may mutate a value: every container the code names is owned, and so is
// every field it names on a role. A call to a Starlark function may
// mutate anything it closes over, so it owns everything in the
// environment. Code that cannot mutate needs nothing: roles are already
// resolved to this process's copies (resolveRoles).
func (p *Process) ownNames(refs *lib.NameRefs, env starlark.StringDict) {
	if p.reach == nil || !refs.MayMutate {
		return
	}
	if p.frozen {
		panic("lazy clone: a frozen process is executing")
	}
	for _, name := range refs.CallIdents {
		if name == "" {
			p.ownEverything(env)
			return
		}
		if _, ok := env[name].(*starlark.Function); ok {
			p.ownEverything(env)
			return
		}
	}
	frame := p.currentThread().currentFrame()
	for _, name := range refs.Names {
		v, ok := env[name]
		if !ok || !needsOwn(v) {
			continue
		}
		nv, ch := p.ownValue(v)
		if !ch {
			continue
		}
		env[name] = nv
		p.rebindOwned(name, nv, frame)
	}
	for name, attrs := range refs.Attrs {
		role, ok := env[name].(*lib.Role)
		if !ok {
			continue
		}
		for _, attr := range attrs {
			p.ownField(role, attr)
		}
	}
	for _, attr := range refs.LooseAttrs {
		for _, role := range p.Roles {
			if role != nil {
				p.ownField(role, attr)
			}
		}
	}
	// Names rebound in other frames were patched in their cached
	// environments by rebindAll; a role bound under another name there is
	// resolved when that frame next references it.
}

// holdsRole reports whether a value is, or holds, a role.
func (p *Process) holdsRole(v starlark.Value) bool {
	switch x := v.(type) {
	case *lib.Role, *lib.RoleStub:
		return true
	case starlark.Tuple:
		return p.tupleHasRole(x)
	}
	return p.reach.hasRole[v]
}

// ownEverything owns every value in the environment and every field of
// every role, the equivalent of an eager clone, for code whose effects
// cannot be told from its text.
func (p *Process) ownEverything(env starlark.StringDict) {
	frame := p.currentThread().currentFrame()
	for name, v := range env {
		if !needsOwn(v) {
			continue
		}
		if nv, ch := p.ownValue(v); ch {
			env[name] = nv
			p.rebindOwned(name, nv, frame)
		}
	}
	for _, role := range p.Roles {
		if role == nil {
			continue
		}
		for _, s := range []*lib.Struct{role.Fields, role.Params} {
			if s == nil {
				continue
			}
			var names []string
			s.EachField(func(n string, fv starlark.Value) {
				if needsOwn(fv) {
					names = append(names, n)
				}
			})
			for _, n := range names {
				p.ownField(role, n)
			}
		}
	}
}

// rebindOwned stores nv under name where the name is bound, following the
// environment's precedence: scope, frame arguments, self, state.
func (p *Process) rebindOwned(name string, nv starlark.Value, frame *CallFrame) {
	for s := frame.scope; s != nil; s = s.parent {
		if _, ok := s.vars[name]; ok {
			s.vars[name] = nv
			return
		}
	}
	if _, ok := frame.vars[name]; ok {
		frame.vars[name] = nv
		return
	}
	if name == "self" && frame.obj != nil {
		if r, ok := nv.(*lib.Role); ok {
			frame.obj = r
		}
		return
	}
	p.Heap.update(name, nv)
}

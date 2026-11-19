package modelchecker

// Block labels and the `threads` builtin module.
//
// A block can be named with `txn:` (or `serial txn:`, `cs, txn:`). The
// threads module answers questions about where threads are paused:
//
//	threads.inside("Server.Pass.txn", role=None)  some thread is inside the block
//	threads.inside("Server.Pass", role=None)      some thread is in that action or func
//	threads.count(path, role=None)                how many threads match
//
// The answers are computed from each frame's program counter, which is
// already part of the state, so labels add no state of their own.
//
// A frame is inside a labeled block when its pc is strictly within the
// block's path: the block has been entered and its end has not been reached.
// A thread paused just before the labeled statement has a pc equal to the
// statement's path, not the block's, so it is outside; end-of-block pcs are
// drained before a thread yields, so a thread paused after the block's last
// statement is outside too.
//
// A path without a label names the whole body of the action or function,
// so inside("Server.Pass") means a Pass is in progress.

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// labelScope is one action or function, top-level or in a role.
type labelScope struct {
	fileIndex int
	role      string            // "" for top-level
	path      string            // e.g. "Roles[0].Actions[1]"
	labels    map[string]string // label -> block path
}

type labelIndex struct {
	roles  map[string]bool
	scopes map[string]*labelScope // "Role.Scope" or "Scope"
	err    error
}

var labelIndexCache sync.Map // *ast.File (files[0]) -> *labelIndex

func getLabelIndex(files []*ast.File) *labelIndex {
	if len(files) == 0 {
		return &labelIndex{roles: map[string]bool{}, scopes: map[string]*labelScope{}}
	}
	if v, ok := labelIndexCache.Load(files[0]); ok {
		return v.(*labelIndex)
	}
	idx := buildLabelIndex(files)
	v, _ := labelIndexCache.LoadOrStore(files[0], idx)
	return v.(*labelIndex)
}

func buildLabelIndex(files []*ast.File) *labelIndex {
	idx := &labelIndex{roles: map[string]bool{}, scopes: map[string]*labelScope{}}
	add := func(fi int, role, name, path string, block *ast.Block) {
		key := name
		if role != "" {
			key = role + "." + name
		}
		s := &labelScope{fileIndex: fi, role: role, path: path, labels: map[string]string{}}
		if old, ok := idx.scopes[key]; ok && idx.err == nil && old.path != path {
			idx.err = fmt.Errorf("threads: %q names more than one action or function", key)
		}
		idx.scopes[key] = s
		if block == nil {
			return
		}
		collectBlockLabels(block.ProtoReflect(), path+".Block", func(label, blockPath string) {
			if _, dup := s.labels[label]; dup && idx.err == nil {
				idx.err = fmt.Errorf("threads: label %q appears more than once in %s", label, key)
			}
			s.labels[label] = blockPath
		})
	}
	for _, file := range files {
		for _, role := range file.Roles {
			idx.roles[role.Name] = true
		}
	}
	for fi, file := range files {
		for i, a := range file.Actions {
			if idx.roles[a.Name] && idx.err == nil {
				idx.err = fmt.Errorf("threads: %q is both a role and a top-level action", a.Name)
			}
			add(fi, "", a.Name, fmt.Sprintf("Actions[%d]", i), a.Block)
		}
		for i, f := range file.Functions {
			if idx.roles[f.Name] && idx.err == nil {
				idx.err = fmt.Errorf("threads: %q is both a role and a top-level function", f.Name)
			}
			add(fi, "", f.Name, fmt.Sprintf("Functions[%d]", i), f.Block)
		}
		for r, role := range file.Roles {
			for i, a := range role.Actions {
				add(fi, role.Name, a.Name, fmt.Sprintf("Roles[%d].Actions[%d]", r, i), a.Block)
			}
			for i, f := range role.Functions {
				add(fi, role.Name, f.Name, fmt.Sprintf("Roles[%d].Functions[%d]", r, i), f.Block)
			}
		}
	}
	return idx
}

// collectBlockLabels walks msg (any AST message) and reports every Block with
// labels, with its path in the GetProtoFieldByPath format.
func collectBlockLabels(msg protoreflect.Message, path string, visit func(label, blockPath string)) {
	if b, ok := msg.Interface().(*ast.Block); ok {
		for _, l := range b.GetLabels() {
			visit(l, path)
		}
	}
	msg.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Kind() != protoreflect.MessageKind {
			return true
		}
		name := goFieldName(string(fd.Name()))
		if fd.IsList() {
			list := v.List()
			for i := 0; i < list.Len(); i++ {
				collectBlockLabels(list.Get(i).Message(), fmt.Sprintf("%s.%s[%d]", path, name, i), visit)
			}
		} else if !fd.IsMap() {
			collectBlockLabels(v.Message(), path+"."+name, visit)
		}
		return true
	})
}

// goFieldName converts a proto field name (if_stmt) to its Go name (IfStmt).
func goFieldName(s string) string {
	var b strings.Builder
	upper := true
	for _, c := range s {
		if c == '_' {
			upper = true
			continue
		}
		if upper && c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		upper = false
		b.WriteRune(c)
	}
	return b.String()
}

// labelTarget is a resolved query path.
type labelTarget struct {
	scope  *labelScope
	prefix string // a frame matches when its pc has this prefix
	exact  string // ... or equals this (the def itself, before its block)
	block  string // the block whose end means the frame is no longer inside
}

func (idx *labelIndex) resolve(path string) (*labelTarget, error) {
	if idx.err != nil {
		return nil, idx.err
	}
	parts := strings.Split(path, ".")
	n := 1
	if idx.roles[parts[0]] {
		n = 2
	}
	if len(parts) < n || len(parts) > n+1 {
		return nil, fmt.Errorf("threads: bad path %q: want [Role.]ActionOrFunc[.label]", path)
	}
	key := strings.Join(parts[:n], ".")
	scope, ok := idx.scopes[key]
	if !ok {
		return nil, fmt.Errorf("threads: no action or function %q", key)
	}
	if len(parts) == n {
		return &labelTarget{scope: scope, prefix: scope.path + ".", exact: scope.path, block: scope.path + ".Block"}, nil
	}
	label := parts[n]
	block, ok := scope.labels[label]
	if !ok {
		known := make([]string, 0, len(scope.labels))
		for l := range scope.labels {
			known = append(known, l)
		}
		sort.Strings(known)
		return nil, fmt.Errorf("threads: no label %q in %s (labels: %s)", label, key, strings.Join(known, ", "))
	}
	return &labelTarget{scope: scope, prefix: block + ".", block: block}, nil
}

func (lt *labelTarget) frameMatches(files []*ast.File, f *CallFrame, role *lib.Role) bool {
	if f == nil || f.FileIndex != lt.scope.fileIndex {
		return false
	}
	if lt.scope.role != "" {
		if f.obj == nil || f.obj.Name != lt.scope.role {
			return false
		}
		if role != nil && !sameRole(f.obj, role) {
			return false
		}
	}
	if lt.exact != "" && f.pc == lt.exact {
		return true
	}
	if !strings.HasPrefix(f.pc, lt.prefix) {
		return false
	}
	return !pastEnd(files[f.FileIndex], f.pc, lt.block)
}

// effectiveFlow is the flow a block runs with: its own, or for a block with
// no flow word (FLOW_UNKNOWN), the flow of the nearest enclosing block that
// has one, as Scope.SetFlow does at run time.
func effectiveFlow(file *ast.File, path string, b *ast.Block) ast.Flow {
	for b.GetFlow() == ast.Flow_FLOW_UNKNOWN {
		i := strings.LastIndex(path, ".Block.")
		if i < 0 {
			return ast.Flow_FLOW_UNKNOWN
		}
		path = path[:i+len(".Block")]
		parent, ok := GetProtoFieldByPath(file, path).(*ast.Block)
		if !ok || parent == nil {
			return ast.Flow_FLOW_UNKNOWN
		}
		b = parent
	}
	return b.GetFlow()
}

var stmtIndexSuffix = regexp.MustCompile(`^(.*)\.Stmts\[(\d+)\]$`)

// pastEnd reports whether a frame paused at pc has already run the last
// statement of the block at root. A thread can pause with its pc on an
// end-of-block marker (`<block>.$`), for example right after an atomic
// block that was the last statement of its parent. Such a pc is past the
// end of root when draining the markers, which runs no spec code, would
// leave root. A loop body's end is not past the end: the loop may run again.
// A parallel block's statements run in any order, so its end is unknown
// until it is drained; it counts as not past the end.
func pastEnd(file *ast.File, pc, root string) bool {
	for strings.HasSuffix(pc, ".$") {
		block := strings.TrimSuffix(pc, ".$")
		if block == root {
			return true
		}
		if !strings.HasSuffix(block, ".Block") {
			return false
		}
		container := strings.TrimSuffix(block, ".Block")
		// The statement whose end this block's end implies.
		var stmt string
		switch {
		case stmtIndexSuffix.MatchString(container):
			stmt = container
		case strings.HasSuffix(container, ".ForStmt"), strings.HasSuffix(container, ".WhileStmt"):
			return false
		default:
			// IfStmt.Branches[i], AnyStmt and the like end their statement.
			i := strings.LastIndex(container, ".Stmts[")
			if i < 0 {
				return false
			}
			j := strings.Index(container[i+1:], ".")
			if j < 0 {
				return false
			}
			stmt = container[:i+1+j]
		}
		m := stmtIndexSuffix.FindStringSubmatch(stmt)
		if m == nil {
			return false
		}
		parent, _ := GetProtoFieldByPath(file, m[1]).(*ast.Block)
		if parent == nil {
			return false
		}
		k, _ := strconv.Atoi(m[2])
		switch effectiveFlow(file, m[1], parent) {
		case ast.Flow_FLOW_PARALLEL:
			return false
		case ast.Flow_FLOW_ONEOF:
			// One statement runs, then the block ends.
		default:
			if k < len(parent.GetStmts())-1 {
				return false
			}
		}
		pc = m[1] + ".$"
	}
	return false
}

func sameRole(a, b *lib.Role) bool {
	if a == b {
		return true
	}
	if a.Name != b.Name {
		return false
	}
	eq, err := starlark.Equal(a.ID, b.ID)
	return err == nil && eq
}

// threadMatches reports whether some frame of t matches lt.
func (lt *labelTarget) threadMatches(files []*ast.File, t *Thread, role *lib.Role) bool {
	for _, f := range t.Stack.RawArray() {
		if lt.frameMatches(files, f, role) {
			return true
		}
	}
	return false
}

// liveThreads are the spec's threads: assertion bodies run on a thread of
// their own in a cloned process, and that thread is left out.
func liveThreads(p *Process) []*Thread {
	out := make([]*Thread, 0, len(p.Threads))
	for _, t := range p.Threads {
		if t == nil || t.Stack == nil || t.Stack.Len() == 0 {
			continue
		}
		if bottom := t.Stack.RawArray()[0]; strings.HasPrefix(bottom.pc, "Invariants[") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// The threads module. Its functions read the running process from the
// starlark thread (lib.ProcessKey), so one module value serves every process.
//
//	threads.inside(path, role=None)       some thread has a frame in the region
//	threads.count(path, role=None)        how many threads do (an int)
//	threads.cardinality(path, role=None)  the same as a 0/1/MANY counter
//	threads.current()                     a handle for the thread running this code
//	threads.all()                         handles for every thread
var threadsModule = &starlarkstruct.Module{
	Name: "threads",
	Members: starlark.StringDict{
		"inside":      starlark.NewBuiltin("threads.inside", threadsQuery),
		"count":       starlark.NewBuiltin("threads.count", threadsQuery),
		"cardinality": starlark.NewBuiltin("threads.cardinality", threadsQuery),
		"current":     starlark.NewBuiltin("threads.current", threadsCurrent),
		"all":         starlark.NewBuiltin("threads.all", threadsAll),
	},
}

func init() {
	lib.ImportableModules["threads"] = threadsModule
}

func processOf(t *starlark.Thread, name string) (*Process, error) {
	p, ok := t.Local(lib.ProcessKey).(*Process)
	if !ok || p == nil {
		return nil, fmt.Errorf("%s: not available here (no running process)", name)
	}
	return p, nil
}

// unpackPathRole reads (path, role=None) and resolves the path.
func unpackPathRole(p *Process, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (*labelTarget, *lib.Role, error) {
	var path string
	var roleVal starlark.Value = starlark.None
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "role?", &roleVal); err != nil {
		return nil, nil, err
	}
	var role *lib.Role
	if roleVal != starlark.None {
		r, ok := roleVal.(*lib.Role)
		if !ok {
			return nil, nil, fmt.Errorf("%s: role must be a role instance or None, got %s", b.Name(), roleVal.Type())
		}
		role = r
	}
	lt, err := getLabelIndex(p.Files).resolve(path)
	if err != nil {
		return nil, nil, err
	}
	if role != nil && lt.scope.role != role.Name {
		return nil, nil, fmt.Errorf("%s: role %s is not a %s", b.Name(), role.String(), describeScopeRole(lt.scope))
	}
	return lt, role, nil
}

func threadsQuery(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	p, err := processOf(t, b.Name())
	if err != nil {
		return nil, err
	}
	lt, role, err := unpackPathRole(p, b, args, kwargs)
	if err != nil {
		return nil, err
	}
	n, many, card := lt.tally(p, role)
	switch b.Name() {
	case "threads.inside":
		return starlark.Bool(n > 0), nil
	case "threads.cardinality":
		return card, nil
	}
	if many > 0 {
		return nil, fmt.Errorf("threads.count: %d matching thread group(s) stand for MANY capped copies, so the exact count is unknown; use threads.cardinality(path) instead", many)
	}
	return starlark.MakeInt(n), nil
}

// tally counts the threads matching lt (after the role filter): n entries,
// many of them standing for MANY capped copies, and the exact cardinality.
func (lt *labelTarget) tally(p *Process, role *lib.Role) (n, many int, card *lib.CardinalityCounter) {
	card = lib.NewCardinalityCounter(0)
	for _, th := range liveThreads(p) {
		if !lt.threadMatches(p.Files, th, role) {
			continue
		}
		n++
		if th.Copies != nil {
			card = card.Plus(th.Copies)
			if th.Copies.IsMany() {
				many++
			}
		} else {
			card = card.Inc()
		}
	}
	return n, many, card
}

func threadsCurrent(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
		return nil, err
	}
	p, err := processOf(t, b.Name())
	if err != nil {
		return nil, err
	}
	// Assertions run at yielded states, where no thread is running.
	if p.fizzCall != nil {
		return nil, fmt.Errorf("threads.current: there is no current thread in an assertion; use threads.all()")
	}
	if p.Current < 0 || p.Current >= len(p.Threads) {
		return nil, fmt.Errorf("threads.current: no thread is running")
	}
	th := p.currentThread()
	if th == nil || th.Stack == nil || th.Stack.Len() == 0 {
		return nil, fmt.Errorf("threads.current: no thread is running")
	}
	return &threadHandle{p: p, t: th}, nil
}

func threadsAll(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := starlark.UnpackPositionalArgs(b.Name(), args, kwargs, 0); err != nil {
		return nil, err
	}
	p, err := processOf(t, b.Name())
	if err != nil {
		return nil, err
	}
	threads := liveThreads(p)
	out := make([]starlark.Value, len(threads))
	for i, th := range threads {
		out[i] = &threadHandle{p: p, t: th}
	}
	return starlark.NewList(out), nil
}

// threadHandle is a thread seen from spec code. It is valid only while the
// code that obtained it runs: it is not a value the state can hold, so it
// cannot be kept across a yield.
type threadHandle struct {
	p *Process
	t *Thread
}

var _ starlark.HasAttrs = (*threadHandle)(nil)

func (h *threadHandle) String() string {
	return fmt.Sprintf("<thread %s>", h.t.actionName())
}
func (h *threadHandle) Type() string         { return "thread" }
func (h *threadHandle) Freeze()              {}
func (h *threadHandle) Truth() starlark.Bool { return true }
func (h *threadHandle) Hash() (uint32, error) {
	return 0, fmt.Errorf("unhashable: thread handles can't be stored")
}
func (h *threadHandle) AttrNames() []string { return []string{"copies", "inside"} }

func (h *threadHandle) Attr(name string) (starlark.Value, error) {
	switch name {
	case "inside":
		return starlark.NewBuiltin("thread.inside", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
			lt, role, err := unpackPathRole(h.p, b, args, kwargs)
			if err != nil {
				return nil, err
			}
			return starlark.Bool(lt.threadMatches(h.p.Files, h.t, role)), nil
		}), nil
	case "copies":
		// ONE for an ordinary thread; ONE or MANY for a capped group.
		if h.t.Copies != nil {
			return h.t.Copies, nil
		}
		return lib.NewCardinalityCounter(1), nil
	}
	return nil, nil
}

func describeScopeRole(s *labelScope) string {
	if s.role == "" {
		return "role (the path names a top-level action or function)"
	}
	return s.role
}

// checkLabels reports label errors found when the spec is loaded:
// duplicate labels in one scope, and a role sharing a name with a
// top-level action or function.
func checkLabels(files []*ast.File) error {
	return getLabelIndex(files).err
}

// labelsInFiles lists every label, for warnings and tests.
func labelsInFiles(files []*ast.File) []string {
	idx := getLabelIndex(files)
	var out []string
	for key, s := range idx.scopes {
		for l := range s.labels {
			out = append(out, key+"."+l)
		}
	}
	slices.Sort(out)
	return out
}

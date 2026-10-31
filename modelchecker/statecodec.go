package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
)

// Process-level state serialization. EncodeProcess writes everything a
// Fork copies from one process to the next (the heap, roles, threads with
// their call stacks and scopes, channel messages, stats, ...) as one
// lib.StateWriter stream, so values shared between them, such as a role
// held in the heap, in Process.Roles and as a frame's receiver, keep one
// identity after decoding. What every process of a run shares (the files,
// the evaluator, globals, modules, the symbol table) is not written; the
// decoder takes it from a template process of the same run.
//
// The stream is the version byte, then a sequence of fields, each a field
// number followed by its payload, ending with field 0. A field with its zero
// value is left out. A reader rejects an unknown field number, so a new
// field can be added with a new number without a version bump: old encodings
// still decode, the new field taking its zero value.
// See docs/state-serialization.md.

const (
	pfEnd = iota
	pfName
	pfCurrent
	pfHeap
	pfThreads
	pfRoles
	pfChannelMessages
	pfChannels
	pfRotationalLastAllocated
	pfStats
	pfReturns
	pfFailedInvariants
	pfWitness
	pfAtoms
	pfLabels
	pfMessages
	pfFlags
	pfFairness
	pfChoiceFairness
	pfTopLevelVars
)

// Bits of pfFlags.
const (
	flagEnabled = 1 << iota
	flagThreadProgress
	flagIsYield
	flagEnableCheckpoint
)

// CodecContext returns the codec context shared by every process of this
// evaluator's run.
func (e *Evaluator) CodecContext() *lib.CodecContext {
	e.codecOnce.Do(func() { e.codecCtx = lib.NewCodecContext() })
	return e.codecCtx
}

// specResolver rebuilds role methods and globals from the spec, so a
// state encoded in one process decodes in another one running the same
// spec.
type specResolver struct {
	files     []*ast.File
	globals   starlark.StringDict
	evaluator *Evaluator
}

func (r *specResolver) RoleMethods(roleName string) (map[string]*starlark.Function, map[string]*starlark.Builtin, error) {
	for _, file := range r.files {
		for _, role := range file.Roles {
			if role.Name != roleName {
				continue
			}
			methods, stmt, err := roleMethodsFromSpec(r.evaluator, file, role, r.globals)
			if err != nil {
				return nil, nil, fmt.Errorf("role %s: %s: %w", roleName, stmt.PyStmt.GetCode(), err)
			}
			return methods, lib.NewRoleMethodBuiltins(role), nil
		}
	}
	return nil, nil, fmt.Errorf("role %s not in the spec", roleName)
}

func (r *specResolver) Global(name string) (starlark.Value, bool) {
	v, ok := r.globals[name]
	return v, ok
}

// codecContext returns the run's codec context, set up for the spec p
// belongs to.
func codecContext(p *Process) *lib.CodecContext {
	ctx := p.Evaluator.CodecContext()
	if ctx.HasRoleResolver() {
		return ctx
	}
	setUpCodecContext(ctx, p)
	return ctx
}

func setUpCodecContext(ctx *lib.CodecContext, p *Process) {
	if ctx.DictionarySize() == 0 {
		ctx.SetDictionary(specDictionary(p.Files))
	}
	ctx.SetRoleResolver(&specResolver{files: p.Files, globals: p.Heap.globals, evaluator: p.Evaluator})
}

const dictionaryRoleInstances = 8

var identifierRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// specDictionary returns the strings a state of this spec is likely to
// hold, built from the spec alone so every process builds the same list:
// the identifiers in the spec (variable, field, role, action and function
// names, and words in string literals), the program counters of its
// statements, and a few fixed names. Shorter strings come first, so the
// most frequent ones get one-byte indexes.
func specDictionary(files []*ast.File) []string {
	words := map[string]bool{}
	for _, w := range []string{"", "yield", "init", "crash", "checkpoint", "record", "params", "fields", "init_values", "enum", "struct"} {
		words[w] = true
	}
	var walk func(v reflect.Value, path string)
	addPath := func(path string) {
		words[path] = true
		words[path+".any"] = true
		words[path+".Block.$"] = true
	}
	walk = func(v reflect.Value, path string) {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return
		}
		if path != "" {
			addPath(path)
		}
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Tag.Get("protobuf") == "" {
				continue
			}
			fv := v.Field(i)
			child := f.Name
			if path != "" {
				child = path + "." + f.Name
			}
			switch {
			case fv.Kind() == reflect.String:
				for _, w := range identifierRe.FindAllString(fv.String(), -1) {
					words[w] = true
				}
			case fv.Kind() == reflect.Slice && fv.Type().Elem().Kind() == reflect.String:
				for j := 0; j < fv.Len(); j++ {
					words[fv.Index(j).String()] = true
				}
			case fv.Kind() == reflect.Slice:
				for j := 0; j < fv.Len(); j++ {
					walk(fv.Index(j), fmt.Sprintf("%s[%d]", child, j))
				}
			case fv.Kind() == reflect.Pointer:
				walk(fv, child)
			}
		}
	}
	for _, f := range files {
		walk(reflect.ValueOf(f), "")
		// Frame names of role actions and functions are qualified by the
		// role, and action names (stats, process names) by the instance
		// too. The instance count isn't in the spec, so the first
		// dictionaryRoleInstances of each role are covered.
		for _, r := range f.Roles {
			var names []string
			for _, a := range r.Actions {
				names = append(names, a.Name)
			}
			for _, fn := range r.Functions {
				names = append(names, fn.Name)
			}
			for _, name := range names {
				words[r.Name+"."+name] = true
				for i := 0; i < dictionaryRoleInstances; i++ {
					words[fmt.Sprintf("%s#%d.%s", r.Name, i, name)] = true
				}
			}
		}
	}
	list := make([]string, 0, len(words))
	for w := range words {
		list = append(list, w)
	}
	sort.Slice(list, func(i, j int) bool {
		if len(list[i]) != len(list[j]) {
			return len(list[i]) < len(list[j])
		}
		return list[i] < list[j]
	})
	return list
}

// EncodeProcess returns the encoding of p. The result is owned by the
// caller.
func EncodeProcess(p *Process) ([]byte, error) {
	w := lib.NewStateWriter(codecContext(p))
	defer w.Release()
	if err := writeProcess(w, p); err != nil {
		return nil, err
	}
	return slices.Clone(w.Bytes()), nil
}

func writeProcess(w *lib.StateWriter, p *Process) error {
	w.WriteUint8(lib.StateCodecVersion)
	if p.Name != "" {
		w.WriteUvarint(pfName)
		w.WriteString(p.Name)
	}
	if p.Current != 0 {
		w.WriteUvarint(pfCurrent)
		w.WriteVarint(int64(p.Current))
	}
	w.WriteUvarint(pfHeap)
	if err := w.WriteStringDict(p.Heap.state); err != nil {
		return fmt.Errorf("heap: %w", err)
	}
	if p.Roles != nil {
		w.WriteUvarint(pfRoles)
		w.WriteUvarint(uint64(len(p.Roles)))
		for _, r := range p.Roles {
			if err := w.WriteValue(r); err != nil {
				return fmt.Errorf("roles: %w", err)
			}
		}
	}
	if len(p.Threads) > 0 {
		w.WriteUvarint(pfThreads)
		w.WriteUvarint(uint64(len(p.Threads)))
		for _, t := range p.Threads {
			if err := writeThread(w, t); err != nil {
				return fmt.Errorf("thread: %w", err)
			}
		}
	}
	if len(p.ChannelMessages) > 0 {
		w.WriteUvarint(pfChannelMessages)
		keys := sortedIntKeys(p.ChannelMessages)
		w.WriteUvarint(uint64(len(keys)))
		for _, k := range keys {
			msgs := p.ChannelMessages[k]
			w.WriteVarint(int64(k))
			w.WriteUvarint(uint64(len(msgs)))
			for _, m := range msgs {
				w.WriteString(m.receiver)
				w.WriteString(m.function)
				if err := writeFrame(w, m.frame); err != nil {
					return fmt.Errorf("channel message: %w", err)
				}
				if err := w.WriteStringDict(m.params); err != nil {
					return fmt.Errorf("channel message: %w", err)
				}
			}
		}
	}
	if len(p.Channels) > 0 {
		w.WriteUvarint(pfChannels)
		keys := sortedIntKeys(p.Channels)
		w.WriteUvarint(uint64(len(keys)))
		for _, k := range keys {
			w.WriteVarint(int64(k))
			if err := w.WriteValue(p.Channels[k]); err != nil {
				return err
			}
		}
	}
	if len(p.RotationalLastAllocated) > 0 {
		w.WriteUvarint(pfRotationalLastAllocated)
		writeStringIntMap(w, p.RotationalLastAllocated)
	}
	if p.Stats != nil {
		w.WriteUvarint(pfStats)
		w.WriteUvarint(uint64(p.Stats.TotalActions))
		counts := make(map[string]int64, len(p.Stats.Counts))
		for k, v := range p.Stats.Counts {
			counts[k] = int64(v)
		}
		writeStringIntMap(w, counts)
	}
	if len(p.Returns) > 0 {
		w.WriteUvarint(pfReturns)
		if err := w.WriteStringDict(p.Returns); err != nil {
			return fmt.Errorf("returns: %w", err)
		}
	}
	if p.FailedInvariants != nil {
		w.WriteUvarint(pfFailedInvariants)
		keys := sortedIntKeys(p.FailedInvariants)
		w.WriteUvarint(uint64(len(keys)))
		for _, k := range keys {
			w.WriteVarint(int64(k))
			writeInts(w, p.FailedInvariants[k])
		}
	}
	if p.Witness != nil {
		w.WriteUvarint(pfWitness)
		w.WriteUvarint(uint64(len(p.Witness)))
		for _, ws := range p.Witness {
			w.WriteUvarint(uint64(len(ws)))
			for _, b := range ws {
				w.WriteBool(b)
			}
		}
	}
	if p.Atoms != nil {
		w.WriteUvarint(pfAtoms)
		w.WriteUvarint(uint64(len(p.Atoms)))
		for _, a := range p.Atoms {
			w.WriteUvarint(a)
		}
	}
	if len(p.Labels) > 0 {
		w.WriteUvarint(pfLabels)
		w.WriteStrings(p.Labels)
	}
	if len(p.Messages) > 0 {
		w.WriteUvarint(pfMessages)
		w.WriteUvarint(uint64(len(p.Messages)))
		for _, m := range p.Messages {
			b, err := proto.Marshal(m)
			if err != nil {
				return err
			}
			w.WriteRawBytes(b)
		}
	}
	var flags uint64
	if p.Enabled {
		flags |= flagEnabled
	}
	if p.ThreadProgress {
		flags |= flagThreadProgress
	}
	if p.IsYield {
		flags |= flagIsYield
	}
	if p.EnableCheckpoint {
		flags |= flagEnableCheckpoint
	}
	if flags != 0 {
		w.WriteUvarint(pfFlags)
		w.WriteUvarint(flags)
	}
	if p.Fairness != 0 {
		w.WriteUvarint(pfFairness)
		w.WriteUvarint(uint64(p.Fairness))
	}
	if p.ChoiceFairness != 0 {
		w.WriteUvarint(pfChoiceFairness)
		w.WriteUvarint(uint64(p.ChoiceFairness))
	}
	if p.topLevelVars != nil {
		w.WriteUvarint(pfTopLevelVars)
		w.WriteStrings(p.topLevelVars)
	}
	w.WriteUvarint(pfEnd)
	return nil
}

func writeThread(w *lib.StateWriter, t *Thread) error {
	if t == nil {
		w.WriteBool(false)
		return nil
	}
	w.WriteBool(true)
	w.WriteVarint(int64(t.Id))
	w.WriteUvarint(uint64(t.Fairness))
	w.WriteBool(t.Aborted)
	frames := t.Stack.RawArray()
	w.WriteUvarint(uint64(len(frames)))
	for _, f := range frames {
		if err := writeFrame(w, f); err != nil {
			return err
		}
	}
	return nil
}

func writeFrame(w *lib.StateWriter, f *CallFrame) error {
	w.WriteUvarint(uint64(f.FileIndex))
	w.WriteString(f.pc)
	w.WriteString(f.Name)
	if err := writeScopeChain(w, f.scope); err != nil {
		return err
	}
	if err := w.WriteStringDict(f.vars); err != nil {
		return err
	}
	w.WriteStrings(f.callerAssignVarNames)
	if f.obj == nil {
		return w.WriteValue(starlark.None)
	}
	return w.WriteValue(f.obj)
}

// writeScopeChain writes the scopes root-first. As in Scope.Clone, only
// the root has vars of its own; the others share the root's.
func writeScopeChain(w *lib.StateWriter, s *Scope) error {
	var chain []*Scope
	for ; s != nil; s = s.parent {
		chain = append(chain, s)
	}
	w.WriteUvarint(uint64(len(chain)))
	for i := len(chain) - 1; i >= 0; i-- {
		s := chain[i]
		w.WriteUvarint(uint64(s.flow))
		writeInts(w, s.skipstmts)
		w.WriteStrings(s.loopVars)
		// 0 is a nil loopRange, n+1 one of n values.
		if s.loopRange == nil {
			w.WriteUvarint(0)
		} else {
			w.WriteUvarint(uint64(len(s.loopRange)) + 1)
			for _, v := range s.loopRange {
				if err := w.WriteValue(v); err != nil {
					return err
				}
			}
		}
		if s.parent == nil {
			if err := w.WriteStringDict(s.vars); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeInts(w *lib.StateWriter, xs []int) {
	w.WriteUvarint(uint64(len(xs)))
	for _, x := range xs {
		w.WriteVarint(int64(x))
	}
}

func writeStringIntMap(w *lib.StateWriter, m map[string]int64) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	w.WriteUvarint(uint64(len(keys)))
	for _, k := range keys {
		w.WriteString(k)
		w.WriteVarint(m[k])
	}
}

func sortedIntKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

// DecodeProcess rebuilds a process from EncodeProcess's output. template is
// any process of the same run: the decoded process takes the run-wide
// parts (files, evaluator, globals, modules, symbol table) from it, as Fork
// takes them from the parent. Parent is left nil for the caller to set.
func DecodeProcess(data []byte, template *Process) (*Process, error) {
	return decodeProcess(data, template, codecContext(template))
}

func decodeProcess(data []byte, template *Process, ctx *lib.CodecContext) (*Process, error) {
	r := lib.NewStateReader(ctx, data)
	if v := r.ReadUint8(); v != lib.StateCodecVersion {
		if r.Err() != nil {
			return nil, r.Err()
		}
		return nil, fmt.Errorf("%w: version %d, want %d", lib.ErrCorruptState, v, lib.StateCodecVersion)
	}
	p := &Process{
		Heap:      &Heap{state: starlark.StringDict{}, globals: template.Heap.globals},
		Evaluator: template.Evaluator,

		durabilitySpec: template.durabilitySpec,
		guidedTrace:    template.guidedTrace,

		Children:    []*Process{},
		Files:       template.Files,
		Returns:     make(starlark.StringDict),
		SymbolTable: template.SymbolTable,
		Modules:     template.Modules,
		Labels:      make([]string, 0),
		Messages:    make([]*ast.Message, 0),
		Threads:     []*Thread{},
		Channels:    make(map[int]*lib.Channel),

		ChannelMessages: make(map[int][]*ChannelMessage),

		assertionBindings: template.assertionBindings,
	}
	if err := readProcessFields(r, p); err != nil {
		return nil, err
	}
	if err := r.Done(); err != nil {
		return nil, err
	}
	if p.Stats == nil {
		p.Stats = NewStats()
	}
	return p, nil
}

func readProcessFields(r *lib.StateReader, p *Process) error {
	for {
		field := r.ReadUvarint()
		if r.Err() != nil {
			return r.Err()
		}
		var err error
		switch field {
		case pfEnd:
			return nil
		case pfName:
			p.Name = r.ReadString()
		case pfCurrent:
			p.Current = int(r.ReadVarint())
		case pfHeap:
			p.Heap.state, err = r.ReadStringDict()
		case pfRoles:
			n := r.ReadCount()
			p.Roles = make([]*lib.Role, n)
			for i := range p.Roles {
				v, err := r.ReadValue()
				if err != nil {
					return err
				}
				role, ok := v.(*lib.Role)
				if !ok {
					return fmt.Errorf("%w: role is %s", lib.ErrCorruptState, v.Type())
				}
				p.Roles[i] = role
			}
		case pfThreads:
			n := r.ReadCount()
			p.Threads = make([]*Thread, n)
			for i := range p.Threads {
				if p.Threads[i], err = readThread(r, p); err != nil {
					return err
				}
			}
		case pfChannelMessages:
			n := r.ReadCount()
			for i := 0; i < n; i++ {
				k := int(r.ReadVarint())
				msgs := make([]*ChannelMessage, r.ReadCount())
				for j := range msgs {
					m := &ChannelMessage{receiver: r.ReadString(), function: r.ReadString()}
					if m.frame, err = readFrame(r); err != nil {
						return err
					}
					if m.params, err = r.ReadStringDict(); err != nil {
						return err
					}
					msgs[j] = m
				}
				p.ChannelMessages[k] = msgs
			}
		case pfChannels:
			n := r.ReadCount()
			for i := 0; i < n; i++ {
				k := int(r.ReadVarint())
				v, err := r.ReadValue()
				if err != nil {
					return err
				}
				ch, ok := v.(*lib.Channel)
				if !ok {
					return fmt.Errorf("%w: channel is %s", lib.ErrCorruptState, v.Type())
				}
				p.Channels[k] = ch
			}
		case pfRotationalLastAllocated:
			p.RotationalLastAllocated = readStringIntMap(r)
		case pfStats:
			p.Stats = &Stats{TotalActions: int(r.ReadUvarint()), Counts: make(map[string]int)}
			for k, v := range readStringIntMap(r) {
				p.Stats.Counts[k] = int(v)
			}
		case pfReturns:
			p.Returns, err = r.ReadStringDict()
		case pfFailedInvariants:
			n := r.ReadCount()
			p.FailedInvariants = make(map[int][]int, n)
			for i := 0; i < n; i++ {
				k := int(r.ReadVarint())
				p.FailedInvariants[k] = readInts(r)
			}
		case pfWitness:
			p.Witness = make([][]bool, r.ReadCount())
			for i := range p.Witness {
				p.Witness[i] = make([]bool, r.ReadCount())
				for j := range p.Witness[i] {
					p.Witness[i][j] = r.ReadBool()
				}
			}
		case pfAtoms:
			p.Atoms = make([]uint64, r.ReadCount())
			for i := range p.Atoms {
				p.Atoms[i] = r.ReadUvarint()
			}
		case pfLabels:
			p.Labels = r.ReadStrings()
		case pfMessages:
			p.Messages = make([]*ast.Message, r.ReadCount())
			for i := range p.Messages {
				m := &ast.Message{}
				if err := proto.Unmarshal(r.ReadRawBytes(), m); err != nil {
					return err
				}
				p.Messages[i] = m
			}
		case pfFlags:
			flags := r.ReadUvarint()
			p.Enabled = flags&flagEnabled != 0
			p.ThreadProgress = flags&flagThreadProgress != 0
			p.IsYield = flags&flagIsYield != 0
			p.EnableCheckpoint = flags&flagEnableCheckpoint != 0
		case pfFairness:
			p.Fairness = ast.FairnessLevel(r.ReadUvarint())
		case pfChoiceFairness:
			p.ChoiceFairness = ast.FairnessLevel(r.ReadUvarint())
		case pfTopLevelVars:
			p.topLevelVars = r.ReadStrings()
			if p.topLevelVars == nil {
				p.topLevelVars = []string{}
			}
		default:
			return fmt.Errorf("%w: unknown process field %d", lib.ErrCorruptState, field)
		}
		if err != nil {
			return err
		}
		if r.Err() != nil {
			return r.Err()
		}
	}
}

func readThread(r *lib.StateReader, p *Process) (*Thread, error) {
	if !r.ReadBool() {
		return nil, r.Err()
	}
	t := &Thread{
		Id:       int(r.ReadVarint()),
		Process:  p,
		Files:    p.Files,
		Stack:    NewCallStack(),
		Fairness: ast.FairnessLevel(r.ReadUvarint()),
		Aborted:  r.ReadBool(),
	}
	n := r.ReadCount()
	for i := 0; i < n; i++ {
		f, err := readFrame(r)
		if err != nil {
			return nil, err
		}
		t.Stack.Push(f)
	}
	return t, r.Err()
}

func readFrame(r *lib.StateReader) (*CallFrame, error) {
	f := &CallFrame{FileIndex: int(r.ReadUvarint()), pc: r.ReadString(), Name: r.ReadString()}
	var err error
	if f.scope, err = readScopeChain(r); err != nil {
		return nil, err
	}
	if f.vars, err = r.ReadStringDict(); err != nil {
		return nil, err
	}
	f.callerAssignVarNames = r.ReadStrings()
	obj, err := r.ReadValue()
	if err != nil {
		return nil, err
	}
	if obj != starlark.None {
		role, ok := obj.(*lib.Role)
		if !ok {
			return nil, fmt.Errorf("%w: frame receiver is %s", lib.ErrCorruptState, obj.Type())
		}
		f.obj = role
	}
	return f, r.Err()
}

func readScopeChain(r *lib.StateReader) (*Scope, error) {
	n := r.ReadCount()
	var s *Scope
	for i := 0; i < n; i++ {
		child := &Scope{
			parent:    s,
			flow:      ast.Flow(r.ReadUvarint()),
			skipstmts: readInts(r),
			loopVars:  r.ReadStrings(),
		}
		if m := r.ReadCount(); m > 0 {
			child.loopRange = make([]starlark.Value, m-1)
			for j := range child.loopRange {
				v, err := r.ReadValue()
				if err != nil {
					return nil, err
				}
				child.loopRange[j] = v
			}
		}
		if s == nil {
			vars, err := r.ReadStringDict()
			if err != nil {
				return nil, err
			}
			child.vars = vars
		} else {
			child.vars = s.vars
		}
		s = child
	}
	return s, r.Err()
}

func readInts(r *lib.StateReader) []int {
	n := r.ReadCount()
	if n == 0 {
		return nil
	}
	xs := make([]int, n)
	for i := range xs {
		xs[i] = int(r.ReadVarint())
	}
	return xs
}

func readStringIntMap(r *lib.StateReader) map[string]int64 {
	n := r.ReadCount()
	m := make(map[string]int64, n)
	for i := 0; i < n; i++ {
		k := r.ReadString()
		m[k] = r.ReadVarint()
	}
	return m
}

// stateCodecRoundTrip makes Fork pass every new process through
// EncodeProcess and DecodeProcess, and check that the decoded process
// encodes to the same bytes. A test aid: with it set, a run must give the
// same results as without. Set to "fresh", each decode uses a new codec
// context, as if the state had been read by another process running the
// same spec.
var stateCodecRoundTrip = os.Getenv("FIZZ_STATE_CODEC_ROUNDTRIP") != ""
var stateCodecRoundTripFresh = os.Getenv("FIZZ_STATE_CODEC_ROUNDTRIP") == "fresh"

func roundTripProcess(p *Process) *Process {
	data, err := EncodeProcess(p)
	if err != nil {
		panic(fmt.Sprintf("state codec: encode: %v", err))
	}
	ctx := codecContext(p)
	if stateCodecRoundTripFresh {
		// The dictionary is built from the spec alone, so sharing the
		// run's saves rebuilding it at every fork.
		fresh := lib.NewCodecContext()
		fresh.ShareDictionary(ctx)
		setUpCodecContext(fresh, p)
		ctx = fresh
	}
	p2, err := decodeProcess(data, p, ctx)
	if err != nil {
		panic(fmt.Sprintf("state codec: decode: %v", err))
	}
	again, err := EncodeProcess(p2)
	if err != nil {
		panic(fmt.Sprintf("state codec: re-encode: %v", err))
	}
	if string(again) != string(data) {
		panic("state codec: decoded process encodes differently")
	}
	p2.Parent = p.Parent
	return p2
}

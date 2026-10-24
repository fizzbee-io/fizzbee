package modelchecker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"sort"

	"github.com/fizzbee-io/fizzbee/lib"
)

// hashHex is the digest every HashCode returns: hex of SHA-256 over the
// canonical binary encoding of the state.
func hashHex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writeScopeChain writes the scope chain root-first. Child scopes share
// their parent's vars map, so the encoding of each distinct map is
// memoized by map pointer and re-appended instead of re-encoded.
func (s *Scope) writeScopeChain(e *lib.StateEncoder, memo map[uintptr][2]int) {
	if s == nil {
		return
	}
	s.parent.writeScopeChain(e, memo)
	key := reflect.ValueOf(s.vars).Pointer()
	if off, ok := memo[key]; ok && s.vars != nil {
		e.AppendRaw(e.Bytes()[off[0]:off[1]])
	} else {
		start := e.Len()
		e.WriteStringDict(s.vars)
		if s.vars != nil {
			memo[key] = [2]int{start, e.Len()}
		}
	}
	skip := sortedCopy(s.skipstmts)
	e.WriteUvarint(uint64(len(skip)))
	for _, n := range skip {
		e.WriteInt64(int64(n))
	}
	if len(s.loopRange) == 0 {
		e.WriteString("")
	} else {
		e.WriteString(fmt.Sprintln(s.loopRange))
	}
}

// writeFrame writes one call frame: its scope chain, program counter and
// receiver role.
func (c *CallFrame) writeFrame(e *lib.StateEncoder) {
	memo := make(map[uintptr][2]int, 2)
	c.scope.writeScopeChain(e, memo)
	e.WriteString(c.pc)
	if c.obj != nil {
		e.WriteString(c.obj.RefString())
	} else {
		e.WriteString("")
	}
}

func (c *CallFrame) HashCode() string {
	e := lib.NewStateEncoder()
	c.writeFrame(e)
	h := hashHex(e.Bytes())
	e.Release()
	return h
}

func (s *CallStack) writeFrames(e *lib.StateEncoder) {
	if s == nil {
		e.WriteUvarint(0)
		return
	}
	arr := s.RawArray()
	e.WriteUvarint(uint64(len(arr)))
	for _, frame := range arr {
		frame.writeFrame(e)
	}
}

func (s *CallStack) HashCode() string {
	if s == nil {
		return ""
	}
	e := lib.NewStateEncoder()
	s.writeFrames(e)
	h := hashHex(e.Bytes())
	e.Release()
	return h
}

func (t *Thread) HashCode() string {
	if t == nil {
		return ""
	}
	e := lib.NewStateEncoder()
	t.Stack.writeFrames(e)
	h := hashHex(e.Bytes())
	e.Release()
	return h
}

// HashCode returns a string hash of the global state.
func (h *Heap) HashCode() string {
	if h.CachedHashCode != "" {
		return h.CachedHashCode
	}
	e := lib.NewStateEncoder()
	e.WriteStringDict(h.state)
	h.CachedHashCode = hashHex(e.Bytes())
	e.Release()
	return h.CachedHashCode
}

func (cm *ChannelMessage) HashCode() string {
	e := lib.NewStateEncoder()
	e.WriteString(cm.receiver)
	e.WriteString(cm.function)
	e.WriteStringDict(cm.params)
	e.WriteString(cm.frame.HashCode())
	h := hashHex(e.Bytes())
	e.Release()
	return h
}

func (p *Process) HashCode() string {
	if p.CachedHashCode != "" {
		return p.CachedHashCode
	}
	threadHashes := make([]string, len(p.Threads))
	p.CachedThreadHashes = make([]string, len(threadHashes))
	for i, thread := range p.Threads {
		threadHashes[i] = thread.HashCode()
		// Having a second copy of the thread hashes as the first one
		// will be sorted to create the process hash
		p.CachedThreadHashes[i] = threadHashes[i]
	}

	e := lib.NewStateEncoder()
	defer e.Release()

	// Use the Current thread's hash first, not the index
	currentThreadHash := ""
	if len(threadHashes) > 0 && p.Current >= 0 {
		currentThreadHash = threadHashes[p.Current]
	}
	e.WriteString(currentThreadHash)

	// Sort the thread hashes to make the hash deterministic. Removed
	// threads leave a nil entry whose hash is "", and those never counted
	// towards the state, so only the non-empty hashes are written.
	sort.Strings(threadHashes)
	live := 0
	for _, hash := range threadHashes {
		if hash != "" {
			live++
		}
	}
	e.WriteUvarint(uint64(live))
	for _, hash := range threadHashes {
		if hash != "" {
			e.WriteString(hash)
		}
	}

	if !excludeReturnsFromState {
		e.WriteStringDict(p.Returns)
	}

	// Include each role's state to distinguish processes with different
	// role states. In value position a role is only its reference, so this
	// is the one place the role's params and fields are hashed.
	e.WriteUvarint(uint64(len(p.Roles)))
	for _, role := range p.Roles {
		e.WriteRole(role)
	}
	// hash the heap variables as well
	e.WriteString(p.Heap.HashCode())

	channelKeys := make([]int, 0, len(p.Channels))
	for k := range p.Channels {
		channelKeys = append(channelKeys, k)
	}
	sort.Ints(channelKeys)
	e.WriteUvarint(uint64(len(channelKeys)))
	for _, k := range channelKeys {
		channel := p.Channels[k]
		e.WriteInt64(int64(k))
		e.WriteString(channel.String())
	}

	channelMsgKeys := make([]int, 0, len(p.ChannelMessages))
	for k := range p.ChannelMessages {
		channelMsgKeys = append(channelMsgKeys, k)
	}
	sort.Ints(channelMsgKeys)
	for _, k := range channelMsgKeys {
		messages := p.ChannelMessages[k]
		// TODO: Sort the messages, for unordered channels
		for _, message := range messages {
			e.WriteInt64(int64(k))
			e.WriteString(message.HashCode())
		}
	}
	p.CachedHashCode = hashHex(e.Bytes())
	return p.CachedHashCode
}

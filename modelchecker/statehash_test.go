package modelchecker

import (
	"testing"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
)

// The process hash must change when any part of the state changes, and
// must not change for things that are not state (thread order, removed
// threads). Each case below changes one part; a part accidentally left out
// of the encoding shows up as an equal hash.
func TestProcessHashCoversEveryPart(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}

	build := func(change func(p *Process)) string {
		p := NewProcess("", files, nil)
		p.Heap.state = starlark.StringDict{"a": starlark.MakeInt(1)}
		p.NewThread()
		frame := p.currentThread().currentFrame()
		frame.pc = "Actions[0].Block.Stmts[0]"
		frame.scope = &Scope{vars: starlark.StringDict{"x": starlark.MakeInt(1)}}
		p.Roles = []*lib.Role{testRole("Node", 1)}
		if change != nil {
			change(p)
		}
		return p.HashCode()
	}
	base := build(nil)
	assert.Equal(t, base, build(nil), "the same state hashes the same")

	differ := map[string]func(p *Process){
		"heap value": func(p *Process) { p.Heap.state["a"] = starlark.MakeInt(2) },
		"heap name":  func(p *Process) { p.Heap.state["b"] = starlark.MakeInt(1) },
		"pc": func(p *Process) {
			p.currentThread().currentFrame().pc = "Actions[0].Block.Stmts[1]"
		},
		"scope var": func(p *Process) {
			p.currentThread().currentFrame().scope.vars["x"] = starlark.MakeInt(2)
		},
		"nested scope": func(p *Process) {
			f := p.currentThread().currentFrame()
			f.scope = &Scope{parent: f.scope, vars: starlark.StringDict{}}
		},
		"skipped stmts": func(p *Process) {
			p.currentThread().currentFrame().scope.skipstmts = []int{0}
		},
		"receiver role": func(p *Process) { p.currentThread().currentFrame().obj = p.Roles[0] },
		"extra thread":  func(p *Process) { p.NewThread() },
		"role field": func(p *Process) {
			p.Roles[0].Fields = lib.FromStringDict(starlark.String("fields"), starlark.StringDict{"n": starlark.MakeInt(9)})
		},
		"extra role": func(p *Process) { p.Roles = append(p.Roles, testRole("Node", 2)) },
		"returns":    func(p *Process) { p.Returns = starlark.StringDict{"Read": starlark.MakeInt(1)} },
		"channel message": func(p *Process) {
			p.ChannelMessages[0] = []*ChannelMessage{{
				receiver: "Node#1", function: "Ping",
				params: starlark.StringDict{"v": starlark.MakeInt(1)},
				frame:  &CallFrame{scope: &Scope{vars: starlark.StringDict{}}},
			}}
		},
	}
	for name, change := range differ {
		assert.NotEqual(t, base, build(change), name)
	}

	// Two threads in either order are the same state (with Current following
	// the same thread), and a removed thread (a nil entry) is not part of it.
	twoThreads := func(swap bool) func(p *Process) {
		return func(p *Process) {
			p.NewThread()
			p.Threads[1].currentFrame().pc = "Actions[1]"
			if swap {
				p.Threads[0], p.Threads[1] = p.Threads[1], p.Threads[0]
				p.Current = 0
			}
		}
	}
	assert.Equal(t, build(twoThreads(false)), build(twoThreads(true)), "thread order")
	assert.Equal(t, base, build(func(p *Process) { p.Threads = append(p.Threads, nil) }), "removed thread")
}

package modelchecker

import (
	ast "fizz/proto"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"testing"
)

func TestThread_FindNextProgramCounter(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)

	thread := NewThread(nil, []*ast.File{file}, 0, "")
	thread.pushFrame(&CallFrame{FileIndex: 0})
	tests := []struct {
		name string
		pc   string
		want string
	}{
		//{
		//	name: "empty to action",
		//	pc:   "",
		//	want: "Actions[0]",
		//},
		{
			name: "action to block",
			pc:   "Actions[0]",
			want: "Actions[0].Block",
		},
		{
			name: "block to stmt",
			pc:   "Actions[0].Block",
			want: "Actions[0].Block.Stmts[0]",
		},
		{
			name: "stmt to next stmt",
			pc:   "Actions[0].Block.Stmts[0]",
			want: "Actions[0].Block.Stmts[1]",
		},
		{
			name: "stmt to next stmt",
			pc:   "Actions[0].Block.Stmts[1]",
			want: "Actions[0].Block.Stmts[2]",
		},
		{
			// Should this enter the nested block instead or should this be handled by executeStmt?
			name: "stmt to next stmt",
			pc:   "Actions[0].Block.Stmts[2]",
			want: "Actions[0].Block.Stmts[3]",
		},
		//{
		//	name: "stmt to nested block",
		//	pc:   "",
		//	want: "Actions[0]",
		//},
		{
			name: "stmt to stmt in nested block",
			pc:   "Actions[0].Block.Stmts[2].Block",
			want: "Actions[0].Block.Stmts[2].Block.Stmts[0]",
		},
		{
			name: "stmt to stmt in nested block",
			pc:   "Actions[0].Block.Stmts[2].Block.Stmts[0]",
			want: "Actions[0].Block.Stmts[2].Block.Stmts[1]",
		},
		{
			name: "exit nested block",
			pc:   "Actions[0].Block.Stmts[2].Block.Stmts[1]",
			want: "Actions[0].Block.Stmts[2].Block.$",
		},
		{
			name: "exit nested block",
			pc:   "Actions[0].Block.Stmts[4]",
			want: "Actions[0].Block.$",
		},
		//{
		//	name: "exit nested block",
		//	pc:   "",
		//	want: "Actions[0]",
		//},
		//{
		//	name: "exit parent block",
		//	pc:   "",
		//	want: "Actions[0]",
		//},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			thread.currentFrame().pc = tt.pc
			if got := thread.FindNextProgramCounter(); got != tt.want {
				t.Errorf("Thread.FindNextProgramCounter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestThread_ExecuteAction(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}
	thread := NewThread(nil, files, 0, "Actions[0]")
	assert.Equal(t, thread.Stack.Len(), 1)
	assert.Equal(t, thread.currentFrame().pc, "Actions[0]")
	thread.executeAction()
	assert.Equal(t, thread.Stack.Len(), 1)
	assert.Equal(t, thread.currentFrame().pc, "Actions[0].Block")
}

func TestThread_ExecuteBlock(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}
	process := NewProcess("", files, nil)
	process.NewThread()
	baseThread := NewThread(process, files, 0, "Actions[0]")
	assert.Equal(t, baseThread.Stack.Len(), 1)
	t.Run("atomic", func(t *testing.T) {
		thread := baseThread.Clone()
		thread.currentFrame().pc = "Actions[0].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, thread.currentPc(), "Actions[0].Block.Stmts[0]")
		assert.Len(t, forks, 0)
		assert.Equal(t, ast.Flow_FLOW_ATOMIC, thread.currentFrame().scope.flow)
	})
	t.Run("serial", func(t *testing.T) {
		thread := baseThread.Clone()
		thread.currentFrame().pc = "Actions[2].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, thread.currentPc(), "Actions[2].Block.Stmts[0]")
		assert.Len(t, forks, 0)
		assert.Equal(t, ast.Flow_FLOW_SERIAL, thread.currentFrame().scope.flow)
	})
	t.Run("oneof", func(t *testing.T) {
		thread := process.Fork().currentThread()
		thread.currentFrame().pc = "Actions[1].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		//assert.Equal(t, thread.currentPc(), "")
		assert.Len(t, forks, 5)
		assert.Len(t, thread.Process.Children, 5)
		assert.Equal(t, ast.Flow_FLOW_ONEOF, thread.currentFrame().scope.flow)
		for i, fork := range forks {
			assert.Equal(t, fmt.Sprintf("Actions[1].Block.Stmts[%d]", i), fork.currentThread().currentPc())
			assert.Equal(t, fork, thread.Process.Children[i])
		}
	})
	t.Run("parallel", func(t *testing.T) {
		thread := process.Fork().currentThread()
		thread.currentFrame().pc = "Actions[3].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		//assert.Equal(t, "", thread.currentPc())
		assert.Len(t, forks, 5)
		assert.Equal(t, ast.Flow_FLOW_PARALLEL, thread.currentFrame().scope.flow)
		assert.Len(t, thread.Process.Children, 5)
		for i, fork := range forks {
			assert.Equal(t, fmt.Sprintf("Actions[3].Block.Stmts[%d]", i), fork.currentThread().currentPc())
			assert.Equal(t, []int{i}, fork.currentThread().currentFrame().scope.skipstmts)
			assert.Equal(t, fork, thread.Process.Children[i])
		}
	})
}

func TestThread_ExecuteStatement(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}

	t.Run("atomic", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[0].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[0].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)
		forks, yield := thread.executeStatement()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[0].Block.Stmts[1]", thread.currentPc())
		assert.Len(t, forks, 0)
		assert.False(t, yield)
		forks, yield = thread.executeStatement()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[0].Block.Stmts[2]", thread.currentPc())
		assert.Len(t, forks, 0)
		assert.False(t, yield)

		thread.currentFrame().pc = "Actions[0].Block.Stmts[4]"
		forks, yield = thread.executeStatement()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[0].Block.$", thread.currentPc())
		assert.Len(t, forks, 0)
		assert.False(t, yield)
	})

	t.Run("serial", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[2].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[2].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)
		forks, _ = thread.executeStatement()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[2].Block.Stmts[1]", thread.currentPc())
		assert.Len(t, forks, 0)
		forks, yield := thread.executeStatement()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[2].Block.Stmts[2]", thread.currentPc())
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		thread.currentFrame().pc = "Actions[2].Block.Stmts[4]"
		forks, yield = thread.executeStatement()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[2].Block.$", thread.currentPc())
		assert.Len(t, forks, 0)
		assert.True(t, yield)
	})

	t.Run("oneof", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[1].Block"
		oneofForks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Len(t, oneofForks, 5)
		assert.Equal(t, ast.Flow_FLOW_ONEOF, thread.currentFrame().scope.flow)
		for i, fork := range oneofForks {
			// skip the 3rd statement that is nested block
			if i == 2 {
				continue
			}
			currentThread := fork.currentThread()
			assert.Equal(t, fmt.Sprintf("Actions[1].Block.Stmts[%d]", i), currentThread.currentPc())
			forks, yield := currentThread.executeStatement()
			assert.Equal(t, currentThread.Stack.Len(), 1)
			assert.Equal(t, "Actions[1].Block.$", currentThread.currentPc())
			assert.Len(t, forks, 0)
			assert.False(t, yield)
		}
	})

	t.Run("parallel", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[3].Block"
		parallelForks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Len(t, parallelForks, 5)
		assert.Equal(t, ast.Flow_FLOW_PARALLEL, thread.currentFrame().scope.flow)
		for i, fork := range parallelForks {
			// skip the 3rd statement that is nested block
			if i == 2 {
				continue
			}
			currentThread := fork.currentThread()
			assert.Equal(t, fmt.Sprintf("Actions[3].Block.Stmts[%d]", i), currentThread.currentPc())
			assert.Equal(t, []int{i}, fork.currentThread().currentFrame().scope.skipstmts)
			forks, yield := currentThread.executeStatement()
			assert.Equal(t, currentThread.Stack.Len(), 1)
			assert.Equal(t, "", currentThread.currentPc())

			//assert.ElementsMatch(t, []int{}, currentThread.currentFrame().scope.skipstmts)
			assert.Len(t, forks, 4)
			assert.True(t, yield)
			for j, fork := range forks {
				nextStmt := j
				if j >= i {
					nextStmt++
				}
				assert.Equal(t, fmt.Sprintf("Actions[3].Block.Stmts[%d]", nextStmt), fork.currentThread().currentPc())
				assert.Equal(t, []int{i, nextStmt}, fork.currentThread().currentFrame().scope.skipstmts)
			}
		}
	})

	t.Run("parallel_final_stmt", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[3].Block"
		parallelForks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Len(t, parallelForks, 5)
		assert.Equal(t, ast.Flow_FLOW_PARALLEL, thread.currentFrame().scope.flow)

		i := 1
		fork := parallelForks[i]

		currentThread := fork.currentThread()
		assert.Equal(t, fmt.Sprintf("Actions[3].Block.Stmts[%d]", i), currentThread.currentPc())
		assert.Equal(t, []int{i}, fork.currentThread().currentFrame().scope.skipstmts)
		fork.currentThread().currentFrame().scope.skipstmts = []int{0, 1, 2, 3, 4}
		forks, yield := currentThread.executeStatement()
		assert.Equal(t, currentThread.Stack.Len(), 1)
		assert.Equal(t, "Actions[3].Block.$", currentThread.currentPc())

		assert.Len(t, forks, 0)
		assert.False(t, yield)

	})

}

func TestThread_ExecuteEndOfBlock(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}

	t.Run("topblock", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[0].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[0].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)
		thread.currentFrame().pc = "Actions[0].Block.$"
		yield := thread.executeEndOfBlock()
		assert.Len(t, process.Threads, 0)
		assert.Equal(t, thread.Stack.Len(), 0)
		assert.True(t, yield)
	})

	t.Run("nested-atomic", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[0].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[0].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)
		thread.currentFrame().pc = "Actions[0].Block.Stmts[2].Block"
		forks = thread.executeBlock()
		assert.Equal(t, 1, thread.Stack.Len())
		assert.Equal(t, "Actions[0].Block.Stmts[2].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)

		thread.currentFrame().pc = "Actions[0].Block.Stmts[2].Block.$"
		yield := thread.executeEndOfBlock()
		assert.Len(t, process.Threads, 1)
		assert.Equal(t, 1, thread.Stack.Len())
		assert.False(t, yield)
	})

	t.Run("nested-serial", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}
		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[2].Block"
		forks := thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[2].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)
		thread.currentFrame().pc = "Actions[2].Block.Stmts[2].Block"
		forks = thread.executeBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.Equal(t, "Actions[2].Block.Stmts[2].Block.Stmts[0]", thread.currentPc())
		assert.Len(t, forks, 0)

		thread.currentFrame().pc = "Actions[2].Block.Stmts[2].Block.$"
		yield := thread.executeEndOfBlock()
		assert.Equal(t, thread.Stack.Len(), 1)
		assert.True(t, yield)
	})
}

func TestThread_Execute(t *testing.T) {
	file, err := parseAstFromString(ActionsWithMultipleBlocks)
	require.Nil(t, err)
	files := []*ast.File{file}

	t.Run("atomic", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}

		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[0]"
		forks, yield := thread.Execute()
		assert.Equal(t, thread.Stack.Len(), 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)
		assert.Len(t, process.Threads, 0)

		assert.Equal(t, starlark.MakeInt(13), process.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(26), process.Heap.globals["b"])
	})
	t.Run("oneof", func(t *testing.T) {
		process := NewProcess("", files, nil)
		process.NewThread()
		process.Heap.globals = starlark.StringDict{"a": starlark.MakeInt(10), "b": starlark.MakeInt(20)}

		thread := process.currentThread()
		assert.Equal(t, thread.Stack.Len(), 1)

		thread.currentFrame().pc = "Actions[1]"
		oneofForks, yield := thread.Execute()
		assert.Equal(t, 1, thread.Stack.Len())
		assert.Len(t, oneofForks, 5)
		assert.False(t, yield)
		assert.Len(t, process.Threads, 1)

		assert.Equal(t, starlark.MakeInt(10), process.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(20), process.Heap.globals["b"])

		fork := oneofForks[0]
		forks, yield := fork.currentThread().Execute()
		assert.Len(t, fork.Threads, 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		assert.Equal(t, starlark.MakeInt(11), fork.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(20), fork.Heap.globals["b"])

		fork = oneofForks[1]
		forks, yield = fork.currentThread().Execute()
		assert.Len(t, fork.Threads, 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		assert.Equal(t, starlark.MakeInt(10), fork.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(22), fork.Heap.globals["b"])

		// oneOfForks[2] is the nested block. check it separately
		//oneOfFork[2]

		nestedForks, yield := oneofForks[2].currentThread().Execute()
		assert.Len(t, nestedForks, 2)
		assert.Equal(t, starlark.MakeInt(10), oneofForks[2].Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(20), oneofForks[2].Heap.globals["b"])
		for _, fork := range nestedForks {
			thread = fork.currentThread()
			assert.Equal(t, 1, thread.Stack.Len())
			assert.False(t, yield)
			assert.Len(t, fork.Threads, 1)

			assert.Equal(t, starlark.MakeInt(10), fork.Heap.globals["a"])
			assert.Equal(t, starlark.MakeInt(20), fork.Heap.globals["b"])
		}
		fork = nestedForks[0]
		forks, yield = fork.currentThread().Execute()
		assert.Len(t, fork.Threads, 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		assert.Equal(t, starlark.MakeInt(11), fork.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(20), fork.Heap.globals["b"])

		fork = nestedForks[1]
		forks, yield = fork.currentThread().Execute()
		assert.Len(t, fork.Threads, 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		assert.Equal(t, starlark.MakeInt(10), fork.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(22), fork.Heap.globals["b"])

		// oneOfForks[3]

		fork = oneofForks[3]
		forks, yield = fork.currentThread().Execute()
		assert.Len(t, fork.Threads, 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		assert.Equal(t, starlark.MakeInt(11), fork.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(20), fork.Heap.globals["b"])

		fork = oneofForks[4]
		forks, yield = fork.currentThread().Execute()
		assert.Len(t, fork.Threads, 0)
		assert.Len(t, forks, 0)
		assert.True(t, yield)

		assert.Equal(t, starlark.MakeInt(10), fork.Heap.globals["a"])
		assert.Equal(t, starlark.MakeInt(22), fork.Heap.globals["b"])
	})

}

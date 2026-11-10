package modelchecker

// Capped thread copies: unbounded concurrency for an action.
//
// When max_concurrent_actions is MANY (-1), for one action in its
// action_options or for all actions without a limit of their own in the
// global options (the default with --experimental_capped_threads), the
// action's threads are counted as groups. A thread slot with Copies set
// stands for ONE or MANY identical threads: same frames, same program
// point, same locals. Any number of the action's threads can be in flight.
// Caution: as with an action that can run forever, a MANY group can act
// again and again, so state that grows with every step, such as a
// counter, makes the state space infinite. The spec has to bound it.
//
//   - A new thread of the action starts as a group of ONE.
//   - At every yield, groups with identical frames merge, and their counts
//     add (CardinalityCounter.Plus): ONE + ONE is MANY.
//   - Scheduling a group of MANY splits one copy off to run. The copies
//     left behind are ONE or MANY, so the split forks two ways, as a
//     `fair oneof` would: liveness treats both options of a fair choice as
//     strongly fair, keyed by the group's slot and pc.
//
//     Why fair: a run that splits a group infinitely often and always
//     keeps MANY is real only if copies keep joining the group (new
//     starts, or copies looping back), since finitely many copies run
//     out. Such a run has a fair counterpart with the same observable
//     behaviour: choose ONE left at a split just before a join. Until the
//     join the two runs differ only in the waiting group's count (ONE
//     instead of MANY), with the same heap and the same frames for every
//     thread; the join makes the group MANY again, and from there the
//     checker states are identical. Without joins the
//     run needs infinitely many threads, which is no run of the spec, and
//     fairness rules it out. capped-threads-fair-split checks the draining
//     case (it fails with an unfair split) and capped-threads-loop the
//     returning one.
//   - A group of ONE runs as an ordinary thread and leaves no copy behind.
//   - On crash_on_yield, one copy of the group that yielded crashes: a
//     group of ONE goes away, and a group of MANY leaves ONE or MANY.
//
// Ordinary threads (Copies nil) are untouched and hash exactly as before.
// The scheduling rule is specified in tests/specs/capped-thread-copies,
// and tests/specs/capped-threads-* check the engine against it.

import (
	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
)

// cappedCopies reports whether threads of action, started by role (nil
// for a top-level action), are grouped into copies. The action_options
// keys that apply are the ones the concurrency limits use: "Action", or
// for a role action "Role.Action", "Role#.Action" and the instance's
// "Role#0.Action". A fixed limit under any of them keeps ordinary
// threads, so that the limit is enforced exactly. Otherwise MANY (-1)
// under any of them, or a MANY global max_concurrent_actions, groups them.
func (p *Processor) cappedCopies(action string, role *lib.Role) bool {
	keys := []string{action}
	if role != nil {
		keys = []string{role.Name + "." + action, role.Name + "#." + action, role.RefStringShort() + "." + action}
	}
	many := false
	for _, k := range keys {
		switch n := p.config.ActionOptions[k].GetMaxConcurrentActions(); {
		case n > 0:
			return false
		case n < 0:
			many = true
		}
	}
	return many || p.config.GetOptions().GetMaxConcurrentActions() < 0
}

// ordinaryThreadsCount is the number of threads that count towards the
// global max_concurrent_actions: groups of copies are unbounded by it.
func (p *Process) ordinaryThreadsCount() int {
	n := 0
	for _, t := range p.Threads {
		if t != nil && t.Copies == nil {
			n++
		}
	}
	return n
}

// splitCopies forks a group of MANY in two before it runs: in each fork
// the copies left behind, ONE or MANY, stay at the group's index, and the
// running copy is a group of ONE in a new slot, which becomes Current.
func (t *Thread) splitCopies() []*Process {
	p := t.Process
	cur := p.Current
	forks := make([]*Process, 0, 2)
	for _, left := range []*lib.CardinalityCounter{lib.NewCardinalityCounter(1), lib.NewCardinalityCounter(2)} {
		f, refs := p.forkWithRefs()
		// The copies left behind keep the group's slot, so the group's
		// thread index is stable across steps, as liveness tracks threads
		// by index. The running copy takes a new slot and leaves no copy
		// behind; at its next yield it merges with any identical group.
		run := t.Clone(refs, nil, 0)
		run.Process = f
		run.Copies = lib.NewCardinalityCounter(1)
		f.Threads[cur].Copies = left
		f.Threads = append(f.Threads, run)
		f.Current = len(f.Threads) - 1
		if stateCodecRoundTrip {
			f = roundTripProcess(f)
		}
		f.Name = "Any:copies left=" + left.String()
		f.ChoiceFairness = ast.FairnessLevel_FAIRNESS_LEVEL_WEAK
		forks = append(forks, f)
	}
	// As for a oneof: keep this step apart from the yield it started from.
	// The name only labels the split node in the graph and traces.
	p.Name = "Split copies: " + t.actionName()
	p.Enable()
	t.currentFrame().pc = t.currentFrame().pc + ".copies"
	return forks
}

// mergeCopies merges groups with identical frames into one slot, adding
// their counts, so that the state is the same however the copies got
// there. The lowest slot of each group survives, and Current moves to it.
//
// The key is the frames: scopes, locals, pc and receiver of every frame.
// The other thread fields need not match. Fairness is set only from the
// action at the root frame, so identical frames already imply it, and the
// liveness checker treats every thread continuation as strongly fair
// anyway: a thread's fairness only marks its action's start link. Id is
// not state, and Aborted is set only on a disabled fork, which never
// reaches a yield.
func (p *Process) mergeCopies() {
	store := lib.NewCappedCountStore[int]()
	merged := false
	for i, t := range p.Threads {
		if t == nil || t.Copies == nil {
			continue
		}
		key := t.framesHash()
		keep, ok := store.Get(key)
		store.AddCount(key, i, t.Copies)
		if !ok {
			continue
		}
		p.Threads[keep].Copies = store.Count(key)
		if p.Current == i {
			p.Current = keep
		}
		p.Threads[i] = nil
		merged = true
	}
	if merged {
		p.CachedHashCode = ""
	}
}

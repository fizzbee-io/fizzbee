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
//     oneof would, with weak fairness: MANY can't be chosen forever, since
//     every real step takes a copy closer to running out.
//   - A group of ONE runs as an ordinary thread and leaves no copy behind.
//   - On crash_on_yield, one copy of the group that yielded crashes: a
//     group of ONE goes away, and a group of MANY leaves ONE or MANY.
//
// Ordinary threads (Copies nil) are untouched and hash exactly as before.
// The scheduling rule is specified in tests/specs/capped-thread-copies,
// and tests/specs/capped-threads-* check the engine against it.

import (
	"strings"

	ast "fizz/proto"
	"github.com/fizzbee-io/fizzbee/lib"
)

// cappedCopies reports whether threads of the action whose root frame is
// named frameName ("Action" or "Role.Action") are grouped into copies: its
// action_options set max_concurrent_actions to MANY (-1), or set no limit
// while the global max_concurrent_actions is MANY. A fixed limit keeps
// ordinary threads.
func (p *Processor) cappedCopies(frameName string) bool {
	keys := []string{frameName}
	if role, action, ok := strings.Cut(frameName, "."); ok {
		keys = append(keys, role+"#."+action)
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
	p.Enable()
	t.currentFrame().pc = t.currentFrame().pc + ".copies"
	return forks
}

// mergeCopies merges groups with identical frames into one slot, adding
// their counts, so that the state is the same however the copies got
// there. Current moves to the surviving slot of its group.
func (p *Process) mergeCopies() {
	store := lib.NewCappedCountStore[int]()
	n := 0
	for i, t := range p.Threads {
		if t == nil || t.Copies == nil {
			continue
		}
		n++
		key := t.framesHash()
		if !store.Has(key) {
			store.Add(key, i)
			if t.Copies.IsMany() {
				store.Add(key, i)
			}
			continue
		}
		store.Add(key, i)
	}
	if n < 2 || store.Len() == n {
		return
	}
	for _, e := range store.Entries() {
		keep := e.Value
		for i, t := range p.Threads {
			if t == nil || t.Copies == nil || i == keep || t.framesHash() != e.Key {
				continue
			}
			if p.Current == i {
				p.Current = keep
			}
			p.Threads[i] = nil
		}
		p.Threads[keep].Copies = e.Count
	}
	p.CachedHashCode = ""
}


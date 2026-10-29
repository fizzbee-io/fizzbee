package liveness

import "fmt"

// Op is the operator of a Formula node.
type Op uint8

const (
	OpAtom Op = iota
	OpTrue
	OpNot
	OpAnd
	OpOr
	OpUntil // strong until
)

// Formula is a linear temporal logic formula over atoms numbered from 0. It
// has no next operator, so repeating a state never changes its truth; the
// product relies on that to let steps inside an action repeat the last
// state's letter. Other operators are built from these, see Eventually,
// Always, Implies and LeadsTo.
type Formula struct {
	Op   Op
	Atom int
	Args []*Formula
}

func Atom(i int) *Formula            { return &Formula{Op: OpAtom, Atom: i} }
func True() *Formula                 { return &Formula{Op: OpTrue} }
func Not(f *Formula) *Formula        { return &Formula{Op: OpNot, Args: []*Formula{f}} }
func And(fs ...*Formula) *Formula    { return &Formula{Op: OpAnd, Args: fs} }
func Or(fs ...*Formula) *Formula     { return &Formula{Op: OpOr, Args: fs} }
func Until(a, b *Formula) *Formula   { return &Formula{Op: OpUntil, Args: []*Formula{a, b}} }
func Eventually(f *Formula) *Formula { return Until(True(), f) }
func Always(f *Formula) *Formula     { return Not(Eventually(Not(f))) }
func Implies(a, b *Formula) *Formula { return Or(Not(a), b) }

// WeakUntil is a until b, or always a: b needn't happen. It is written as
// not (not b until (not a and not b)), which needs one until instead of two.
func WeakUntil(a, b *Formula) *Formula { return Not(Until(Not(b), And(Not(a), Not(b)))) }

// LeadsTo is always (a implies eventually b).
func LeadsTo(a, b *Formula) *Formula { return Always(Implies(a, Eventually(b))) }

// automaton is the generalized Büchi automaton of a formula in the
// Vardi-Wolper style. A state is a guess of which until subformulas hold at
// the current position, as a bitmask; together with the letter (the atoms'
// values, also a bitmask) it fixes the truth of every subformula there. A
// run is accepting when it visits each until's acceptance set infinitely
// often, which rules out putting off the until's right side forever.
type automaton struct {
	f      *Formula
	untils []*Formula // inner untils before the ones containing them
	index  map[*Formula]int
}

func newAutomaton(f *Formula) (*automaton, error) {
	a := &automaton{f: f, index: map[*Formula]int{}}
	var collect func(*Formula) error
	collect = func(f *Formula) error {
		for _, g := range f.Args {
			if err := collect(g); err != nil {
				return err
			}
		}
		switch f.Op {
		case OpAtom:
			if f.Atom < 0 || f.Atom >= 64 {
				return fmt.Errorf("atom %d out of range 0..63", f.Atom)
			}
		case OpNot:
			if len(f.Args) != 1 {
				return fmt.Errorf("not takes one operand, got %d", len(f.Args))
			}
		case OpUntil:
			if len(f.Args) != 2 {
				return fmt.Errorf("until takes two operands, got %d", len(f.Args))
			}
			if _, ok := a.index[f]; !ok {
				a.index[f] = len(a.untils)
				a.untils = append(a.untils, f)
			}
		}
		return nil
	}
	if err := collect(f); err != nil {
		return nil, err
	}
	if len(a.untils) > 64 {
		return nil, fmt.Errorf("formula has %d temporal operators; at most 64 are supported", len(a.untils))
	}
	return a, nil
}

// eval is the truth of f at a position with this letter and state.
func (a *automaton) eval(f *Formula, letter, s uint64) bool {
	switch f.Op {
	case OpAtom:
		return letter&(1<<uint(f.Atom)) != 0
	case OpTrue:
		return true
	case OpNot:
		return !a.eval(f.Args[0], letter, s)
	case OpAnd:
		for _, g := range f.Args {
			if !a.eval(g, letter, s) {
				return false
			}
		}
		return true
	case OpOr:
		for _, g := range f.Args {
			if a.eval(g, letter, s) {
				return true
			}
		}
		return false
	case OpUntil:
		return s&(1<<uint(a.index[f])) != 0
	}
	panic(fmt.Sprintf("unknown formula op %d", f.Op))
}

// successors calls fn with each state that can follow state s, read with
// letter, when the next letter is next. With first set there is no previous
// position, and the state must satisfy the formula instead.
//
// a U b holds at a position exactly when b does, or a does and a U b holds
// at the next one. So the guess for a U b is forced where b holds (true) or
// where neither a nor b does (false), and it is carried to the next position
// where a holds but b doesn't.
func (a *automaton) successors(letter, s uint64, first bool, next uint64, fn func(uint64)) {
	var assign func(k int, t uint64)
	assign = func(k int, t uint64) {
		if k == len(a.untils) {
			if !first || a.eval(a.f, next, t) {
				fn(t)
			}
			return
		}
		u, bit := a.untils[k], uint64(1)<<uint(k)
		// Inner untils come first, so their bits in t are already set.
		left, right := a.eval(u.Args[0], next, t), a.eval(u.Args[1], next, t)
		canBeFalse, canBeTrue := !right, right || left
		if !first && a.eval(u.Args[0], letter, s) && !a.eval(u.Args[1], letter, s) {
			if s&bit != 0 {
				canBeFalse = false
			} else {
				canBeTrue = false
			}
		}
		if canBeFalse {
			assign(k+1, t)
		}
		if canBeTrue {
			assign(k+1, t|bit)
		}
	}
	assign(0, 0)
}

// accept is the bitmask of acceptance sets a position is in: the k-th set
// holds the positions where until k is not promised or is fulfilled.
func (a *automaton) accept(letter, s uint64) uint64 {
	var m uint64
	for k, u := range a.untils {
		if s&(1<<uint(k)) == 0 || a.eval(u.Args[1], letter, s) {
			m |= 1 << uint(k)
		}
	}
	return m
}

// stuckAccepts reports whether the run can stay at a position with this
// letter forever and be accepting. On a word that repeats one letter, a U b
// holds exactly when b does, and an accepting run must guess every until
// correctly, so the state must be that valuation.
func (a *automaton) stuckAccepts(letter, s uint64) bool {
	var t uint64
	for k, u := range a.untils {
		if a.eval(u.Args[1], letter, t) {
			t |= 1 << uint(k)
		}
	}
	return s == t
}

// Product is the product of a state graph with a formula's automaton. A
// fair loop or a fair stutter in Graph is a fair run of the state graph that
// satisfies the formula, so callers pass the negation of the property.
type Product struct {
	Graph *Graph
	Node  []int // the state graph node of each product node
	Edge  []int // the state graph edge of each product edge
}

// NewProduct builds the reachable part of the product of g with f. The
// letter of node n is letter[n] where labeled[n] is set. A node without a
// label, such as a step inside an action, repeats the letter of the last
// labeled node before it, and nodes before the first label are not part of
// the word at all. canStop[n] reports whether a run may end by staying at n
// forever (an observation node); whether staying is fair is the engine's
// call. g's Enabled, ReadsEnabled and WeakUnobserved carry over, so fairness
// is judged on the state graph's own actions.
func NewProduct(g *Graph, f *Formula, labeled []bool, letter []uint64, canStop []bool) (*Product, error) {
	if err := g.validate(); err != nil {
		return nil, err
	}
	a, err := newAutomaton(f)
	if err != nil {
		return nil, err
	}
	g.buildOut()

	type key struct {
		n       int
		started bool
		letter  uint64
		s       uint64
	}
	p := &Product{Graph: &Graph{Actions: g.Actions, NumAcceptSets: len(a.untils)}}
	pg := p.Graph
	ids := map[key]int{}
	var keys []key
	node := func(k key) int {
		if id, ok := ids[k]; ok {
			return id
		}
		id := len(keys)
		ids[k] = id
		keys = append(keys, k)
		p.Node = append(p.Node, k.n)
		pg.Enabled = append(pg.Enabled, g.enabled(k.n))
		if g.ReadsEnabled != nil {
			pg.ReadsEnabled = append(pg.ReadsEnabled, g.ReadsEnabled[k.n])
		}
		if g.WeakUnobserved != nil {
			pg.WeakUnobserved = append(pg.WeakUnobserved, g.WeakUnobserved[k.n])
		}
		var acc uint64
		stuck := false
		if k.started {
			acc = a.accept(k.letter, k.s)
			stuck = canStop[k.n] && a.stuckAccepts(k.letter, k.s)
		}
		pg.Accept = append(pg.Accept, acc)
		// A loop before the first labeled node has no word.
		pg.LoopAllowed = append(pg.LoopAllowed, k.started)
		pg.FailsIfStuck = append(pg.FailsIfStuck, stuck)
		return id
	}
	// next calls fn with the product nodes a step into graph node to leads
	// to from product node k (or from before the run, when k is nil).
	next := func(k *key, to int, fn func(int)) {
		if !labeled[to] {
			if k == nil || !k.started {
				fn(node(key{n: to}))
				return
			}
			a.successors(k.letter, k.s, false, k.letter, func(t uint64) {
				fn(node(key{n: to, started: true, letter: k.letter, s: t}))
			})
			return
		}
		l := letter[to]
		if k == nil || !k.started {
			a.successors(0, 0, true, l, func(t uint64) {
				fn(node(key{n: to, started: true, letter: l, s: t}))
			})
			return
		}
		a.successors(k.letter, k.s, false, l, func(t uint64) {
			fn(node(key{n: to, started: true, letter: l, s: t}))
		})
	}
	for _, r := range g.Init {
		next(nil, r, func(id int) { pg.Init = append(pg.Init, id) })
	}
	for i := 0; i < len(keys); i++ {
		k := keys[i]
		for _, e := range g.out[k.n] {
			ge := g.Edges[e]
			next(&k, ge.To, func(id int) {
				pg.Edges = append(pg.Edges, Edge{From: i, To: id, Action: ge.Action})
				p.Edge = append(p.Edge, e)
			})
		}
	}
	pg.NumNodes = len(keys)
	return p, nil
}

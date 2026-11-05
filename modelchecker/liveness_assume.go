package modelchecker

import (
	"fmt"
	"slices"
	"strings"

	ast "fizz/proto"
)

// assumedFairness maps each action an assertion's @assume names, as
// "Action" or "Role.Action", to the fairness the assertion checks it with.
func assumedFairness(a *ast.Assumption) map[string]ast.FairnessLevel {
	if a == nil {
		return nil
	}
	levels := map[string]ast.FairnessLevel{}
	for _, list := range []struct {
		names []string
		level ast.FairnessLevel
	}{
		{a.GetFinite(), ast.FairnessLevel_FAIRNESS_LEVEL_FAULT},
		{a.GetWeakFair(), ast.FairnessLevel_FAIRNESS_LEVEL_WEAK},
		{a.GetStrongFair(), ast.FairnessLevel_FAIRNESS_LEVEL_STRONG},
		{a.GetUnfair(), ast.FairnessLevel_FAIRNESS_LEVEL_UNFAIR},
	} {
		for _, name := range list.names {
			levels[name] = list.level
		}
	}
	if len(levels) == 0 {
		return nil
	}
	return levels
}

func hasAssumptions(invariant *ast.Invariant) bool {
	return assumedFairness(invariant.GetAssumption()) != nil
}

// applyAssumptions gives every link of an action named in a the fairness a
// assumes for it, as if the action were declared that way, and returns a
// function that restores the links. Every step of an action's thread
// carries the action's name (Link.Action), so an override covers the steps
// after its first yield too, just like the action's own modifier.
//
// Both liveness engines read fairness from the links, so overriding them
// for the length of one assertion's check makes both check that assertion
// under its assumptions. The checks run one at a time, after exploration.
func applyAssumptions(nodes []*Node, a *ast.Assumption) (restore func()) {
	levels := assumedFairness(a)
	if levels == nil {
		return func() {}
	}
	saved := map[*Link]ast.FairnessLevel{}
	set := func(l *Link) {
		if l.Action == "" || l.IsCrashLink() {
			return
		}
		f, ok := levels[l.Action]
		if !ok {
			return
		}
		if _, seen := saved[l]; !seen {
			saved[l] = l.Fairness
		}
		l.Fairness = f
	}
	for _, n := range nodes {
		for _, l := range n.Outbound {
			set(l)
		}
		for _, l := range n.Inbound {
			set(l)
		}
	}
	return func() {
		for l, f := range saved {
			l.Fairness = f
		}
	}
}

// withAssumptions runs f on s or, when invariant has @assume, on the graph
// rebuilt under the fairness it assumes. The links are restored afterwards.
func (s *sccGraph) withAssumptions(invariant *ast.Invariant, f func(*sccGraph)) {
	if !hasAssumptions(invariant) {
		f(s)
		return
	}
	restore := applyAssumptions(s.nodes, invariant.GetAssumption())
	defer restore()
	t := newPartialSccGraph(s.rootNode, s.nodes, s.isOpen)
	t.quiet = s.quiet
	f(t)
}

// HasAssumptions reports whether an assertion of file has @assume.
func HasAssumptions(file *ast.File) bool {
	for _, invariant := range file.Invariants {
		if hasAssumptions(invariant) {
			return true
		}
	}
	return false
}

// ValidateAssumptions checks that every @assume names an action of file,
// as "Action" or "Role.Action", that Init isn't named, that each action is
// named at most once per assertion, and that only liveness assertions
// assume anything.
func ValidateAssumptions(file *ast.File) error {
	actions := map[string]bool{}
	for _, action := range file.Actions {
		if action.Name != "Init" {
			actions[action.Name] = true
		}
	}
	for _, role := range file.Roles {
		for _, action := range role.Actions {
			if action.Name != "Init" {
				actions[role.Name+"."+action.Name] = true
			}
		}
	}
	for _, invariant := range file.Invariants {
		a := invariant.GetAssumption()
		if a == nil {
			continue
		}
		alwaysEventually, eventuallyAlways := livenessOperators(invariant)
		if invariant.Formula == nil && !alwaysEventually && !eventuallyAlways {
			return fmt.Errorf("assertion %s: @assume applies only to liveness assertions", invariant.Name)
		}
		seen := map[string]string{}
		for _, list := range []struct {
			key   string
			names []string
		}{{"finite", a.GetFinite()}, {"weak_fair", a.GetWeakFair()}, {"strong_fair", a.GetStrongFair()}, {"unfair", a.GetUnfair()}} {
			for _, name := range list.names {
				if !actions[name] {
					return fmt.Errorf("assertion %s: @assume(%s=...) names %q, which is not an action; use \"Action\" or \"Role.Action\"", invariant.Name, list.key, name)
				}
				if prev, ok := seen[name]; ok {
					return fmt.Errorf("assertion %s: @assume names %q in both %s and %s", invariant.Name, name, prev, list.key)
				}
				seen[name] = list.key
			}
		}
	}
	return nil
}

// assumedFairSymmetricActions lists the actions of symmetric roles that an
// assertion assumes fair, as Role.Action.
func assumedFairSymmetricActions(file *ast.File) []string {
	symmetric := map[string]bool{}
	for _, role := range file.Roles {
		if slices.Contains(role.Modifiers, "symmetric") {
			symmetric[role.Name] = true
		}
	}
	var actions []string
	for _, invariant := range file.Invariants {
		a := invariant.GetAssumption()
		for _, name := range append(slices.Clone(a.GetWeakFair()), a.GetStrongFair()...) {
			if roleName, _, ok := strings.Cut(name, "."); ok && symmetric[roleName] && !slices.Contains(actions, name) {
				actions = append(actions, name)
			}
		}
	}
	return actions
}

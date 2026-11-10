package modelchecker

// Liveness tracking for cardinality counters.
//
// A counter's MANY stands for every count of two or more, so dec() on MANY
// may return MANY again, and the abstract graph has loops where a counter is
// decremented forever and never incremented. No real run does that: a real
// count is a finite, nonnegative integer. The liveness engine drops such
// loops (compassion; see liveness.Check). For that, every step from one
// yield point to the next records, per counter location, whether the step
// lowered or raised the real count there.
//
// The step's start state maps each counter object to the location it is at.
// inc() and dec() return counters that remember which start counter they
// came from and the net change since (lib.CardinalityCounter.Lineage). At
// the next yield point every location is checked:
//
//   - the same object as at the start, at the same location: no change;
//   - derived from the start counter of this same location: the net change,
//     which is exact, since each inc() and dec() changes the real count by
//     one whatever abstract value is chosen. Negative records a decrement,
//     positive an increment, zero nothing;
//   - anything else (a counter from another location, a new counter, one
//     whose lineage is unknown): an increment, since the change is unknown.
//
// Recording an increment is always sound: it only keeps more loops. Events
// are keyed by location, coarsened where symmetry or list shifts can rename
// locations between states: fields of symmetric roles become Role/field,
// list elements list[*], dict and genericmap entries keyed by symmetric
// values dict[*].
// Coarsening merges counts, and a decrease of the merged count still can't
// go on forever, so it is sound too.

import (
	"sort"
	"strconv"
	"strings"

	"github.com/fizzbee-io/fizzbee/lib"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const (
	cardDecLabel = "__card_dec:"
	cardIncLabel = "__card_inc:"
)

// cardLocation is one place a counter is stored. path names the exact
// location within one state; key is the possibly coarser name events use.
type cardLocation struct {
	path, key string
	value     *lib.CardinalityCounter
	// set replaces the value at the location; nil when it can't be.
	set func(*lib.CardinalityCounter)
}

// cardinalityLocations lists the counters stored in p's state: state
// variables, role fields, and inside lists, tuples, dicts and records.
func (p *Process) cardinalityLocations() []cardLocation {
	var out []cardLocation
	if p.Heap != nil {
		names := make([]string, 0, len(p.Heap.state))
		for name := range p.Heap.state {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			name := name
			collectCardinality(p.Heap.state[name], name, name, func(c *lib.CardinalityCounter) {
				p.Heap.state[name] = c
			}, &out, 0)
		}
	}
	for _, role := range p.Roles {
		if role == nil || role.Fields == nil {
			continue
		}
		prefix := role.RefStringShort()
		keyPrefix := prefix
		if role.Symmetric {
			keyPrefix = role.Name + "/"
		} else {
			keyPrefix = prefix + "."
		}
		for _, name := range role.Fields.AttrNames() {
			v, err := role.Fields.Attr(name)
			if err != nil || v == nil {
				continue
			}
			fields, name := role.Fields, name
			collectCardinality(v, prefix+"."+name, keyPrefix+name, func(c *lib.CardinalityCounter) {
				_ = fields.SetField(name, c)
			}, &out, 0)
		}
	}
	return out
}

func collectCardinality(v starlark.Value, path, key string, set func(*lib.CardinalityCounter), out *[]cardLocation, depth int) {
	if depth > 20 || v == nil {
		return
	}
	switch x := v.(type) {
	case *lib.CardinalityCounter:
		*out = append(*out, cardLocation{path: path, key: key, value: x, set: set})
	case *starlark.List:
		for i := 0; i < x.Len(); i++ {
			i := i
			collectCardinality(x.Index(i), path+"["+strconv.Itoa(i)+"]", key+"[*]", func(c *lib.CardinalityCounter) {
				_ = x.SetIndex(i, c)
			}, out, depth+1)
		}
	case starlark.Tuple:
		// Tuples are immutable: their counters can't be replaced.
		for i, el := range x {
			collectCardinality(el, path+"["+strconv.Itoa(i)+"]", key+"["+strconv.Itoa(i)+"]", nil, out, depth+1)
		}
	case mapping:
		for _, item := range x.Items() {
			k := item[0]
			ks := k.String()
			kk := ks
			if containsSymmetricValue(k) {
				kk = "*"
			}
			collectCardinality(item[1], path+"["+ks+"]", key+"["+kk+"]", func(c *lib.CardinalityCounter) {
				_ = x.SetKey(k, c)
			}, out, depth+1)
		}
	case *lib.Struct:
		for _, name := range x.AttrNames() {
			fv, err := x.Attr(name)
			if err != nil {
				continue
			}
			name := name
			collectCardinality(fv, path+"."+name, key+"."+name, func(c *lib.CardinalityCounter) {
				_ = x.SetField(name, c)
			}, out, depth+1)
		}
	case *starlarkstruct.Struct:
		for _, name := range x.AttrNames() {
			fv, err := x.Attr(name)
			if err != nil {
				continue
			}
			collectCardinality(fv, path+"."+name, key+"."+name, nil, out, depth+1)
		}
	}
}

// mapping is what dict and genericmap share.
type mapping interface {
	Items() []starlark.Tuple
	SetKey(k, v starlark.Value) error
}

func containsSymmetricValue(v starlark.Value) bool {
	switch x := v.(type) {
	case *lib.SymmetricValue, *lib.Role:
		return true
	case starlark.Tuple:
		for _, el := range x {
			if containsSymmetricValue(el) {
				return true
			}
		}
	}
	return false
}

// startCardinalityStep records where each counter is at the start of the
// step about to run.
func (p *Process) startCardinalityStep() {
	start := map[*lib.CardinalityCounter]string{}
	for _, loc := range p.cardinalityLocations() {
		if prev, ok := start[loc.value]; ok && prev != loc.path {
			start[loc.value] = "" // shared by two locations: ambiguous
			continue
		}
		start[loc.value] = loc.path
	}
	p.cardStep = lib.NewCardinalityStep(start)
}

// endCardinalityStep runs at the yield point ending the step. It adds the
// step's counter events to p.Labels, gives every location its own counter
// object so the next step can tell locations apart, and clears the step.
func (p *Process) endCardinalityStep() {
	step := p.cardStep
	p.cardStep = nil
	locs := p.cardinalityLocations()
	events := map[string]bool{}
	seen := map[*lib.CardinalityCounter]bool{}
	for _, loc := range locs {
		if label := cardinalityEvent(step, loc); label != "" {
			events[label] = true
		}
		if seen[loc.value] && loc.set != nil {
			loc.set(lib.CloneCardinalityCounter(loc.value))
		}
		seen[loc.value] = true
	}
	labels := make([]string, 0, len(events))
	for l := range events {
		labels = append(labels, l)
	}
	sort.Strings(labels)
	p.Labels = append(p.Labels, labels...)
}

func cardinalityEvent(step *lib.CardinalityStep, loc cardLocation) string {
	inc := cardIncLabel + loc.key
	if step == nil {
		return inc
	}
	if path, ok := step.Start[loc.value]; ok {
		if path == loc.path {
			return ""
		}
		return inc
	}
	base, delta, ok := loc.value.Lineage(step)
	if !ok {
		return inc
	}
	if path, ok := step.Start[base]; !ok || path != loc.path {
		return inc
	}
	switch {
	case delta < 0:
		return cardDecLabel + loc.key
	case delta > 0:
		return inc
	}
	return ""
}

// cardinalityEvents returns the counter keys a link's labels decrement and
// increment, numbering new keys in ids.
func cardinalityEvents(labels []string, ids map[string]int) (dec, inc []int) {
	id := func(k string) int {
		if v, ok := ids[k]; ok {
			return v
		}
		ids[k] = len(ids)
		return ids[k]
	}
	for _, l := range labels {
		if strings.HasPrefix(l, cardDecLabel) {
			dec = append(dec, id(l[len(cardDecLabel):]))
		} else if strings.HasPrefix(l, cardIncLabel) {
			inc = append(inc, id(l[len(cardIncLabel):]))
		}
	}
	return dec, inc
}

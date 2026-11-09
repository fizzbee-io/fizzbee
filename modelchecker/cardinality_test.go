package modelchecker

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	ast "fizz/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCardinalityEvents checks the increment and decrement events each step
// records, directly on the links, so a tracking bug can't hide behind the
// liveness verdict. Every action runs once from the same start state.
func TestCardinalityEvents(t *testing.T) {
	file, err := readAstFromFile(filepath.Join("testdata", "cardinality", "Events.json"))
	require.Nil(t, err)
	noDeadlock := false
	stateConfig := &ast.StateSpaceOptions{
		Options:           &ast.Options{MaxActions: 2, MaxConcurrentActions: 1},
		DeadlockDetection: &noDeadlock,
	}
	p := NewProcessor([]*ast.File{file}, stateConfig, false, 0, "", "", false, nil, nil, "")
	root, _, err := p.Start()
	require.Nil(t, err)
	nodes, _, _, _ := GetAllNodes(root, 2)

	// Each action starts at a link out of a yield point; its events are on
	// the links up to the next yield point.
	got := map[string]map[string]bool{}
	for _, n := range nodes {
		if n.Name != "yield" {
			continue
		}
		for _, start := range n.Outbound {
			action := start.Action
			if got[action] == nil {
				got[action] = map[string]bool{}
			}
			queue := []*Link{start}
			for len(queue) > 0 {
				link := queue[0]
				queue = queue[1:]
				for _, l := range link.Labels {
					if strings.HasPrefix(l, cardDecLabel) || strings.HasPrefix(l, cardIncLabel) {
						got[action][l] = true
					}
				}
				if link.Node.Name != "yield" {
					queue = append(queue, link.Node.Outbound...)
				}
			}
		}
	}
	events := func(action string) []string {
		var out []string
		for l := range got[action] {
			out = append(out, strings.Replace(l, "__card_", "", 1))
		}
		sort.Strings(out)
		return out
	}

	for action, want := range map[string][]string{
		// A decrement, and a net decrement within one step.
		"Dec":       {"dec:a"},
		"DecIncDec": {"dec:a"},
		// No net change: nothing.
		"IncDec": nil,
		// Copies through locals and function arguments keep their origin.
		"ThroughLocals":   {"dec:a"},
		"ThroughFunction": {"dec:a"},
		// A move between dict entries.
		"Move": {"dec:d[\"idle\"]", "inc:d[\"busy\"]"},
		// A value from elsewhere is an unknown change: an increment.
		"Copy":  {"inc:b"},
		"Reset": {"inc:a"},
		// Reading without storing changes nothing.
		"ReadOnly": nil,
		// List elements share one key.
		"ListElement": {"dec:l[*]"},
		"Swap":        {"inc:l[*]"},
		// Role fields; a symmetric role's fields share one key per field.
		"Server.Drain": {"dec:Server#0.pending"},
		"Worker.Drain": {"dec:Worker/pending"},
	} {
		_, ran := got[action]
		assert.True(t, ran, "%s never ran", action)
		assert.Equal(t, want, events(action), action)
	}
}

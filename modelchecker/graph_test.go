package modelchecker

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRemoveMergeNodes(t *testing.T) {

	t.Run("TestRemoveMergeNodes", func(t *testing.T) {
		// Example usage
		// Construct your graph as needed
		nodeC := &Node{Process: &Process{Name: "C"}}
		nodeB := &Node{Outbound: []*Link{&Link{Node: nodeC}}}
		nodeA := &Node{Process: &Process{Name: "A"}, Outbound: []*Link{&Link{Node: nodeB}}}
		nodeN1 := &Node{Process: &Process{Name: "init"}, Outbound: []*Link{&Link{Node: nodeA}}}

		// Print the original graph
		fmt.Println("Original Graph:")
		printGraph(nodeN1)

		// Remove merge nodes
		RemoveMergeNodes(nodeN1)

		// Print the modified graph
		fmt.Println("\nModified Graph:")
		printGraph(nodeN1)
		assert.Equal(t, "init", nodeN1.Process.Name)
		assert.Equal(t, "C", nodeN1.Outbound[0].Node.Process.Name)
		assert.Len(t, nodeN1.Outbound[0].Node.Outbound, 0)
	})
	t.Run("TestRemoveMergeNodes", func(t *testing.T) {
		// Example usage
		// Construct your graph as needed
		nodeF := &Node{Process: &Process{Name: "F"}}
		nodeE := &Node{Outbound: []*Link{&Link{Node: nodeF}}}
		nodeD := &Node{Process: &Process{Name: "D"}, Outbound: []*Link{&Link{Node: nodeE}}}

		nodeC := &Node{Process: &Process{Name: "C"}, Outbound: []*Link{&Link{Node: nodeD}}}
		nodeB := &Node{Outbound: []*Link{&Link{Node: nodeC}}}
		nodeA := &Node{Process: &Process{Name: "A"}, Outbound: []*Link{&Link{Node: nodeB}}}
		nodeN1 := &Node{Process: &Process{Name: "init"}, Outbound: []*Link{&Link{Node: nodeA}}}

		// Print the original graph
		fmt.Println("Original Graph:")
		printGraph(nodeN1)

		// Remove merge nodes
		RemoveMergeNodes(nodeN1)

		// Print the modified graph
		fmt.Println("\nModified Graph:")
		printGraph(nodeN1)
		assert.Equal(t, "init", nodeN1.Process.Name)
		assert.Equal(t, "C", nodeN1.Outbound[0].Node.Process.Name)
		assert.Equal(t, "F", nodeN1.Outbound[0].Node.Outbound[0].Node.Process.Name)
		assert.Len(t, nodeN1.Outbound[0].Node.Outbound[0].Node.Outbound, 0)
	})

}

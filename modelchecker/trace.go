package modelchecker

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// GuidedTrace represents a sequence of link names to follow
type GuidedTrace struct {
	LinkNames    []string
	currentIndex int
}

// ParseTraceFile reads and parses a trace file
// Format: Each non-empty, non-comment line is a link name
func ParseTraceFile(filename string) (*GuidedTrace, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open trace file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var linkNames []string
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		linkNames = append(linkNames, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading trace file: %w", err)
	}

	if len(linkNames) == 0 {
		return nil, fmt.Errorf("trace file is empty")
	}

	return &GuidedTrace{LinkNames: linkNames}, nil
}

// GetNextLinkName returns the next expected link name
func (t *GuidedTrace) GetNextLinkName() (string, error) {
	if t.currentIndex >= len(t.LinkNames) {
		return "", fmt.Errorf("trace exhausted: no more links")
	}
	return t.LinkNames[t.currentIndex], nil
}

// Advance moves to the next link in the trace
func (t *GuidedTrace) Advance() {
	t.currentIndex++
}

// IsExhausted returns true if all trace links have been executed
func (t *GuidedTrace) IsExhausted() bool {
	return t.currentIndex >= len(t.LinkNames)
}

// GetCurrentIndex returns the current link index
func (t *GuidedTrace) GetCurrentIndex() int {
	return t.currentIndex
}

// ShouldScheduleNode determines if a node should be scheduled based on the trace
// Returns true if the node should be added to the queue
// This is called before adding nodes to the queue to filter based on trace
func (p *Processor) ShouldScheduleNode(node *Node) bool {
	if len(node.guidedTrace.LinkNames) == 0 {
		return true // Not in trace mode, schedule everything
	}

	// Init node always gets scheduled (no inbound links)
	if len(node.Inbound) == 0 {
		return true
	}

	// Get the link name from the first inbound link
	linkName := node.Inbound[0].Name

	// thread-x links always get scheduled (thread continuations)
	if strings.HasPrefix(linkName, "thread-") {
		return true
	}

	// Check if trace is exhausted
	if node.guidedTrace.IsExhausted() {
		return false
	}

	// Get the next expected link name from trace
	expectedLinkName, err := node.guidedTrace.GetNextLinkName()
	if err != nil {
		return false
	}

	// Schedule only if link name matches
	if linkName == expectedLinkName {
		node.guidedTrace.Advance()
		p.guidedTrace.Advance()
		return true
	}

	return false
}

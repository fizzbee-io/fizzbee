package lib

import "testing"

func TestQueueEmpty(t *testing.T) {
	q := NewQueue[int]()
	if !q.Empty() {
		t.Fatal("new queue should be empty")
	}
	q.Enqueue(1)
	if q.Empty() {
		t.Fatal("queue with one element should not be empty")
	}
	q.Dequeue()
	if !q.Empty() {
		t.Fatal("drained queue should be empty")
	}
}

func TestStackEmpty(t *testing.T) {
	s := NewStack[int]()
	if !s.Empty() {
		t.Fatal("new stack should be empty")
	}
	s.Push(1)
	if s.Empty() {
		t.Fatal("stack with one element should not be empty")
	}
	s.Pop()
	if !s.Empty() {
		t.Fatal("drained stack should be empty")
	}
}

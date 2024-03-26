package lib

import (
    "container/list"
    "golang.org/x/sys/cpu"
    "sync"
    "unsafe"
)

const CacheLineSize = int(unsafe.Sizeof(cpu.CacheLinePad{}))
const queueBatchCapacity = CacheLineSize

type Queue[T any] struct {
    lock sync.Mutex // you don't have to do this if you don't want thread safety
    list *list.List

    count int
}

func NewQueue[T any]() *Queue[T] {
    return &Queue[T]{sync.Mutex{}, list.New(), 0}
}

func (q *Queue[T]) Enqueue(v T) *Queue[T] {
    q.lock.Lock()
    defer q.lock.Unlock()

    back := q.list.Back()
    if back == nil || len(back.Value.([]T)) >= queueBatchCapacity {
        newArray := make([]T, 0, queueBatchCapacity)
        back = q.list.PushBack(newArray)
    }
    back.Value = append(back.Value.([]T), v)
    q.count++
    return q
}

func (q *Queue[T]) Dequeue() (T, bool) {
    q.lock.Lock()
    defer q.lock.Unlock()
    front := q.list.Front()
    if front == nil {
        var v T
        return v, false
    }
    res := front.Value.([]T)[0]
    front.Value = front.Value.([]T)[1:]
    if len(front.Value.([]T)) == 0 {
        q.list.Remove(front)
    }
    q.count--
    return res, true
}

func (q *Queue[T]) Count() int {
    q.lock.Lock()
    defer q.lock.Unlock()
    return q.count
}

// Pop Don't usePreviously used a different package
// https://github.com/zeroflucs-given/generics/tree/main/collections
// So temporarily keeping the old method names for backward compatibility
// Deprecated: Use Dequeue instead
func (q *Queue[T]) Pop() (bool, T) {
    // This is left to be consistent with the stack interface
    v, found := q.Dequeue()
    return found, v
}

// Push Don't usePreviously used a different package
// https://github.com/zeroflucs-given/generics/tree/main/collections
// So temporarily keeping the old method names for backward compatibility
// Deprecated: Use Enqueue instead
func (q *Queue[T]) Push(v T) error {
    q.Enqueue(v)
    return nil
}

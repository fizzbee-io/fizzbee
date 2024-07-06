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

func (q *Queue[T]) Len() int {
    return q.Count()
}

func (q *Queue[T]) Empty() bool {
    return q.Empty()
}

func (q *Queue[T]) Add(t T) {
    q.Enqueue(t)
}

func (q *Queue[T]) Remove() (T, bool) {
    return q.Dequeue()
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
    var v T
    if front == nil {
        return v, false
    }
    res := front.Value.([]T)[0]
    front.Value.([]T)[0] = v
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

// Ensures Queue implements LinearCollection
var _ LinearCollection[interface{}] = (*(Queue[interface{}]))(nil)

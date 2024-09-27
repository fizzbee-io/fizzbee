package lib

import (
    "math/rand"
    "sync"
)

type RandomQueue[T any] struct {
    lock sync.Mutex // you don't have to do this if you don't want thread safety
    arr []T
    rand rand.Rand
}

func (r *RandomQueue[T]) Add(t T) {
    r.lock.Lock()
    defer r.lock.Unlock()
    r.arr = append(r.arr, t)
}

func (r *RandomQueue[T]) Remove() (T, bool) {
    r.lock.Lock()
    defer r.lock.Unlock()
    var v T
    n := len(r.arr)
    if n == 0 {
        return v, false
    }
    // Remove a random element
    idx := r.rand.Intn(n)
    v = r.arr[idx]
    r.arr = append(r.arr[:idx], r.arr[idx+1:]...)
    return v, true
}

func (r *RandomQueue[T]) Clear(n int) {
    // Remove the first n elements
    r.lock.Lock()
    defer r.lock.Unlock()
    if n > len(r.arr) {
        n = len(r.arr)
    }
    r.arr = r.arr[n:]
}

func (r *RandomQueue[T]) ClearAll() {
    r.lock.Lock()
    defer r.lock.Unlock()
    r.arr = r.arr[:0]
}


func (r *RandomQueue[T]) Retain(n int) {
    // Retain the first n elements and remove all others
    r.lock.Lock()
    defer r.lock.Unlock()
    if n < len(r.arr) {
        r.arr = r.arr[:n]
    }
}

func (r *RandomQueue[T]) Len() int {
    return len(r.arr)
}

func (r *RandomQueue[T]) Empty() bool {
    return len(r.arr) == 0
}

func NewRandomQueue[T any](random rand.Rand) *RandomQueue[T] {
    return &RandomQueue[T]{sync.Mutex{}, make([]T, 0), random}
}

// Ensures Queue implements LinearCollection
var _ LinearCollection[interface{}] = (*(RandomQueue[interface{}]))(nil)
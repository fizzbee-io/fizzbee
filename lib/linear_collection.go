package lib

type LinearCollection[T any] interface {
    Len() int
    Empty() bool
    Add(T)
    Remove() (T, bool)
}

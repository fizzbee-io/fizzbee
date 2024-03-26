package modelchecker

import (
    "fmt"
    "strings"
)

type ModelError struct {
    Msg string
    Process *Process
    NestedError error
}

func NewModelError(msg string, process *Process, nestedError error) *ModelError {
    return &ModelError{
        Msg: msg,
        Process: process,
        NestedError: nestedError,
    }
}

func (e *ModelError) Error() string {
    return e.Msg
}

func (e *ModelError) SprintStackTrace() string {
    builder := strings.Builder{}
    builder.WriteString(fmt.Sprintf("Error: %s\n", e.Msg))
    //if e.NestedError != nil {
    //    builder.WriteString(fmt.Sprintf("nested: %s\n", e.NestedError.Error()))
    //}
    if len(e.Process.Threads) == 0 {
        return builder.String()
    }
    thread := e.Process.currentThread()
    frames := thread.Stack.RawArrayCopy()
    for i := len(frames) - 1; i >= 0; i-- {
        builder.WriteString(fmt.Sprintf("     %s\n", frames[i].pc))
    }
    return builder.String()
}

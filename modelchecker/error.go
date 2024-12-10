package modelchecker

import (
    "fizz/proto"
    "fmt"
    "strings"
)

type ModelError struct {
    SourceInfo *proto.SourceInfo
    Msg string
    Process *Process
    NestedError error
}

func NewModelError(sourceInfo *proto.SourceInfo, msg string, process *Process, nestedError error) *ModelError {
    return &ModelError{
        SourceInfo: sourceInfo,
        Msg: msg,
        Process: process,
        NestedError: nestedError,
    }
}

func (e *ModelError) Error() string {
    prefix := ""
    if e.SourceInfo != nil && e.SourceInfo.GetStart() != nil {
        if e.SourceInfo.GetEnd() != nil && e.SourceInfo.GetEnd().GetLine() > e.SourceInfo.GetStart().GetLine(){
            prefix = fmt.Sprintf("Between lines %d and %d: ", e.SourceInfo.GetStart().GetLine(), e.SourceInfo.GetEnd().GetLine())
        } else {
            prefix = fmt.Sprintf("Line %d: ", e.SourceInfo.GetStart().GetLine())
        }
    }
    return prefix + e.Msg
}

func (e *ModelError) SprintStackTrace() string {
    builder := strings.Builder{}
    builder.WriteString(fmt.Sprintf("Error: %s\n", e.Msg))
    //if e.NestedError != nil {
    //    builder.WriteString(fmt.Sprintf("nested: %s\n", e.NestedError.Error()))
    //}
    if e.Process.GetThreadsCount() == 0 {
        return builder.String()
    }
    thread := e.Process.currentThread()
    frames := thread.Stack.RawArrayCopy()
    for i := len(frames) - 1; i >= 0; i-- {
        builder.WriteString(fmt.Sprintf("     %s\n", frames[i].pc))
    }
    return builder.String()
}

// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	stderrors "errors"
	"fmt"
	"runtime"
	"strings"

	jujuerrors "github.com/juju/errors"
)

// Traced represents an error that has had its location recorded for where the
// error was raised. This is useful for recording error stacks for errors that
// have not been annotated with contextual information as they flow through a
// programs stack.
type Traced interface {
	error

	// Location returns the path-qualified function name where the error was
	// created and the line number
	Location() (function string, line int)
}

// traced is internal implementation of the [Traced] interface for recording the
// location of an error.
type traced struct {
	// error is the wrapped error being recorded.
	error

	// function is the package path-qualified function name where the
	// error was created.
	function string

	// line is the line number the error was created on inside of function
	line int
}

// ErrorStack recursively unwinds an error chain by repeatedly calling
// [stderrors.Unwrap] until no new errors are returned. A new line is outputted
// to the resultant string for each error in the chain. If an error in the chain
// has been traced the errors location information will also be outputted with
// the error message.
func ErrorStack(err error) string {
	chain := []string{}
	for err != nil {
		switch x := err.(type) {
		case link:
		case Traced:
			file, line := x.Location()
			chain = append(chain, fmt.Sprintf(
				"%s:%d: %s", file, line, err.Error()),
			)
		// Special case to handle errors that have been traced with juju/errors.
		// This can be deleted in good time once we have removed all uses of
		// this.
		case jujuerrors.Locationer:
			file, line := x.Location()
			chain = append(chain, fmt.Sprintf(
				"%s:%d: %s", file, line, err.Error()),
			)
		default:
			chain = append(chain, err.Error())
		}
		err = stderrors.Unwrap(err)
	}

	return strings.Join(chain, "\n")
}

// getLocation records the package path-qualified function name of the error at
// callDepth stack frames above the call.
func getLocation(callDepth int) (string, int) {
	rpc := make([]uintptr, 1)
	n := runtime.Callers(callDepth+2, rpc[:])
	if n < 1 {
		return "", 0
	}
	frame, _ := runtime.CallersFrames(rpc).Next()
	return frame.Function, frame.Line
}

// Location implements the Traced interface and returns the function and line
// number for this error.
func (t traced) Location() (string, int) {
	return t.function, t.line
}

// newTraced is responsible for constructing a new [traced] error recording the
// call site at n - callDepth in the stack.
func newTraced(err error, callDepth int) traced {
	t := traced{error: err}
	t.function, t.line = getLocation(callDepth + 1)
	return t
}

// Unwrap returns the error being wrapped by this traced error.
func (t traced) Unwrap() error {
	return t.error
}

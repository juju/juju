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

// frameTracer is an implementation of [Traced] that records the program counter
// of where an error was traced. The work of resolving the trace to a location
// is deferred till a call to [frameTracer.Location] is made.
type frameTracer struct {
	// error is the wrapped error being recorded.
	error

	// pc is the program counter for the call site of the traced error.
	pc uintptr
}

// Capture is responsible for recording the location where this function was
// called from in the error supplied. This allows errors that are being passed
// up through a stack to have extra information attached to them at call sites.
//
// Captured errors should only be used in scenario's where adding extra context
// to an error is not necessary or can't be done.
//
// [ErrorStack] can be used to gather all of the capture sites about an error.
func Capture(err error) Traced {
	if err == nil {
		return nil
	}

	return newFrameTracer(err, 1)
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

// Location returns the path-qualified function name where the error was created
// and the line number.
func (f frameTracer) Location() (string, int) {
	frame, _ := runtime.CallersFrames([]uintptr{f.pc}).Next()
	return frame.Function, frame.Line
}

// newFrameTracer is responsible for constructing a new [frameTracer] error
// recording the program counter at call site skip.
func newFrameTracer(err error, skip int) frameTracer {
	pcs := make([]uintptr, 1)
	n := runtime.Callers(skip+2, pcs)
	pc := uintptr(0)
	if n != 0 {
		pc = pcs[0]
	}
	return frameTracer{
		error: err,
		pc:    pc,
	}
}

// Unwrap returns the underlying error that was wrapped by the frameTracer.
func (f frameTracer) Unwrap() error {
	return f.error
}

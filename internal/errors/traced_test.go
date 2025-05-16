// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	stdtesting "testing"

	jujuerrors "github.com/juju/errors"
)

func returnError() error {
	return New("please have my error")
}

// TestErrorStack is responsible for testing the various types of error chains
// that could be provided to ErrorStack() and asserting what should be the
// output for that chain.
func TestErrorStack(t *stdtesting.T) {
	t.Run("TracedErrorf", func(t *stdtesting.T) {
		err := Errorf("my error")
		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func1:25: my error",
			"my error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("TracedNew", func(t *stdtesting.T) {
		err := New("my error")

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func2:37: my error",
			"my error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	// AnnotatedStack is here to test the functionality of error chains when
	// there is an annotated error in the middle.
	t.Run("AnnotatedStack", func(t *stdtesting.T) {
		err := New("start error")
		err = err.Add(ConstError("added error"))
		err = Errorf("step %w", err)

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func3:54: step start error",
			"step start error",
			"start error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("jujuerrors.Trace", func(t *stdtesting.T) {
		err := New("start error")
		errVal := jujuerrors.Trace(err)
		err = Errorf("new error: %w", errVal)

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func4:70: new error: start error",
			"new error: start error",
			"github.com/juju/juju/internal/errors.TestErrorStack.func4:69: start error",
			"github.com/juju/juju/internal/errors.TestErrorStack.func4:68: start error",
			"start error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("callSiteTrace", func(t *stdtesting.T) {
		err := Errorf("func error %w", returnError())

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func5:86: func error please have my error",
			"func error please have my error",
			"github.com/juju/juju/internal/errors.returnError:17: please have my error",
			"please have my error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("GoRoutineError", func(t *stdtesting.T) {
		ch := make(chan error)

		go func() {
			ch <- New("goroutine error")
		}()

		err := <-ch
		close(ch)

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func6.1:104: goroutine error",
			"goroutine error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("NonTracedChain", func(t *stdtesting.T) {
		err := errors.New("foo bar")
		err = fmt.Errorf("traced 1: %w", err)
		err = fmt.Errorf("traced 2: %w", err)

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"traced 2: traced 1: foo bar",
			"traced 1: foo bar",
			"foo bar",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("CapturedError", func(t *stdtesting.T) {
		err := Capture(errors.New("test"))

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func8:137: test",
			"test",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})
}

// TestErrorCapture is asserting the schematics around [Capture].
func TestErrorCapture(t *stdtesting.T) {
	t.Run("CaptureNil", func(t *stdtesting.T) {
		if Capture(nil) != nil {
			t.Error("expected passing nil to Capture() will result in a nil Traced error")
		}
	})

	t.Run("Capture", func(t *stdtesting.T) {
		traced := Capture(errors.New("test"))
		funcName, line := traced.Location()

		if funcName != "github.com/juju/juju/internal/errors.TestErrorCapture.func2" {
			t.Errorf(
				"Capture() returned %q for function name, expected %q",
				funcName,
				"github.com/juju/juju/internal/errors.TestErrorCapture.func2",
			)
		}
		if line != 159 {
			t.Errorf("Capture() returned %d for line number, expected %d", line, 159)
		}
	})
}

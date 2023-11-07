// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	"reflect"
	"strings"
	"testing"

	jujuerrors "github.com/juju/errors"
)

// TestErrorStack is responsible for testing the various types of error chains
// that could be provided to ErrorStack() and asserting what should be the
// output for that chain.
func TestErrorStack(t *testing.T) {
	// BasicStack is here to make a very simple error chain with several traced
	// errors. We check that ErrorStack returns the right line number and files
	// in the stack.
	t.Run("BasicStack", func(t *testing.T) {
		err := New("my error")
		err = err.Trace()
		err = err.Trace()

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func1:24: my error",
			"github.com/juju/juju/internal/errors.TestErrorStack.func1:23: my error",
			"my error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	// AnnotatedStack is here to test the functionality of error chains when
	// there is an annotated error in the middle.
	t.Run("AnnotatedStack", func(t *testing.T) {
		err := New("start error")
		err = err.Add(ConstError("added error"))
		err = err.Trace()

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func2:42: start error",
			"start error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("jujuerrors.Trace", func(t *testing.T) {
		err := New("start error")
		errVal := jujuerrors.Trace(err)
		err = Errorf("new error: %w", errVal).Trace()

		stack := ErrorStack(err)
		expected := strings.Join([]string{
			"github.com/juju/juju/internal/errors.TestErrorStack.func3:57: new error: start error",
			"new error: start error",
			"github.com/juju/juju/internal/errors.TestErrorStack.func3:56: start error",
			"start error",
		}, "\n")
		if !reflect.DeepEqual(stack, expected) {
			t.Errorf("test call to ErrorStack() returned %#v expected %#v", stack, expected)
		}
	})

	t.Run("NonTracedChain", func(t *testing.T) {
		err := New("foo bar")
		err = Errorf("traced 1: %w", err)
		err = Errorf("traced 2: %w", err)

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
}

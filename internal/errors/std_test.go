// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	"fmt"
	"io/fs"
	"testing"
)

// complexErrorMessage is a testing error interface that also defines a basic
// message function so that the interface cannot be satisfied by any error.
type complexErrorMessage interface {
	error
	ComplexMessage() string
}

// complexError is a simple error type for testing.
type complexError struct {
	Message string
}

// Error implements the error interface.
func (c *complexError) Error() string {
	return c.Message
}

// ComplexMessage implements the complexErrorMessage interface.
func (c *complexError) ComplexMessage() string {
	return c.Message
}

// complexErrorOther is a simple error type for testing that also implements the
// complexErrorMessage interface.
type complexErrorOther struct {
	Message string
}

// As lets complexErrorOther also become a complexError when being evaluated in
// a As context.
func (c *complexErrorOther) As(e any) bool {
	if ce, ok := e.(**complexError); ok {
		*ce = &complexError{
			Message: c.Message,
		}
		return true
	}
	return false
}

// Error implements the error interface.
func (c *complexErrorOther) Error() string {
	return c.Message
}

// ComplexMessage implements the complexErrorMessage interface.
func (c *complexErrorOther) ComplexMessage() string {
	return c.Message
}

func ExampleAsType() {
	pErr := fs.PathError{
		Path: "/dev/null",
	}

	err := Errorf("wrapped path error: %w", &pErr)
	if pathErr, ok := AsType[*fs.PathError](err); ok {
		fmt.Printf("path error with path %s", pathErr.Path)
	}

	// Output:
	// path error with path /dev/null
}

// TestAsType generates a error chain containing error types that we want to
// extract with AsType and confirm that both the type is found and the type
// value returned is what we expect.
func TestAsType(t *testing.T) {
	// FindErrorAtChainEnd is wrapping a complexError within another error and
	// checking to see that we can still extract a type of *complexError when
	// the error is at the end of the chain.
	t.Run("FindErrorAtChainEnd", func(t *testing.T) {
		err := Errorf("some error with more info: %w", &complexError{"some value"})
		te, is := AsType[*complexError](err)
		if !is {
			t.Fatal("AsType for *complexError not found in chain")
		}

		if te.Message != "some value" {
			t.Error("Expected AsType error value to be equal to \"some value\"")
		}
	})

	// FindErrorAtChainMiddle is wrapping a complexError within another error
	// and checking to see that we can still extract a type of *complexError
	// when the error is in the middle of the chain.
	t.Run("FindErrorAtChainMiddle", func(t *testing.T) {
		err := Errorf("start error: %w: %w", &complexError{"middle"}, New("end"))
		te, is := AsType[*complexError](err)
		if !is {
			t.Fatal("AsType for *complexError not found in chain")
		}
		if te.Message != "middle" {
			t.Error("Expected AsType error value to be equal to \"middle\"")
		}
	})

	// FindErrorAtChainStart is wrapping a complexError within another error
	// and checking to see that we can still extract a type of *complexError
	// when the error is at the start of the chain.
	t.Run("FindErrorAtChainStart", func(t *testing.T) {
		te, is := AsType[*complexError](&complexError{"start"})
		if !is {
			t.Fatal("AsType for *complexError not found in chain")
		}
		if te.Message != "start" {
			t.Error("Expected AsType error value to be equal to \"start\"")
		}
	})

	// AsTypeInChainWithInterface is wrapping a complexError within another
	// error and checking to see that we can extract it back out with a
	// interface type of complexErrorMessage.
	t.Run("AsTypeInChainWithInterface", func(t *testing.T) {
		ce := &complexError{"error message"}
		err := fmt.Errorf("error wrapping: %w", ce)

		te, is := AsType[complexErrorMessage](err)
		if !is {
			t.Fatal("AsType for complexErrorMessage not found in chain")
		}
		if te.ComplexMessage() != "error message" {
			t.Error("Expected AsType error value to be equal to \"error message\"")
		}
	})

	// AsTypeInChainWithAsImplementation is wrapping a complexErrorOther within
	// another error and checking to see if we can extract the same error out as
	// another error type based on the fact that complexErrorOther implements
	// the As() method.
	t.Run("AsTypeInChainWithAsImplementation", func(t *testing.T) {
		ce := &complexErrorOther{"error message"}
		err := fmt.Errorf("error wrapping: %w", ce)

		te, is := AsType[*complexError](err)
		if !is {
			t.Fatal("AsType for complexError not found in chain")
		}
		if te.Message != "error message" {
			t.Error("Expected AsType error value to be equal to \"error message\"")
		}
	})

	// AsTypeNotFound is wrapping a complexError within another error and
	// checking to see that we can not extract a error from the chain for a type
	// that does not exist.
	t.Run("AsTypeNotFound", func(t *testing.T) {
		ce := &complexError{"error message"}
		err := fmt.Errorf("error wrapping: %w", ce)

		te, is := AsType[*complexErrorOther](err)
		if is {
			t.Error("AsType for complexErrorOther was found in the chain")
		}

		if te != nil {
			t.Error("AsType did not return zero value for not found type")
		}
	})
}

func ExampleHasType() {
	pErr := fs.PathError{
		Path: "/dev/null",
	}

	err := Errorf("wrapped path error: %w", &pErr)
	fmt.Println(HasType[*fs.PathError](err))

	// Output:
	// true
}

// TestHasType generates a error chain containing error types that we want to
// extract with AsType and confirm that type is found.
func TestHasType(t *testing.T) {
	// FindErrorAtChainEnd is wrapping a complexError within another error and
	// checking to see that *complexError exists in the chain.
	t.Run("FindErrorAtChainEnd", func(t *testing.T) {
		err := Errorf("some error with more info: %w", &complexError{"some value"})
		is := HasType[*complexError](err)
		if !is {
			t.Fatal("HasType for *complexError not found in chain")
		}
	})

	// FindErrorAtChainMiddle is wrapping a complexError within another error
	// and checking to see that *complexError exists in the chain.
	t.Run("FindErrorAtChainMiddle", func(t *testing.T) {
		err := Errorf("start error: %w: %w", &complexError{"middle"}, New("end"))
		is := HasType[*complexError](err)
		if !is {
			t.Fatal("HasType for *complexError not found in chain")
		}
	})

	// FindErrorAtChainStart is wrapping a complexError within another error
	// and checking to see that *complexError exists in the chain.
	t.Run("FindErrorAtChainStart", func(t *testing.T) {
		is := HasType[*complexError](&complexError{"start"})
		if !is {
			t.Fatal("HasType for *complexError not found in chain")
		}
	})

	// AsTypeInChainWithInterface is wrapping a complexError within another
	// error and checking to see that we can find the error with an interface
	// type.
	t.Run("HasTypeInChainWithInterface", func(t *testing.T) {
		ce := &complexError{"error message"}
		err := fmt.Errorf("error wrapping: %w", ce)

		is := HasType[complexErrorMessage](err)
		if !is {
			t.Fatal("HasType for complexErrorMessage not found in chain")
		}
	})

	// HasTypeInChainWithAsImplementation is wrapping a complexErrorOther within
	// another error and checking to see if we can find the error as a different
	// type based on the fact that the other type implements the As() function.
	t.Run("AsTypeInChainWithAsImplementation", func(t *testing.T) {
		ce := &complexErrorOther{"error message"}
		err := fmt.Errorf("error wrapping: %w", ce)

		is := HasType[*complexError](err)
		if !is {
			t.Fatal("HasType for complexError not found in chain")
		}
	})

	// HasTypeNotFound is wrapping a complexError within another error and
	// checking to see that HasType() returns false for a error that doesn't
	// exist in the chain.
	t.Run("HasTypeNotFound", func(t *testing.T) {
		ce := &complexError{"error message"}
		err := fmt.Errorf("error wrapping: %w", ce)

		is := HasType[*complexErrorOther](err)
		if is {
			t.Error("AsType for complexErrorOther was found in the chain")
		}
	})
}

// TestErrorf is a placeholder for Errorf testing. We skip this test for now as
// we are just proxying stdlib.
func TestErrorf(t *testing.T) {
	t.SkipNow()
}

// TestIs is a placeholder for Is testing. We skip this test for now as we are
// just proxying stdlib.
func TestIs(t *testing.T) {
	t.SkipNow()
}

func TestIsOf(t *testing.T) {
	t.Run("ReturnsFalseForEmptyTargets", func(t *testing.T) {
		err := New("test error")
		if IsOneOf(err) {
			t.Errorf("IsOf with empty target list should return false")
		}
	})

	t.Run("IsOfMultiple", func(t *testing.T) {
		type1 := New("type 1")
		type2 := New("type 2")
		err := Errorf("%w", type1)

		if !IsOneOf(err, type2, type1) {
			t.Errorf("IsOf expected to find type1 in error chain")
		}
	})
}

// TestJoin tests the Join function to ensure it correctly combines errors and verifies their presence using the Is function.
func TestJoin(t *testing.T) {
	fooErr := fmt.Errorf("foo")
	barErr := fmt.Errorf("bar")
	err := Join(fooErr, barErr)
	if !Is(err, fooErr) || !Is(err, barErr) {
		t.Errorf("Join(%v, %v) = %v, want: Is(%v, fooErr) && Is(%v, barErr)", fooErr, barErr, err, err, err)
	}
}

// TestJoinWithMixedNil validates the Join function when given a mix of non-nil and nil errors,
// ensuring that nil errors are discarded and the resultant error wraps all non-nil errors correctly.
func TestJoinWithMixedNil(t *testing.T) {
	fooErr := fmt.Errorf("foo")
	barErr := fmt.Errorf("bar")
	err := Join(fooErr, nil, barErr)
	if !Is(err, fooErr) || !Is(err, barErr) {
		t.Errorf("Join(%v, %v) = %v, want: Is(%v, fooErr) && Is(%v, barErr)", fooErr, barErr, err, err, err)
	}
}

// TestJoinWithEmptyArray verifies that calling Join with a nil or empty array results in a nil error.
func TestJoinWithEmptyArray(t *testing.T) {
	if err := Join(); err != nil {
		t.Errorf("Join(nil) = %v, want: nil", err)
	}
}

// TestJoinWithArrayOfNil verifies that the Join function correctly handles an array of nil errors by returning nil.
func TestJoinWithArrayOfNil(t *testing.T) {
	if err := Join(nil, nil, nil); err != nil {
		t.Errorf("Join(nil,nil,nil) = %v, want: nil", err)
	}
}

// TestNew is a placeholder for New testing. We skip this test for now as we
// are just proxying stdlib.
func TestNew(t *testing.T) {
	t.SkipNow()
}

// TestUnwrap is a placeholder for Unwrap testing. We skip this test for now as
// we are just proxying stdlib.
func TestUnwrap(t *testing.T) {
	t.SkipNow()
}

// TestIsOneOf is testing [IsOneOf] and the various combinations of errors and
// targets we are likely to expect.
func TestIsOneOf(t *testing.T) {
	t.Run("ReturnsFalseForEmptyTargets", func(t *testing.T) {
		err := New("test error")
		if IsOneOf(err) {
			t.Error("IsOf with empty target list should return false")
		}
	})

	t.Run("ReturnsFalseForNilError", func(t *testing.T) {
		err := New("test error")
		if IsOneOf(nil, err) {
			t.Error("IsOneOf with nil error should return false")
		}

		if IsOneOf(nil) {
			t.Error("IsOneOf with nil error with empty target list should return false")
		}
	})

	t.Run("IsOneOfMultiple", func(t *testing.T) {
		type1 := New("type 1")
		type2 := New("type 2")
		err := Errorf("%w", type1)

		if !IsOneOf(err, type2, type1) {
			t.Error("IsOf expected to find type1 in error chain")
		}
	})
}

// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import testing "testing"

// TestLinkErrorWrap asserts that when wrapping a error with link{} the Error()
// method returns verbatim that of what the wrapped errors Error() method would
// have returned.
func TestLinkErrorWrap(t *testing.T) {
	err := New("i am a teapot")
	lerr := link{err}

	if lerr.Error() != err.Error() {
		t.Errorf("link.Error() %q is not equal to %q",
			lerr.Error(), err.Error(),
		)
	}
}

// TestLinkErrorAdd asserts the Add implementation of link to make sure that it
// is conforming to the Add() method of the Error interface.
func TestLinkErrorAdd(t *testing.T) {
	err := New("i am a teapot")
	cerr := ConstError("not found")

	e := link{err}.Add(cerr)

	// AssertErrorMessage is checking that adding a error does not change the
	// error message returned by Error()
	t.Run("AssertErrorMessage", func(t *testing.T) {
		err := New("my error")
		added := err.Add(ConstError("test add"))

		if added.Error() != err.Error() {
			t.Errorf("Expected that calling Add() on err does not change Error() message, got %q expected %q",
				added.Error(), err.Error(),
			)
		}
	})

	// AssertIsSupport is asserting that Is returns true of false for errors
	// that do exist in the chain.
	t.Run("AssertIsSupport", func(t *testing.T) {

		// Test that we satisfy is for both errors.
		if !Is(e, cerr) {
			t.Errorf("Add(ex) -> Error does not satisfy Is(err, ex)")
		}
		if !Is(e, err) {
			t.Errorf("ey.Add(ex) -> Error does not satisfy Is(err, ey)")
		}
	})

	t.Run("AssertAsSupport", func(t *testing.T) {
		var asErr ConstError
		if !As(e, &asErr) {
			t.Errorf("Add(ex) -> Error does not satisfy As(err, ex)")
		}
	})

	t.Run("AssertUnwrap()", func(t *testing.T) {
		for unwrapErr := Unwrap(e); unwrapErr != nil; unwrapErr = Unwrap(unwrapErr) {
			if unwrapErr == cerr {
				t.Errorf("calling Unwrap() on err from Add(ex) produced the error ex when it shouldn't have appeared")
			}
		}
	})
}

func TestAnnotatedErrorWrap(t *testing.T) {
	err := New("ipv6 is great")
	notFound := ConstError("not found")
	aerror := annotated{err, notFound}

	t.Run("AssertErrorMessage", func(t *testing.T) {
		if aerror.Error() != err.Error() {
			t.Errorf("aggregate.Error() %q is not equal to %q",
				aerror.Error(), err.Error(),
			)
		}
	})

	t.Run("AssertIsSupport", func(t *testing.T) {
		// Test that we satisfy is for both errors.
		if !Is(aerror, notFound) {
			t.Errorf("annotated error Is() does not return true for annotation error")
		}

		if !Is(aerror, err) {
			t.Errorf("annotated error Is() does not return true for wrapped error")
		}
	})

	t.Run("AssertAsSupport", func(t *testing.T) {
		var cerr ConstError
		if !As(aerror, &cerr) {
			t.Errorf("annotated error As() does not return true for annotation error")
		}
	})
}

// TestAnnotatedErrorUnwrap tests that the Unwrap() []error method of annotated
// error returns both the correct number of errors (2) and that the order of
// the errors slice is what we expect.
func TestAnnotatedErrorUnwrap(t *testing.T) {
	err := New("ipv6 is great")
	notFound := ConstError("not found")
	aerror := annotated{err, notFound}

	errs := aerror.Unwrap()
	if len(errs) != 2 {
		t.Fatalf("annotated error Unwrap() returned %d errors, expected 2", len(errs))
	}

	if !Is(errs[0], err) {
		t.Errorf("annotated error Unwrap()[0] Is does not return true for wrapped error")
	}
	if !Is(errs[1], notFound) {
		t.Errorf("annotated error Unwrap()[0] Is does not return true for wrapped error")
	}
}

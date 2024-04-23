// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"fmt"

	dqlite "github.com/canonical/go-dqlite/driver"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"
)

type asError struct {
	Message string
}

func (a asError) Error() string {
	return a.Message
}

type errorsSuite struct{}

var _ = gc.Suite(&errorsSuite{})

// TestCoerceForNilError checks that if you pass a nil error to CoerceError you
// get back a nil error.
func (e *errorsSuite) TestCoerceForNilError(c *gc.C) {
	err := CoerceError(nil)
	c.Check(err, jc.ErrorIsNil)
}

// TestMaskErrorIsHidesSqlErrors is testing that if we construct a maskError
// with with an error chain that contains either sqlite or dqlite errors calls
// to [errors.Is] will return false and mask the errors presence.
func (e *errorsSuite) TestMaskErrorIsHidesSqlErrors(c *gc.C) {
	tests := []struct {
		Name  string
		Error error
		Rval  bool
	}{
		{
			Name: "Test sqlite3 errors are hidden from Is()",
			Error: sqlite3.Error{
				Code:         sqlite3.ErrAbort,
				ExtendedCode: sqlite3.ErrBusyRecovery,
			},
			Rval: false,
		},
		{
			Name: "Test dqlite errors are hidden from Is()",
			Error: dqlite.Error{
				Code:    dqlite.ErrBusy,
				Message: "something went wrong",
			},
			Rval: false,
		},
	}

	for _, test := range tests {
		err := maskError{fmt.Errorf("%q %w", test.Name, test.Error)}
		c.Check(test.Rval, gc.Equals, errors.Is(err, test.Error), gc.Commentf(test.Name))
	}
}

// TestMaskErrorIsNoHide is here to check that if maskError contains non sql
// errors within its chain that it doesn't attempt to hide their existence.
func (e *errorsSuite) TestMaskErrorIsNoHide(c *gc.C) {
	origError := errors.New("test error")
	err := fmt.Errorf("wrap orig error: %w", origError)
	maskErr := maskError{err}
	c.Check(errors.Is(maskErr, origError), jc.IsTrue)

	sqlErr := sqlite3.Error{
		Code:         sqlite3.ErrAbort,
		ExtendedCode: sqlite3.ErrBusyRecovery,
	}

	err = fmt.Errorf("double wrap %w %w", sqlErr, origError)
	maskErr = maskError{err}
	c.Check(errors.Is(maskErr, origError), jc.IsTrue)
}

// TestMaskErrorAsNoHide is here to check that if maskError contains non sql
// errors within its chain that it doesn't attempt to hide their existence.
func (e *errorsSuite) TestMaskErrorAsNoHide(c *gc.C) {
	origError := asError{"ipv6 rocks"}
	err := fmt.Errorf("wrap orig error: %w", origError)
	maskErr := maskError{err}

	var rval asError
	c.Check(errors.As(maskErr, &rval), jc.IsTrue)

	sqlErr := sqlite3.Error{
		Code:         sqlite3.ErrAbort,
		ExtendedCode: sqlite3.ErrBusyRecovery,
	}

	err = fmt.Errorf("double wrap %w %w", sqlErr, origError)
	maskErr = maskError{err}
	c.Check(errors.As(maskErr, &rval), jc.IsTrue)
}

// TestMaskErrorAsHidesSqlLiteErrors is here to assert that if we try and
// extract a sqlite error from a [maskError] that we get back false even though
// it does exist.
func (e *errorsSuite) TestMaskErrorAsHidesSqlLiteErrors(c *gc.C) {
	var rval sqlite3.Error
	err := maskError{sqlite3.Error{
		Code:         sqlite3.ErrAbort,
		ExtendedCode: sqlite3.ErrBusyRecovery,
	}}

	c.Check(errors.As(err, &rval), jc.IsFalse)
}

// TestMaskErrorAsHidesSqlLiteErrors is here to assert that if we try and
// extract a dqlite error from a [maskError] that we get back false even though
// it does exist.
func (e *errorsSuite) TestMaskErrorAsHidesDQLiteErrors(c *gc.C) {
	var rval dqlite.Error
	err := maskError{dqlite.Error{
		Code:    dqlite.ErrBusy,
		Message: "something went wrong",
	}}

	c.Check(errors.As(err, &rval), jc.IsFalse)
}

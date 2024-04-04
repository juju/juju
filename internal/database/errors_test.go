// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package database

import (
	"errors"

	dqlite "github.com/canonical/go-dqlite/driver"
	"github.com/juju/testing"
	"github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"
)

type errorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&errorSuite{})

func (s *errorSuite) TestIsErrConstraintUnique(c *gc.C) {
	c.Check(IsErrConstraintUnique(nil), checkers.IsFalse)

	dErr := dqlite.Error{}
	c.Check(IsErrConstraintUnique(dErr), checkers.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(IsErrConstraintUnique(dErr), checkers.IsTrue)

	sErr := sqlite3.Error{}
	c.Check(IsErrConstraintUnique(sErr), checkers.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(IsErrConstraintUnique(sErr), checkers.IsTrue)
}

func (s *errorSuite) TestIsErrCode(c *gc.C) {
	c.Check(isErrCode(nil, sqlite3.ErrConstraintCheck), checkers.IsFalse)
	c.Check(isErrCode(sqlite3.ErrBusy, sqlite3.ErrConstraintCheck), checkers.IsFalse)
	c.Check(isErrCode(sqlite3.ErrLocked, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	dErr := dqlite.Error{}
	c.Check(isErrCode(dErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(isErrCode(dErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	dErr.Code = int(sqlite3.ErrConstraintUnique)
	c.Check(isErrCode(dErr, sqlite3.ErrConstraintUnique), checkers.IsTrue)

	sErr := sqlite3.Error{}
	c.Check(isErrCode(sErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(isErrCode(sErr, sqlite3.ErrConstraintCheck), checkers.IsFalse)

	sErr.ExtendedCode = sqlite3.ErrConstraintUnique
	c.Check(isErrCode(sErr, sqlite3.ErrConstraintUnique), checkers.IsTrue)
}

// TestIsError checks that IsError is reporting true for dqlite and sqlite based
// errors.
func (s *errorSuite) TestIsError(c *gc.C) {
	tests := []struct {
		Name string
		T    any
		Rval bool
	}{
		{
			Name: "Check DQlite pointer errors",
			T: &dqlite.Error{
				Code:    dqlite.ErrBusy,
				Message: "some message",
			},
			Rval: true,
		},
		{
			Name: "Check DQlite non pointer errors",
			T: dqlite.Error{
				Code:    dqlite.ErrBusy,
				Message: "some message",
			},
			Rval: true,
		},
		{
			Name: "Check SQlite pointer errors",
			T: &sqlite3.Error{
				Code:         sqlite3.ErrAbort,
				ExtendedCode: sqlite3.ErrBusyRecovery,
			},
			Rval: true,
		},
		{
			Name: "Check SQlite non pointer errors",
			T: sqlite3.Error{
				Code:         sqlite3.ErrAbort,
				ExtendedCode: sqlite3.ErrBusyRecovery,
			},
			Rval: true,
		},
		{
			Name: "Check non database errors",
			T:    errors.New("I am a teapot"),
			Rval: false,
		},
		{
			Name: "Check nil target",
			T:    nil,
			Rval: false,
		},
	}

	for _, test := range tests {
		c.Check(IsError(test.T), gc.Equals, test.Rval, gc.Commentf(test.Name))
	}
}

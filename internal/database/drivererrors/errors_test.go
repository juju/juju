// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package drivererrors

import (
	dqlite "github.com/canonical/go-dqlite/v2/driver"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/mattn/go-sqlite3"

	"github.com/juju/juju/internal/database/driver"
)

type errorSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&errorSuite{})

func (s *errorSuite) TestIsErrRetryable(c *tc.C) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "driver error busy error",
			err:      &driver.Error{Code: driver.ErrBusy},
			expected: true,
		},
		{
			name:     "sqlite3 error busy error",
			err:      sqlite3.ErrBusy,
			expected: true,
		},
		{
			name:     "sqlite3 err locked",
			err:      sqlite3.ErrBusy,
			expected: true,
		},
		{
			name:     "database is locked",
			err:      errors.New("database is locked"),
			expected: true,
		},
		{
			name:     "cannot start a transaction within a transaction",
			err:      errors.New("cannot start a transaction within a transaction"),
			expected: true,
		},
		{
			name:     "bad connection",
			err:      errors.New("bad connection"),
			expected: true,
		},
		{
			name:     "checkpoint in progress",
			err:      errors.New("checkpoint in progress"),
			expected: true,
		},
	}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		c.Check(IsErrRetryable(test.err), tc.Equals, test.expected)
	}
}

func (s *errorSuite) TestIsConstraintError(c *tc.C) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name: "sqlite3 err locked",
			err: dqlite.Error{
				Code: int(sqlite3.ErrLocked),
			},
			expected: false,
		},
		{
			name: "sqlite3 err busy",
			err: dqlite.Error{
				Code: int(sqlite3.ErrBusy),
			},
			expected: false,
		},
		{
			name: "constraint check",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintCheck),
			},
			expected: true,
		},
		{
			name: "constraint foreign key",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintForeignKey),
			},
			expected: true,
		},
		{
			name: "constraint not null",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintNotNull),
			},
			expected: true,
		},
		{
			name: "constraint primary key",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintPrimaryKey),
			},
			expected: true,
		},
		{
			name: "constraint trigger",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintTrigger),
			},
			expected: true,
		},
		{
			name: "constraint unique",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintUnique),
			},
			expected: true,
		},
		{
			name: "constraint row id",
			err: dqlite.Error{
				Code: int(sqlite3.ErrConstraintRowID),
			},
			expected: true,
		},
	}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		c.Check(IsConstraintError(test.err), tc.Equals, test.expected)
	}
}

// TestIsError checks that IsError is reporting true for dqlite
// and sqlite based errors.
func (s *errorSuite) TestIsError(c *tc.C) {
	tests := []struct {
		Name string
		Err  error
		Rval bool
	}{
		{
			Name: "Check DQlite pointer errors",
			Err: &dqlite.Error{
				Code:    dqlite.ErrBusy,
				Message: "some message",
			},
			Rval: false,
		},
		{
			Name: "Check SQlite non pointer errors",
			Err: sqlite3.Error{
				Code:         sqlite3.ErrAbort,
				ExtendedCode: sqlite3.ErrBusyRecovery,
			},
			Rval: true,
		},
		{
			Name: "Check DQlite non pointer errors",
			Err: dqlite.Error{
				Code:    dqlite.ErrBusy,
				Message: "some message",
			},
			Rval: true,
		},
		{
			Name: "Check SQlite pointer errors",
			Err: &sqlite3.Error{
				Code:         sqlite3.ErrAbort,
				ExtendedCode: sqlite3.ErrBusyRecovery,
			},
			Rval: false,
		},
		{
			Name: "Check non database errors",
			Err:  errors.New("I am a teapot"),
			Rval: false,
		},
		{
			Name: "Check nil target",
			Err:  nil,
			Rval: false,
		},
	}

	for _, test := range tests {
		c.Check(IsError(test.Err), tc.Equals, test.Rval, tc.Commentf(test.Name))
	}
}

// TestIsErrorTarget checks that IsErrorTarget is reporting true for dqlite
// and sqlite based errors.
func (s *errorSuite) TestIsErrorTarget(c *tc.C) {
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
		c.Check(IsErrorTarget(test.T), tc.Equals, test.Rval, tc.Commentf(test.Name))
	}
}

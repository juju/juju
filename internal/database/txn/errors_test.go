// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package txn_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/database/driver"
	"github.com/juju/juju/internal/database/txn"
)

type isErrRetryableSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&isErrRetryableSuite{})

func (s *isErrRetryableSuite) TestIsErrRetryable(c *gc.C) {
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
		c.Check(txn.IsErrRetryable(test.err), gc.Equals, test.expected)
	}
}

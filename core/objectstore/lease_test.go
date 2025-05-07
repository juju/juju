// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	coreerrors "github.com/juju/juju/core/errors"
)

type LeaseSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&LeaseSuite{})

func (s *LeaseSuite) TestParseLeaseHolderName(c *tc.C) {
	tests := []struct {
		name     string
		expected error
	}{{
		name:     "objectstore",
		expected: nil,
	}, {
		name:     "foo",
		expected: coreerrors.NotValid,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		c.Assert(ParseLeaseHolderName(test.name), tc.ErrorIs, test.expected)
	}
}

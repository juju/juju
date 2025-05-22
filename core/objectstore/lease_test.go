// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type LeaseSuite struct {
	testhelpers.IsolationSuite
}

func TestLeaseSuite(t *testing.T) {
	tc.Run(t, &LeaseSuite{})
}

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

// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

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
		name:     "0",
		expected: nil,
	}, {
		name:     "d54d55a8-bb53-45f9-abf9-2ba79329f111",
		expected: nil,
	}, {
		name:     "",
		expected: coreerrors.NotValid,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		c.Assert(ParseLeaseHolderName(test.name), tc.ErrorIs, test.expected)
	}
}

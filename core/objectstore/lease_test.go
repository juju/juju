// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
)

type LeaseSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LeaseSuite{})

func (s *LeaseSuite) TestParseLeaseHolderName(c *gc.C) {
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
		c.Assert(ParseLeaseHolderName(test.name), jc.ErrorIs, test.expected)
	}
}

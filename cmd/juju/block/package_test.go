// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package block_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

func TestAll(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}

type ProtectionCommandSuite struct {
	testing.FakeJujuHomeSuite
}

func (s *ProtectionCommandSuite) assertErrorMatches(c *gc.C, err error, expected string) {
	c.Assert(
		err,
		gc.ErrorMatches,
		expected)
}

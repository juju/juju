// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugins

import (
	"testing"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
)

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

type PluginsSuite struct{}

var _ = gc.Suite(&PluginsSuite{})

func (s *PluginsSuite) TestRegisterPlugin(c *gc.C) {
	Register("foo")
	c.Assert(IsBuiltIn("foo"), jc.IsTrue)
	c.Assert(IsBuiltIn("bar"), jc.IsFalse)
}

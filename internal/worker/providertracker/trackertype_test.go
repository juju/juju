// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type trackerTypeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&trackerTypeSuite{})

func (s *trackerTypeSuite) TestSingularNamespace(c *gc.C) {
	single := SingularType("foo")
	namespace, ok := single.SingularNamespace()
	c.Assert(ok, jc.IsTrue)
	c.Check(namespace, gc.Equals, "foo")
}

func (s *trackerTypeSuite) TestMultiType(c *gc.C) {
	namespace, ok := MultiType().SingularNamespace()
	c.Assert(ok, jc.IsFalse)
	c.Check(namespace, gc.Equals, "")
}

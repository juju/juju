// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
)

type trackerTypeSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&trackerTypeSuite{})

func (s *trackerTypeSuite) TestSingularNamespace(c *tc.C) {
	single := SingularType("foo")
	namespace, ok := single.SingularNamespace()
	c.Assert(ok, tc.IsTrue)
	c.Check(namespace, tc.Equals, "foo")
}

func (s *trackerTypeSuite) TestMultiType(c *tc.C) {
	namespace, ok := MultiType().SingularNamespace()
	c.Assert(ok, tc.IsFalse)
	c.Check(namespace, tc.Equals, "")
}

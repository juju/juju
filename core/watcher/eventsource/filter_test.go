// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
)

type filterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&filterSuite{})

func (s *filterSuite) TestPredicateFilter(c *gc.C) {
	predicate := func(s string) bool {
		return s == "bar"
	}
	f := PredicateFilter("foo", changestream.All, predicate)
	c.Check(f.Namespace(), gc.Equals, "foo")
	c.Check(f.ChangeMask(), gc.Equals, changestream.All)

	received := f.ChangePredicate()
	c.Assert(received, gc.NotNil)
	c.Check(received("bar"), jc.IsTrue)
	c.Check(received("foo"), jc.IsFalse)
}

func (s *filterSuite) TestNamespaceFilter(c *gc.C) {
	f := NamespaceFilter("foo", changestream.All)
	c.Check(f.Namespace(), gc.Equals, "foo")
	c.Check(f.ChangeMask(), gc.Equals, changestream.All)

	received := f.ChangePredicate()
	c.Assert(received, gc.NotNil)
	c.Check(received("bar"), jc.IsTrue)
	c.Check(received("foo"), jc.IsTrue)
}

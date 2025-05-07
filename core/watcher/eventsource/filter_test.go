// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/changestream"
)

type filterSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&filterSuite{})

func (s *filterSuite) TestPredicateFilter(c *tc.C) {
	predicate := func(s string) bool {
		return s == "bar"
	}
	f := PredicateFilter("foo", changestream.All, predicate)
	c.Check(f.Namespace(), tc.Equals, "foo")
	c.Check(f.ChangeMask(), tc.Equals, changestream.All)

	received := f.ChangePredicate()
	c.Assert(received, tc.NotNil)
	c.Check(received("bar"), tc.IsTrue)
	c.Check(received("foo"), tc.IsFalse)
}

func (s *filterSuite) TestNamespaceFilter(c *tc.C) {
	f := NamespaceFilter("foo", changestream.All)
	c.Check(f.Namespace(), tc.Equals, "foo")
	c.Check(f.ChangeMask(), tc.Equals, changestream.All)

	received := f.ChangePredicate()
	c.Assert(received, tc.NotNil)
	c.Check(received("bar"), tc.IsTrue)
	c.Check(received("foo"), tc.IsTrue)
}

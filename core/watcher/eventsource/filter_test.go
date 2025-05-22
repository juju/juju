// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/goleak"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/internal/testhelpers"
)

type filterSuite struct {
	testhelpers.IsolationSuite
}

func TestFilterSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &filterSuite{})
}

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

func (s *filterSuite) TestContainsPredicate(c *tc.C) {
	predicate := ContainsPredicate([]string{"foo", "bar"})
	c.Check(predicate("foo"), tc.IsTrue)
	c.Check(predicate("bar"), tc.IsTrue)
	c.Check(predicate("baz"), tc.IsFalse)
}

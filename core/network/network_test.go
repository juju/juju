// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"sort"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/testing"
)

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestGenerateVirtualMACAddress(c *gc.C) {
	mac := network.GenerateVirtualMACAddress()
	c.Check(mac, gc.Matches, "^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$")
}

func (NetworkSuite) TestIDSetSize(c *gc.C) {
	// Empty sets are empty.
	s := network.MakeIDSet()
	c.Assert(s.Size(), gc.Equals, 0)

	// Size returns number of unique values.
	s = network.MakeIDSet("foo", "foo", "bar")
	c.Assert(s.Size(), gc.Equals, 2)
}

func (NetworkSuite) TestIDSetEmpty(c *gc.C) {
	s := network.MakeIDSet()
	assertValues(c, s)
}

func (NetworkSuite) TestIDSetInitialValues(c *gc.C) {
	values := []network.Id{"foo", "bar", "baz"}
	s := network.MakeIDSet(values...)
	assertValues(c, s, values...)
}

func (NetworkSuite) TestIDSetIsEmpty(c *gc.C) {
	// Empty sets are empty.
	s := network.MakeIDSet()
	c.Assert(s.IsEmpty(), gc.Equals, true)

	// Non-empty sets are not empty.
	s = network.MakeIDSet("foo")
	c.Assert(s.IsEmpty(), gc.Equals, false)
}

func (NetworkSuite) TestIDSetAdd(c *gc.C) {
	s := network.MakeIDSet()
	s.Add("foo")
	s.Add("foo")
	s.Add("bar")
	assertValues(c, s, "foo", "bar")
}

func (NetworkSuite) TestIDSetContains(c *gc.C) {
	s := network.MakeIDSet("foo", "bar")
	c.Assert(s.Contains("foo"), gc.Equals, true)
	c.Assert(s.Contains("bar"), gc.Equals, true)
	c.Assert(s.Contains("baz"), gc.Equals, false)
}

// Helper methods for the tests.
func assertValues(c *gc.C, s network.IDSet, expected ...network.Id) {
	values := s.Values()

	// Expect an empty slice, not a nil slice for values.
	if expected == nil {
		expected = make([]network.Id, 0)
	}

	sort.Slice(expected, func(i, j int) bool {
		return expected[i] < expected[j]
	})
	sort.Slice(values, func(i, j int) bool {
		return values[i] < values[j]
	})

	c.Assert(values, gc.DeepEquals, expected)
	c.Assert(s.Size(), gc.Equals, len(expected))

	// Check the sorted values too.
	sorted := s.SortedValues()
	c.Assert(sorted, gc.DeepEquals, expected)
}

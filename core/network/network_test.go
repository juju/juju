// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"sort"

	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/core/network"
)

type NetworkSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestGenerateVirtualMACAddress(c *tc.C) {
	mac := network.GenerateVirtualMACAddress()
	c.Check(mac, tc.Matches, "^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$")
}

func (NetworkSuite) TestIDSetSize(c *tc.C) {
	// Empty sets are empty.
	s := network.MakeIDSet()
	c.Assert(s.Size(), tc.Equals, 0)

	// Size returns number of unique values.
	s = network.MakeIDSet("foo", "foo", "bar")
	c.Assert(s.Size(), tc.Equals, 2)
}

func (NetworkSuite) TestIDSetEmpty(c *tc.C) {
	s := network.MakeIDSet()
	assertValues(c, s)
}

func (NetworkSuite) TestIDSetInitialValues(c *tc.C) {
	values := []network.Id{"foo", "bar", "baz"}
	s := network.MakeIDSet(values...)
	assertValues(c, s, values...)
}

func (NetworkSuite) TestIDSetIsEmpty(c *tc.C) {
	// Empty sets are empty.
	s := network.MakeIDSet()
	c.Assert(s.IsEmpty(), tc.Equals, true)

	// Non-empty sets are not empty.
	s = network.MakeIDSet("foo")
	c.Assert(s.IsEmpty(), tc.Equals, false)
}

func (NetworkSuite) TestIDSetAdd(c *tc.C) {
	s := network.MakeIDSet()
	s.Add("foo")
	s.Add("foo")
	s.Add("bar")
	assertValues(c, s, "foo", "bar")
}

func (NetworkSuite) TestIDSetContains(c *tc.C) {
	s := network.MakeIDSet("foo", "bar")
	c.Assert(s.Contains("foo"), tc.Equals, true)
	c.Assert(s.Contains("bar"), tc.Equals, true)
	c.Assert(s.Contains("baz"), tc.Equals, false)
}

func (NetworkSuite) TestIDSetDifference(c *tc.C) {
	s1 := network.MakeIDSet("foo", "bar")
	s2 := network.MakeIDSet("foo", "baz", "bang")
	diff1 := s1.Difference(s2)
	diff2 := s2.Difference(s1)

	assertValues(c, diff1, "bar")
	assertValues(c, diff2, "baz", "bang")
}

// Helper methods for the tests.
func assertValues(c *tc.C, s network.IDSet, expected ...network.Id) {
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

	c.Assert(values, tc.DeepEquals, expected)
	c.Assert(s.Size(), tc.Equals, len(expected))

	// Check the sorted values too.
	sorted := s.SortedValues()
	c.Assert(sorted, tc.DeepEquals, expected)
}

func (s *NetworkSuite) TestSubnetsForAddresses(c *tc.C) {
	addrs := []string{
		"10.10.10.10",
		"75ae:3af:968e:3a33:55e2:6379:fa67:d790",
		"some.host.name",
	}

	c.Check(network.SubnetsForAddresses(addrs), tc.DeepEquals, []string{
		"10.10.10.10/32",
		"75ae:3af:968e:3a33:55e2:6379:fa67:d790/128",
	})
}

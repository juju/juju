// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"sort"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type NetworkSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestGenerateVirtualMACAddress(c *tc.C) {
	mac := network.GenerateVirtualMACAddress()
	c.Check(mac, tc.Matches, "^([0-9A-Fa-f]{2}[:-]){5}([0-9A-Fa-f]{2})$")
}

func (s *NetworkSuite) TestIDSetSize(c *tc.C) {
	// Empty sets are empty.
	set := network.MakeIDSet()
	c.Assert(set.Size(), tc.Equals, 0)

	// Size returns number of unique values.
	set = network.MakeIDSet("foo", "foo", "bar")
	c.Assert(set.Size(), tc.Equals, 2)
}

func (s *NetworkSuite) TestIDSetEmpty(c *tc.C) {
	set := network.MakeIDSet()
	assertValues(c, set)
}

func (s *NetworkSuite) TestIDSetInitialValues(c *tc.C) {
	values := []network.Id{"foo", "bar", "baz"}
	set := network.MakeIDSet(values...)
	assertValues(c, set, values...)
}

func (s *NetworkSuite) TestIDSetIsEmpty(c *tc.C) {
	// Empty sets are empty.
	set := network.MakeIDSet()
	c.Assert(set.IsEmpty(), tc.Equals, true)

	// Non-empty sets are not empty.
	set = network.MakeIDSet("foo")
	c.Assert(set.IsEmpty(), tc.Equals, false)
}

func (s *NetworkSuite) TestIDSetAdd(c *tc.C) {
	set := network.MakeIDSet()
	set.Add("foo")
	set.Add("foo")
	set.Add("bar")
	assertValues(c, set, "foo", "bar")
}

func (s *NetworkSuite) TestIDSetContains(c *tc.C) {
	set := network.MakeIDSet("foo", "bar")
	c.Assert(set.Contains("foo"), tc.Equals, true)
	c.Assert(set.Contains("bar"), tc.Equals, true)
	c.Assert(set.Contains("baz"), tc.Equals, false)
}

func (s *NetworkSuite) TestIDSetDifference(c *tc.C) {
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

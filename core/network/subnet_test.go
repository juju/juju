// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"net"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/internal/testhelpers"
)

type subnetSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&subnetSuite{})

func (*subnetSuite) TestFindSubnetIDsForAZ(c *tc.C) {
	testCases := []struct {
		name           string
		zoneName       string
		subnetsToZones map[network.Id][]string
		expected       []network.Id
		expectedErr    error
	}{
		{
			name:           "empty",
			zoneName:       "",
			subnetsToZones: make(map[network.Id][]string),
			expected:       make([]network.Id, 0),
			expectedErr:    coreerrors.NotFound,
		},
		{
			name:     "no match",
			zoneName: "fuzz",
			subnetsToZones: map[network.Id][]string{
				"bar": {"foo", "baz"},
			},
			expected:    make([]network.Id, 0),
			expectedErr: coreerrors.NotFound,
		},
		{
			name:     "match",
			zoneName: "foo",
			subnetsToZones: map[network.Id][]string{
				"bar": {"foo", "baz"},
			},
			expected: []network.Id{"bar"},
		},
		{
			name:     "multi-match",
			zoneName: "foo",
			subnetsToZones: map[network.Id][]string{
				"bar":   {"foo", "baz"},
				"other": {"aaa", "foo", "xxx"},
			},
			expected: []network.Id{"bar", "other"},
		},
		{
			name:     "empty zone match",
			zoneName: "",
			subnetsToZones: map[network.Id][]string{
				"bar":   {},
				"other": {},
			},
			expected: []network.Id{"bar", "other"},
		},
	}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.name)

		res, err := network.FindSubnetIDsForAvailabilityZone(t.zoneName, t.subnetsToZones)
		if t.expectedErr != nil {
			c.Check(err, tc.ErrorIs, t.expectedErr)
		} else {
			c.Assert(err, tc.IsNil)
			c.Check(res, tc.DeepEquals, t.expected)
		}
	}
}

func (*subnetSuite) TestFilterInFanNetwork(c *tc.C) {
	testCases := []struct {
		name     string
		subnets  []network.Id
		expected []network.Id
	}{
		{
			name:     "empty",
			subnets:  make([]network.Id, 0),
			expected: []network.Id(nil),
		},
		{
			name: "no match",
			subnets: []network.Id{
				"aaa-bbb-ccc",
				"xxx-yyy-zzz",
			},
			expected: []network.Id{
				"aaa-bbb-ccc",
				"xxx-yyy-zzz",
			},
		},
		{
			name: "match",
			subnets: []network.Id{
				"aaa-bbb-ccc",
				"foo-INFAN-bar",
				"xxx-yyy-zzz",
			},
			expected: []network.Id{
				"aaa-bbb-ccc",
				"xxx-yyy-zzz",
			},
		},
	}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.name)

		res := network.FilterInFanNetwork(t.subnets)
		c.Check(res, tc.DeepEquals, t.expected)
	}
}

func (*subnetSuite) TestIsInFanNetwork(c *tc.C) {
	testCases := []struct {
		name     string
		subnet   network.Id
		expected bool
	}{
		{
			name:     "empty",
			subnet:   network.Id(""),
			expected: false,
		},
		{
			name:     "no match",
			subnet:   network.Id("foo-1asd-fan-network"),
			expected: false,
		},
		{
			name:     "match",
			subnet:   network.Id("foo-1asd-INFAN-network"),
			expected: true,
		},
	}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.name)

		res := network.IsInFanNetwork(t.subnet)
		c.Check(res, tc.Equals, t.expected)
	}
}

func (*subnetSuite) TestSubnetInfosEquality(c *tc.C) {
	s1 := network.SubnetInfos{
		{ID: "1"},
		{ID: "2"},
	}

	s2 := network.SubnetInfos{
		{ID: "2"},
		{ID: "1"},
	}

	s3 := append(s2, network.SubnetInfo{ID: "3"})

	c.Check(s1.EqualTo(s2), tc.IsTrue)
	c.Check(s1.EqualTo(s3), tc.IsFalse)
}

func (*subnetSuite) TestSubnetInfosSpaceIDs(c *tc.C) {
	s := network.SubnetInfos{
		{ID: "1", SpaceID: network.AlphaSpaceId},
		{ID: "2", SpaceID: network.AlphaSpaceId},
		{ID: "3", SpaceID: "666"},
	}

	c.Check(s.SpaceIDs().SortedValues(), tc.DeepEquals, []string{network.AlphaSpaceId, "666"})
}

func (*subnetSuite) TestSubnetInfosGetByCIDR(c *tc.C) {
	s := network.SubnetInfos{
		{ID: "1", CIDR: "10.10.10.0/25", ProviderId: "1"},
		{ID: "2", CIDR: "10.10.10.0/25", ProviderId: "2"},
		{ID: "4", CIDR: "20.20.20.0/25"},
	}

	_, err := s.GetByCIDR("invalid")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)

	subs, err := s.GetByCIDR("30.30.30.0/25")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subs, tc.HasLen, 0)

	subs, err = s.GetByCIDR("10.10.10.0/25")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subs.EqualTo(s[:2]), tc.IsTrue)

	// Check fallback CIDR-in-CIDR matching when CIDR is carved out of a
	// subnet CIDR.
	subs, err = s.GetByCIDR("10.10.10.0/31")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subs.EqualTo(s[:2]), tc.IsTrue, tc.Commentf("expected input that is a subset of the subnet CIDRs to be matched to a subnet"))

	// Same check as above but using a different network IP which is still
	// contained within the 10.10.10.0/25 subnets from the SubnetInfos list.
	subs, err = s.GetByCIDR("10.10.10.8/31")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subs.EqualTo(s[:2]), tc.IsTrue, tc.Commentf("expected input that is a subset of the subnet CIDRs to be matched to a subnet"))

	subs, err = s.GetByCIDR("10.10.0.0/24")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(subs, tc.HasLen, 0, tc.Commentf("expected input that is a superset of the subnet CIDRs not to be matched to any subnet"))
}

func (*subnetSuite) TestSubnetInfosGetByID(c *tc.C) {
	s := network.SubnetInfos{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
	}

	c.Check(s.GetByID("1"), tc.NotNil)
	c.Check(s.ContainsID("1"), tc.IsTrue)

	c.Check(s.GetByID("9"), tc.IsNil)
	c.Check(s.ContainsID("9"), tc.IsFalse)
}

func (*subnetSuite) TestSubnetInfosGetByAddress(c *tc.C) {
	s := network.SubnetInfos{
		{ID: "1", CIDR: "10.10.10.0/24", ProviderId: "1"},
		{ID: "2", CIDR: "10.10.10.0/24", ProviderId: "2"},
		{ID: "3", CIDR: "20.20.20.0/24"},
	}

	_, err := s.GetByAddress("invalid")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)

	subs, err := s.GetByAddress("10.10.10.5")
	c.Assert(err, tc.ErrorIsNil)

	// We need to check these explicitly, because the IPNets of the original
	// members will now be populated, making them differ.
	c.Assert(subs, tc.HasLen, 2)
	c.Check(subs[0].ProviderId, tc.Equals, network.Id("1"))
	c.Check(subs[1].ProviderId, tc.Equals, network.Id("2"))

	subs, err = s.GetByAddress("30.30.30.5")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(subs, tc.HasLen, 0)
}

func (*subnetSuite) TestSubnetInfosAllSubnetInfos(c *tc.C) {
	s := network.SubnetInfos{
		{ID: "1", CIDR: "10.10.10.0/24", ProviderId: "1"},
		{ID: "2", CIDR: "10.10.10.0/24", ProviderId: "2"},
		{ID: "3", CIDR: "20.20.20.0/24"},
	}

	allSubs, err := s.AllSubnetInfos()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(allSubs, tc.DeepEquals, s)
}

func (*subnetSuite) TestIPRangeForCIDR(c *tc.C) {
	specs := []struct {
		cidr     string
		expFirst net.IP
		expLast  net.IP
	}{
		{
			cidr:     "10.20.30.0/24",
			expFirst: net.ParseIP("10.20.30.0"),
			expLast:  net.ParseIP("10.20.30.255"),
		},
		{
			cidr:     "10.20.28.0/22",
			expFirst: net.ParseIP("10.20.28.0"),
			expLast:  net.ParseIP("10.20.31.255"),
		},
		{
			cidr:     "10.1.2.42/29",
			expFirst: net.ParseIP("10.1.2.40"),
			expLast:  net.ParseIP("10.1.2.47"),
		},
		{
			cidr:     "10.1.2.42/32",
			expFirst: net.ParseIP("10.1.2.42"),
			expLast:  net.ParseIP("10.1.2.42"),
		},
		{
			cidr:     "2002::1234:abcd:ffff:c0a8:101/64",
			expFirst: net.ParseIP("2002:0000:0000:1234:0000:0000:0000:0000"),
			expLast:  net.ParseIP("2002::1234:ffff:ffff:ffff:ffff"),
		},
		{
			cidr:     "2001:db8:85a3::8a2e:370:7334/128",
			expFirst: net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"),
			expLast:  net.ParseIP("2001:0db8:85a3:0000:0000:8a2e:0370:7334"),
		},
	}

	for i, spec := range specs {
		c.Logf("%d. check that range for %q is [%s, %s]", i, spec.cidr, spec.expFirst, spec.expLast)
		gotFirst, gotLast, err := network.IPRangeForCIDR(spec.cidr)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(gotFirst.String(), tc.Equals, spec.expFirst.String())
		c.Assert(gotLast.String(), tc.Equals, spec.expLast.String())
	}
}

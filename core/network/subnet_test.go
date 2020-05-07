// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
)

type subnetSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&subnetSuite{})

func (*subnetSuite) TestFindSubnetIDsForAZ(c *gc.C) {
	testCases := []struct {
		name           string
		zoneName       string
		subnetsToZones map[network.Id][]string
		expected       []network.Id
		expectedErr    func(error) bool
	}{
		{
			name:           "empty",
			zoneName:       "",
			subnetsToZones: make(map[network.Id][]string),
			expected:       make([]network.Id, 0),
			expectedErr:    errors.IsNotFound,
		},
		{
			name:     "no match",
			zoneName: "fuzz",
			subnetsToZones: map[network.Id][]string{
				"bar": {"foo", "baz"},
			},
			expected:    make([]network.Id, 0),
			expectedErr: errors.IsNotFound,
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
	}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.name)

		res, err := network.FindSubnetIDsForAvailabilityZone(t.zoneName, t.subnetsToZones)
		if t.expectedErr != nil {
			c.Check(t.expectedErr(err), jc.IsTrue)
		} else {
			c.Assert(err, gc.IsNil)
			c.Check(res, gc.DeepEquals, t.expected)
		}
	}
}

func (*subnetSuite) TestFilterInFanNetwork(c *gc.C) {
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
		c.Check(res, gc.DeepEquals, t.expected)
	}
}

func (*subnetSuite) TestSubnetInfosEquality(c *gc.C) {
	s1 := network.SubnetInfos{
		{ID: "1"},
		{ID: "2"},
	}

	s2 := network.SubnetInfos{
		{ID: "2"},
		{ID: "1"},
	}

	s3 := append(s2, network.SubnetInfo{ID: "3"})

	c.Check(s1.EqualTo(s2), jc.IsTrue)
	c.Check(s1.EqualTo(s3), jc.IsFalse)
}

func (*subnetSuite) TestSubnetInfosSpaceIDs(c *gc.C) {
	s := network.SubnetInfos{
		{ID: "1", SpaceID: network.AlphaSpaceId},
		{ID: "2", SpaceID: network.AlphaSpaceId},
		{ID: "3", SpaceID: "666"},
	}

	c.Check(s.SpaceIDs().SortedValues(), jc.DeepEquals, []string{network.AlphaSpaceId, "666"})
}

func (*subnetSuite) TestSubnetInfosGetByUnderLayCIDR(c *gc.C) {
	s := network.SubnetInfos{
		{
			ID:      "1",
			FanInfo: &network.FanCIDRs{FanLocalUnderlay: "10.10.10.0/24"},
		},
		{
			ID:      "2",
			FanInfo: &network.FanCIDRs{FanLocalUnderlay: "20.20.20.0/24"},
		},
		{
			ID:      "3",
			FanInfo: &network.FanCIDRs{FanLocalUnderlay: "20.20.20.0/24"},
		},
	}

	_, err := s.GetByUnderlayCIDR("invalid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)

	overlays, err := s.GetByUnderlayCIDR(s[0].FanLocalUnderlay())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(overlays, gc.DeepEquals, network.SubnetInfos{s[0]})

	overlays, err = s.GetByUnderlayCIDR(s[1].FanLocalUnderlay())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(overlays, gc.DeepEquals, network.SubnetInfos{s[1], s[2]})

	overlays, err = s.GetByUnderlayCIDR("30.30.30.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(overlays, gc.HasLen, 0)
}

func (*subnetSuite) TestSubnetInfosGetByCIDR(c *gc.C) {
	s := network.SubnetInfos{
		{ID: "1", CIDR: "10.10.10.0/24", ProviderId: "1"},
		{ID: "2", CIDR: "10.10.10.0/24", ProviderId: "2"},
		{ID: "3", CIDR: "20.20.20.0/24"},
	}

	_, err := s.GetByCIDR("invalid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)

	subs, err := s.GetByCIDR("30.30.30.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subs, gc.HasLen, 0)

	subs, err = s.GetByCIDR("10.10.10.0/24")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subs, gc.DeepEquals, s[:2])
}

func (*subnetSuite) TestSubnetInfosGetByID(c *gc.C) {
	s := network.SubnetInfos{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
	}

	c.Check(s.GetByID("1"), gc.NotNil)
	c.Check(s.ContainsID("1"), jc.IsTrue)

	c.Check(s.GetByID("9"), gc.IsNil)
	c.Check(s.ContainsID("9"), jc.IsFalse)
}

func (*subnetSuite) TestSubnetInfosGetByAddress(c *gc.C) {
	s := network.SubnetInfos{
		{ID: "1", CIDR: "10.10.10.0/24", ProviderId: "1"},
		{ID: "2", CIDR: "10.10.10.0/24", ProviderId: "2"},
		{ID: "3", CIDR: "20.20.20.0/24"},
	}

	_, err := s.GetByAddress("invalid")
	c.Check(err, jc.Satisfies, errors.IsNotValid)

	subs, err := s.GetByAddress("10.10.10.5")
	c.Assert(err, jc.ErrorIsNil)

	// We need to check these explicitly, because the IPNets of the original
	// members will now be populated, making them differ.
	c.Assert(subs, gc.HasLen, 2)
	c.Check(subs[0].ProviderId, gc.Equals, network.Id("1"))
	c.Check(subs[1].ProviderId, gc.Equals, network.Id("2"))

	subs, err = s.GetByAddress("30.30.30.5")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subs, gc.HasLen, 0)
}

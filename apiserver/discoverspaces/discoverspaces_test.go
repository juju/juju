// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type DiscoverSpacesSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *discoverspaces.API
}

var _ = gc.Suite(&DiscoverSpacesSuite{})

func (s *DiscoverSpacesSuite) SetUpSuite(c *gc.C) {
	s.StubNetwork.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *DiscoverSpacesSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *DiscoverSpacesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubZonedEnvironName,
		apiservertesting.WithZones,
		apiservertesting.WithSpaces,
		apiservertesting.WithSubnets)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewUserTag("admin"),
		Controller: true,
	}

	var err error
	s.facade, err = discoverspaces.NewAPIWithBacking(
		apiservertesting.BackingInstance, s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *DiscoverSpacesSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *DiscoverSpacesSuite) TestModelConfigFailure(c *gc.C) {
	apiservertesting.BackingInstance.SetErrors(errors.New("boom"))

	result, err := s.facade.ModelConfig()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{})

	apiservertesting.BackingInstance.CheckCallNames(c, "ModelConfig")
}

func (s *DiscoverSpacesSuite) TestModelConfigSuccess(c *gc.C) {
	result, err := s.facade.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{
		Config: apiservertesting.BackingInstance.EnvConfig.AllAttrs(),
	})

	apiservertesting.BackingInstance.CheckCallNames(c, "ModelConfig")
}

func (s *DiscoverSpacesSuite) TestListSpaces(c *gc.C) {
	result, err := s.facade.ListSpaces()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := []params.ProviderSpace{{
		Name: "default",
		Subnets: []params.Subnet{
			{CIDR: "192.168.0.0/24",
				ProviderId: "provider-192.168.0.0/24",
				SpaceTag:   "space-default",
				Zones:      []string{"foo"},
				Status:     "in-use"},
			{CIDR: "192.168.3.0/24",
				ProviderId: "provider-192.168.3.0/24",
				VLANTag:    23,
				SpaceTag:   "space-default",
				Zones:      []string{"bar", "bam"}}}}, {
		Name: "dmz",
		Subnets: []params.Subnet{
			{CIDR: "192.168.1.0/24",
				ProviderId: "provider-192.168.1.0/24",
				VLANTag:    23,
				SpaceTag:   "space-dmz",
				Zones:      []string{"bar", "bam"}}}}, {
		Name: "private",
		Subnets: []params.Subnet{
			{CIDR: "192.168.2.0/24",
				ProviderId: "provider-192.168.2.0/24",
				SpaceTag:   "space-private",
				Zones:      []string{"foo"},
				Status:     "in-use"}},
	}}
	c.Assert(result.Results, jc.DeepEquals, expectedResult)
	apiservertesting.BackingInstance.CheckCallNames(c, "AllSpaces")
}

func (s *DiscoverSpacesSuite) TestListSpacesFailure(c *gc.C) {
	apiservertesting.BackingInstance.SetErrors(errors.New("boom"))

	result, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.DiscoverSpacesResults{})

	apiservertesting.BackingInstance.CheckCallNames(c, "AllSpaces")
}

func (s *DiscoverSpacesSuite) TestAddSubnetsParamsCombinations(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithZones,
		apiservertesting.WithSpaces,
		apiservertesting.WithSubnets)

	args := params.AddSubnetsParams{Subnets: []params.AddSubnetParams{{
		// No ProviderId
		SubnetProviderId: "",
		SubnetTag:        "subnet-10.10.0.0/24",
		VLANTag:          3,
		Zones:            []string{"a", "b", "c"},
		SpaceTag:         "space-dmz",
	}, {
		// No subnet tag
		SubnetProviderId: "1",
		SubnetTag:        "",
		VLANTag:          3,
		Zones:            []string{"a", "b", "c"},
		SpaceTag:         "space-dmz",
	}, {
		// Invalid subnet
		SubnetProviderId: "1",
		SubnetTag:        "subnet-10.10.10.10",
		VLANTag:          3,
		Zones:            []string{"a", "b", "c"},
		SpaceTag:         "space-dmz",
	}, {
		// Invalid space
		SubnetProviderId: "1",
		SubnetTag:        "subnet-10.10.10.10",
		VLANTag:          3,
		Zones:            []string{"a", "b", "c"},
		SpaceTag:         "application-blemp",
	}, {
		// Non-existent space
		SubnetProviderId: "1",
		SubnetTag:        "subnet-10.10.10.0/24",
		VLANTag:          3,
		Zones:            []string{"a", "b", "c"},
		SpaceTag:         "space-thing",
	}, {
		// Successful - ipv6
		SubnetProviderId:  "sn-ipv6",
		ProviderNetworkId: "antennas",
		SubnetTag:         "subnet-2001:db8::/32",
		VLANTag:           0,
		Zones:             []string{"a", "b", "c"},
		SpaceTag:          "space-dmz",
	}, {
		// Successful - no zones
		SubnetProviderId: "sn-no-zone",
		SubnetTag:        "subnet-10.10.10.0/24",
		VLANTag:          3,
		Zones:            nil,
		SpaceTag:         "space-dmz",
	}, {
		// Successful - no space
		SubnetProviderId: "sn-no-space",
		SubnetTag:        "subnet-10.10.10.0/24",
		VLANTag:          3,
		Zones:            []string{"a", "b", "c"},
		SpaceTag:         "",
	}}}

	expectedErrors := []struct {
		message   string
		satisfier func(error) bool
	}{
		{"SubnetProviderId is required", nil},
		{"SubnetTag is required", nil},
		{`SubnetTag is invalid: "subnet-10.10.10.10" is not a valid subnet tag`, nil},
		{`SpaceTag is invalid: "application-blemp" is not a valid space tag`, nil},
		{`space "thing" not found`, params.IsCodeNotFound},
		{"", nil},
		{"", nil},
		{"", nil},
	}
	expectedBackingInfos := []networkingcommon.BackingSubnetInfo{{
		ProviderId:        "sn-ipv6",
		ProviderNetworkId: "antennas",
		CIDR:              "2001:db8::/32",
		VLANTag:           0,
		AvailabilityZones: []string{"a", "b", "c"},
		SpaceName:         "dmz",
	}, {
		ProviderId:        "sn-no-zone",
		ProviderNetworkId: "",
		CIDR:              "10.10.10.0/24",
		VLANTag:           3,
		AvailabilityZones: nil,
		SpaceName:         "dmz",
	}, {
		ProviderId:        "sn-no-space",
		ProviderNetworkId: "",
		CIDR:              "10.10.10.0/24",
		VLANTag:           3,
		AvailabilityZones: []string{"a", "b", "c"},
		SpaceName:         "",
	}}
	c.Check(expectedErrors, gc.HasLen, len(args.Subnets))
	results, err := s.facade.AddSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Results), gc.Equals, len(args.Subnets))
	for i, result := range results.Results {
		c.Logf("result #%d: expected: %q", i, expectedErrors[i].message)
		if expectedErrors[i].message == "" {
			if !c.Check(result.Error, gc.IsNil) {
				c.Logf("unexpected error: %v; args: %#v", result.Error, args.Subnets[i])
			}
			continue
		}
		if !c.Check(result.Error, gc.NotNil) {
			c.Logf("unexpected success; args: %#v", args.Subnets[i])
			continue
		}
		c.Check(result.Error.Message, gc.Equals, expectedErrors[i].message)
		if expectedErrors[i].satisfier != nil {
			c.Check(result.Error, jc.Satisfies, expectedErrors[i].satisfier)
		} else {
			c.Check(result.Error.Code, gc.Equals, "")
		}
	}

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AllSpaces"),
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[0]),
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[1]),
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[2]),
	)
	apiservertesting.ResetStub(apiservertesting.SharedStub)

	// Finally, check that no params yields no results.
	results, err = s.facade.AddSubnets(params.AddSubnetsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

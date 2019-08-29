// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/facades/client/subnets"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	providercommon "github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type SubnetsSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *subnets.API

	callContext context.ProviderCallContext
}

var _ = gc.Suite(&SubnetsSuite{})

func (s *SubnetsSuite) SetUpSuite(c *gc.C) {
	s.StubNetwork.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *SubnetsSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *SubnetsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewUserTag("admin"),
		Controller: false,
	}

	s.callContext = context.NewCloudCallContext()
	var err error
	s.facade, err = subnets.NewAPIWithBacking(
		apiservertesting.BackingInstance,
		s.callContext,
		s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *SubnetsSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

// AssertAllZonesResult makes it easier to verify AllZones results.
func (s *SubnetsSuite) AssertAllZonesResult(c *gc.C, got params.ZoneResults, expected []providercommon.AvailabilityZone) {
	results := make([]params.ZoneResult, len(expected))
	for i, zone := range expected {
		results[i].Name = zone.Name()
		results[i].Available = zone.Available()
	}
	c.Assert(got, jc.DeepEquals, params.ZoneResults{Results: results})
}

// AssertAllSpacesResult makes it easier to verify AllSpaces results.
func (s *SubnetsSuite) AssertAllSpacesResult(c *gc.C, got params.SpaceResults, expected []networkingcommon.BackingSpace) {
	seen := set.Strings{}
	results := []params.SpaceResult{}
	for _, space := range expected {
		if seen.Contains(space.Name()) {
			continue
		}
		seen.Add(space.Name())
		result := params.SpaceResult{}
		result.Tag = names.NewSpaceTag(space.Name()).String()
		results = append(results, result)
	}
	c.Assert(got, jc.DeepEquals, params.SpaceResults{Results: results})
}

func (s *SubnetsSuite) TestNewAPIWithBacking(c *gc.C) {
	// Clients are allowed.
	facade, err := subnets.NewAPIWithBacking(
		apiservertesting.BackingInstance,
		s.callContext,
		s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = subnets.NewAPIWithBacking(
		apiservertesting.BackingInstance,
		s.callContext,
		s.resources, agentAuthorizer,
	)
	c.Assert(err, jc.DeepEquals, common.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

func (s *SubnetsSuite) TestAllZonesWhenBackingAvailabilityZonesFails(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(errors.NotSupportedf("zones"))

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches, "zones not supported")
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
	)
}

func (s *SubnetsSuite) TestAllZonesUsesBackingZonesWhenAvailable(c *gc.C) {
	results, err := s.facade.AllZones()
	c.Assert(err, jc.ErrorIsNil)
	s.AssertAllZonesResult(c, results, apiservertesting.BackingInstance.Zones)

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesUpdates(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)

	results, err := s.facade.AllZones()
	c.Assert(err, jc.ErrorIsNil)
	s.AssertAllZonesResult(c, results, apiservertesting.ProviderInstance.Zones)

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.ZonedEnvironCall("AvailabilityZones", s.callContext),
		apiservertesting.BackingCall("SetAvailabilityZones", apiservertesting.ProviderInstance.Zones),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndSetFails(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	apiservertesting.SharedStub.SetErrors(
		nil,                             // Backing.AvailabilityZones
		nil,                             // Backing.ModelConfig
		nil,                             // Backing.CloudSpec
		nil,                             // Provider.Open
		nil,                             // ZonedEnviron.AvailabilityZones
		errors.NotSupportedf("setting"), // Backing.SetAvailabilityZones
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: setting not supported`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.ZonedEnvironCall("AvailabilityZones", s.callContext),
		apiservertesting.BackingCall("SetAvailabilityZones", apiservertesting.ProviderInstance.Zones),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndFetchingZonesFails(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	apiservertesting.SharedStub.SetErrors(
		nil,                     // Backing.AvailabilityZones
		nil,                     // Backing.ModelConfig
		nil,                     // Backing.CloudSpec
		nil,                     // Provider.Open
		errors.NotValidf("foo"), // ZonedEnviron.AvailabilityZones
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: foo not valid`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.ZonedEnvironCall("AvailabilityZones", s.callContext),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndModelConfigFails(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	apiservertesting.SharedStub.SetErrors(
		nil,                        // Backing.AvailabilityZones
		errors.NotFoundf("config"), // Backing.ModelConfig
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: opening environment: config not found`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("ModelConfig"),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndOpenFails(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	apiservertesting.SharedStub.SetErrors(
		nil,                        // Backing.AvailabilityZones
		nil,                        // Backing.ModelConfig
		nil,                        // Backing.CloudSpec
		errors.NotValidf("config"), // Provider.Open
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: opening environment: config not valid`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndZonesNotSupported(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubEnvironName, apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	// ZonedEnviron not supported

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: availability zones not supported`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
	)
}

func (s *SubnetsSuite) TestAllSpacesWithExistingSuccess(c *gc.C) {
	s.testAllSpacesSuccess(c, apiservertesting.WithSpaces)
}

func (s *SubnetsSuite) TestAllSpacesNoExistingSuccess(c *gc.C) {
	s.testAllSpacesSuccess(c, apiservertesting.WithoutSpaces)
}

func (s *SubnetsSuite) testAllSpacesSuccess(c *gc.C, withBackingSpaces apiservertesting.SetUpFlag) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithZones, withBackingSpaces, apiservertesting.WithSubnets)

	results, err := s.facade.AllSpaces()
	c.Assert(err, jc.ErrorIsNil)
	s.AssertAllSpacesResult(c, results, apiservertesting.BackingInstance.Spaces)

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AllSpaces"),
	)
}

func (s *SubnetsSuite) TestAllSpacesFailure(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(errors.NotFoundf("boom"))

	results, err := s.facade.AllSpaces()
	c.Assert(err, gc.ErrorMatches, "boom not found")
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(results, jc.DeepEquals, params.SpaceResults{})

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
		apiservertesting.BackingCall("AllSpaces"),
	)
}

func (s *SubnetsSuite) CheckAddSubnetsFails(
	c *gc.C, envName string,
	withZones, withSpaces, withSubnets apiservertesting.SetUpFlag,
	expectedError string,
	expectedSatisfies func(error) bool,
) {
	apiservertesting.BackingInstance.SetUp(c, envName, withZones, withSpaces, withSubnets)

	// These calls always happen.
	expectedCalls := []apiservertesting.StubMethodCall{
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
	}

	// Subnets is also always called. but the receiver is different.
	switch envName {
	case apiservertesting.StubNetworkingEnvironName:
		expectedCalls = append(
			expectedCalls,
			apiservertesting.NetworkingEnvironCall("Subnets", s.callContext, instance.UnknownId, []network.Id(nil)),
		)
	case apiservertesting.StubZonedNetworkingEnvironName:
		expectedCalls = append(
			expectedCalls,
			apiservertesting.ZonedNetworkingEnvironCall("Subnets", s.callContext, instance.UnknownId, []network.Id(nil)),
		)
	}

	if !withSubnets {
		// Set provider subnets to empty for this test.
		originalSubnets := make([]network.SubnetInfo, len(apiservertesting.ProviderInstance.Subnets))
		copy(originalSubnets, apiservertesting.ProviderInstance.Subnets)
		apiservertesting.ProviderInstance.Subnets = []network.SubnetInfo{}

		defer func() {
			apiservertesting.ProviderInstance.Subnets = make([]network.SubnetInfo, len(originalSubnets))
			copy(apiservertesting.ProviderInstance.Subnets, originalSubnets)
		}()

		if envName == apiservertesting.StubEnvironName || envName == apiservertesting.StubNetworkingEnvironName {
			// networking is either not supported or no subnets are
			// defined, so expected the same calls for each of the two
			// arguments to AddSubnets() below.
			expectedCalls = append(expectedCalls, expectedCalls...)
		}
	} else {
		// Having subnets implies spaces will be cached as well.
		expectedCalls = append(expectedCalls, apiservertesting.BackingCall("AllSpaces"))
	}

	if withSpaces && withSubnets {
		// Having both subnets and spaces means we'll also cache zones.
		expectedCalls = append(expectedCalls, apiservertesting.BackingCall("AvailabilityZones"))
	}

	if !withZones && withSpaces {
		// Set provider zones to empty for this test.
		originalZones := make([]providercommon.AvailabilityZone, len(apiservertesting.ProviderInstance.Zones))
		copy(originalZones, apiservertesting.ProviderInstance.Zones)
		apiservertesting.ProviderInstance.Zones = []providercommon.AvailabilityZone{}

		defer func() {
			apiservertesting.ProviderInstance.Zones = make([]providercommon.AvailabilityZone, len(originalZones))
			copy(apiservertesting.ProviderInstance.Zones, originalZones)
		}()

		// updateZones tries to constructs a ZonedEnviron with these calls.
		zoneCalls := append([]apiservertesting.StubMethodCall{},
			apiservertesting.BackingCall("ModelConfig"),
			apiservertesting.BackingCall("CloudSpec"),
			apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		)
		// Receiver can differ according to envName, but
		// AvailabilityZones() will be called on either receiver.
		switch envName {
		case apiservertesting.StubZonedEnvironName:
			zoneCalls = append(
				zoneCalls,
				apiservertesting.ZonedEnvironCall("AvailabilityZones", s.callContext),
			)
		case apiservertesting.StubZonedNetworkingEnvironName:
			zoneCalls = append(
				zoneCalls,
				apiservertesting.ZonedNetworkingEnvironCall("AvailabilityZones", s.callContext),
			)
		}
		// Finally after caching provider zones backing zones are
		// updated.
		zoneCalls = append(
			zoneCalls,
			apiservertesting.BackingCall("SetAvailabilityZones", apiservertesting.ProviderInstance.Zones),
		)

		// Now, because we have 2 arguments to AddSubnets() below, we
		// need to expect the same zoneCalls twice, with a
		// AvailabilityZones backing lookup between them.
		expectedCalls = append(expectedCalls, zoneCalls...)
		expectedCalls = append(expectedCalls, apiservertesting.BackingCall("AvailabilityZones"))
		expectedCalls = append(expectedCalls, zoneCalls...)
	}

	// Pass 2 arguments covering all cases we need.
	args := params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{{
			CIDR:     "10.42.0.0/16",
			SpaceTag: "space-dmz",
			Zones:    []string{"zone1"},
		}, {
			SubnetProviderId: "vlan-42",
			SpaceTag:         "space-private",
			Zones:            []string{"zone3"},
		}},
	}
	results, err := s.facade.AddSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, len(args.Subnets))
	for _, result := range results.Results {
		if !c.Check(result.Error, gc.NotNil) {
			continue
		}
		c.Check(result.Error, gc.ErrorMatches, expectedError)
		if expectedSatisfies != nil {
			c.Check(result.Error, jc.Satisfies, expectedSatisfies)
		} else {
			c.Check(result.Error.Code, gc.Equals, "")
		}
	}

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, expectedCalls...)
}

func (s *SubnetsSuite) TestAddSubnetsWithNoProviderSubnetsFails(c *gc.C) {
	s.CheckAddSubnetsFails(
		c, apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithoutZones, apiservertesting.WithoutSpaces, apiservertesting.WithoutSubnets,
		"no subnets defined",
		nil,
	)
}

func (s *SubnetsSuite) TestAddSubnetsWithNoBackingSpacesFails(c *gc.C) {
	s.CheckAddSubnetsFails(
		c, apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithoutZones, apiservertesting.WithoutSpaces, apiservertesting.WithSubnets,
		"no spaces defined",
		nil,
	)
}

func (s *SubnetsSuite) TestAddSubnetsWithNoProviderZonesFails(c *gc.C) {
	s.CheckAddSubnetsFails(
		c, apiservertesting.StubZonedNetworkingEnvironName,
		apiservertesting.WithoutZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets,
		"no zones defined",
		nil,
	)
}

func (s *SubnetsSuite) TestAddSubnetsWhenNetworkingEnvironNotSupported(c *gc.C) {
	s.CheckAddSubnetsFails(
		c, apiservertesting.StubEnvironName,
		apiservertesting.WithoutZones, apiservertesting.WithoutSpaces, apiservertesting.WithoutSubnets,
		"model networking features not supported",
		params.IsCodeNotSupported,
	)
}

func (s *SubnetsSuite) TestAddSubnetAPI(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	results, err := s.facade.AddSubnets(params.AddSubnetsParams{
		Subnets: []params.AddSubnetParams{
			{
				SpaceTag: "space-dmz",
				CIDR:     "10.42.0.0/16",
				Zones:    []string{"zone1"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *SubnetsSuite) TestAddSubnetAPIv2(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)
	apiV2 := &subnets.APIv2{s.facade}
	results, err := apiV2.AddSubnets(params.AddSubnetsParamsV2{
		Subnets: []params.AddSubnetParamsV2{
			{
				SpaceTag:  "space-dmz",
				SubnetTag: "subnet-10.42.0.0/16",
				Zones:     []string{"zone1"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *SubnetsSuite) TestListSubnetsAndFiltering(c *gc.C) {
	expected := []params.Subnet{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-zadf00d",
		ProviderNetworkId: "godspeed",
		VLANTag:           0,
		Life:              "",
		SpaceTag:          "space-private",
		Zones:             []string{"zone1"},
		Status:            "",
	}, {
		CIDR:              "2001:db8::/32",
		ProviderId:        "sn-ipv6",
		ProviderNetworkId: "",
		VLANTag:           0,
		Life:              "",
		SpaceTag:          "space-dmz",
		Zones:             []string{"zone1", "zone3"},
		Status:            "",
	}}
	// No filtering.
	args := params.SubnetsFilters{}
	subnets, err := s.facade.ListSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets.Results, jc.DeepEquals, expected)

	// Filter by space only.
	args.SpaceTag = "space-dmz"
	subnets, err = s.facade.ListSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets.Results, jc.DeepEquals, expected[1:])

	// Filter by zone only.
	args.SpaceTag = ""
	args.Zone = "zone3"
	subnets, err = s.facade.ListSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets.Results, jc.DeepEquals, expected[1:])

	// Filter by both space and zone.
	args.SpaceTag = "space-private"
	args.Zone = "zone1"
	subnets, err = s.facade.ListSubnets(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(subnets.Results, jc.DeepEquals, expected[:1])
}

func (s *SubnetsSuite) TestListSubnetsInvalidSpaceTag(c *gc.C) {
	args := params.SubnetsFilters{SpaceTag: "invalid"}
	_, err := s.facade.ListSubnets(args)
	c.Assert(err, gc.ErrorMatches, `"invalid" is not a valid tag`)
}

func (s *SubnetsSuite) TestListSubnetsAllSubnetError(c *gc.C) {
	boom := errors.New("no subnets for you")
	apiservertesting.BackingInstance.SetErrors(boom)
	_, err := s.facade.ListSubnets(params.SubnetsFilters{})
	c.Assert(err, gc.ErrorMatches, "no subnets for you")
}

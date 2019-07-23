// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

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
	facade     subnets.SubnetsAPI

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

func (s *SubnetsSuite) TestAddSubnetsParamsCombinations(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubNetworkingEnvironName, apiservertesting.WithZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)

	args := params.AddSubnetsParams{Subnets: []params.AddSubnetParams{{
		// nothing set; early exit: no calls
	}, {
		// neither tag nor id set: the rest is ignored; same as above
		SpaceTag: "any",
		Zones:    []string{"any", "ignored"},
	}, {
		// both tag and id set; same as above
		SubnetTag:        "any",
		SubnetProviderId: "any",
	}, {
		// lookup by id needed, no cached subnets; ModelConfig(): error
		SubnetProviderId: "any",
	}, {
		// same as above, need to cache subnets; ModelConfig(): ok; Open(): error
		SubnetProviderId: "ignored",
	}, {
		// as above, caching again; ModelConfig(), Open(): ok; Subnets(): error
		SubnetProviderId: "unimportant",
	}, {
		// exactly as above, except all 3 calls ok; cached lookup: id not found
		SubnetProviderId: "missing",
	}, {
		// cached lookup by id (no calls): not found error
		SubnetProviderId: "void",
	}, {
		// cached lookup by id: ok; parsing space tag: invalid tag error
		SubnetProviderId: "sn-deadbeef",
		SpaceTag:         "invalid",
	}, {
		// as above, but slightly different error: invalid space tag error
		SubnetProviderId: "sn-zadf00d",
		SpaceTag:         "unit-foo",
	}, {
		// as above; yet another similar error (valid tag with another kind)
		SubnetProviderId: "vlan-42",
		SpaceTag:         "unit-foo-0",
	}, {
		// invalid tag (no kind): error (no calls)
		SubnetTag: "invalid",
	}, {
		// invalid subnet tag (another kind): same as above
		SubnetTag: "application-bar",
	}, {
		// cached lookup by missing CIDR: not found error
		SubnetTag: "subnet-1.2.3.0/24",
	}, {
		// cached lookup by duplicate CIDR: multiple choices error
		SubnetTag: "subnet-10.10.0.0/24",
	}, {
		// cached lookup by CIDR with empty provider id: ok; space tag is required error
		SubnetTag: "subnet-10.20.0.0/16",
	}, {
		// cached lookup by id with invalid CIDR: cannot be added error
		SubnetProviderId: "sn-invalid",
	}, {
		// cached lookup by id with empty CIDR: cannot be added error
		SubnetProviderId: "sn-empty",
	}, {
		// cached lookup by id with incorrectly specified CIDR: cannot be added error
		SubnetProviderId: "sn-awesome",
	}, {
		// cached lookup by CIDR: ok; valid tag; caching spaces: AllSpaces(): error
		SubnetTag: "subnet-10.30.1.0/24",
		SpaceTag:  "space-unverified",
	}, {
		// exactly as above, except AllSpaces(): ok; cached lookup: space not found
		SubnetTag: "subnet-2001:db8::/32",
		SpaceTag:  "space-missing",
	}, {
		// both cached lookups (CIDR, space): ok; no provider or given zones: error
		SubnetTag: "subnet-10.42.0.0/16",
		SpaceTag:  "space-dmz",
	}, {
		// like above; with provider zones, extra given: error
		SubnetProviderId: "vlan-42",
		SpaceTag:         "space-private",
		Zones: []string{
			"zone2",   // not allowed, existing, unavailable
			"zone3",   // allowed, existing, available
			"missing", // not allowed, non-existing
			"zone3",   // duplicates are ignored (should they ?)
			"zone1",   // not allowed, existing, available
		},
	}, {
		// like above; no provider, only given zones; caching: AllZones(): error
		SubnetTag: "subnet-10.42.0.0/16",
		SpaceTag:  "space-dmz",
		Zones:     []string{"any", "ignored"},
	}, {
		// as above, but unknown zones given: cached: AllZones(): ok; unknown zones error
		SubnetTag: "subnet-10.42.0.0/16",
		SpaceTag:  "space-dmz",
		Zones:     []string{"missing", "gone"},
	}, {
		// as above, but unknown and unavailable zones given: same error (no calls)
		SubnetTag: "subnet-10.42.0.0/16",
		SpaceTag:  "space-dmz",
		Zones:     []string{"zone4", "missing", "zone2"},
	}, {
		// as above, but unavailable zones given: Zones contains unavailable error
		SubnetTag: "subnet-10.42.0.0/16",
		SpaceTag:  "space-dmz",
		Zones:     []string{"zone2", "zone4"},
	}, {
		// as above, but available and unavailable zones given: same error as above
		SubnetTag: "subnet-10.42.0.0/16",
		SpaceTag:  "space-dmz",
		Zones:     []string{"zone4", "zone3"},
	}, {
		// everything succeeds, using caches as needed, until: AddSubnet(): error
		SubnetProviderId: "sn-ipv6",
		SpaceTag:         "space-dmz",
		Zones:            []string{"zone1"},
		// restriction of provider zones [zone1, zone3]
	}, {
		// cached lookups by CIDR, space: ok; duplicated provider id: unavailable zone2
		SubnetTag: "subnet-10.99.88.0/24",
		SpaceTag:  "space-dmz",
		Zones:     []string{"zone2"},
		// due to the duplicate ProviderId provider zones from subnet
		// with the last ProviderId=sn-deadbeef are used
		// (10.10.0.0/24); [zone2], not the 10.99.88.0/24 provider
		// zones: [zone1, zone2].
	}, {
		// same as above, but AddSubnet(): ok; success (backing verified later)
		SubnetProviderId: "sn-ipv6",
		SpaceTag:         "space-dmz",
		Zones:            []string{"zone1"},
		// restriction of provider zones [zone1, zone3]
	}, {
		// success (CIDR lookup; with provider (no given) zones): AddSubnet(): ok
		SubnetTag: "subnet-10.30.1.0/24",
		SpaceTag:  "space-private",
		// Zones not given, so provider zones are used instead: [zone3]
	}, {
		// success (id lookup; given zones match provider zones) AddSubnet(): ok
		SubnetProviderId: "sn-zadf00d",
		SpaceTag:         "space-private",
		Zones:            []string{"zone1"},
	}}}
	apiservertesting.SharedStub.SetErrors(
		// caching subnets (1st attempt): fails
		errors.NotFoundf("config"), // BackingInstance.ModelConfig (1st call)

		// caching subnets (2nd attepmt): fails
		nil,                          // BackingInstance.ModelConfig (2nd call)
		nil,                          // BackingInstance.CloudSpec (1st call)
		errors.NotFoundf("provider"), // ProviderInstance.Open (1st call)

		// caching subnets (3rd attempt): fails
		nil,                         // BackingInstance.ModelConfig (3rd call)
		nil,                         // BackingInstance.CloudSpec (2nd call)
		nil,                         // ProviderInstance.Open (2nd call)
		errors.NotFoundf("subnets"), // NetworkingEnvironInstance.Subnets (1st call)

		// caching subnets (4th attempt): succeeds
		nil, // BackingInstance.ModelConfig (4th call)
		nil, // BackingInstance.CloudSpec (3rd call)
		nil, // ProviderInstance.Open (3rd call)
		nil, // NetworkingEnvironInstance.Subnets (2nd call)

		// caching spaces (1st and 2nd attempts)
		errors.NotFoundf("spaces"), // BackingInstance.AllSpaces (1st call)
		nil,                        // BackingInstance.AllSpaces (2nd call)

		// cacing zones (1st and 2nd attempts)
		errors.NotFoundf("zones"), // BackingInstance.AvailabilityZones (1st call)
		nil,                       // BackingInstance.AvailabilityZones (2nd call)

		// validation done; adding subnets to backing store
		errors.NotFoundf("state"), // BackingInstance.AddSubnet (1st call)
		// the next 3 BackingInstance.AddSubnet calls succeed(2nd call)
	)

	expectedErrors := []struct {
		message   string
		satisfier func(error) bool
	}{
		{"either SubnetTag or SubnetProviderId is required", nil},
		{"either SubnetTag or SubnetProviderId is required", nil},
		{"SubnetTag and SubnetProviderId cannot be both set", nil},
		{"opening environment: config not found", params.IsCodeNotFound},
		{"opening environment: provider not found", params.IsCodeNotFound},
		{"cannot get provider subnets: subnets not found", params.IsCodeNotFound},
		{`subnet with CIDR "" and ProviderId "missing" not found`, params.IsCodeNotFound},
		{`subnet with CIDR "" and ProviderId "void" not found`, params.IsCodeNotFound},
		{`given SpaceTag is invalid: "invalid" is not a valid tag`, nil},
		{`given SpaceTag is invalid: "unit-foo" is not a valid unit tag`, nil},
		{`given SpaceTag is invalid: "unit-foo-0" is not a valid space tag`, nil},
		{`given SubnetTag is invalid: "invalid" is not a valid tag`, nil},
		{`given SubnetTag is invalid: "application-bar" is not a valid subnet tag`, nil},
		{`subnet with CIDR "1.2.3.0/24" not found`, params.IsCodeNotFound},
		{
			`multiple subnets with CIDR "10.10.0.0/24": ` +
				`retry using ProviderId from: "sn-deadbeef", "sn-zadf00d"`, nil,
		},
		{"SpaceTag is required", nil},
		{`subnet with CIDR "invalid" and ProviderId "sn-invalid": invalid CIDR`, nil},
		{`subnet with ProviderId "sn-empty": empty CIDR`, nil},
		{
			`subnet with ProviderId "sn-awesome": ` +
				`incorrect CIDR format "0.1.2.3/4", expected "0.0.0.0/4"`, nil,
		},
		{"cannot validate given SpaceTag: spaces not found", params.IsCodeNotFound},
		{`space "missing" not found`, params.IsCodeNotFound},
		{"Zones cannot be discovered from the provider and must be set", nil},
		{`Zones contain zones not allowed by the provider: "missing", "zone1", "zone2"`, nil},
		{"given Zones cannot be validated: zones not found", params.IsCodeNotFound},
		{`Zones contain unknown zones: "gone", "missing"`, nil},
		{`Zones contain unknown zones: "missing"`, nil},
		{`Zones contain unavailable zones: "zone2", "zone4"`, nil},
		{`Zones contain unavailable zones: "zone4"`, nil},
		{"state not found", params.IsCodeNotFound},
		{`Zones contain unavailable zones: "zone2"`, nil},
		{"", nil},
		{"", nil},
		{"", nil},
	}
	expectedBackingInfos := []networkingcommon.BackingSubnetInfo{{
		ProviderId:        "sn-ipv6",
		ProviderNetworkId: "",
		CIDR:              "2001:db8::/32",
		VLANTag:           0,
		AvailabilityZones: []string{"zone1"},
		SpaceName:         "dmz",
	}, {
		ProviderId:        "vlan-42",
		ProviderNetworkId: "",
		CIDR:              "10.30.1.0/24",
		VLANTag:           42,
		AvailabilityZones: []string{"zone3"},
		SpaceName:         "private",
	}, {
		ProviderId:        "sn-zadf00d",
		ProviderNetworkId: "godspeed",
		CIDR:              "10.10.0.0/24",
		VLANTag:           0,
		AvailabilityZones: []string{"zone1"},
		SpaceName:         "private",
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
		// caching subnets (1st attempt): fails
		apiservertesting.BackingCall("ModelConfig"),

		// caching subnets (2nd attepmt): fails
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),

		// caching subnets (3rd attempt): fails
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.NetworkingEnvironCall("Subnets", s.callContext, instance.UnknownId, []network.Id(nil)),

		// caching subnets (4th attempt): succeeds
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.NetworkingEnvironCall("Subnets", s.callContext, instance.UnknownId, []network.Id(nil)),

		// caching spaces (1st and 2nd attempts)
		apiservertesting.BackingCall("AllSpaces"),
		apiservertesting.BackingCall("AllSpaces"),

		// cacing zones (1st and 2nd attempts)
		apiservertesting.BackingCall("AvailabilityZones"),
		apiservertesting.BackingCall("AvailabilityZones"),

		// validation done; adding subnets to backing store
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[0]),
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[0]),
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[1]),
		apiservertesting.BackingCall("AddSubnet", expectedBackingInfos[2]),
	)
	apiservertesting.ResetStub(apiservertesting.SharedStub)

	// Finally, check that no params yields no results.
	results, err = s.facade.AddSubnets(params.AddSubnetsParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.NotNil)
	c.Assert(results.Results, gc.HasLen, 0)

	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
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
			SubnetTag: "subnet-10.42.0.0/16",
			SpaceTag:  "space-dmz",
			Zones:     []string{"zone1"},
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

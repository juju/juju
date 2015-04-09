// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/subnets"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	providercommon "github.com/juju/juju/provider/common"
	coretesting "github.com/juju/juju/testing"
)

type SubnetsSuite struct {
	coretesting.BaseSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     subnets.API
}

var _ = gc.Suite(&SubnetsSuite{})

func (s *SubnetsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	BackingInstance.SetUp(c, StubZonedEnvironName, true)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewUserTag("admin"),
		EnvironManager: false,
	}

	var err error
	s.facade, err = subnets.NewAPI(BackingInstance, s.resources, s.authorizer)
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

func (s *SubnetsSuite) TestNewAPI(c *gc.C) {
	// Clients are allowed.
	facade, err := subnets.NewAPI(BackingInstance, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	CheckMethodCalls(c, SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = subnets.NewAPI(BackingInstance, s.resources, agentAuthorizer)
	c.Assert(err, jc.DeepEquals, common.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	CheckMethodCalls(c, SharedStub)
}

func (s *SubnetsSuite) TestAllZonesWhenBackingAvailabilityZonesFails(c *gc.C) {
	SharedStub.SetErrors(errors.NotSupportedf("zones"))

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches, "zones not supported")
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
	)
}

func (s *SubnetsSuite) TestAllZonesUsesBackingZonesWhenAvailable(c *gc.C) {
	results, err := s.facade.AllZones()
	c.Assert(err, jc.ErrorIsNil)
	s.AssertAllZonesResult(c, results, BackingInstance.Zones)

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesUpdates(c *gc.C) {
	BackingInstance.SetUp(c, StubZonedEnvironName, false)

	results, err := s.facade.AllZones()
	c.Assert(err, jc.ErrorIsNil)
	s.AssertAllZonesResult(c, results, ProviderInstance.Zones)

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
		BackingCall("EnvironConfig"),
		ProviderCall("Open", BackingInstance.EnvConfig),
		ZonedEnvironCall("AvailabilityZones"),
		BackingCall("SetAvailabilityZones", ProviderInstance.Zones),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndSetFails(c *gc.C) {
	BackingInstance.SetUp(c, StubZonedEnvironName, false)
	SharedStub.SetErrors(
		nil, // Backing.AvailabilityZones
		nil, // Backing.EnvironConfig
		nil, // Provider.Open
		nil, // ZonedEnviron.AvailabilityZones
		errors.NotSupportedf("setting"), // Backing.SetAvailabilityZones
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: setting not supported`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
		BackingCall("EnvironConfig"),
		ProviderCall("Open", BackingInstance.EnvConfig),
		ZonedEnvironCall("AvailabilityZones"),
		BackingCall("SetAvailabilityZones", ProviderInstance.Zones),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndFetchingZonesFails(c *gc.C) {
	BackingInstance.SetUp(c, StubZonedEnvironName, false)
	SharedStub.SetErrors(
		nil, // Backing.AvailabilityZones
		nil, // Backing.EnvironConfig
		nil, // Provider.Open
		errors.NotValidf("foo"), // ZonedEnviron.AvailabilityZones
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: foo not valid`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
		BackingCall("EnvironConfig"),
		ProviderCall("Open", BackingInstance.EnvConfig),
		ZonedEnvironCall("AvailabilityZones"),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndEnvironConfigFails(c *gc.C) {
	BackingInstance.SetUp(c, StubZonedEnvironName, false)
	SharedStub.SetErrors(
		nil, // Backing.AvailabilityZones
		errors.NotFoundf("config"), // Backing.EnvironConfig
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: getting environment config: config not found`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
		BackingCall("EnvironConfig"),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndOpenFails(c *gc.C) {
	BackingInstance.SetUp(c, StubZonedEnvironName, false)
	SharedStub.SetErrors(
		nil, // Backing.AvailabilityZones
		nil, // Backing.EnvironConfig
		errors.NotValidf("config"), // Provider.Open
	)

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: getting environment: config not valid`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotValid)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
		BackingCall("EnvironConfig"),
		ProviderCall("Open", BackingInstance.EnvConfig),
	)
}

func (s *SubnetsSuite) TestAllZonesWithNoBackingZonesAndZonesNotSupported(c *gc.C) {
	BackingInstance.SetUp(c, StubEnvironName, false) // ZonedEnviron not supported

	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches,
		`cannot update known zones: availability zones not supported`,
	)
	// Verify the cause is not obscured.
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

	CheckMethodCalls(c, SharedStub,
		BackingCall("AvailabilityZones"),
		BackingCall("EnvironConfig"),
		ProviderCall("Open", BackingInstance.EnvConfig),
	)
}

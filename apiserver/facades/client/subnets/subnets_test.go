// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	networkcommonmocks "github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/subnets"
	"github.com/juju/juju/apiserver/facades/client/subnets/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// SubnetSuite uses mocks for testing.
// All future facade tests should be added to this suite.
type SubnetSuite struct {
	mockBacking          *mocks.MockBacking
	mockResource         *facademocks.MockResources
	mockCloudCallContext *context.CloudCallContext
	mockAuthorizer       *facademocks.MockAuthorizer

	api *subnets.API
}

var _ = gc.Suite(&SubnetSuite{})

func (s *SubnetSuite) TearDownTest(c *gc.C) {
	s.api = nil
}

func (s *SubnetSuite) TestSubnetsByCIDR(c *gc.C) {
	ctrl := s.setupSubnetsAPI(c)
	defer ctrl.Finish()

	cidrs := []string{"10.10.10.0/24", "10.10.20.0/24", "not-a-cidr"}

	subnet := networkcommonmocks.NewMockBackingSubnet(ctrl)
	sExp := subnet.EXPECT()
	sExp.ID().Return("1")
	sExp.CIDR().Return("10.10.20.0/24")
	sExp.SpaceName().Return("space")
	sExp.VLANTag().Return(0)
	sExp.ProviderId().Return(network.Id("0"))
	sExp.ProviderNetworkId().Return(network.Id("1"))
	sExp.AvailabilityZones().Return([]string{"bar", "bam"})
	sExp.Life().Return(state.Alive)

	bExp := s.mockBacking.EXPECT()
	gomock.InOrder(
		bExp.SubnetsByCIDR(cidrs[0]).Return(nil, errors.New("bad-mongo")),
		bExp.SubnetsByCIDR(cidrs[1]).Return([]networkingcommon.BackingSubnet{subnet}, nil),
		// No call for cidrs[2]; the input is invalidated.
	)

	arg := params.CIDRParams{CIDRS: cidrs}
	res, err := s.api.SubnetsByCIDR(arg)
	c.Assert(err, jc.ErrorIsNil)

	results := res.Results
	c.Assert(results, gc.HasLen, 3)

	c.Check(results[0].Error.Message, gc.Equals, "bad-mongo")
	c.Check(results[1].Subnets, gc.HasLen, 1)
	c.Check(results[2].Error.Message, gc.Equals, `CIDR "not-a-cidr" not valid`)
}

func (s *SubnetSuite) setupSubnetsAPI(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockResource = facademocks.NewMockResources(ctrl)
	s.mockCloudCallContext = context.NewEmptyCloudCallContext()
	s.mockBacking = mocks.NewMockBacking(ctrl)

	s.mockAuthorizer = facademocks.NewMockAuthorizer(ctrl)
	s.mockAuthorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	s.mockAuthorizer.EXPECT().AuthClient().Return(true)

	s.mockBacking.EXPECT().ModelTag().Return(names.NewModelTag("123"))

	var err error
	s.api, err = subnets.NewAPIWithBacking(s.mockBacking, s.mockCloudCallContext, s.mockResource, s.mockAuthorizer)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl
}

// SubnetsSuite is the old testing suite based on testing stubs.
// This should be phased out in favour of mock-based tests.
// The testing network infrastructure should also be removed as soon as can be
// managed.
type SubnetsSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *subnets.API

	callContext context.ProviderCallContext
}

type stubBacking struct {
	*apiservertesting.StubBacking
}

func (sb *stubBacking) SubnetsByCIDR(_ string) ([]networkingcommon.BackingSubnet, error) {
	panic("should not be called")
}

func (sb *stubBacking) ControllerConfig() (controller.Config, error) {
	return controller.Config{"controller-uuid": coretesting.ControllerTag.Id()}, nil
}

type stubBackingBrokenController struct {
	*apiservertesting.StubBacking
}

func (sb *stubBackingBrokenController) SubnetsByCIDR(_ string) ([]networkingcommon.BackingSubnet, error) {
	panic("should not be called")
}

func (sb *stubBackingBrokenController) ControllerConfig() (controller.Config, error) {
	return controller.Config{}, errors.New("broken controller")
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

	s.callContext = context.NewEmptyCloudCallContext()
	var err error
	s.facade, err = subnets.NewAPIWithBacking(
		&stubBacking{apiservertesting.BackingInstance},
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
func (s *SubnetsSuite) AssertAllZonesResult(c *gc.C, got params.ZoneResults, expected network.AvailabilityZones) {
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
		&stubBacking{apiservertesting.BackingInstance},
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
		&stubBacking{apiservertesting.BackingInstance},
		s.callContext,
		s.resources, agentAuthorizer,
	)
	c.Assert(err, jc.DeepEquals, apiservererrors.ErrPerm)
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

func (s *SubnetsSuite) TestZonedEnvironControllerConfigFail(c *gc.C) {
	var err error
	s.facade, err = subnets.NewAPIWithBacking(
		&stubBackingBrokenController{apiservertesting.BackingInstance},
		s.callContext,
		s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	apiservertesting.BackingInstance.Zones = network.AvailabilityZones{}

	// zonedEnviron is a private method, so use AllZones() as the top level method
	// because it invokes zonedEnviron
	results, err := s.facade.AllZones()
	c.Assert(err, gc.ErrorMatches, "cannot update known zones: getting controller config: broken controller")
	c.Assert(results, jc.DeepEquals, params.ZoneResults{})

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
		`cannot update known zones: opening environment: retrieving model config: config not found`,
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
		`cannot update known zones: opening environment: creating environ for model \"stub-zoned-environ\" \(.*\): config not valid`,
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

func (s *SubnetsSuite) TestListSubnetsAndFiltering(c *gc.C) {
	expected := []params.Subnet{{
		CIDR:              "10.10.0.0/24",
		ProviderId:        "sn-zadf00d",
		ProviderNetworkId: "godspeed",
		VLANTag:           0,
		Life:              life.Alive,
		SpaceTag:          "space-private",
		Zones:             []string{"zone1"},
	}, {
		CIDR:              "2001:db8::/32",
		ProviderId:        "sn-ipv6",
		ProviderNetworkId: "",
		VLANTag:           0,
		Life:              life.Alive,
		SpaceTag:          "space-dmz",
		Zones:             []string{"zone1", "zone3"},
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

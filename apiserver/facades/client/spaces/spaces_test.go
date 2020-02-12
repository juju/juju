// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"fmt"
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	networkcommonmocks "github.com/juju/juju/apiserver/common/networkingcommon/mocks"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/facades/client/spaces/mocks"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	environmocks "github.com/juju/juju/environs/mocks"
	"github.com/juju/juju/state"
	statemocks "github.com/juju/juju/state/mocks"
	coretesting "github.com/juju/juju/testing"
)

// This package contains the move from stub to mocking. Therefore the package is not "*._test"
// We contain the old spaces_test while slowly migrating to the new mocking model.
type SpaceTestMockSuite struct {
	mockBacking          *mocks.MockBacking
	mockResource         *facademocks.MockResources
	mockBlockChecker     *mocks.MockBlockChecker
	mockCloudCallContext *context.CloudCallContext
	mockAuthorizer       *facademocks.MockAuthorizer

	mockOpFactory      *mocks.MockOpFactory
	mockModelOperation *statemocks.MockModelOperation

	api *spaces.API
}

var _ = gc.Suite(&SpaceTestMockSuite{})

func (s *SpaceTestMockSuite) TearDownTest(c *gc.C) {
	s.api = nil
}

func (s *SpaceTestMockSuite) TestShowSpaceDefault(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.expectDefaultSpace(ctrl, "default", nil, nil)
	s.expectEndpointBindings(s.getDefaultApplicationEndpoints(), nil)
	s.expectMachines(ctrl, s.getDefaultSpaces(), nil, nil)

	expectedApplications := []string{"mysql", "mediawiki"}
	sort.Strings(expectedApplications)
	args := s.getShowSpaceArg("default")

	expected := params.ShowSpaceResults{Results: []params.ShowSpaceResult{
		{
			Space: params.Space{Id: "1", Name: "default", Subnets: []params.Subnet{{
				CIDR:              "192.168.0.0/24",
				ProviderId:        "0",
				ProviderNetworkId: "1",
				ProviderSpaceId:   "",
				VLANTag:           0,
				Life:              "alive",
				SpaceTag:          args.Entities[0].Tag,
				Zones:             []string{"bar", "bam"},
				Status:            "in-use",
			}}},
			Error: nil,
			// Applications = 2, as 2 applications are having a bind on that space.
			Applications: expectedApplications,
			// MachineCount = 2, as two machines has constraints on the space.
			MachineCount: 2,
		},
	}}

	res, err := s.api.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, jc.DeepEquals, expected)
}

func (s *SpaceTestMockSuite) TestShowSpaceErrorGettingSpace(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", bamErr, nil)
	args := s.getShowSpaceArg("default")

	res, err := s.api.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching space %q: %v", args.Entities[0].Tag, bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *SpaceTestMockSuite) TestShowSpaceErrorGettingSubnets(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil, bamErr)
	args := s.getShowSpaceArg("default")

	res, err := s.api.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching subnets: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *SpaceTestMockSuite) TestShowSpaceErrorGettingApplications(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil, nil)
	s.expectEndpointBindings(s.getDefaultApplicationEndpoints(), bamErr)

	args := s.getShowSpaceArg("default")

	res, err := s.api.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching applications: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *SpaceTestMockSuite) TestShowSpaceErrorGettingMachines(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, "default", nil, nil)
	s.expectEndpointBindings(s.getDefaultApplicationEndpoints(), nil)
	s.expectMachines(ctrl, s.getDefaultSpaces(), bamErr, nil)

	args := s.getShowSpaceArg("default")
	res, err := s.api.ShowSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("fetching machine count: %v", bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *SpaceTestMockSuite) TestRenameSpaceErrorToAlreadyExist(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()

	s.expectDefaultSpace(ctrl, "blub", nil, nil)

	from, to := "bla", "blub"
	args := s.getRenameArgs(from, to)

	res, err := s.api.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("space: %q already exists", to)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *SpaceTestMockSuite) TestRenameSpaceErrorUnexpectedError(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, bamErr, nil)

	args := s.getRenameArgs(from, to)

	res, err := s.api.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := fmt.Sprintf("retrieving space: %q unexpected error, besides not found: %v", to, bamErr.Error())
	c.Assert(res.Results[0].Error, gc.ErrorMatches, expectedErr)
}

func (s *SpaceTestMockSuite) TestRenameSpaceErrorRename(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	bamErr := errors.New("bam")
	s.expectDefaultSpace(ctrl, to, errors.NotFoundf(""), nil)
	args := s.getRenameArgs(from, to)

	s.mockOpFactory.EXPECT().NewRenameSpaceModelOp(from, to).Return(nil, bamErr)

	res, err := s.api.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, bamErr.Error())
}

func (s *SpaceTestMockSuite) TestRenameAlphaSpaceError(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "alpha", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.api.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.ErrorMatches, "the alpha space cannot be renamed")
}

func (s *SpaceTestMockSuite) TestRenameSpaceSuccess(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	s.mockOpFactory.EXPECT().NewRenameSpaceModelOp(from, to).Return(s.mockModelOperation, nil)
	s.expectDefaultSpace(ctrl, to, errors.NotFoundf("abc"), nil)
	s.mockBacking.EXPECT().ApplyOperation(s.mockModelOperation).Return(nil)
	args := s.getRenameArgs(from, to)

	res, err := s.api.RenameSpace(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *SpaceTestMockSuite) TestRenameSpaceErrorProviderSpacesSupport(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, true)
	defer ctrl.Finish()
	defer unreg()
	from, to := "bla", "blub"

	args := s.getRenameArgs(from, to)

	res, err := s.api.RenameSpace(args)
	c.Assert(err, gc.ErrorMatches, "renaming provider-sourced spaces not supported")
	c.Assert(res, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult(nil)})
}

func (s *SpaceTestMockSuite) TestMoveToSpaceSuccess(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, false)
	defer ctrl.Finish()
	defer unreg()
	spaceTag := names.NewSpaceTag("myspace")
	aCIDR := "10.0.0.0/24"

	spacesMock := networkcommonmocks.NewMockBackingSpace(ctrl)
	spacesMock.EXPECT().Id().Return("1").Times(2)
	s.mockBacking.EXPECT().SpaceByName(spaceTag.Id()).Return(spacesMock, nil)

	subnetMock := networkcommonmocks.NewMockBackingSubnet(ctrl)
	subnetMock.EXPECT().SpaceID().Return("0")
	s.mockBacking.EXPECT().SubnetByCIDR(aCIDR).Return(subnetMock, nil)
	s.mockOpFactory.EXPECT().NewUpdateSpaceModelOp("1", []networkingcommon.BackingSubnet{subnetMock}).Return(s.mockModelOperation, nil)
	s.mockBacking.EXPECT().ApplyOperation(s.mockModelOperation).Return(nil)

	args := params.MoveToSpacesParams{MoveToSpace: []params.MoveToSpaceParams{
		{
			CIDRs:    []string{aCIDR},
			SpaceTag: spaceTag.String(),
		},
	}}

	res, err := s.api.MoveToSpace(args)

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results[0].Error, gc.IsNil)
}

func (s *SpaceTestMockSuite) TestMoveToSpaceErrorProviderSpacesSupport(c *gc.C) {
	ctrl, unreg := s.setupSpacesAPI(c, true, true)
	defer ctrl.Finish()
	defer unreg()
	spaceName := "myspace"

	args := params.MoveToSpacesParams{MoveToSpace: []params.MoveToSpaceParams{
		{
			CIDRs:    []string{"192.168.1.0/16"},
			SpaceTag: names.NewSpaceTag(spaceName).String(),
		},
	}}

	res, err := s.api.MoveToSpace(args)

	c.Assert(err, gc.ErrorMatches, "renaming provider-sourced spaces not supported")
	c.Assert(res, gc.DeepEquals, params.MoveToSpaceResults{Results: []params.MoveToSpaceResult(nil)})
}

func (s *SpaceTestMockSuite) getShowSpaceArg(name string) params.Entities {
	spaceTag := names.NewSpaceTag(name)
	args := params.Entities{
		Entities: []params.Entity{{spaceTag.String()}},
	}
	return args
}

func (s *SpaceTestMockSuite) getDefaultApplicationEndpoints() []spaces.ApplicationEndpointBindingsShim {
	endpoints := []spaces.ApplicationEndpointBindingsShim{{
		AppName:  "mysql",
		Bindings: map[string]string{"db": "1", "slave": "alpha"},
	}, {
		AppName:  "mediawiki",
		Bindings: map[string]string{"db": "1", "back": "alpha"},
	},
	}
	return endpoints
}

func (s *SpaceTestMockSuite) getDefaultSpaces() set.Strings {
	strings := set.NewStrings("1", "2")
	return strings
}

func (s *SpaceTestMockSuite) setupSpacesAPI(c *gc.C, supportSpaces bool, isProviderSpaces bool) (*gomock.Controller, func()) {
	ctrl := gomock.NewController(c)
	s.mockResource = facademocks.NewMockResources(ctrl)
	s.mockCloudCallContext = context.NewCloudCallContext()
	s.mockBlockChecker = mocks.NewMockBlockChecker(ctrl)
	s.mockBlockChecker.EXPECT().ChangeAllowed().Return(nil).AnyTimes()
	s.mockBacking = mocks.NewMockBacking(ctrl)
	s.mockOpFactory = mocks.NewMockOpFactory(ctrl)
	s.mockModelOperation = statemocks.NewMockModelOperation(ctrl)

	s.mockAuthorizer = facademocks.NewMockAuthorizer(ctrl)
	s.mockAuthorizer.EXPECT().HasPermission(gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	s.mockAuthorizer.EXPECT().AuthClient().Return(true)

	s.mockBacking.EXPECT().ModelTag().Return(names.NewModelTag("123"))
	s.mockBacking.EXPECT().ModelConfig().Return(nil, nil)

	mockNetworkEnviron := environmocks.NewMockNetworkingEnviron(ctrl)
	mockNetworkEnviron.EXPECT().SupportsSpaces(gomock.Any()).Return(supportSpaces, nil).AnyTimes()
	mockNetworkEnviron.EXPECT().SupportsProviderSpaces(gomock.Any()).Return(isProviderSpaces, nil).AnyTimes()
	mockProvider := environmocks.NewMockCloudEnvironProvider(ctrl)
	mockProvider.EXPECT().Open(gomock.Any()).Return(mockNetworkEnviron, nil)

	unreg := environs.RegisterProvider("mock-provider", mockProvider)

	cloudspec := environs.CloudSpec{
		Type:             "mock-provider",
		Name:             "cloud-name",
		Endpoint:         "endpoint",
		IdentityEndpoint: "identity-endpoint",
		StorageEndpoint:  "storage-endpoint",
	}

	s.mockBacking.EXPECT().CloudSpec().Return(cloudspec, nil)

	var err error
	s.api, err = spaces.NewAPIWithBacking(s.mockBacking, s.mockBlockChecker, s.mockCloudCallContext, s.mockResource, s.mockAuthorizer, s.mockOpFactory)
	c.Assert(err, jc.ErrorIsNil)
	return ctrl, unreg
}

func (s *SpaceTestMockSuite) expectEndpointBindings(endpoints []spaces.ApplicationEndpointBindingsShim, err error) {
	s.mockBacking.EXPECT().AllEndpointBindings().Return(endpoints, err)
}

// expectDefaultSpace configures a default space mock with default subnet settings
func (s *SpaceTestMockSuite) expectDefaultSpace(ctrl *gomock.Controller, name string, spacesErr, subnetErr error) {
	subnetMock := networkcommonmocks.NewMockBackingSubnet(ctrl)
	subnetMock.EXPECT().CIDR().Return("192.168.0.0/24").AnyTimes()
	subnetMock.EXPECT().SpaceID().Return("1").AnyTimes()
	subnetMock.EXPECT().SpaceName().Return(name).AnyTimes()
	subnetMock.EXPECT().VLANTag().Return(0).AnyTimes()
	subnetMock.EXPECT().ProviderId().Return(network.Id("0")).AnyTimes()
	subnetMock.EXPECT().ProviderNetworkId().Return(network.Id("1")).AnyTimes()
	subnetMock.EXPECT().AvailabilityZones().Return([]string{"bar", "bam"}).AnyTimes()
	subnetMock.EXPECT().Status().Return("in-use").AnyTimes()
	subnetMock.EXPECT().Life().Return(life.Value("alive")).AnyTimes()
	subnetMock.EXPECT().ID().Return("111").AnyTimes()

	spacesMock := networkcommonmocks.NewMockBackingSpace(ctrl)
	spacesMock.EXPECT().Id().Return("1").AnyTimes()
	spacesMock.EXPECT().Name().Return(name).AnyTimes()
	spacesMock.EXPECT().Subnets().Return([]networkingcommon.BackingSubnet{subnetMock}, subnetErr).AnyTimes()
	if spacesErr != nil {
		s.mockBacking.EXPECT().SpaceByName(name).Return(nil, spacesErr)
	} else {
		s.mockBacking.EXPECT().SpaceByName(name).Return(spacesMock, nil)
	}
}

func (s *SpaceTestMockSuite) expectMachines(ctrl *gomock.Controller, addresses set.Strings, machErr, addressesErr error) {
	mockMachine := mocks.NewMockMachine(ctrl)
	// With this we can ensure that the function correctly adds up multiple machines.
	anotherMockMachine := mocks.NewMockMachine(ctrl)
	if machErr != nil {
		mockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr).AnyTimes()
		anotherMockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr).AnyTimes()
	} else {
		mockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr)
		anotherMockMachine.EXPECT().AllSpaces().Return(addresses, addressesErr)
	}
	mockMachines := []spaces.Machine{mockMachine, anotherMockMachine}
	s.mockBacking.EXPECT().AllMachines().Return(mockMachines, machErr)
}

func (s *SpaceTestMockSuite) getRenameArgs(from, to string) params.RenameSpacesParams {
	spaceTagFrom := names.NewSpaceTag(from)
	spaceTagTo := names.NewSpaceTag(to)
	args := params.RenameSpacesParams{SpacesRenames: []params.RenameSpaceParams{
		{
			FromSpaceTag: spaceTagFrom.String(),
			ToSpaceTag:   spaceTagTo.String(),
		},
	}}
	return args
}

type stubBacking struct {
	*apiservertesting.StubBacking
}

func (sb *stubBacking) ApplyOperation(state.ModelOperation) error {
	panic("should not be called")
}

func (sb *stubBacking) RenameSpace(settingsChanges settings.ItemChanges, constraints map[string]constraints.Value, fromSpaceName, toName string) error {
	panic("should not be called")
}

func (sb *stubBacking) ConstraintsBySpace(spaceName string) (map[string]constraints.Value, error) {
	panic("should not be called")
}

func (sb *stubBacking) ControllerConfig() (controller.Config, error) {
	panic("should not be called")
}

func (sb *stubBacking) Constraints() (constraints.Value, error) {
	panic("should not be called")
}

func (sb *stubBacking) SpaceByName(name string) (networkingcommon.BackingSpace, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllEndpointBindings() ([]spaces.ApplicationEndpointBindingsShim, error) {
	panic("should not be called")
}

func (sb *stubBacking) AllMachines() ([]spaces.Machine, error) {
	panic("should not be called")
}

// This is the old testing suite
type SpacesSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *spaces.API

	callContext  context.ProviderCallContext
	blockChecker mockBlockChecker
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) SetUpSuite(c *gc.C) {
	s.StubNetwork.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *SpacesSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *SpacesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubZonedNetworkingEnvironName,
		apiservertesting.WithZones,
		apiservertesting.WithSpaces,
		apiservertesting.WithSubnets,
	)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewUserTag("admin"),
		Controller: false,
	}

	s.callContext = context.NewCloudCallContext()
	s.blockChecker = mockBlockChecker{}
	var err error
	s.facade, err = spaces.NewAPIWithBacking(
		&stubBacking{apiservertesting.BackingInstance},
		&s.blockChecker,
		s.callContext,
		s.resources, s.authorizer, nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *SpacesSuite) TestNewAPIWithBacking(c *gc.C) {
	// Clients are allowed.
	facade, err := spaces.NewAPIWithBacking(
		&stubBacking{apiservertesting.BackingInstance},
		&s.blockChecker,
		s.callContext,
		s.resources, s.authorizer, nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = spaces.NewAPIWithBacking(
		&stubBacking{apiservertesting.BackingInstance},
		&s.blockChecker,
		context.NewCloudCallContext(),
		s.resources,
		agentAuthorizer, nil,
	)
	c.Assert(err, jc.DeepEquals, common.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

type checkAddSpacesParams struct {
	Name       string
	Subnets    []string
	Error      string
	MakesCall  bool
	Public     bool
	ProviderId string
}

func (s *SpacesSuite) checkAddSpaces(c *gc.C, p checkAddSpacesParams) {
	arg := params.CreateSpaceParams{
		Public:     p.Public,
		ProviderId: p.ProviderId,
	}
	if p.Name != "" {
		arg.SpaceTag = "space-" + p.Name
	}
	if len(p.Subnets) > 0 {
		arg.CIDRs = p.Subnets
	}

	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{arg},
	}

	results, err := s.facade.CreateSpaces(args)

	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	if p.Error == "" {
		c.Assert(results.Results[0].Error, gc.IsNil)
	} else {
		c.Assert(results.Results[0].Error, gc.NotNil)
		c.Assert(results.Results[0].Error, gc.ErrorMatches, p.Error)
	}

	baseCalls := []apiservertesting.StubMethodCall{
		apiservertesting.BackingCall("ModelConfig"),
		apiservertesting.BackingCall("CloudSpec"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.ZonedNetworkingEnvironCall("SupportsSpaces", s.callContext),
	}

	// If we have an expected error, no calls to SubnetByCIDR() nor
	// AddSpace() should be made.  Check the methods called and
	// return.  The exception is TestAddSpacesAPIError cause an
	// error after SubnetByCIDR() is called.
	if p.Error != "" && !subnetCallMade() {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, baseCalls...)
		return
	}

	allCalls := baseCalls
	subnetIDs := []string{}
	for _, cidr := range p.Subnets {
		allCalls = append(allCalls, apiservertesting.BackingCall("SubnetByCIDR", cidr))
		for _, fakeSN := range apiservertesting.BackingInstance.Subnets {
			if fakeSN.CIDR() == cidr {
				subnetIDs = append(subnetIDs, fakeSN.ID())
			}
		}
	}

	// Only add the call to AddSpace() if there are no errors
	// which have continued to this point.
	if p.Error == "" {
		allCalls = append(allCalls, apiservertesting.BackingCall("AddSpace", p.Name, network.Id(p.ProviderId), subnetIDs, p.Public))
	}
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, allCalls...)
}

func subnetCallMade() bool {
	for _, call := range apiservertesting.SharedStub.Calls() {
		if call.FuncName == "SubnetByCIDR" {
			return true
		}
	}
	return false
}

func (s *SpacesSuite) TestAddSpacesOneSubnet(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.10.0.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesTwoSubnets(c *gc.C) {
	apiservertesting.BackingInstance.AdditionalSubnets()
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.10.0.0/24", "10.0.2.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesManySubnets(c *gc.C) {
	apiservertesting.BackingInstance.AdditionalSubnets()
	p := checkAddSpacesParams{
		Name: "foo",
		Subnets: []string{"10.10.0.0/24", "10.0.2.0/24",
			"10.0.3.0/24", "10.0.4.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesCreateInvalidSpace(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "-",
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"space--" is not a valid space tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesCreateInvalidCIDR(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"bar"},
		Error:   `"bar" is not a valid CIDR`,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesPublic(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.10.0.0/24"},
		Public:  true,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesProviderId(c *gc.C) {
	p := checkAddSpacesParams{
		Name:       "foo",
		Subnets:    []string{"10.10.0.0/24"},
		ProviderId: "foobar",
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesEmptySpaceName(c *gc.C) {
	p := checkAddSpacesParams{
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"" is not a valid tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesNoSubnets(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: nil,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesAPIError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                                // Backing.ModelConfig()
		nil,                                // Backing.CloudSpec()
		nil,                                // Provider.Open()
		nil,                                // ZonedNetworkingEnviron.SupportsSpaces()
		errors.AlreadyExistsf("space-foo"), // Backing.AddSpace()
	)
	p := checkAddSpacesParams{
		Name:      "foo",
		Subnets:   []string{"10.10.0.0/24"},
		MakesCall: true,
		Error:     "space-foo already exists",
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestShowSpaceError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	entities := params.Entities{}
	_, err := s.facade.ShowSpace(entities)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(args)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesProviderOpenError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // Provider.Open()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(args)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	args := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(args)
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestListSpacesDefault(c *gc.C) {
	expected := []params.Space{{
		Id:   "1",
		Name: "default",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.0.0/24",
			ProviderId: "provider-192.168.0.0/24",
			Zones:      []string{"foo"},
			Status:     "in-use",
			SpaceTag:   "space-default",
		}, {
			CIDR:       "192.168.3.0/24",
			ProviderId: "provider-192.168.3.0/24",
			VLANTag:    23,
			Zones:      []string{"bar", "bam"},
			SpaceTag:   "space-default",
		}},
	}, {
		Id:   "2",
		Name: "dmz",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.1.0/24",
			ProviderId: "provider-192.168.1.0/24",
			VLANTag:    23,
			Zones:      []string{"bar", "bam"},
			SpaceTag:   "space-dmz",
		}},
	}, {
		Id:   "3",
		Name: "private",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.2.0/24",
			ProviderId: "provider-192.168.2.0/24",
			Zones:      []string{"foo"},
			Status:     "in-use",
			SpaceTag:   "space-private",
		}},
	}}

	result, err := s.facade.ListSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, jc.DeepEquals, expected)
}

func (s *SpacesSuite) TestListSpacesAllSpacesError(c *gc.C) {
	boom := errors.New("backing boom")
	apiservertesting.BackingInstance.SetErrors(boom)
	_, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "getting environ: backing boom")
}

func (s *SpacesSuite) TestListSpacesSubnetsError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                                 // Backing.ModelConfig()
		nil,                                 // Backing.CloudSpec()
		nil,                                 // Provider.Open()
		nil,                                 // ZonedNetworkingEnviron.supportsSpaces()
		nil,                                 // Backing.AllSpaces()
		errors.New("space0 subnets failed"), // Space.Subnets()
		errors.New("space1 subnets failed"), // Space.Subnets()
		errors.New("space2 subnets failed"), // Space.Subnets()
	)

	results, err := s.facade.ListSpaces()
	for i, space := range results.Results {
		errmsg := fmt.Sprintf("fetching subnets: space%d subnets failed", i)
		c.Assert(space.Error, gc.ErrorMatches, errmsg)
	}
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SpacesSuite) TestListSpacesSubnetsSingleSubnetError(c *gc.C) {
	boom := errors.New("boom")
	apiservertesting.SharedStub.SetErrors(
		nil,  // Backing.ModelConfig()
		nil,  // Backing.CloudSpec()
		nil,  // Provider.Open()
		nil,  // ZonedNetworkingEnviron.supportsSpaces()
		nil,  // Backing.AllSpaces()
		nil,  // Space.Subnets() (1st no error)
		boom, // Space.Subnets() (2nd with error)
	)

	results, err := s.facade.ListSpaces()
	for i, space := range results.Results {
		if i == 1 {
			c.Assert(space.Error, gc.ErrorMatches, "fetching subnets: boom")
		} else {
			c.Assert(space.Error, gc.IsNil)
		}
	}
	c.Assert(err, jc.ErrorIsNil)
}

func (s *SpacesSuite) TestListSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.supportsSpaces()
	)

	_, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestReloadSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.supportsSpaces()
	)
	err := s.facade.ReloadSpaces()
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestReloadSpacesBlocked(c *gc.C) {
	s.blockChecker.SetErrors(common.ServerError(common.OperationBlockedError("test block")))
	err := s.facade.ReloadSpaces()
	c.Assert(err, gc.ErrorMatches, "test block")
	c.Assert(err, jc.Satisfies, params.IsCodeOperationBlocked)
}

func (s *SpacesSuite) TestCreateSpacesBlocked(c *gc.C) {
	s.blockChecker.SetErrors(common.ServerError(common.OperationBlockedError("test block")))
	_, err := s.facade.CreateSpaces(params.CreateSpacesParams{})
	c.Assert(err, gc.ErrorMatches, "test block")
	c.Assert(err, jc.Satisfies, params.IsCodeOperationBlocked)
}

func (s *SpacesSuite) TestCreateSpacesAPIv4(c *gc.C) {
	apiV4 := &spaces.APIv4{&spaces.APIv5{s.facade}}
	results, err := apiV4.CreateSpaces(params.CreateSpacesParamsV4{
		Spaces: []params.CreateSpaceParamsV4{
			{
				SpaceTag:   "space-foo",
				SubnetTags: []string{"subnet-10.10.0.0/24"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *SpacesSuite) TestCreateSpacesAPIv4FailCIDR(c *gc.C) {
	apiV4 := &spaces.APIv4{&spaces.APIv5{s.facade}}
	results, err := apiV4.CreateSpaces(params.CreateSpacesParamsV4{
		Spaces: []params.CreateSpaceParamsV4{
			{
				SpaceTag:   "space-foo",
				SubnetTags: []string{"subnet-bar"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `"bar" is not a valid CIDR`)
}

func (s *SpacesSuite) TestCreateSpacesAPIv4FailTag(c *gc.C) {
	apiV4 := &spaces.APIv4{&spaces.APIv5{s.facade}}
	results, err := apiV4.CreateSpaces(params.CreateSpacesParamsV4{
		Spaces: []params.CreateSpaceParamsV4{
			{
				SpaceTag:   "space-foo",
				SubnetTags: []string{"bar"},
			},
		},
	})
	c.Assert(err, gc.IsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `"bar" is not valid SubnetTag`)
}

func (s *SpacesSuite) TestReloadSpacesUserDenied(c *gc.C) {
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewUserTag("regular")
	facade, err := spaces.NewAPIWithBacking(
		&stubBacking{apiservertesting.BackingInstance},
		&s.blockChecker,
		context.NewCloudCallContext(),
		s.resources, agentAuthorizer, nil,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = facade.ReloadSpaces()
	c.Check(err, gc.ErrorMatches, "permission denied")
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

func (s *SpacesSuite) TestSuppportsSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewCloudCallContext())
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestSuppportsSpacesEnvironNewError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // environs.New()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewCloudCallContext())
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestSuppportsSpacesWithoutNetworking(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubEnvironName,
		apiservertesting.WithoutZones,
		apiservertesting.WithoutSpaces,
		apiservertesting.WithoutSubnets)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewCloudCallContext())
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *SpacesSuite) TestSuppportsSpacesWithoutSpaces(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubNetworkingEnvironName,
		apiservertesting.WithoutZones,
		apiservertesting.WithoutSpaces,
		apiservertesting.WithoutSubnets)

	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		nil,                // environs.New()
		errors.New("boom"), // Backing.supportsSpaces()
	)

	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewCloudCallContext())
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *SpacesSuite) TestSuppportsSpaces(c *gc.C) {
	err := spaces.SupportsSpaces(&stubBacking{apiservertesting.BackingInstance}, context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
}

type mockBlockChecker struct {
	jtesting.Stub
}

func (c *mockBlockChecker) ChangeAllowed() error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}

func (c *mockBlockChecker) RemoveAllowed() error {
	c.MethodCall(c, "RemoveAllowed")
	return c.NextErr()
}

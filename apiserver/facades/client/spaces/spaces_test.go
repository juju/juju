// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"fmt"

	"github.com/juju/errors"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	coretesting "github.com/juju/juju/testing"
)

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
		apiservertesting.BackingInstance,
		&s.blockChecker,
		s.callContext,
		s.resources, s.authorizer,
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
		apiservertesting.BackingInstance,
		&s.blockChecker,
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
	facade, err = spaces.NewAPIWithBacking(
		apiservertesting.BackingInstance,
		&s.blockChecker,
		context.NewCloudCallContext(),
		s.resources,
		agentAuthorizer,
	)
	c.Assert(err, jc.DeepEquals, common.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

type checkAddSpacesParams struct {
	Name      string
	Subnets   []string
	Error     string
	MakesCall bool
	Public    bool
}

func (s *SpacesSuite) checkAddSpaces(c *gc.C, p checkAddSpacesParams) {
	args := params.CreateSpaceParams{}
	if p.Name != "" {
		args.SpaceTag = "space-" + p.Name
	}

	if len(p.Subnets) > 0 {
		args.CIDRs = p.Subnets
	}
	args.Public = p.Public

	spaces := params.CreateSpacesParams{}
	spaces.Spaces = append(spaces.Spaces, args)
	results, err := s.facade.CreateSpaces(spaces)

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

	// If we have an expected error, no calls to Subnet() nor
	// AddSpace() should be made.  Check the methods called and
	// return.  The exception is TestAddSpacesAPIError cause an
	// error after Subnet() is called.
	if p.Error != "" && !subnetCallMade() {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, baseCalls...)
		return
	}

	allCalls := baseCalls
	subnetIDs := []string{}
	for _, cidr := range p.Subnets {
		allCalls = append(allCalls, apiservertesting.BackingCall("Subnet", cidr))
		for _, fakeSN := range apiservertesting.BackingInstance.Subnets {
			if fakeSN.CIDR() == cidr {
				subnetIDs = append(subnetIDs, fakeSN.ID())
			}
		}
	}

	// Only add the call to AddSpace() if there are no errors
	// which have continued to this point.
	if p.Error == "" {
		allCalls = append(allCalls, apiservertesting.BackingCall("AddSpace", p.Name, network.Id(""), subnetIDs, p.Public))
	}
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, allCalls...)
}

func subnetCallMade() bool {
	for _, call := range apiservertesting.SharedStub.Calls() {
		if call.FuncName == "Subnet" {
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
		nil,                                 // ZonedNetworkingEnviron.SupportsSpaces()
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
		nil,  // ZonedNetworkingEnviron.SupportsSpaces()
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
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	_, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestReloadSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
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
	apiV4 := &spaces.APIv4{s.facade}
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
	apiV4 := &spaces.APIv4{s.facade}
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
	apiV4 := &spaces.APIv4{s.facade}
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
		apiservertesting.BackingInstance,
		&s.blockChecker,
		context.NewCloudCallContext(),
		s.resources, agentAuthorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = facade.ReloadSpaces()
	c.Check(err, gc.ErrorMatches, "permission denied")
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
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

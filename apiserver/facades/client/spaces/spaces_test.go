// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/spaces"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

type SpacesSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     spaces.API

	callContext context.ProviderCallContext
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
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedNetworkingEnvironName, apiservertesting.WithZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:        names.NewUserTag("admin"),
		Controller: false,
	}

	s.callContext = context.NewCloudCallContext()
	var err error
	s.facade, err = spaces.NewAPIWithBacking(
		apiservertesting.BackingInstance,
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
		for _, cidr := range p.Subnets {
			args.SubnetTags = append(args.SubnetTags, "subnet-"+cidr)
		}
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

	// AddSpace from the api always uses an empty ProviderId.
	addSpaceCalls := append(baseCalls, apiservertesting.BackingCall("AddSpace", p.Name, network.Id(""), p.Subnets, p.Public))

	if p.Error == "" || p.MakesCall {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, addSpaceCalls...)
	} else {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, baseCalls...)
	}
}

func (s *SpacesSuite) TestAddSpacesOneSubnet(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.0.0.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesTwoSubnets(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.0.0.0/24", "10.0.1.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesManySubnets(c *gc.C) {
	p := checkAddSpacesParams{
		Name: "foo",
		Subnets: []string{"10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24",
			"10.0.3.0/24", "10.0.4.0/24", "10.0.5.0/24", "10.0.6.0/24"},
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestAddSpacesAPIError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil, // Backing.ModelConfig()
		nil, // Backing.CloudSpec()
		nil, // Provider.Open()
		nil, // ZonedNetworkingEnviron.SupportsSpaces()
		errors.AlreadyExistsf("space-foo"), // Backing.AddSpace()
	)
	p := checkAddSpacesParams{
		Name:      "foo",
		Subnets:   []string{"10.0.0.0/24"},
		MakesCall: true,
		Error:     "space-foo already exists",
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestCreateInvalidSpace(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "-",
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"space--" is not a valid space tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestCreateInvalidSubnet(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"bar"},
		Error:   `"subnet-bar" is not a valid subnet tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestPublic(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.0.0.0/24"},
		Public:  true,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestEmptySpaceName(c *gc.C) {
	p := checkAddSpacesParams{
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"" is not a valid tag`,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestNoSubnets(c *gc.C) {
	p := checkAddSpacesParams{
		Name:    "foo",
		Subnets: nil,
	}
	s.checkAddSpaces(c, p)
}

func (s *SpacesSuite) TestListSpacesDefault(c *gc.C) {
	expected := []params.Space{{
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
		Name: "dmz",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.1.0/24",
			ProviderId: "provider-192.168.1.0/24",
			VLANTag:    23,
			Zones:      []string{"bar", "bam"},
			SpaceTag:   "space-dmz",
		}},
	}, {
		Name: "private",
		Subnets: []params.Subnet{{
			CIDR:       "192.168.2.0/24",
			ProviderId: "provider-192.168.2.0/24",
			Zones:      []string{"foo"},
			Status:     "in-use",
			SpaceTag:   "space-private",
		}},
	}}
	spaces, err := s.facade.ListSpaces()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(spaces.Results, jc.DeepEquals, expected)
}

func (s *SpacesSuite) TestListSpacesAllSpacesError(c *gc.C) {
	boom := errors.New("backing boom")
	apiservertesting.BackingInstance.SetErrors(boom)
	_, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "getting environ: backing boom")
}

func (s *SpacesSuite) TestListSpacesSubnetsError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil, // Backing.ModelConfig()
		nil, // Backing.CloudSpec()
		nil, // Provider.Open()
		nil, // ZonedNetworkingEnviron.SupportsSpaces()
		nil, // Backing.AllSpaces()
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

func (s *SpacesSuite) TestCreateSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	spaces := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(spaces)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesProviderOpenError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // Provider.Open()
	)

	spaces := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(spaces)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil, // Backing.ModelConfig()
		nil, // Backing.CloudSpec()
		nil, // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	spaces := params.CreateSpacesParams{}
	_, err := s.facade.CreateSpaces(spaces)
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestListSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil, // Backing.ModelConfig()
		nil, // Backing.CloudSpec()
		nil, // Provider.Open
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	_, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestReloadSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil, // Backing.ModelConfig()
		nil, // Backing.CloudSpec()
		nil, // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)
	err := s.facade.ReloadSpaces()
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestReloadSpacesUserDenied(c *gc.C) {
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewUserTag("regular")
	facade, err := spaces.NewAPIWithBacking(
		apiservertesting.BackingInstance,
		context.NewCloudCallContext(),
		s.resources, agentAuthorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	err = facade.ReloadSpaces()
	c.Check(err, gc.ErrorMatches, "permission denied")
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

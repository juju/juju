// Copyright 201y Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	"github.com/juju/errors"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/spaces"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/network"
	coretesting "github.com/juju/juju/testing"
)

type SpacesSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     spaces.API
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
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
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
	results, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, spaces)

	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	if p.Error == "" {
		c.Assert(results.Results[0].Error, gc.IsNil)
	} else {
		c.Assert(results.Results[0].Error, gc.NotNil)
		c.Assert(results.Results[0].Error, gc.ErrorMatches, p.Error)
	}

	baseCalls := []apiservertesting.StubMethodCall{
		apiservertesting.BackingCall("EnvironConfig"),
		apiservertesting.ProviderCall("Open", apiservertesting.BackingInstance.EnvConfig),
		apiservertesting.ZonedNetworkingEnvironCall("SupportsSpaces"),
	}

	// AddSpace from the api always uses an empty ProviderId.
	addSpaceCalls := append(baseCalls, apiservertesting.BackingCall("AddSpace", p.Name, network.Id(""), p.Subnets, p.Public))

	if p.Error == "" || p.MakesCall {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, addSpaceCalls...)
	} else {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, baseCalls...)
	}
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

func (s *SpacesSuite) TestCreateSpacesEnvironConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.EnvironConfig()
	)

	spaces := params.CreateSpacesParams{}
	_, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, spaces)
	c.Assert(err, gc.ErrorMatches, "getting environment config: boom")
}

func (s *SpacesSuite) TestCreateSpacesProviderOpenError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.EnvironConfig()
		errors.New("boom"), // Provider.Open()
	)

	spaces := params.CreateSpacesParams{}
	_, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, spaces)
	c.Assert(err, gc.ErrorMatches, "validating environment config: boom")
}

func (s *SpacesSuite) TestCreateSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil, // Backing.EnvironConfig()
		nil, // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	spaces := params.CreateSpacesParams{}
	_, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, spaces)
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

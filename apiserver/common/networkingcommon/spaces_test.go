// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package networkingcommon_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/networkingcommon"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	coretesting "github.com/juju/juju/testing"
)

type SpacesSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork
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
		apiservertesting.WithSubnets)
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

type checkCreateSpacesParams struct {
	Name       string
	Subnets    []string
	Error      string
	Public     bool
	ProviderId string
}

func (s *SpacesSuite) checkCreateSpaces(c *gc.C, p checkCreateSpacesParams) {
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
	args.ProviderId = p.ProviderId

	spaces := params.CreateSpacesParams{}
	spaces.Spaces = append(spaces.Spaces, args)
	callCtx := context.NewCloudCallContext()
	results, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, callCtx, spaces)

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
		apiservertesting.ZonedNetworkingEnvironCall("SupportsSpaces", callCtx),
	}

	addSpaceCalls := append(baseCalls, apiservertesting.BackingCall("AddSpace", p.Name, network.Id(p.ProviderId), p.Subnets, p.Public))

	if p.Error == "" {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, addSpaceCalls...)
	} else {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub, baseCalls...)
	}
}

func (s *SpacesSuite) TestCreateInvalidSpace(c *gc.C) {
	p := checkCreateSpacesParams{
		Name:    "-",
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"space--" is not a valid space tag`,
	}
	s.checkCreateSpaces(c, p)
}

func (s *SpacesSuite) TestCreateInvalidSubnet(c *gc.C) {
	p := checkCreateSpacesParams{
		Name:    "foo",
		Subnets: []string{"bar"},
		Error:   `"subnet-bar" is not a valid subnet tag`,
	}
	s.checkCreateSpaces(c, p)
}

func (s *SpacesSuite) TestPublic(c *gc.C) {
	p := checkCreateSpacesParams{
		Name:    "foo",
		Subnets: []string{"10.0.0.0/24"},
		Public:  true,
	}
	s.checkCreateSpaces(c, p)
}

func (s *SpacesSuite) TestProviderId(c *gc.C) {
	p := checkCreateSpacesParams{
		Name:       "foo",
		Subnets:    []string{"10.0.0.0/24"},
		ProviderId: "foobar",
	}
	s.checkCreateSpaces(c, p)
}

func (s *SpacesSuite) TestEmptySpaceName(c *gc.C) {
	p := checkCreateSpacesParams{
		Subnets: []string{"10.0.0.0/24"},
		Error:   `"" is not a valid tag`,
	}
	s.checkCreateSpaces(c, p)
}

func (s *SpacesSuite) TestNoSubnets(c *gc.C) {
	p := checkCreateSpacesParams{
		Name:    "foo",
		Subnets: nil,
	}
	s.checkCreateSpaces(c, p)
}

func (s *SpacesSuite) TestCreateSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	spaces := params.CreateSpacesParams{}
	_, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext(), spaces)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesProviderOpenError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // Provider.Open()
	)

	spaces := params.CreateSpacesParams{}
	_, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext(), spaces)
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestCreateSpacesNotSupportedError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                            // Backing.ModelConfig()
		nil,                            // Backing.CloudSpec()
		nil,                            // Provider.Open()
		errors.NotSupportedf("spaces"), // ZonedNetworkingEnviron.SupportsSpaces()
	)

	spaces := params.CreateSpacesParams{}
	_, err := networkingcommon.CreateSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext(), spaces)
	c.Assert(err, gc.ErrorMatches, "spaces not supported")
}

func (s *SpacesSuite) TestSuppportsSpacesModelConfigError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		errors.New("boom"), // Backing.ModelConfig()
	)

	err := networkingcommon.SupportsSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext())
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestSuppportsSpacesEnvironNewError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(
		nil,                // Backing.ModelConfig()
		nil,                // Backing.CloudSpec()
		errors.New("boom"), // environs.New()
	)

	err := networkingcommon.SupportsSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext())
	c.Assert(err, gc.ErrorMatches, "getting environ: boom")
}

func (s *SpacesSuite) TestSuppportsSpacesWithoutNetworking(c *gc.C) {
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubEnvironName,
		apiservertesting.WithoutZones,
		apiservertesting.WithoutSpaces,
		apiservertesting.WithoutSubnets)

	err := networkingcommon.SupportsSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext())
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
		errors.New("boom"), // Backing.SupportsSpaces()
	)

	err := networkingcommon.SupportsSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext())
	c.Assert(err, jc.Satisfies, errors.IsNotSupported)
}

func (s *SpacesSuite) TestSuppportsSpaces(c *gc.C) {
	err := networkingcommon.SupportsSpaces(apiservertesting.BackingInstance, context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
}

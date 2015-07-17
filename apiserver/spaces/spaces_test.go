// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/spaces"
	apiservertesting "github.com/juju/juju/apiserver/testing"
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
	apiservertesting.BackingInstance.SetUp(c, apiservertesting.StubZonedEnvironName, apiservertesting.WithZones, apiservertesting.WithSpaces, apiservertesting.WithSubnets)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewUserTag("admin"),
		EnvironManager: false,
	}

	var err error
	s.facade, err = spaces.NewAPI(apiservertesting.BackingInstance, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *SpacesSuite) TestNewAPI(c *gc.C) {
	// Clients are allowed.
	facade, err := spaces.NewAPI(apiservertesting.BackingInstance, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(facade, gc.NotNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)

	// Agents are not allowed
	agentAuthorizer := s.authorizer
	agentAuthorizer.Tag = names.NewMachineTag("42")
	facade, err = spaces.NewAPI(apiservertesting.BackingInstance, s.resources, agentAuthorizer)
	c.Assert(err, jc.DeepEquals, common.ErrPerm)
	c.Assert(facade, gc.IsNil)
	// No calls so far.
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

type checkCreateSpaceParams struct {
	Name      string
	Subnets   []string
	Error     string
	MakesCall bool
}

func (s *SpacesSuite) checkCreateSpace(c *gc.C, p checkCreateSpaceParams) {
	args := params.CreateSpaceParams{}
	if p.Name != "" {
		args.SpaceTag = "space-" + p.Name
	}
	if len(p.Subnets) > 0 {
		for _, cidr := range p.Subnets {
			args.SubnetTags = append(args.SubnetTags, "subnet-"+cidr)
		}
	}

	results := s.facade.CreateSpace(args)
	if p.Error == "" {
		c.Assert(results.Error, gc.IsNil)
	} else {
		c.Assert(results.Error, gc.NotNil)
		c.Assert(results.Error, gc.ErrorMatches, p.Error)
	}

	if p.Error == "" || p.MakesCall {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub,
			apiservertesting.BackingCall("CreateSpace", spaces.BackingSpaceInfo{Name: p.Name, Subnets: p.Subnets}),
		)
	} else {
		apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
	}

}

func (s *SpacesSuite) TestCreateNoSpace(c *gc.C) {
	// Try calling CreateSpace with no arguments. We expect this to do nothing.
	args := params.CreateSpaceParams{}
	results := s.facade.CreateSpace(args)
	c.Assert(results.Error, gc.IsNil)

	// No spaces created because the subnets list was empty, so no API calls
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)

	// Now try with a space name, no subnets; should still do nothing.
	args.SpaceTag = "space-foo"
	results = s.facade.CreateSpace(args)
	c.Assert(results.Error, gc.IsNil)

	// Even with a space name, no spaces created because the subnets list
	// was empty, so no API calls
	apiservertesting.CheckMethodCalls(c, apiservertesting.SharedStub)
}

func (s *SpacesSuite) TestCreateSpaceOneSubnet(c *gc.C) {
	p := checkCreateSpaceParams{
		Name:    "foo",
		Subnets: []string{"10.0.0.0/24"}}
	s.checkCreateSpace(c, p)
}

func (s *SpacesSuite) TestCreateSpaceTwoSubnets(c *gc.C) {
	p := checkCreateSpaceParams{
		Name:    "foo",
		Subnets: []string{"10.0.0.0/24", "10.0.1.0/24"}}
	s.checkCreateSpace(c, p)
}

func (s *SpacesSuite) TestCreateSpaceManySubnets(c *gc.C) {
	p := checkCreateSpaceParams{
		Name: "foo",
		Subnets: []string{"10.0.0.0/24", "10.0.1.0/24", "10.0.2.0/24",
			"10.0.3.0/24", "10.0.4.0/24", "10.0.5.0/24", "10.0.6.0/24"}}
	s.checkCreateSpace(c, p)
}

func (s *SpacesSuite) TestCreateSpaceAPIError(c *gc.C) {
	apiservertesting.SharedStub.SetErrors(errors.AlreadyExistsf("space-foo"))
	p := checkCreateSpaceParams{
		Name:      "foo",
		Subnets:   []string{"10.0.0.0/24"},
		MakesCall: true,
		Error:     "cannot add space: space-foo already exists"}
	s.checkCreateSpace(c, p)
}

func (s *SpacesSuite) TestCreateInvalidSpace(c *gc.C) {
	p := checkCreateSpaceParams{
		Name:    "-",
		Subnets: []string{"10.0.0.0/24"},
		Error:   "given SpaceTag is invalid: \"space--\" is not a valid space tag"}
	s.checkCreateSpace(c, p)
}

func (s *SpacesSuite) TestCreateInvalidSubnet(c *gc.C) {
	p := checkCreateSpaceParams{
		Name:    "foo",
		Subnets: []string{"bar"},
		Error:   "given SubnetTag is invalid: \"subnet-bar\" is not a valid subnet tag"}
	s.checkCreateSpace(c, p)
}

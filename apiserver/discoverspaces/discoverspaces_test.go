// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/discoverspaces"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/testing"
)

type DiscoverSpacesSuite struct {
	coretesting.BaseSuite
	apiservertesting.StubNetwork

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer
	facade     *discoverspaces.DiscoverSpacesAPI
}

var _ = gc.Suite(&DiscoverSpacesSuite{})

func (s *DiscoverSpacesSuite) SetUpSuite(c *gc.C) {
	s.StubNetwork.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *DiscoverSpacesSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *DiscoverSpacesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	apiservertesting.BackingInstance.SetUp(
		c,
		apiservertesting.StubZonedEnvironName,
		apiservertesting.WithZones,
		apiservertesting.WithSpaces,
		apiservertesting.WithSubnets)

	s.resources = common.NewResources()
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:            names.NewUserTag("admin"),
		EnvironManager: true,
	}

	var err error
	s.facade, err = discoverspaces.NewDiscoverSpacesAPIWithBacking(
		apiservertesting.BackingInstance, s.resources, s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.facade, gc.NotNil)
}

func (s *DiscoverSpacesSuite) TearDownTest(c *gc.C) {
	if s.resources != nil {
		s.resources.StopAll()
	}
	s.BaseSuite.TearDownTest(c)
}

func (s *DiscoverSpacesSuite) TestModelConfigFailure(c *gc.C) {
	apiservertesting.BackingInstance.SetErrors(errors.New("boom"))

	result, err := s.facade.ModelConfig()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{})

	apiservertesting.BackingInstance.CheckCallNames(c, "ModelConfig")
}

func (s *DiscoverSpacesSuite) TestModelConfigSuccess(c *gc.C) {
	result, err := s.facade.ModelConfig()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelConfigResult{
		Config: apiservertesting.BackingInstance.EnvConfig.AllAttrs(),
	})

	apiservertesting.BackingInstance.CheckCallNames(c, "ModelConfig")
}

func (s *DiscoverSpacesSuite) TestListSpaces(c *gc.C) {
	result, err := s.facade.ListSpaces()
	c.Assert(err, jc.ErrorIsNil)

	expectedResult := []params.ProviderSpace{{
		Name: "default",
		Subnets: []params.Subnet{
			{CIDR: "192.168.0.0/24",
				ProviderId: "provider-192.168.0.0/24",
				SpaceTag:   "space-default",
				Zones:      []string{"foo"},
				Status:     "in-use"},
			{CIDR: "192.168.3.0/24",
				ProviderId: "provider-192.168.3.0/24",
				VLANTag:    23,
				SpaceTag:   "space-default",
				Zones:      []string{"bar", "bam"}}}}, {
		Name: "dmz",
		Subnets: []params.Subnet{
			{CIDR: "192.168.1.0/24",
				ProviderId: "provider-192.168.1.0/24",
				VLANTag:    23,
				SpaceTag:   "space-dmz",
				Zones:      []string{"bar", "bam"}}}}, {
		Name: "private",
		Subnets: []params.Subnet{
			{CIDR: "192.168.2.0/24",
				ProviderId: "provider-192.168.2.0/24",
				SpaceTag:   "space-private",
				Zones:      []string{"foo"},
				Status:     "in-use"}},
	}}
	c.Assert(result.Results, jc.DeepEquals, expectedResult)
	apiservertesting.BackingInstance.CheckCallNames(c, "AllSpaces")
}

func (s *DiscoverSpacesSuite) TestListSpacesFailure(c *gc.C) {
	apiservertesting.BackingInstance.SetErrors(errors.New("boom"))

	result, err := s.facade.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "boom")
	c.Assert(result, jc.DeepEquals, params.DiscoverSpacesResults{})

	apiservertesting.BackingInstance.CheckCallNames(c, "AllSpaces")
}

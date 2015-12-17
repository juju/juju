// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"errors"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/apiserver/params"
)

type networkConfigSuite struct {
	uniterSuite
	commonRelationSuiteMixin

	apiCaller base.APICallCloser
	called    int

	apiRelation *uniter.Relation
}

var _ = gc.Suite(&networkConfigSuite{})

func (s *networkConfigSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)
	s.commonRelationSuiteMixin.SetUpTest(c, s.uniterSuite)

	var err error
	s.apiRelation, err = s.uniter.Relation(s.stateRelation.Tag().(names.RelationTag))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkConfigSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *networkConfigSuite) patchAndCallNetworkConfigFacade(
	c *gc.C, cfg []params.NetworkConfig, errs []error, facadeError error) (
	[]params.NetworkConfig, error) {

	_, err := s.stateRelation.Unit(s.wordpressUnit)
	relationTag := s.stateRelation.Tag()
	unitTag := s.wordpressUnit.Tag()

	apiUnit, err := s.uniter.Unit(unitTag.(names.UnitTag))
	c.Assert(err, jc.ErrorIsNil)

	var called bool
	uniter.PatchUnitFacadeCall(s, apiUnit, func(request string, args, response interface{}) error {
		called = true
		c.Assert(request, gc.Equals, "NetworkConfig")
		result := response.(*params.NetworkConfigs)
		result.Results = cfg
		result.Errors = errs
		return facadeError
	})

	return s.uniter.NetworkConfig(relationTag.(names.RelationTag), unitTag.(names.UnitTag))
}

func (s *networkConfigSuite) TestValidRelationUnitWithNetwork(c *gc.C) {
	cfg := []params.NetworkConfig{
		{
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			NetworkName:   "net1",
			InterfaceName: "eth0",
			Disabled:      false,
		},
	}

	netConfig, err := s.patchAndCallNetworkConfigFacade(c, cfg, nil, nil)

	c.Assert(netConfig, jc.DeepEquals, cfg)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *networkConfigSuite) TestValidRelationUnitWithNoNetworks(c *gc.C) {
	cfg := []params.NetworkConfig{}
	netConfig, err := s.patchAndCallNetworkConfigFacade(c, cfg, nil, nil)
	c.Assert(netConfig, jc.DeepEquals, []params.NetworkConfig{})
	c.Assert(err, gc.ErrorMatches, "expected at least 1 result, got 0")
}

func (s *networkConfigSuite) TestValidRelationUnitWithError(c *gc.C) {
	cfg := []params.NetworkConfig{
		{
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			NetworkName:   "net1",
			InterfaceName: "eth0",
			Disabled:      false,
		},
	}

	netConfig, err := s.patchAndCallNetworkConfigFacade(c, cfg, []error{errors.New("ping")}, nil)

	c.Assert(netConfig, jc.DeepEquals, []params.NetworkConfig{})
	c.Assert(err, gc.ErrorMatches, "ping")
}

func (s *networkConfigSuite) TestValidRelationUnitWithFacadeError(c *gc.C) {
	cfg := []params.NetworkConfig{
		{
			MACAddress:    "aa:bb:cc:dd:ee:f0",
			NetworkName:   "net1",
			InterfaceName: "eth0",
			Disabled:      false,
		},
	}

	netConfig, err := s.patchAndCallNetworkConfigFacade(c, cfg, nil, errors.New("pong"))

	c.Assert(netConfig, jc.DeepEquals, []params.NetworkConfig{})
	c.Assert(err, gc.ErrorMatches, "pong")
}

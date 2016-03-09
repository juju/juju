// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/meterstatus"
	meterstatustesting "github.com/juju/juju/apiserver/meterstatus/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	jujufactory "github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&meterStatusSuite{})

type meterStatusSuite struct {
	jujutesting.JujuConnSuite

	authorizer apiservertesting.FakeAuthorizer
	resources  *common.Resources

	factory *jujufactory.Factory

	unit *state.Unit

	status meterstatus.MeterStatus
}

func (s *meterStatusSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.factory = jujufactory.NewFactory(s.State)
	s.unit = s.factory.MakeUnit(c, nil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming unit 0 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.unit.UnitTag(),
	}

	// Create the resource registry separately to track invocations to
	// Register.
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	status, err := meterstatus.NewMeterStatusAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	s.status = status
}

func (s *meterStatusSuite) TestGetMeterStatusUnauthenticated(c *gc.C) {
	service, err := s.unit.Service()
	c.Assert(err, jc.ErrorIsNil)
	otherunit := s.factory.MakeUnit(c, &jujufactory.UnitParams{Service: service})
	args := params.Entities{Entities: []params.Entity{{otherunit.Tag().String()}}}
	result, err := s.status.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.ErrorMatches, "permission denied")
	c.Assert(result.Results[0].Code, gc.Equals, "")
	c.Assert(result.Results[0].Info, gc.Equals, "")
}

func (s *meterStatusSuite) TestGetMeterStatusBadTag(c *gc.C) {
	tags := []string{
		"user-admin",
		"unit-nosuchunit",
		"thisisnotatag",
		"machine-0",
		"model-blah",
	}
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i] = params.Entity{Tag: tag}
	}
	result, err := s.status.GetMeterStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(tags))
	for i, result := range result.Results {
		c.Logf("checking result %d", i)
		c.Assert(result.Code, gc.Equals, "")
		c.Assert(result.Info, gc.Equals, "")
		c.Assert(result.Error, gc.ErrorMatches, "permission denied")
	}
}

func (s *meterStatusSuite) TestGetMeterStatus(c *gc.C) {
	meterstatustesting.TestGetMeterStatus(c, s.status, s.unit)
}

func (s *meterStatusSuite) TestWatchMeterStatus(c *gc.C) {
	meterstatustesting.TestWatchMeterStatus(c, s.status, s.unit, s.State, s.resources)
}

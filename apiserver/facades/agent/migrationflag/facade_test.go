// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/migrationflag"
	"github.com/juju/juju/rpc/params"
	coretesting "github.com/juju/juju/testing"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (*FacadeSuite) TestAcceptsMachineAgent(c *gc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{machine: true})
	c.Check(err, jc.ErrorIsNil)
	c.Check(facade, gc.NotNil)
}

func (*FacadeSuite) TestAcceptsUnitAgent(c *gc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{machine: true})
	c.Check(err, jc.ErrorIsNil)
	c.Check(facade, gc.NotNil)
}

func (*FacadeSuite) TestAcceptsApplicationAgent(c *gc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{application: true})
	c.Check(err, jc.ErrorIsNil)
	c.Check(facade, gc.NotNil)
}

func (*FacadeSuite) TestRejectsNonAgent(c *gc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{})
	c.Check(err, gc.Equals, apiservererrors.ErrPerm)
	c.Check(facade, gc.IsNil)
}

func (*FacadeSuite) TestPhaseSuccess(c *gc.C) {
	stub := &testing.Stub{}
	backend := newMockBackend(stub)
	facade, err := migrationflag.New(backend, nil, authOK)
	c.Assert(err, jc.ErrorIsNil)

	results := facade.Phase(context.Background(), entities(
		coretesting.ModelTag.String(),
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, gc.HasLen, 2)
	stub.CheckCallNames(c, "MigrationPhase", "MigrationPhase")

	for _, result := range results.Results {
		c.Check(result.Error, gc.IsNil)
		c.Check(result.Phase, gc.Equals, "REAP")
	}
}

func (*FacadeSuite) TestPhaseErrors(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(errors.New("ouch"))
	backend := newMockBackend(stub)
	facade, err := migrationflag.New(backend, nil, authOK)
	c.Assert(err, jc.ErrorIsNil)

	// 3 entities: unparseable, unauthorized, call error.
	results := facade.Phase(context.Background(), entities(
		"urgle",
		unknownModel,
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, gc.HasLen, 3)
	stub.CheckCallNames(c, "MigrationPhase")

	c.Check(results.Results, jc.DeepEquals, []params.PhaseResult{{
		Error: &params.Error{
			Message: `"urgle" is not a valid tag`,
		}}, {
		Error: &params.Error{
			Message: "permission denied",
			Code:    "unauthorized access",
		}}, {
		Error: &params.Error{
			Message: "ouch",
		},
	}})
}

func (*FacadeSuite) TestWatchSuccess(c *gc.C) {
	stub := &testing.Stub{}
	backend := newMockBackend(stub)
	resources := common.NewResources()
	facade, err := migrationflag.New(backend, resources, authOK)
	c.Assert(err, jc.ErrorIsNil)

	results := facade.Watch(context.Background(), entities(
		coretesting.ModelTag.String(),
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, gc.HasLen, 2)
	stub.CheckCallNames(c, "WatchMigrationPhase", "WatchMigrationPhase")

	check := func(result params.NotifyWatchResult) {
		c.Check(result.Error, gc.IsNil)
		resource := resources.Get(result.NotifyWatcherId)
		c.Check(resource, gc.NotNil)
	}
	first := results.Results[0]
	second := results.Results[1]
	check(first)
	check(second)
	c.Check(first.NotifyWatcherId, gc.Not(gc.Equals), second.NotifyWatcherId)
}

func (*FacadeSuite) TestWatchErrors(c *gc.C) {
	stub := &testing.Stub{}
	stub.SetErrors(errors.New("blort")) // trigger channel closed error
	backend := newMockBackend(stub)
	resources := common.NewResources()
	facade, err := migrationflag.New(backend, resources, authOK)
	c.Assert(err, jc.ErrorIsNil)

	// 3 entities: unparseable, unauthorized, closed channel.
	results := facade.Watch(context.Background(), entities(
		"urgle",
		unknownModel,
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, gc.HasLen, 3)
	stub.CheckCallNames(c, "WatchMigrationPhase")

	c.Check(results.Results, jc.DeepEquals, []params.NotifyWatchResult{{
		Error: &params.Error{
			Message: `"urgle" is not a valid tag`,
		}}, {
		Error: &params.Error{
			Message: "permission denied",
			Code:    "unauthorized access",
		}}, {
		Error: &params.Error{
			Message: "blort",
		}},
	})
	c.Check(resources.Count(), gc.Equals, 0)
}

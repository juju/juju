// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationflag_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/agent/migrationflag"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testhelpers.IsolationSuite
}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &FacadeSuite{})
}

func (*FacadeSuite) TestAcceptsMachineAgent(c *tc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{machine: true})
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (*FacadeSuite) TestAcceptsUnitAgent(c *tc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{machine: true})
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (*FacadeSuite) TestAcceptsApplicationAgent(c *tc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{application: true})
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (*FacadeSuite) TestRejectsNonAgent(c *tc.C) {
	facade, err := migrationflag.New(nil, nil, agentAuth{})
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
	c.Check(facade, tc.IsNil)
}

func (*FacadeSuite) TestPhaseSuccess(c *tc.C) {
	stub := &testhelpers.Stub{}
	backend := newMockBackend(stub)
	facade, err := migrationflag.New(backend, nil, authOK)
	c.Assert(err, tc.ErrorIsNil)

	results := facade.Phase(c.Context(), entities(
		coretesting.ModelTag.String(),
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 2)
	stub.CheckCallNames(c, "MigrationPhase", "MigrationPhase")

	for _, result := range results.Results {
		c.Check(result.Error, tc.IsNil)
		c.Check(result.Phase, tc.Equals, "REAP")
	}
}

func (*FacadeSuite) TestPhaseErrors(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(errors.New("ouch"))
	backend := newMockBackend(stub)
	facade, err := migrationflag.New(backend, nil, authOK)
	c.Assert(err, tc.ErrorIsNil)

	// 3 entities: unparseable, unauthorized, call error.
	results := facade.Phase(c.Context(), entities(
		"urgle",
		unknownModel,
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 3)
	stub.CheckCallNames(c, "MigrationPhase")

	c.Check(results.Results, tc.DeepEquals, []params.PhaseResult{{
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

func (*FacadeSuite) TestWatchSuccess(c *tc.C) {
	stub := &testhelpers.Stub{}
	backend := newMockBackend(stub)
	resources := common.NewResources()
	facade, err := migrationflag.New(backend, resources, authOK)
	c.Assert(err, tc.ErrorIsNil)

	results := facade.Watch(c.Context(), entities(
		coretesting.ModelTag.String(),
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 2)
	stub.CheckCallNames(c, "WatchMigrationPhase", "WatchMigrationPhase")

	check := func(result params.NotifyWatchResult) {
		c.Check(result.Error, tc.IsNil)
		resource := resources.Get(result.NotifyWatcherId)
		c.Check(resource, tc.NotNil)
	}
	first := results.Results[0]
	second := results.Results[1]
	check(first)
	check(second)
	c.Check(first.NotifyWatcherId, tc.Not(tc.Equals), second.NotifyWatcherId)
}

func (*FacadeSuite) TestWatchErrors(c *tc.C) {
	stub := &testhelpers.Stub{}
	stub.SetErrors(errors.New("blort")) // trigger channel closed error
	backend := newMockBackend(stub)
	resources := common.NewResources()
	facade, err := migrationflag.New(backend, resources, authOK)
	c.Assert(err, tc.ErrorIsNil)

	// 3 entities: unparseable, unauthorized, closed channel.
	results := facade.Watch(c.Context(), entities(
		"urgle",
		unknownModel,
		coretesting.ModelTag.String(),
	))
	c.Assert(results.Results, tc.HasLen, 3)
	stub.CheckCallNames(c, "WatchMigrationPhase")

	c.Check(results.Results, tc.DeepEquals, []params.NotifyWatchResult{{
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
	c.Check(resources.Count(), tc.Equals, 0)
}

// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"context"
	"errors"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/testing"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/agent/machineactions"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&FacadeSuite{})

func (*FacadeSuite) TestAcceptsMachineAgent(c *tc.C) {
	facade, err := machineactions.NewFacade(nil, nil, agentAuth{machine: true})
	c.Check(err, tc.ErrorIsNil)
	c.Check(facade, tc.NotNil)
}

func (*FacadeSuite) TestOtherAgent(c *tc.C) {
	facade, err := machineactions.NewFacade(nil, nil, agentAuth{})
	c.Check(err, tc.Equals, apiservererrors.ErrPerm)
	c.Check(facade, tc.IsNil)
}

func (*FacadeSuite) TestRunningActions(c *tc.C) {
	stub := &testing.Stub{}
	auth := agentAuth{
		machine: true,
	}
	backend := &mockBackend{
		stub: stub,
	}

	facade, err := machineactions.NewFacade(backend, nil, auth)
	c.Assert(err, tc.ErrorIsNil)

	stub.SetErrors(errors.New("boom"))
	results := facade.RunningActions(context.Background(), entities(
		"valid", // we will cause this one to err out
		"valid",
		"invalid",
		"unauthorized",
	))

	c.Assert(results, tc.DeepEquals, params.ActionsByReceivers{
		Actions: []params.ActionsByReceiver{{
			Receiver: "valid",
			Error:    apiservererrors.ServerError(errors.New("boom")),
		}, {
			Receiver: "valid",
			Actions:  actions,
		}, {
			Error: apiservererrors.ServerError(apiservererrors.ErrBadId),
		}, {
			Receiver: "unauthorized",
			Error:    apiservererrors.ServerError(apiservererrors.ErrPerm),
		}},
	})
	stub.CheckCallNames(c, "TagToActionReceiverFn", "ConvertActions", "ConvertActions")
}

// entities is a convenience constructor for params.Entities.
func entities(tags ...string) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag
	}
	return entities
}

// agentAuth implements facade.Authorizer for use in the tests.
type agentAuth struct {
	facade.Authorizer
	machine bool
}

// AuthMachineAgent is part of the facade.Authorizer interface.
func (auth agentAuth) AuthMachineAgent() bool {
	return auth.machine
}

func (auth agentAuth) AuthOwner(tag names.Tag) bool {
	if tag.String() == "valid" {
		return true
	}
	return false
}

// mockBackend implements machineactions.Backend for use in the tests.
type mockBackend struct {
	machineactions.Backend
	stub *testing.Stub
}

func (mock *mockBackend) TagToActionReceiverFn(findEntity func(names.Tag) (state.Entity, error)) func(string) (state.ActionReceiver, error) {
	mock.stub.AddCall("TagToActionReceiverFn", findEntity)
	return tagToActionReceiver
}

func tagToActionReceiver(tag string) (state.ActionReceiver, error) {
	switch tag {
	case "valid":
		return validReceiver, nil
	case "unauthorized":
		return unauthorizedReceiver, nil
	default:
		return nil, errors.New("invalid actionReceiver tag")
	}
}

var validReceiver = fakeActionReceiver{tag: validTag}
var unauthorizedReceiver = fakeActionReceiver{tag: unauthorizedTag}
var validTag = fakeTag{s: "valid"}
var unauthorizedTag = fakeTag{s: "unauthorized"}

type fakeActionReceiver struct {
	state.ActionReceiver
	tag fakeTag
}

func (mock fakeActionReceiver) Tag() names.Tag {
	return mock.tag
}

type fakeTag struct {
	names.Tag
	s string
}

func (mock fakeTag) String() string {
	return mock.s
}

func (mock *mockBackend) ConvertActions(ar state.ActionReceiver, fn common.GetActionsFn) ([]params.ActionResult, error) {
	mock.stub.AddCall("ConvertActions", ar, fn)
	if err := mock.stub.NextErr(); err != nil {
		return nil, err
	}
	return actions, nil
}

var actions = []params.ActionResult{{Action: &params.Action{Name: "foo"}}}

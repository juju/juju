// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	commontesting "github.com/juju/juju/apiserver/common/testing"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type runSuite struct {
	jujutesting.JujuConnSuite
	commontesting.BlockHelper

	client *action.ActionAPI
}

var _ = gc.Suite(&runSuite{})

func (s *runSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.BlockHelper = commontesting.NewBlockHelper(s.APIState)
	s.AddCleanup(func(*gc.C) { s.BlockHelper.Close() })

	var err error
	auth := apiservertesting.FakeAuthorizer{
		Tag: s.AdminUserTag(c),
	}
	s.client, err = action.NewActionAPI(s.State, nil, auth, action.FakeLeadership{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *runSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *runSuite) addUnit(c *gc.C, application *state.Application) *state.Unit {
	unit, err := application.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *runSuite) AssertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *runSuite) TestBlockRunOnAllMachines(c *gc.C) {
	// block all changes
	s.BlockAllChanges(c, "TestBlockRunOnAllMachines")
	_, err := s.client.RunOnAllMachines(
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	s.AssertBlocked(c, err, "TestBlockRunOnAllMachines")
}

func (s *runSuite) TestBlockRunMachineAndApplication(c *gc.C) {
	// block all changes
	s.BlockAllChanges(c, "TestBlockRunMachineAndApplication")
	_, err := s.client.Run(
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.AssertBlocked(c, err, "TestBlockRunMachineAndApplication")
}

func (s *runSuite) TestRunMachineAndApplication(c *gc.C) {
	// We only test that we create the actions correctly
	// There is no need to test anything else at this level.
	expectedPayload := map[string]interface{}{
		"command":          "hostname",
		"timeout":          int64(0),
		"workload-context": false,
	}
	parallel := true
	executionGroup := "group"
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: "unit-magic-0", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
			{Receiver: "unit-magic-1", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
			{Receiver: "machine-0", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
		},
	}
	s.addMachine(c)

	charm := s.AddTestingCharm(c, "dummy")
	magic, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "magic", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "20.04/stable"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.client.Run(
		params.RunParams{
			Commands:       "hostname",
			Machines:       []string{"0"},
			Applications:   []string{"magic"},
			Parallel:       &parallel,
			ExecutionGroup: &executionGroup,
		})
	op, err := s.client.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.Actions, gc.HasLen, 3)

	emptyActionTag := names.ActionTag{}
	for i, r := range op.Actions {
		c.Assert(r.Action, gc.NotNil)
		c.Assert(r.Action.Tag, gc.Not(gc.Equals), emptyActionTag)
		c.Assert(r.Action.Name, gc.Equals, "juju-exec")
		c.Assert(r.Action.Receiver, gc.Equals, arg.Actions[i].Receiver)
		c.Assert(r.Action.Parameters, jc.DeepEquals, expectedPayload)
		c.Assert(r.Action.Parallel, jc.DeepEquals, &parallel)
		c.Assert(r.Action.ExecutionGroup, jc.DeepEquals, &executionGroup)
	}
}

func (s *runSuite) TestRunApplicationWorkload(c *gc.C) {
	// We only test that we create the actions correctly
	// There is no need to test anything else at this level.
	expectedPayload := map[string]interface{}{
		"command":          "hostname",
		"timeout":          int64(0),
		"workload-context": true,
	}
	parallel := true
	executionGroup := "group"
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: "unit-magic-0", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
			{Receiver: "unit-magic-1", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
		},
	}
	s.addMachine(c)

	charm := s.AddTestingCharm(c, "dummy")
	magic, err := s.State.AddApplication(state.AddApplicationArgs{
		Name: "magic", Charm: charm,
		CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "20.04/stable"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.client.Run(
		params.RunParams{
			Commands:        "hostname",
			Applications:    []string{"magic"},
			WorkloadContext: true,
			Parallel:        &parallel,
			ExecutionGroup:  &executionGroup,
		})
	op, err := s.client.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.Actions, gc.HasLen, 2)

	emptyActionTag := names.ActionTag{}
	for i, r := range op.Actions {
		c.Assert(r.Action, gc.NotNil)
		c.Assert(r.Action.Tag, gc.Not(gc.Equals), emptyActionTag)
		c.Assert(r.Action.Name, gc.Equals, "juju-exec")
		c.Assert(r.Action.Receiver, gc.Equals, arg.Actions[i].Receiver)
		c.Assert(r.Action.Parameters, jc.DeepEquals, expectedPayload)
		c.Assert(r.Action.Parallel, jc.DeepEquals, &parallel)
		c.Assert(r.Action.ExecutionGroup, jc.DeepEquals, &executionGroup)
	}
}

func (s *runSuite) TestRunOnAllMachines(c *gc.C) {
	// We only test that we create the actions correctly
	// There is no need to test anything else at this level.
	expectedPayload := map[string]interface{}{
		"command":          "hostname",
		"timeout":          testing.LongWait.Nanoseconds(),
		"workload-context": false,
	}
	parallel := true
	executionGroup := "group"
	arg := params.Actions{
		Actions: []params.Action{
			{Receiver: "machine-0", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
			{Receiver: "machine-1", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
			{Receiver: "machine-2", Name: "juju-exec", Parameters: expectedPayload, Parallel: &parallel, ExecutionGroup: &executionGroup},
		},
	}
	// Make three machines.
	s.addMachine(c)
	s.addMachine(c)
	s.addMachine(c)

	s.client.RunOnAllMachines(
		params.RunParams{
			Commands:       "hostname",
			Timeout:        testing.LongWait,
			Parallel:       &parallel,
			ExecutionGroup: &executionGroup,
		})
	op, err := s.client.EnqueueOperation(arg)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(op.Actions, gc.HasLen, 3)

	emptyActionTag := names.ActionTag{}
	for i, r := range op.Actions {
		c.Assert(r.Action, gc.NotNil)
		c.Assert(r.Action.Tag, gc.Not(gc.Equals), emptyActionTag)
		c.Assert(r.Action.Name, gc.Equals, "juju-exec")
		c.Assert(r.Action.Receiver, gc.Equals, arg.Actions[i].Receiver)
		c.Assert(r.Action.Parameters, jc.DeepEquals, expectedPayload)
		c.Assert(r.Action.Parallel, jc.DeepEquals, &parallel)
		c.Assert(r.Action.ExecutionGroup, jc.DeepEquals, &executionGroup)
	}

}

func (s *runSuite) TestRunRequiresAdmin(c *gc.C) {
	alpha := names.NewUserTag("alpha@bravo")
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	client, err := action.NewActionAPI(s.State, nil, auth, action.FakeLeadership{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.Run(params.RunParams{})
	c.Assert(errors.Is(err, apiservererrors.ErrPerm), jc.IsTrue)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(s.State, nil, auth, action.FakeLeadership{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.Run(params.RunParams{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *runSuite) TestRunOnAllMachinesRequiresAdmin(c *gc.C) {
	alpha := names.NewUserTag("alpha@bravo")
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	client, err := action.NewActionAPI(s.State, nil, auth, action.FakeLeadership{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.RunOnAllMachines(params.RunParams{})
	c.Assert(errors.Is(err, apiservererrors.ErrPerm), jc.IsTrue)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(s.State, nil, auth, action.FakeLeadership{})
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.RunOnAllMachines(params.RunParams{})
	c.Assert(err, jc.ErrorIsNil)
}

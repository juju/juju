// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/action"
	"github.com/juju/juju/apiserver/common"
	commontesting "github.com/juju/juju/apiserver/common/testing"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	jujutesting "github.com/juju/juju/juju/testing"
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
	s.client, err = action.NewActionAPI(s.State, nil, auth)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *runSuite) addMachine(c *gc.C) *state.Machine {
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *runSuite) addUnit(c *gc.C, service *state.Application) *state.Unit {
	unit, err := service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine()
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *runSuite) TestGetAllUnitNames(c *gc.C) {
	charm := s.AddTestingCharm(c, "dummy")
	magic, err := s.State.AddApplication(state.AddApplicationArgs{Name: "magic", Charm: charm})
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	notAssigned, err := s.State.AddApplication(state.AddApplicationArgs{Name: "not-assigned", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	_, err = notAssigned.AddUnit()
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "no-units", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)

	wordpress, err := s.State.AddApplication(state.AddApplicationArgs{Name: "wordpress", Charm: s.AddTestingCharm(c, "wordpress")})
	c.Assert(err, jc.ErrorIsNil)
	wordpress0 := s.addUnit(c, wordpress)
	_, err = s.State.AddApplication(state.AddApplicationArgs{Name: "logging", Charm: s.AddTestingCharm(c, "logging")})
	c.Assert(err, jc.ErrorIsNil)

	eps, err := s.State.InferEndpoints("logging", "wordpress")
	c.Assert(err, jc.ErrorIsNil)
	rel, err := s.State.AddRelation(eps...)
	c.Assert(err, jc.ErrorIsNil)
	ru, err := rel.Unit(wordpress0)
	c.Assert(err, jc.ErrorIsNil)
	err = ru.EnterScope(nil)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range []struct {
		message  string
		expected []string
		units    []string
		services []string
		error    string
	}{{
		message: "no units, expected nil slice",
	}, {
		message:  "asking for an empty string service",
		services: []string{""},
		error:    `"" is not a valid application name`,
	}, {
		message: "asking for an empty string unit",
		units:   []string{""},
		error:   `invalid unit name ""`,
	}, {
		message:  "asking for a service that isn't there",
		services: []string{"foo"},
		error:    `application "foo" not found`,
	}, {
		message:  "service with no units is not really an error",
		services: []string{"no-units"},
	}, {
		message:  "A service with units",
		services: []string{"magic"},
		expected: []string{"magic/0", "magic/1"},
	}, {
		message:  "Asking for just a unit",
		units:    []string{"magic/0"},
		expected: []string{"magic/0"},
	}, {
		message:  "Asking for just a subordinate unit",
		units:    []string{"logging/0"},
		expected: []string{"logging/0"},
	}, {
		message:  "Asking for a unit, and the service",
		services: []string{"magic"},
		units:    []string{"magic/0"},
		expected: []string{"magic/0", "magic/1"},
	}} {
		c.Logf("%v: %s", i, test.message)
		result, err := action.GetAllUnitNames(s.State, test.units, test.services)
		if test.error == "" {
			c.Check(err, jc.ErrorIsNil)
			var units []string
			for _, unit := range result {
				units = append(units, unit.Id())
			}
			c.Check(units, jc.SameContents, test.expected)
		} else {
			c.Check(err, gc.ErrorMatches, test.error)
		}
	}
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

func (s *runSuite) TestBlockRunMachineAndService(c *gc.C) {
	// block all changes
	s.BlockAllChanges(c, "TestBlockRunMachineAndService")
	_, err := s.client.Run(
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.AssertBlocked(c, err, "TestBlockRunMachineAndService")
}

func (s *runSuite) TestRunMachineAndService(c *gc.C) {
	// We only test that we create the actions correctly
	// There is no need to test anything else at this level.
	expectedPayload := map[string]interface{}{
		"command": "hostname",
		"timeout": int64(0),
	}
	expectedArgs := params.Actions{
		Actions: []params.Action{
			{Receiver: "unit-magic-0", Name: "juju-run", Parameters: expectedPayload},
			{Receiver: "unit-magic-1", Name: "juju-run", Parameters: expectedPayload},
			{Receiver: "machine-0", Name: "juju-run", Parameters: expectedPayload},
		},
	}
	called := false
	s.PatchValue(action.QueueActions, func(client *action.ActionAPI, args params.Actions) (params.ActionResults, error) {
		called = true
		c.Assert(args, jc.DeepEquals, expectedArgs)
		return params.ActionResults{}, nil
	})

	s.addMachine(c)

	charm := s.AddTestingCharm(c, "dummy")
	magic, err := s.State.AddApplication(state.AddApplicationArgs{Name: "magic", Charm: charm})
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.client.Run(
		params.RunParams{
			Commands:     "hostname",
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	c.Assert(called, jc.IsTrue)
}

func (s *runSuite) TestRunOnAllMachines(c *gc.C) {
	// We only test that we create the actions correctly
	// There is no need to test anything else at this level.
	expectedPayload := map[string]interface{}{
		"command": "hostname",
		"timeout": testing.LongWait.Nanoseconds(),
	}
	expectedArgs := params.Actions{
		Actions: []params.Action{
			{Receiver: "machine-0", Name: "juju-run", Parameters: expectedPayload},
			{Receiver: "machine-1", Name: "juju-run", Parameters: expectedPayload},
			{Receiver: "machine-2", Name: "juju-run", Parameters: expectedPayload},
		},
	}
	called := false
	s.PatchValue(action.QueueActions, func(client *action.ActionAPI, args params.Actions) (params.ActionResults, error) {
		called = true
		c.Assert(args, jc.DeepEquals, expectedArgs)
		return params.ActionResults{}, nil
	})
	// Make three machines.
	s.addMachine(c)
	s.addMachine(c)
	s.addMachine(c)

	s.client.RunOnAllMachines(
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	c.Assert(called, jc.IsTrue)
}

func (s *runSuite) TestRunRequiresAdmin(c *gc.C) {
	alpha := names.NewUserTag("alpha@bravo")
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	client, err := action.NewActionAPI(s.State, nil, auth)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.Run(params.RunParams{})
	c.Assert(errors.Cause(err), gc.Equals, common.ErrPerm)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(s.State, nil, auth)
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
	client, err := action.NewActionAPI(s.State, nil, auth)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.RunOnAllMachines(params.RunParams{})
	c.Assert(errors.Cause(err), gc.Equals, common.ErrPerm)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(s.State, nil, auth)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.RunOnAllMachines(params.RunParams{})
	c.Assert(err, jc.ErrorIsNil)
}

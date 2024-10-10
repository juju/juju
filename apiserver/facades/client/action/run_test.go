// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/pkg/errors"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type runSuite struct {
	jujutesting.ApiServerSuite

	blockCommandService *action.MockBlockCommandService

	client *action.ActionAPI
}

var _ = gc.Suite(&runSuite{})

func (s *runSuite) TestBlockRunOnAllMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// block all changes
	s.blockAllChanges(c, "TestBlockRunOnAllMachines")
	_, err := s.client.RunOnAllMachines(
		context.Background(),
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	s.assertBlocked(c, err, "TestBlockRunOnAllMachines")
}

func (s *runSuite) TestBlockRunMachineAndApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// block all changes
	s.blockAllChanges(c, "TestBlockRunMachineAndApplication")
	_, err := s.client.Run(
		context.Background(),
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.assertBlocked(c, err, "TestBlockRunMachineAndApplication")
}

func (s *runSuite) TestRunMachineAndApplication(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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

	st := s.ControllerModel(c).State()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	charm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
	magic, err := st.AddApplication(
		s.modelConfigService(c),
		state.AddApplicationArgs{
			Name: "magic", Charm: charm,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "20.04/stable"}},
		},
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.client.Run(
		context.Background(),
		params.RunParams{
			Commands:       "hostname",
			Machines:       []string{"0"},
			Applications:   []string{"magic"},
			Parallel:       &parallel,
			ExecutionGroup: &executionGroup,
		})
	op, err := s.client.EnqueueOperation(context.Background(), arg)
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
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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

	st := s.ControllerModel(c).State()

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	charm := f.MakeCharm(c, &factory.CharmParams{Name: "dummy"})
	magic, err := st.AddApplication(
		s.modelConfigService(c),
		state.AddApplicationArgs{
			Name: "magic", Charm: charm,
			CharmOrigin: &state.CharmOrigin{Platform: &state.Platform{OS: "ubuntu", Channel: "20.04/stable"}},
		},
		jujutesting.NewObjectStore(c, s.ControllerModelUUID()),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.addUnit(c, magic)
	s.addUnit(c, magic)

	s.client.Run(
		context.Background(),
		params.RunParams{
			Commands:        "hostname",
			Applications:    []string{"magic"},
			WorkloadContext: true,
			Parallel:        &parallel,
			ExecutionGroup:  &executionGroup,
		})
	op, err := s.client.EnqueueOperation(context.Background(), arg)
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
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

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
		context.Background(),
		params.RunParams{
			Commands:       "hostname",
			Timeout:        testing.LongWait,
			Parallel:       &parallel,
			ExecutionGroup: &executionGroup,
		})
	op, err := s.client.EnqueueOperation(context.Background(), arg)
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
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	alpha := names.NewUserTag("alpha@bravo")
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	st := s.ControllerModel(c).State()
	client, err := action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.blockCommandService)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.Run(context.Background(), params.RunParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.blockCommandService)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.Run(context.Background(), params.RunParams{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *runSuite) TestRunOnAllMachinesRequiresAdmin(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound)

	alpha := names.NewUserTag("alpha@bravo")
	auth := apiservertesting.FakeAuthorizer{
		Tag:         alpha,
		HasWriteTag: alpha,
	}
	st := s.ControllerModel(c).State()
	client, err := action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.blockCommandService)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.RunOnAllMachines(context.Background(), params.RunParams{})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)

	auth.AdminTag = alpha
	client, err = action.NewActionAPI(st, nil, auth, action.FakeLeadership{}, s.blockCommandService)
	c.Assert(err, jc.ErrorIsNil)
	_, err = client.RunOnAllMachines(context.Background(), params.RunParams{})
	c.Assert(err, jc.ErrorIsNil)
}

// modelConfigService is a convenience function to get the controller model's
// model config service inside a test.
func (s *runSuite) modelConfigService(c *gc.C) state.ModelConfigService {
	return s.ControllerDomainServices(c).Config()
}

func (s *runSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.blockCommandService = action.NewMockBlockCommandService(ctrl)

	var err error
	auth := apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}
	s.client, err = action.NewActionAPI(s.ControllerModel(c).State(), nil, auth, action.FakeLeadership{}, s.blockCommandService)
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}

func (s *runSuite) addMachine(c *gc.C) *state.Machine {
	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(s.modelConfigService(c), state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	return machine
}

func (s *runSuite) addUnit(c *gc.C, application *state.Application) *state.Unit {
	unit, err := application.AddUnit(s.modelConfigService(c), state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = unit.AssignToNewMachine(s.modelConfigService(c))
	c.Assert(err, jc.ErrorIsNil)
	return unit
}

func (s *runSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), gc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *runSuite) blockAllChanges(c *gc.C, msg string) {
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)
}

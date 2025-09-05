// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"errors"
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/core/unit"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

// enqueueSuite groups tests for ActionAPI.EnqueueOperation and related helpers.
// It embeds MockBaseSuite to provide common mocks.
type enqueueSuite struct {
	MockBaseSuite
}

// TestEnqueueSuite runs the tests for ActionAPI.EnqueueOperation
func TestEnqueueSuite(t *stdtesting.T) {
	tc.Run(t, &enqueueSuite{})
}

// TestEnqueue_PermissionDenied verifies that enqueuing an operation without proper permission returns ErrPerm.
func (s *enqueueSuite) TestEnqueue_PermissionDenied(c *tc.C) {

	defer s.setupMocks(c).Finish()
	// Arrange : FakeAuthorizer without write permission should yield ErrPerm
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService, s.BlockCommandService, s.ModelInfoService,
		s.OperationService, modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)

	// Act
	_, err = api.EnqueueOperation(context.Background(), params.Actions{Actions: []params.Action{{Receiver: "app/0", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestEnqueue_NoActions verifies that enqueuing an operation with no actions results
// in an appropriate error response.
func (s *enqueueSuite) TestEnqueue_NoActions(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{})

	// Assert
	c.Assert(err, tc.ErrorMatches, "no actions specified")
}

// TestEnqueue_SingleUnit verifies the enqueue operation for a single unit with
// actions, parameters, and execution groups.
func (s *enqueueSuite) TestEnqueue_SingleUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	api := s.NewActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName:     "do",
		Parameters:     map[string]interface{}{"k": "v"},
		IsParallel:     true,
		ExecutionGroup: "grp",
	}
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), []operation.ActionArgs{{
		ActionTarget: operation.ActionTarget{
			Unit: "app/0",
		},
		TaskArgs: taskArgs,
	},
	}).Return(operation.RunResult{
		OperationID: "1",
		Units: []operation.UnitTaskResult{{
			ReceiverName: "app/0",
			TaskInfo: operation.TaskInfo{
				ID:       "1",
				TaskArgs: taskArgs,
			}}}}, nil)

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{
		Receiver:       "unit-app-0",
		Name:           "do",
		Parameters:     map[string]interface{}{"k": "v"},
		Parallel:       ptr(true),
		ExecutionGroup: ptr("grp")}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-1")
	c.Assert(res.Actions, tc.HasLen, 1)
	c.Check(res.Actions[0].Error, tc.IsNil)
	c.Check(res.Actions[0].Action, tc.DeepEquals, &params.Action{
		Tag:            "action-1",
		Receiver:       "unit-app-0",
		Name:           "do",
		Parameters:     map[string]interface{}{"k": "v"},
		Parallel:       ptr(true),
		ExecutionGroup: ptr("grp"),
	})
}

// TestEnqueue_LeaderReceiver verifies the enqueue operation behavior when
// the receiver is the leader unit of an application.
func (s *enqueueSuite) TestEnqueue_LeaderReceiver(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName: "do",
	}
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), []operation.ActionArgs{{
		ActionTarget: operation.ActionTarget{
			LeaderUnit: "myapp",
		},
		TaskArgs: taskArgs,
	},
	}).Return(operation.RunResult{
		OperationID: "2",
		Units: []operation.UnitTaskResult{{
			ReceiverName: "myapp/0",
			IsLeader:     true,
			TaskInfo: operation.TaskInfo{
				ID:       "1",
				TaskArgs: taskArgs,
			}}}}, nil)

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "myapp/leader",
		Name: "do", Parallel: ptr(false)}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-2")
	c.Assert(len(res.Actions), tc.Equals, 1)
	c.Assert(res.Actions[0].Error, tc.IsNil)
	c.Assert(res.Actions[0].Action, tc.NotNil)
	c.Check(res.Actions[0].Action.Receiver, tc.Equals, "unit-myapp-0")
}

// TestEnqueue_Defaults verifies the enqueue operation applies default values
// for isParallel and ExecutionGroup parameters.
func (s *enqueueSuite) TestEnqueue_Defaults(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	api := s.NewActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName:     "do-default",
		IsParallel:     false,
		ExecutionGroup: "", // defaulted to ""
	}
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), []operation.ActionArgs{{
		ActionTarget: operation.ActionTarget{
			Unit: "app/0",
		},
		TaskArgs: taskArgs,
	}}).Return(operation.RunResult{OperationID: "404" /*placeholder, we check the input args */}, nil)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{
		Receiver: "unit-app-0",
		Name:     "do-default",
		// default values for isParallel and Execution group
	}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnqueue_MultipleActions validates the enqueue operation for multiple
// actions with correct execution order and parameters.
func (s *enqueueSuite) TestEnqueue_MultipleActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.ActionArgs) (operation.RunResult, error) {
			c.Assert(len(got), tc.Equals, 3)
			// order: leader, app/2, app/0
			c.Assert(got[0].LeaderUnit, tc.DeepEquals, "app")
			c.Assert(got[1].Unit, tc.DeepEquals, unit.Name("app/2"))
			c.Assert(got[2].Unit, tc.DeepEquals, unit.Name("app/0"))
			ti1 := operation.TaskInfo{ID: "1", TaskArgs: got[0].TaskArgs}
			ti2 := operation.TaskInfo{ID: "2", TaskArgs: got[1].TaskArgs}
			ti3 := operation.TaskInfo{ID: "3", TaskArgs: got[2].TaskArgs}
			return operation.RunResult{OperationID: "3",
				Units: []operation.UnitTaskResult{
					{ReceiverName: "app/0", TaskInfo: ti3},
					{ReceiverName: "app/2", TaskInfo: ti2},
					{ReceiverName: "app/9", IsLeader: true, TaskInfo: ti1},
				},
			}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "app/leader",
		Name: "x"}, {Receiver: "unit-app-2", Name: "y"}, {Receiver: "unit-app-0", Name: "z"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Actions), tc.Equals, 3)
	c.Check(res.Actions[0].Action.Name, tc.Equals, "x")
	c.Check(res.Actions[0].Action.Receiver, tc.Equals, "unit-app-9")
	c.Check(res.Actions[1].Action.Name, tc.Equals, "y")
	c.Check(res.Actions[1].Action.Receiver, tc.Equals, "unit-app-2")
	c.Check(res.Actions[2].Action.Name, tc.Equals, "z")
	c.Check(res.Actions[2].Action.Receiver, tc.Equals, "unit-app-0")
}

// TestEnqueue_SomeInvalid validates the behavior of the EnqueueOperation method
// when some provided actions are invalid (receiver with a bad tag)
func (s *enqueueSuite) TestEnqueue_SomeInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.ActionArgs) (operation.RunResult, error) {
			c.Assert(len(got), tc.Equals, 1)
			ti := operation.TaskInfo{ID: "1", TaskArgs: got[0].TaskArgs}
			return operation.RunResult{OperationID: "4", Units: []operation.UnitTaskResult{{ReceiverName: unit.Name(
				"app/3"), TaskInfo: ti}}}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "badformat/0",
		Name: "do"}, {Receiver: "unit-app-3", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-4")
	c.Assert(res.Actions[0].Error, tc.NotNil)
	c.Assert(res.Actions[1].Error, tc.IsNil)
}

// TestEnqueue_AllInvalid_NoServiceCall verifies that no service call is made
// when all actions have invalid receivers.
func (s *enqueueSuite) TestEnqueue_AllInvalid_NoServiceCall(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	// Ensure Run is not called
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), gomock.Any()).Times(0)

	// Act: all tag receiver are invalid
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "bad1", Name: "do"}, {Receiver: "also/bad", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "")
	c.Assert(res.Actions[0].Error, tc.NotNil)
	c.Assert(res.Actions[1].Error, tc.NotNil)
}

// TestEnqueue_ServiceError checks that EnqueueOperation returns an error when the OperationService.Run fails.
func (s *enqueueSuite) TestEnqueue_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), gomock.Any()).Return(operation.RunResult{}, fmt.Errorf("boom"))
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}}})
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestEnqueue_UnexpectedExtraResult verifies the behavior when an unexpected
// extra result is returned during operation execution.
func (s *enqueueSuite) TestEnqueue_UnexpectedExtraResult(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.ActionArgs) (operation.RunResult, error) {
			return operation.RunResult{
				OperationID: "5",
				Units: []operation.UnitTaskResult{
					{
						ReceiverName: "otherapp/9", // this result is not expected
						TaskInfo:     operation.TaskInfo{ID: "0"},
					}}}, nil
		})
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}}})
	c.Assert(err, tc.ErrorMatches, "unexpected result for \"otherapp/9\"")
}

// TestEnqueue_MissingResultPerActionError verifies that EnqueueOperation
// returns an error when results are missing for actions.
func (s *enqueueSuite) TestEnqueue_MissingResultPerActionError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.NewActionAPI(c)
	// Arrange
	s.OperationService.EXPECT().StartActionOperation(gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, got []operation.ActionArgs) (operation.RunResult, error) {
			// only return app/0 result; missing app/1
			ti := operation.TaskInfo{ID: "0", TaskArgs: got[0].TaskArgs}
			return operation.RunResult{OperationID: "8", Units: []operation.UnitTaskResult{{ReceiverName: unit.Name(
				"app/0"), TaskInfo: ti}}}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}, {Receiver: "unit-app-1", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "operation-8")
	c.Assert(res.Actions[0].Error, tc.IsNil)
	c.Assert(res.Actions[1].Error, tc.NotNil)
}

// runSuite groups tests for ActionAPI.Run and related helpers.
// It embeds MockBaseSuite to provide common mocks.
type runSuite struct {
	MockBaseSuite
}

// TestRunSuite runs the runSuite tests.
func TestRunSuite(t *stdtesting.T) {
	tc.Run(t, &runSuite{})
}

func (s *runSuite) NewActionAPI(c *tc.C) *ActionAPI {
	// Don't block
	s.BlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("",
		blockcommanderrors.NotFound).AnyTimes()
	return s.MockBaseSuite.NewActionAPI(c)
}

// TestRun_BlockOnAllMachines ensures RunOnAllMachines is blocked when a
// change block is active.
func (s *runSuite) TestRun_BlockOnAllMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: Use parent mock action API to handle specifically the block changes
	client := s.MockBaseSuite.NewActionAPI(c)

	// block all changes
	s.blockAllChanges(c, "TestRun_BlockOnAllMachines")
	_, err := client.RunOnAllMachines(
		c.Context(),
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	s.assertBlocked(c, err, "TestRun_BlockOnAllMachines")
}

// TestBlockRunMachineAndApplication ensures Run is blocked with mixed
// targets when a change block is active.
func (s *runSuite) TestRun_BlockMachineAndApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: Use parent mock action API to handle specifically the block changes
	client := s.MockBaseSuite.NewActionAPI(c)

	// block all changes
	s.blockAllChanges(c, "TestRun_BlockMachineAndApplication")
	_, err := client.Run(
		c.Context(),
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.assertBlocked(c, err, "TestRun_BlockMachineAndApplication")
}

// TestRun_PermissionDenied verifies Run enforces admin permission and
// does not call OperationService when denied.
func (s *runSuite) TestRun_PermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService,
		s.BlockCommandService, s.ModelInfoService, s.OperationService,
		modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	// Ensure the service is not called
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err = api.Run(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestRun_RejectNestedExec ensures commands containing juju-exec/juju-run
// are rejected by Run.
func (s *runSuite) TestRun_RejectNestedExec(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err1 := api.Run(c.Context(), params.RunParams{Commands: "foo; juju-exec bar", Timeout: time.Second})
	_, err2 := api.Run(c.Context(), params.RunParams{Commands: "x && juju-run y", Timeout: time.Second})

	// Assert
	c.Assert(err1, tc.ErrorMatches, "cannot use \".*\" as an action command")
	c.Assert(err2, tc.ErrorMatches, "cannot use \".*\" as an action command")
}

// TestRun_SuccessMapping verifies input mapping to OperationService.Run and
// output mapping to params.EnqueuedActions.
func (s *runSuite) TestRun_SuccessMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	runParams := params.RunParams{
		Applications:   []string{"a1", "a2"},
		Machines:       []string{"0", "42"},
		Units:          []string{"app/1", "db/0"},
		Commands:       "echo hello",
		Timeout:        5 * time.Second,
		Parallel:       ptr(false),
		ExecutionGroup: ptr("eg-1"),
	}
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, target operation.Target, args operation.ExecArgs) (operation.RunResult, error) {
			c.Check(target.Applications, tc.DeepEquals, []string{"a1", "a2"})
			c.Check(target.Machines, tc.DeepEquals, []machine.Name{"0", "42"})
			c.Check(target.Units, tc.DeepEquals, []unit.Name{"app/1", "db/0"})
			c.Check(args, tc.Equals, operation.ExecArgs{
				Command:        runParams.Commands,
				Timeout:        runParams.Timeout,
				Parallel:       *runParams.Parallel,
				ExecutionGroup: *runParams.ExecutionGroup,
			})
			return operation.RunResult{OperationID: "1"}, nil
		})

	// Act
	res, err := api.Run(c.Context(), runParams)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.OperationTag, tc.Equals, "operation-1")
	c.Check(len(res.Actions), tc.Equals, 0)
}

// TestRun_Defaults verifies defaulting for Parallel and ExecutionGroup.
func (s *runSuite) TestRun_Defaults(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	runParams := params.RunParams{Commands: "whoami", Timeout: time.Second}

	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), operation.ExecArgs{
		Command:        runParams.Commands,
		Timeout:        runParams.Timeout,
		Parallel:       false,
		ExecutionGroup: "",
	}).Return(operation.RunResult{OperationID: "2"}, nil)
	// Act
	res, err := api.Run(c.Context(), runParams)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.OperationTag, tc.Equals, "operation-2")
}

// TestRun_ResultMapping ensures toEnqueuedActions maps unit and machine
// results correctly.
func (s *runSuite) TestRun_ResultMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		operation.RunResult{
			OperationID: "9",
			Machines: []operation.MachineTaskResult{{
				ReceiverName: "2",
				TaskInfo: operation.TaskInfo{ID: "1",
					TaskArgs: operation.TaskArgs{ActionName: coreoperation.JujuExecActionName}},
			}},
			Units: []operation.UnitTaskResult{{
				ReceiverName: "app/0",
				TaskInfo: operation.TaskInfo{ID: "1",
					TaskArgs: operation.TaskArgs{ActionName: coreoperation.JujuExecActionName}},
			}},
		}, nil)

	// Act
	res, err := api.Run(c.Context(), params.RunParams{Commands: "true", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.OperationTag, tc.Equals, "operation-9")
	c.Assert(res.Actions, tc.HasLen, 2)
	// machine action
	c.Check(res.Actions[0].Action.Receiver, tc.Equals, "machine-2")
	c.Check(res.Actions[0].Action.Tag, tc.Equals, names.NewActionTag("1").String())
	// unit action
	c.Check(res.Actions[1].Action.Receiver, tc.Equals, "unit-app-0")
	c.Check(res.Actions[1].Action.Tag, tc.Equals, names.NewActionTag("1").String())
}

// TestRun_ServiceError verifies service error is propagated by Run.
func (s *runSuite) TestRun_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		operation.RunResult{}, fmt.Errorf("boom"))

	// Act
	_, err := api.Run(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestRun_EmptyTarget passes through empty target slices when filters are
// empty.
func (s *runSuite) TestRun_EmptyTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, target operation.Target, _ operation.ExecArgs) (operation.RunResult, error) {
			c.Check(target.Applications, tc.HasLen, 0)
			c.Check(target.Machines, tc.HasLen, 0)
			c.Check(target.Units, tc.HasLen, 0)
			return operation.RunResult{}, errors.New("no target")
		})

	// Act
	_, err := api.Run(c.Context(), params.RunParams{Commands: "true", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "no target")
}

// TestRun_BlockServiceError ensures a non-NotFound block service error
// is propagated and service Run is not called.
func (s *runSuite) TestRun_BlockServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent mock action API to handle specifically the block changes
	api := s.MockBaseSuite.NewActionAPI(c)
	s.BlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", fmt.Errorf("block-fail"))
	s.OperationService.EXPECT().StartExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.Run(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "block-fail")
}

func (s *runSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	var obtained *params.Error
	c.Assert(errors.As(err, &obtained), tc.IsTrue)
	c.Assert(obtained, tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *runSuite) blockAllChanges(c *tc.C, msg string) {
	s.BlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)
}

// runAllSuite groups tests for ActionAPI.RunOnAllMachines.
// It embeds MockBaseSuite to provide common mocks.
type runAllSuite struct{ MockBaseSuite }

// TestRunAllSuite runs the runAllSuite tests.
func TestRunAllSuite(t *stdtesting.T) { tc.Run(t, &runAllSuite{}) }

func (s *runAllSuite) NewActionAPI(c *tc.C) *ActionAPI {
	// Don't block
	s.BlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("",
		blockcommanderrors.NotFound).AnyTimes()
	// Default to IAAS model type
	s.ModelInfoService.EXPECT().
		GetModelInfo(gomock.Any()).
		Return(coremodel.ModelInfo{Type: coremodel.IAAS}, nil).
		AnyTimes()
	return s.MockBaseSuite.NewActionAPI(c)
}

// TestRunOnAllMachines_PermissionDenied verifies admin permission is
// enforced and the service is not called.
func (s *runAllSuite) TestRunOnAllMachines_PermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api, err := NewActionAPI(auth, s.Leadership, s.ApplicationService,
		s.BlockCommandService, s.ModelInfoService, s.OperationService,
		modeltesting.GenModelUUID(c))
	c.Assert(err, tc.ErrorIsNil)
	// Ensure the operation service is not invoked
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err = api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestRunOnAllMachines_ChangeBlockedError propagates ChangeAllowed
// error from the block service and does not call RunOnAllMachines.
func (s *runSuite) TestRunOnAllMachines_ChangeBlockedError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent NewActionAPI to keep standard authorizer setup
	api := s.MockBaseSuite.NewActionAPI(c)
	// Force a generic error from block service path used by ChangeAllowed
	s.BlockCommandService.EXPECT().
		GetBlockSwitchedOn(gomock.Any(), gomock.Any()).
		Return("", errors.New("block-error"))
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "cmd", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "block-error")
}

// TestRunOnAllMachines_NonIAASModel returns an error when the model
// type is not IAAS.
func (s *runSuite) TestRunOnAllMachines_NonIAASModel(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent NewActionAPI to keep standard authorizer and modelInfo setup
	api := s.MockBaseSuite.NewActionAPI(c)
	// Don't block
	s.BlockCommandService.EXPECT().
		GetBlockSwitchedOn(gomock.Any(), gomock.Any()).
		Return("", blockcommanderrors.NotFound).
		AnyTimes()
	s.ModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{Type: coremodel.CAAS}, nil)
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "cmd", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "cannot run on all machines with a caas model")
}

// TestRunOnAllMachines_ModelInfoError propagates an error when
// fetching model info fails.
func (s *runAllSuite) TestRunOnAllMachines_ModelInfoError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent NewActionAPI to keep standard authorizer and modelInfo setup
	api := s.MockBaseSuite.NewActionAPI(c)
	// Don't block
	s.BlockCommandService.EXPECT().
		GetBlockSwitchedOn(gomock.Any(), gomock.Any()).
		Return("", blockcommanderrors.NotFound).
		AnyTimes()
	s.ModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, errors.New("mi boom"))
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "cmd", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "mi boom")
}

// TestRunOnAllMachines_RejectNestedExec rejects nested juju-exec or
// juju-run commands.
func (s *runAllSuite) TestRunOnAllMachines_RejectNestedExec(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err1 := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "juju-exec foo", Timeout: time.Second})
	_, err2 := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "bar; juju-run baz", Timeout: time.Second})

	// Assert
	c.Assert(err1, tc.ErrorMatches, "cannot use \".*\" as an action command")
	c.Assert(err2, tc.ErrorMatches, "cannot use \".*\" as an action command")
}

// TestRunOnAllMachines_ServiceError propagates a service error and
// ensures args mapping is correct.
func (s *runAllSuite) TestRunOnAllMachines_ServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), gomock.Any()).
		Return(operation.RunResult{}, errors.New("service fail"))

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "whoami", Timeout: 2 * time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "service fail")
}

// TestRunOnAllMachines_Success verifies success mapping to
// params.EnqueuedActions.
func (s *runAllSuite) TestRunOnAllMachines_Success(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	// Don't block
	api := s.NewActionAPI(c)
	params := params.RunParams{
		Commands:       "whoami",
		Timeout:        time.Second,
		Parallel:       ptr(true),
		ExecutionGroup: ptr("test"),
	}
	s.OperationService.EXPECT().StartExecOperationOnAllMachines(gomock.Any(), operation.ExecArgs{
		Command:        params.Commands,
		Timeout:        params.Timeout,
		Parallel:       *params.Parallel,
		ExecutionGroup: *params.ExecutionGroup,
	}).Return(
		operation.RunResult{OperationID: "77"}, nil)

	// Act
	res, err := api.RunOnAllMachines(c.Context(), params)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.OperationTag, tc.Equals, "operation-77")
}

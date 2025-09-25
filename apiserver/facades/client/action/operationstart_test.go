// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"errors"
	"fmt"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
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

// TestEnqueuePermissionDenied verifies that enqueuing an operation without proper permission returns ErrPerm.
func (s *enqueueSuite) TestEnqueuePermissionDenied(c *tc.C) {

	defer s.setupMocks(c).Finish()
	// Arrange: FakeAuthorizer without write permission should yield ErrPerm
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api := s.newActionAPIWithAuthorizer(c, auth)

	// Act
	_, err := api.EnqueueOperation(context.Background(), params.Actions{Actions: []params.Action{{Receiver: "app/0", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestEnqueueNoActions verifies that enqueuing an operation with no actions results
// in an appropriate error response.
func (s *enqueueSuite) TestEnqueueNoActions(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.newActionAPI(c)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{})

	// Assert
	c.Assert(err, tc.ErrorMatches, "no actions specified")
}

// TestEnqueueSingleUnit verifies the enqueue operation for a single unit with
// actions, parameters, and execution groups.
func (s *enqueueSuite) TestEnqueueSingleUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	api := s.newActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName:     "do",
		Parameters:     map[string]interface{}{"k": "v"},
		IsParallel:     true,
		ExecutionGroup: "grp",
	}
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), []operation.ActionReceiver{{Unit: "app/0"}},
		taskArgs).
		Return(operation.RunResult{
			OperationID: "1",
			Units: []operation.UnitTaskResult{{
				ReceiverName: "app/0",
				TaskInfo: operation.TaskInfo{
					ID:             "2",
					ActionName:     taskArgs.ActionName,
					Parameters:     taskArgs.Parameters,
					IsParallel:     taskArgs.IsParallel,
					ExecutionGroup: &taskArgs.ExecutionGroup,
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
		Tag:            "action-2",
		Receiver:       "unit-app-0",
		Name:           "do",
		Parameters:     map[string]interface{}{"k": "v"},
		Parallel:       ptr(true),
		ExecutionGroup: ptr("grp"),
	})
}

// TestEnqueueLeaderReceiver verifies the enqueue operation behavior when
// the receiver is the leader unit of an application.
func (s *enqueueSuite) TestEnqueueLeaderReceiver(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.newActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName: "do",
	}
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), []operation.ActionReceiver{{LeaderUnit: "myapp"}},
		taskArgs).Return(operation.RunResult{
		OperationID: "2",
		Units: []operation.UnitTaskResult{{
			ReceiverName: "myapp/0",
			IsLeader:     true,
			TaskInfo: operation.TaskInfo{
				ID: "3",
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

// TestEnqueueDefaults verifies the enqueue operation applies default values
// for isParallel and ExecutionGroup parameters.
func (s *enqueueSuite) TestEnqueueDefaults(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange:
	api := s.newActionAPI(c)
	taskArgs := operation.TaskArgs{
		ActionName:     "do-default",
		IsParallel:     false,
		ExecutionGroup: "", // defaulted to ""
	}
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), []operation.ActionReceiver{{Unit: "app/0"}},
		taskArgs).Return(operation.RunResult{OperationID: "404" /*placeholder, we check the input args */}, nil)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{
		Receiver: "unit-app-0",
		Name:     "do-default",
		// default values for isParallel and Execution group
	}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestEnqueueMultipleActions validates the enqueue operation for multiple
// actions with the correct execution order and parameters.
func (s *enqueueSuite) TestEnqueueMultipleActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	api := s.newActionAPI(c)
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, gotReceivers []operation.ActionReceiver,
			gotParams operation.TaskArgs) (operation.RunResult,
			error) {
			c.Assert(len(gotReceivers), tc.Equals, 3)
			// order: leader, app/2, app/0
			c.Assert(gotReceivers[0].LeaderUnit, tc.DeepEquals, "app")
			c.Assert(gotReceivers[1].Unit, tc.DeepEquals, unit.Name("app/2"))
			c.Assert(gotReceivers[2].Unit, tc.DeepEquals, unit.Name("app/0"))
			ti1 := operation.TaskInfo{ID: "1", ActionName: gotParams.ActionName}
			ti2 := operation.TaskInfo{ID: "2", ActionName: gotParams.ActionName}
			ti3 := operation.TaskInfo{ID: "3", ActionName: gotParams.ActionName}
			return operation.RunResult{OperationID: "0",
				Units: []operation.UnitTaskResult{
					{ReceiverName: "app/0", TaskInfo: ti3},
					{ReceiverName: "app/2", TaskInfo: ti2},
					{ReceiverName: "app/9", IsLeader: true, TaskInfo: ti1},
				},
			}, nil
		})

	// Act
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "app/leader",
		Name: "x"}, {Receiver: "unit-app-2", Name: "x"}, {Receiver: "unit-app-0", Name: "x"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(res.Actions), tc.Equals, 3)
	c.Check(res.Actions[0].Action.Name, tc.Equals, "x")
	c.Check(res.Actions[0].Action.Receiver, tc.Equals, "unit-app-9")
	c.Check(res.Actions[1].Action.Name, tc.Equals, "x")
	c.Check(res.Actions[1].Action.Receiver, tc.Equals, "unit-app-2")
	c.Check(res.Actions[2].Action.Name, tc.Equals, "x")
	c.Check(res.Actions[2].Action.Receiver, tc.Equals, "unit-app-0")
}

// TestEnqueueMultipleActionsErrors verifies that enqueueing multiple actions
// results in appropriate errors when conditions fail.
func (s *enqueueSuite) TestEnqueueMultipleActionsErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	api := s.newActionAPI(c)
	// Ensure AddActionOperation is not called
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{
		{
			Receiver: "app/leader",
			Name:     "x",
		}, {
			Receiver:       "unit-app-2",
			Name:           "y",
			Parameters:     map[string]interface{}{"a": 1},
			Parallel:       ptr(true),
			ExecutionGroup: ptr("eg-1"),
		}}})

	// Assert
	// this trick is necessary to get ride of the \n in the error message which
	// mess up the comparison
	err = fmt.Errorf("%v", strings.Replace(err.Error(), "\n", " ", -1))
	c.Check(err, tc.ErrorMatches, ".*action name mismatch.*")
	c.Check(err, tc.ErrorMatches, ".*parallel mismatch.*")
	c.Check(err, tc.ErrorMatches, ".*execution group mismatch.*")
	c.Check(err, tc.ErrorMatches, ".*parameters mismatch.*")
}

// TestEnqueueSomeInvalid validates the behavior of the EnqueueOperation method
// when some provided actions are invalid (receiver with a bad tag)
func (s *enqueueSuite) TestEnqueueSomeInvalid(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.newActionAPI(c)
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, gotReceivers []operation.ActionReceiver, args operation.TaskArgs) (operation.RunResult,
			error) {
			c.Assert(len(gotReceivers), tc.Equals, 1)
			ti := operation.TaskInfo{ID: "5", ActionName: args.ActionName}
			return operation.RunResult{OperationID: "4", Units: []operation.UnitTaskResult{{ReceiverName: "app/3", TaskInfo: ti}}}, nil
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

// TestEnqueueAllInvalid_NoServiceCall verifies that no service call is made
// when all actions have invalid receivers.
func (s *enqueueSuite) TestEnqueueAllInvalid_NoServiceCall(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.newActionAPI(c)
	// Ensure Run is not called
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act: all tag receiver are invalid
	res, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "bad1", Name: "do"}, {Receiver: "also/bad", Name: "do"}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res.OperationTag, tc.Equals, "")
	c.Assert(res.Actions[0].Error, tc.NotNil)
	c.Assert(res.Actions[1].Error, tc.NotNil)
}

// TestEnqueueServiceError checks that EnqueueOperation returns an error when the OperationService.Run fails.
func (s *enqueueSuite) TestEnqueueServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.newActionAPI(c)
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(operation.RunResult{}, fmt.Errorf("boom"))
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}}})
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestEnqueueUnexpectedExtraResult verifies the behavior when an unexpected
// extra result is returned during operation execution.
func (s *enqueueSuite) TestEnqueueUnexpectedExtraResult(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.newActionAPI(c)
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ []operation.ActionReceiver,
			_ operation.TaskArgs) (operation.RunResult, error) {
			return operation.RunResult{
				OperationID: "5",
				Units: []operation.UnitTaskResult{
					{
						ReceiverName: "otherapp/9", // this result is not expected
						TaskInfo:     operation.TaskInfo{ID: "6"},
					}}}, nil
		})
	_, err := api.EnqueueOperation(c.Context(), params.Actions{Actions: []params.Action{{Receiver: "unit-app-0",
		Name: "do"}}})
	c.Assert(err, tc.ErrorMatches, "unexpected result for \"otherapp/9\"")
}

// TestEnqueueMissingResultPerActionError verifies that EnqueueOperation
// returns an error when results are missing for actions.
func (s *enqueueSuite) TestEnqueueMissingResultPerActionError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	api := s.newActionAPI(c)
	// Arrange
	s.OperationService.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(ctx context.Context, _ []operation.ActionReceiver, args operation.TaskArgs) (operation.RunResult, error) {
			// only return app/0 result; missing app/1
			ti := operation.TaskInfo{ID: "9"}
			return operation.RunResult{OperationID: "8", Units: []operation.UnitTaskResult{{ReceiverName: "app/0", TaskInfo: ti}}}, nil
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
	return s.MockBaseSuite.newActionAPI(c)
}

// TestRunBlockOnAllMachines ensures RunOnAllMachines is blocked when a
// change block is active.
func (s *runSuite) TestRunBlockOnAllMachines(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: Use parent mock action API to handle specifically the block changes
	client := s.MockBaseSuite.newActionAPI(c)

	// block all changes
	s.blockAllChanges(c, "TestRunBlockOnAllMachines")
	_, err := client.RunOnAllMachines(
		c.Context(),
		params.RunParams{
			Commands: "hostname",
			Timeout:  testing.LongWait,
		})
	s.assertBlocked(c, err, "TestRunBlockOnAllMachines")
}

// TestBlockRunMachineAndApplication ensures Run is blocked with mixed
// targets when a change block is active.
func (s *runSuite) TestRunBlockMachineAndApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: Use parent mock action API to handle specifically the block changes
	client := s.MockBaseSuite.newActionAPI(c)

	// block all changes
	s.blockAllChanges(c, "TestRunBlockMachineAndApplication")
	_, err := client.Run(
		c.Context(),
		params.RunParams{
			Commands:     "hostname",
			Timeout:      testing.LongWait,
			Machines:     []string{"0"},
			Applications: []string{"magic"},
		})
	s.assertBlocked(c, err, "TestRunBlockMachineAndApplication")
}

// TestRunPermissionDenied verifies Run enforces admin permission and
// does not call OperationService when denied.
func (s *runSuite) TestRunPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api := s.newActionAPIWithAuthorizer(c, auth)
	// Ensure the service is not called
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.Run(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestRunRejectNestedExec ensures commands containing juju-exec/juju-run
// are rejected by Run.
func (s *runSuite) TestRunRejectNestedExec(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err1 := api.Run(c.Context(), params.RunParams{Commands: "foo; juju-exec bar", Timeout: time.Second})
	_, err2 := api.Run(c.Context(), params.RunParams{Commands: "x && juju-run y", Timeout: time.Second})

	// Assert
	c.Assert(err1, tc.ErrorMatches, "cannot use \".*\" as an action command")
	c.Assert(err2, tc.ErrorMatches, "cannot use \".*\" as an action command")
}

// TestRunSuccessMapping verifies input mapping to OperationService.Run and
// output mapping to params.EnqueuedActions.
func (s *runSuite) TestRunSuccessMapping(c *tc.C) {
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
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, target operation.Receivers, args operation.ExecArgs) (operation.RunResult, error) {
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

// TestRunDefaults verifies defaulting for Parallel and ExecutionGroup.
func (s *runSuite) TestRunDefaults(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	runParams := params.RunParams{Commands: "whoami", Timeout: time.Second}

	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), operation.ExecArgs{
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

// TestRunResultMapping ensures toEnqueuedActions maps unit and machine
// results correctly.
func (s *runSuite) TestRunResultMapping(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		operation.RunResult{
			OperationID: "9",
			Machines: []operation.MachineTaskResult{{
				ReceiverName: "2",
				TaskInfo: operation.TaskInfo{
					ID:         "10",
					ActionName: coreoperation.JujuExecActionName,
				},
			}},
			Units: []operation.UnitTaskResult{{
				ReceiverName: "app/0",
				TaskInfo: operation.TaskInfo{
					ID:         "11",
					ActionName: coreoperation.JujuExecActionName,
				},
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
	c.Check(res.Actions[0].Action.Tag, tc.Equals, names.NewActionTag("10").String())
	// unit action
	c.Check(res.Actions[1].Action.Receiver, tc.Equals, "unit-app-0")
	c.Check(res.Actions[1].Action.Tag, tc.Equals, names.NewActionTag("11").String())
}

// TestRunServiceError verifies service error is propagated by Run.
func (s *runSuite) TestRunServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Return(
		operation.RunResult{}, fmt.Errorf("boom"))

	// Act
	_, err := api.Run(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

// TestRunEmptyTarget passes through empty target slices when filters are
// empty.
func (s *runSuite) TestRunEmptyTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, target operation.Receivers, _ operation.ExecArgs) (operation.RunResult, error) {
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

// TestRunBlockServiceError ensures a non-NotFound block service error
// is propagated and service Run is not called.
func (s *runSuite) TestRunBlockServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent mock action API to handle specifically the block changes
	api := s.MockBaseSuite.newActionAPI(c)
	s.BlockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", fmt.Errorf("block-fail"))
	s.OperationService.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

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
	return s.MockBaseSuite.newActionAPI(c)
}

// TestRunOnAllMachinesPermissionDenied verifies admin permission is
// enforced and the service is not called.
func (s *runAllSuite) TestRunOnAllMachinesPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	auth := apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("readonly")}
	api := s.newActionAPIWithAuthorizer(c, auth)

	// Ensure the operation service is not invoked
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// TestRunOnAllMachinesChangeBlockedError propagates ChangeAllowed
// error from the block service and does not call RunOnAllMachines.
func (s *runSuite) TestRunOnAllMachinesChangeBlockedError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent newActionAPI to keep standard authorizer setup
	api := s.MockBaseSuite.newActionAPI(c)
	// Force a generic error from block service path used by ChangeAllowed
	s.BlockCommandService.EXPECT().
		GetBlockSwitchedOn(gomock.Any(), gomock.Any()).
		Return("", errors.New("block-error"))
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "cmd", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "block-error")
}

// TestRunOnAllMachinesNonIAASModel returns an error when the model
// type is not IAAS.
func (s *runSuite) TestRunOnAllMachinesNonIAASModel(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent newActionAPI to keep standard authorizer and modelInfo setup
	api := s.MockBaseSuite.newActionAPI(c)
	// Don't block
	s.BlockCommandService.EXPECT().
		GetBlockSwitchedOn(gomock.Any(), gomock.Any()).
		Return("", blockcommanderrors.NotFound).
		AnyTimes()
	s.ModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{Type: coremodel.CAAS}, nil)
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "cmd", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "cannot run on all machines with a caas model")
}

// TestRunOnAllMachinesModelInfoError propagates an error when
// fetching model info fails.
func (s *runAllSuite) TestRunOnAllMachinesModelInfoError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange: use parent newActionAPI to keep standard authorizer and modelInfo setup
	api := s.MockBaseSuite.newActionAPI(c)
	// Don't block
	s.BlockCommandService.EXPECT().
		GetBlockSwitchedOn(gomock.Any(), gomock.Any()).
		Return("", blockcommanderrors.NotFound).
		AnyTimes()
	s.ModelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, errors.New("mi boom"))
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "cmd", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "mi boom")
}

// TestRunOnAllMachinesRejectNestedExec rejects nested juju-exec or
// juju-run commands.
func (s *runAllSuite) TestRunOnAllMachinesRejectNestedExec(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err1 := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "juju-exec foo", Timeout: time.Second})
	_, err2 := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "bar; juju-run baz", Timeout: time.Second})

	// Assert
	c.Assert(err1, tc.ErrorMatches, "cannot use \".*\" as an action command")
	c.Assert(err2, tc.ErrorMatches, "cannot use \".*\" as an action command")
}

// TestRunOnAllMachinesServiceError propagates a service error and
// ensures args mapping is correct.
func (s *runAllSuite) TestRunOnAllMachinesServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any()).
		Return(operation.RunResult{}, errors.New("service fail"))

	// Act
	_, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "whoami", Timeout: 2 * time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "service fail")
}

// TestRunOnAllMachinesSuccess verifies success mapping to
// params.EnqueuedActions.
func (s *runAllSuite) TestRunOnAllMachinesSuccess(c *tc.C) {
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
	s.OperationService.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), operation.ExecArgs{
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

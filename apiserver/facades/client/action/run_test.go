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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

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
		Parallel:       func(b bool) *bool { return &b }(false),
		ExecutionGroup: func(s string) *string { return &s }("eg-1"),
	}
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			c.Assert(len(got), tc.Equals, 1)
			rp := got[0]
			c.Check(rp.Applications, tc.DeepEquals, []string{"a1", "a2"})
			c.Check(rp.Machines, tc.DeepEquals, []machine.Name{"0", "42"})
			c.Check(rp.Units, tc.DeepEquals, []unit.Name{"app/1", "db/0"})
			c.Check(rp.ActionName, tc.Equals, coreoperation.JujuExecActionName)
			c.Check(rp.IsParallel, tc.Equals, false)
			c.Check(rp.ExecutionGroup, tc.Equals, "eg-1")
			c.Check(rp.Parameters["command"], tc.Equals, "echo hello")
			c.Check(rp.Parameters["timeout"], tc.Equals, (5 * time.Second).Nanoseconds())
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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			rp := got[0]
			c.Check(rp.IsParallel, tc.Equals, false)
			c.Check(rp.ExecutionGroup, tc.Equals, "")
			c.Check(rp.ActionName, tc.Equals, coreoperation.JujuExecActionName)
			c.Check(rp.Parameters["command"], tc.Equals, "whoami")
			c.Check(rp.Parameters["timeout"], tc.Equals, (1 * time.Second).Nanoseconds())
			return operation.RunResult{OperationID: "2"}, nil
		})

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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Return(
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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Return(
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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, got []operation.RunArgs) (operation.RunResult, error) {
			rp := got[0]
			c.Check(rp.Applications, tc.HasLen, 0)
			c.Check(rp.Machines, tc.HasLen, 0)
			c.Check(rp.Units, tc.HasLen, 0)
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
	s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).Times(0)

	// Act
	_, err := api.Run(c.Context(), params.RunParams{Commands: "echo x", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorMatches, "block-fail")
}

// TestRun_TimeoutConversion checks precise timeout conversion to ns for a few
// edge values.
func (s *runSuite) TestRun_TimeoutConversion(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	api := s.NewActionAPI(c)
	times := []time.Duration{0, 123 * time.Millisecond, 1 * time.Nanosecond}
	for _, t := range times {
		s.OperationService.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ context.Context, got []operation.RunArgs) (operation.RunResult, error) {
				c.Check(got[0].Parameters["timeout"], tc.Equals, t.Nanoseconds())
				return operation.RunResult{}, errors.New("no target") // ignore result
			})

		// Act
		_, _ = api.Run(c.Context(), params.RunParams{Commands: "cmd", Timeout: t})
	}
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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).Times(0)

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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, tp operation.TaskArgs) (operation.RunResult, error) {
			c.Check(tp.ActionName, tc.Equals, "juju-exec")
			c.Check(tp.IsParallel, tc.Equals, false) // default
			c.Check(tp.ExecutionGroup, tc.Equals, "")
			c.Check(tp.Parameters["command"], tc.Equals, "whoami")
			c.Check(tp.Parameters["timeout"], tc.Equals, (2 * time.Second).Nanoseconds())
			return operation.RunResult{}, errors.New("service fail")
		})

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
	s.OperationService.EXPECT().RunOnAllMachines(gomock.Any(), gomock.Any()).Return(
		operation.RunResult{OperationID: "77"}, nil)

	// Act
	res, err := api.RunOnAllMachines(c.Context(), params.RunParams{Commands: "true", Timeout: time.Second})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res.OperationTag, tc.Equals, "operation-77")
}

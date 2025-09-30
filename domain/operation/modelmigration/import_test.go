// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/juju/description/v10"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/operation/internal"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	importService *MockImportService
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.importService = NewMockImportService(ctrl)
	c.Cleanup(func() { s.importService = nil })
	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		service: s.importService,
		logger:  loggertesting.WrapCheckLog(c),
	}
}

// TestExecuteSuccess verifies that Execute builds the correct arguments from the
// model and calls ImportOperations.
func (s *importSuite) TestExecuteSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	m := description.NewModel(description.ModelArgs{})
	now := time.Now().UTC()

	// Operation present in the model
	m.AddOperation(description.OperationArgs{
		Id:        "op-1",
		Summary:   "sum 1",
		Enqueued:  now.Add(-3 * time.Hour),
		Started:   now.Add(-2 * time.Hour),
		Completed: now.Add(-1 * time.Hour),
		Status:    corestatus.Completed.String(),
		Fail:      "",
	})

	// One action associated to op-1
	m.AddAction(description.ActionArgs{
		Id:        "a-1",
		Receiver:  "0",
		Name:      "do-it",
		Operation: "op-1",
		Parameters: map[string]any{
			"p1": "v1",
		},
		Parallel:       true,
		ExecutionGroup: "grp-1",
		Enqueued:       now.Add(-3 * time.Hour),
		Started:        now.Add(-2 * time.Hour),
		Completed:      now.Add(-1 * time.Hour),
		Status:         corestatus.Completed.String(),
		Message:        "ok",
		Results: map[string]any{
			"r": 42,
		},
		Messages: []description.ActionMessage{
			taskMessage{TaskLog: operation.TaskLog{Timestamp: now.Add(-110 * time.Minute), Message: "log-1"}},
			taskMessage{TaskLog: operation.TaskLog{Timestamp: now.Add(-100 * time.Minute), Message: "log-2"}},
		},
	})

	s.importService.EXPECT().InsertMigratingOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args internal.ImportOperationsArgs) error {
			c.Assert(len(args), tc.Equals, 1)
			opArg := args[0]
			c.Check(opArg.ID, tc.Equals, "op-1")
			c.Check(opArg.Summary, tc.Equals, "sum 1")
			c.Check(opArg.Enqueued.Equal(now.Add(-3*time.Hour)), tc.IsTrue)
			c.Check(opArg.Started.Equal(now.Add(-2*time.Hour)), tc.IsTrue)
			c.Check(opArg.Completed.Equal(now.Add(-1*time.Hour)), tc.IsTrue)
			c.Check(opArg.IsParallel, tc.Equals, true)
			c.Check(opArg.ExecutionGroup, tc.Equals, "grp-1")
			c.Check(opArg.Status, tc.Equals, corestatus.Completed)
			c.Check(opArg.Fail, tc.Equals, "")

			// Operation-level fields populated from the action
			c.Check(opArg.ActionName, tc.Equals, "do-it")
			c.Check(opArg.Parameters, tc.DeepEquals, map[string]any{"p1": "v1"})

			// Tasks
			c.Assert(len(opArg.Tasks), tc.Equals, 1)
			t := opArg.Tasks[0]
			c.Check(t.ID, tc.Equals, "a-1")
			c.Check(t.MachineName, tc.Equals, machine.Name("0"))
			c.Check(t.Enqueued.Equal(now.Add(-3*time.Hour)), tc.IsTrue)
			c.Check(t.Started.Equal(now.Add(-2*time.Hour)), tc.IsTrue)
			c.Check(t.Completed.Equal(now.Add(-1*time.Hour)), tc.IsTrue)
			c.Check(t.Status, tc.Equals, corestatus.Completed)
			c.Check(t.Message, tc.Equals, "ok")
			c.Check(t.Output, tc.DeepEquals, map[string]any{"r": 42})
			c.Check(len(t.Log), tc.Equals, 2)
			c.Check(t.Log[0].Message, tc.Equals, "log-1")
			c.Check(t.Log[1].Message, tc.Equals, "log-2")
			return nil
		},
	)

	// Act
	i := s.newImportOperation(c)
	err := i.Execute(c.Context(), m)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestExecuteSuccessTwoTasksTwoOps validates that Execute handles
// two operations with two actions each successfully. It checks that params
// are correctly propagated to the tasks and dispatched to the correct ops.
func (s *importSuite) TestExecuteSuccessTwoTasksTwoOps(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	m := description.NewModel(description.ModelArgs{})

	// Operation present in the model
	m.AddOperation(description.OperationArgs{
		Id:      "op-1",
		Summary: "sum 1",
	})
	m.AddOperation(description.OperationArgs{
		Id:      "op-2",
		Summary: "sum 2",
	})

	// template for action of op-1
	opTaskTmpl1 := description.ActionArgs{
		Operation: "op-1",

		// params that should be the same for both actions
		Receiver:       "happy/0",
		Name:           "do-it",
		Parallel:       true,
		ExecutionGroup: "G1",
		Parameters: map[string]any{
			"foo": "bar",
		},
	}

	// template for action of op-2
	opTaskTmpl2 := description.ActionArgs{
		Operation: "op-2",

		// params that should be the same for both actions
		Receiver:       "0/lxd/1",
		Name:           "do-not-do-it",
		Parallel:       false,
		ExecutionGroup: "G2",
		Parameters: map[string]any{
			"bar": "foo",
		},
	}

	task11, task12 := opTaskTmpl1, opTaskTmpl1
	task21, task22 := opTaskTmpl2, opTaskTmpl2
	task11.Id = "a-1"
	task12.Id = "a-2"
	task12.Receiver = "happy/1"
	task21.Id = "a-3"
	task22.Id = "a-4"
	task22.Receiver = "0/lxd/2"

	m.AddAction(task11)
	m.AddAction(task12)
	m.AddAction(task21)
	m.AddAction(task22)

	s.importService.EXPECT().InsertMigratingOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args internal.ImportOperationsArgs) error {
			c.Assert(len(args), tc.Equals, 2)
			op1 := args[0]
			c.Check(op1.ID, tc.Equals, "op-1")
			c.Check(op1.Summary, tc.Equals, "sum 1")
			c.Check(op1.IsParallel, tc.Equals, true)
			c.Check(op1.ExecutionGroup, tc.Equals, "G1")
			c.Check(op1.Parameters, tc.DeepEquals, map[string]any{"foo": "bar"})
			c.Check(op1.Tasks, tc.SameContents, []internal.ImportTaskArg{
				{ID: "a-1", UnitName: "happy/0"},
				{ID: "a-2", UnitName: "happy/1"}})

			op2 := args[1]
			c.Check(op2.ID, tc.Equals, "op-2")
			c.Check(op2.Summary, tc.Equals, "sum 2")
			c.Check(op2.IsParallel, tc.Equals, false)
			c.Check(op2.ExecutionGroup, tc.Equals, "G2")
			c.Check(op2.Parameters, tc.DeepEquals, map[string]any{"bar": "foo"})
			c.Check(op2.Tasks, tc.SameContents, []internal.ImportTaskArg{
				{ID: "a-3", MachineName: "0/lxd/1"},
				{ID: "a-4", MachineName: "0/lxd/2"}})

			return nil
		},
	)

	// Act
	i := s.newImportOperation(c)
	err := i.Execute(c.Context(), m)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
}

// TestExecuteInconsistentOperationArgs verifies that Execute fails and
// returns an error when operation actions have inconsistent attributes.
func (s *importSuite) TestExecuteInconsistentOperationArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange model with one operation and two actions with differing flags.
	m := description.NewModel(description.ModelArgs{})
	now := time.Now().UTC()

	m.AddOperation(description.OperationArgs{
		Id:       "op-1",
		Summary:  "sum",
		Enqueued: now.Add(-2 * time.Hour),
		Status:   corestatus.Running.String(),
	})

	// First action:
	m.AddAction(description.ActionArgs{
		Id:        "a-1",
		Operation: "op-1",

		// params that should be the same for both actions
		Receiver:       "happy/0",
		Name:           "do-it",
		Parallel:       true,
		ExecutionGroup: "G1",
		Parameters: map[string]any{
			"foo": "bar",
		},
	})
	// Second action: parallel=false, group=G2
	m.AddAction(description.ActionArgs{
		Id:        "a-2",
		Operation: "op-1",

		// params that should be the same for both actions, but different
		Receiver:       "grumpy/0",
		Name:           "do-not-do-it",
		Parallel:       false,
		ExecutionGroup: "G2",
		Parameters: map[string]any{
			"bar": "foo",
		},
	})

	// Act: building args should fail due to inconsistent attributes.
	i := s.newImportOperation(c)
	err := i.Execute(c.Context(), m)
	// Assert: error mentions inconsistency
	c.Assert(err, tc.NotNil)
	c.Check(err.Error(), tc.Contains, "application is not consistent")
	c.Check(err.Error(), tc.Contains, "parallel flag is not consistent")
	c.Check(err.Error(), tc.Contains, "execution group is not consistent")
	c.Check(err.Error(), tc.Contains, "parameters are not consistent")
	c.Check(err.Error(), tc.Contains, "action is not consistent")
}

// When there are operations but no actions, defaults should be applied
// (IsParallel=false, ExecutionGroup="").
func (s *importSuite) TestExecuteNoActionsDefaults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := description.NewModel(description.ModelArgs{})
	m.AddOperation(description.OperationArgs{Id: "op-1", Status: corestatus.Completed.String()})

	s.importService.EXPECT().InsertMigratingOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args internal.ImportOperationsArgs) error {
			c.Assert(len(args), tc.Equals, 1)
			opArg := args[0]
			c.Check(opArg.IsParallel, tc.Equals, false)
			c.Check(opArg.ExecutionGroup, tc.Equals, "")
			// No tasks.
			c.Check(len(opArg.Tasks), tc.Equals, 0)
			return nil
		})

	i := &importOperation{service: s.importService, logger: loggertesting.WrapCheckLog(c)}
	err := i.Execute(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)
}

// TestExecuteServiceError ensures errors from ImportOperations are wrapped and returned.
func (s *importSuite) TestExecuteServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := description.NewModel(description.ModelArgs{})
	m.AddOperation(description.OperationArgs{Id: "op-1"})

	s.importService.EXPECT().InsertMigratingOperations(gomock.Any(), gomock.Any()).Return(errors.New("boom"))

	i := s.newImportOperation(c)
	err := i.Execute(c.Context(), m)
	c.Assert(err, tc.ErrorMatches, "importing operations: .*boom.*")
}

// TestExecuteUnknownTaskOperation ensures an error is returned when actions reference
// unknown operation IDs.
func (s *importSuite) TestExecuteUnknownTaskOperation(c *tc.C) {
	defer s.setupMocks(c).Finish()

	m := description.NewModel(description.ModelArgs{})
	m.AddOperation(description.OperationArgs{Id: "op-1"})
	m.AddAction(description.ActionArgs{Id: "a-unknown", Operation: "op-unknown", Name: "x", Receiver: "0", Status: corestatus.Running.String()})

	i := s.newImportOperation(c)
	err := i.Execute(c.Context(), m)
	c.Assert(err, tc.ErrorMatches, "tasks with unknown operation ids: .*op-unknown.*")
}

// TestExecuteNoOperations returns nil and does not call the service when there are no operations.
func (s *importSuite) TestExecuteNoOperations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	m := description.NewModel(description.ModelArgs{})
	i := s.newImportOperation(c)
	err := i.Execute(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)
}

// TestRollbackNoOperations verifies rollback is a no-op without operations.
func (s *importSuite) TestRollbackNoOperations(c *tc.C) {
	defer s.setupMocks(c).Finish()
	m := description.NewModel(description.ModelArgs{})
	i := s.newImportOperation(c)
	err := i.Rollback(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)
}

// TestRollbackCallsService verifies DeleteImportedOperations is called when operations exist.
func (s *importSuite) TestRollbackCallsService(c *tc.C) {
	defer s.setupMocks(c).Finish()
	m := description.NewModel(description.ModelArgs{})
	m.AddOperation(description.OperationArgs{Id: "op-1"})

	s.importService.EXPECT().DeleteImportedOperations(gomock.Any()).Return(nil)

	i := s.newImportOperation(c)
	err := i.Rollback(c.Context(), m)
	c.Assert(err, tc.ErrorIsNil)
}

// TestRollbackServiceError ensures rollback errors are wrapped correctly.
func (s *importSuite) TestRollbackServiceError(c *tc.C) {
	defer s.setupMocks(c).Finish()
	m := description.NewModel(description.ModelArgs{})
	m.AddOperation(description.OperationArgs{Id: "op-1"})

	s.importService.EXPECT().DeleteImportedOperations(gomock.Any()).Return(errors.New("boom"))

	i := s.newImportOperation(c)
	err := i.Rollback(c.Context(), m)
	c.Assert(err, tc.ErrorMatches, "operation import rollback failed: .*boom.*")
}

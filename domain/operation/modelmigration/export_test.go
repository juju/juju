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

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	exportService *MockExportService
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.exportService = NewMockExportService(ctrl)
	c.Cleanup(func() {
		s.exportService = nil
	})
	return ctrl
}

func (s *exportSuite) newExportOperation(c *tc.C) *exportOperation {
	return &exportOperation{
		exportService: s.exportService,
		logger:        loggertesting.WrapCheckLog(c),
	}
}

// TestExecuteSuccessWithPagination verifies that the export operation
// retrieves all operations from the export service, and adds them to the
// model.
func (s *exportSuite) TestExecuteSuccessWithPagination(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	dst := description.NewModel(description.ModelArgs{})
	now := time.Now()

	// Create two pages of operations.
	op1 := operation.OperationInfo{
		OperationID: "op-1",
		Summary:     "summary 1",
		Enqueued:    now.Add(-2 * time.Hour),
		Started:     now.Add(-1 * time.Hour),
		Completed:   now,
		Status:      corestatus.Completed,
		Machines: []operation.MachineTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID:         "m-1-a",
					ActionName: "do-a",
					Enqueued:   now.Add(-2 * time.Hour),
					Started:    now.Add(-1 * time.Hour),
					Completed:  now,
					Status:     corestatus.Completed,
					Message:    "ok",
				},
				ReceiverName: "0",
			},
		},
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID:         "u-1-b",
					ActionName: "do-b",
					Enqueued:   now.Add(-2 * time.Hour),
					// Not started nor completed
					Status:  corestatus.Running,
					Message: "done",
				},
				ReceiverName: "app/0",
			},
		},
	}
	op2 := operation.OperationInfo{
		OperationID: "op-2",
		Summary:     "summary 2",
		Enqueued:    now.Add(-2 * time.Hour),
		Started:     now.Add(-1 * time.Minute),
		Completed:   now.Add(-1 * time.Second),
		Status:      corestatus.Running,
	}

	// Expect pagination: first page truncated, second not.
	s.exportService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args operation.QueryArgs) (operation.QueryResult, error) {
			// first call, offset should be 0
			if args.Offset == nil || *args.Offset != 0 {
				c.Fatalf("expected first offset 0, got %+v", args.Offset)
			}
			return operation.QueryResult{Operations: []operation.OperationInfo{op1}, Truncated: true}, nil
		},
	)
	s.exportService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, args operation.QueryArgs) (operation.QueryResult, error) {
			// second call, offset should be 1 (we already returned one op)
			if args.Offset == nil || *args.Offset != 1 {
				c.Fatalf("expected second offset 1, got %+v", args.Offset)
			}
			return operation.QueryResult{Operations: []operation.OperationInfo{op2}, Truncated: false}, nil
		},
	)

	// Act
	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	// Validate operations and actions were added to the model.
	ops := dst.Operations()
	c.Assert(len(ops), tc.Equals, 2)
	// operations ordering should be the same as returned by pages
	c.Assert(ops[0].Id(), tc.Equals, "op-1")
	c.Assert(ops[0].Summary(), tc.Equals, "summary 1")
	c.Assert(ops[0].Enqueued(), tc.Equals, op1.Enqueued)
	c.Assert(ops[0].Started(), tc.Equals, op1.Started)
	c.Assert(ops[0].Completed(), tc.Equals, op1.Completed)
	c.Assert(ops[0].Status(), tc.Equals, op1.Status.String())

	// Spawned/Complete task counts come from toDescriptionAction incrementing.
	c.Assert(ops[0].SpawnedTaskCount(), tc.Equals, 2)
	c.Assert(ops[0].CompleteTaskCount(), tc.Equals, 1)

	actions := dst.Actions()
	c.Assert(len(actions), tc.Equals, 2)
	// Verify key fields of first action
	c.Assert(actions[0].Id(), tc.Equals, "m-1-a")
	c.Assert(actions[0].Receiver(), tc.Equals, "0")
	c.Assert(actions[0].Name(), tc.Equals, "do-a")
	c.Assert(actions[0].Operation(), tc.Equals, "op-1")
	c.Assert(actions[0].Enqueued(), tc.Equals, now.Add(-2*time.Hour))
	c.Assert(actions[0].Started(), tc.Equals, now.Add(-1*time.Hour))
	c.Assert(actions[0].Completed(), tc.Equals, now)
	c.Assert(actions[0].Status(), tc.Equals, corestatus.Completed.String())
	c.Assert(actions[0].Message(), tc.Equals, "ok")

	// Second operation has no tasks
	c.Assert(ops[1].Id(), tc.Equals, "op-2")
	c.Assert(ops[1].SpawnedTaskCount(), tc.Equals, 0)
	c.Assert(ops[1].CompleteTaskCount(), tc.Equals, 0)
}

// TestExecuteGetOperationsError verifies that an error from the export service
// is propagated to the caller.
func (s *exportSuite) TestExecuteGetOperationsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dst := description.NewModel(description.ModelArgs{})

	// Return an error on first page retrieval.
	s.exportService.EXPECT().GetOperations(gomock.Any(), gomock.Any()).Return(operation.QueryResult{}, errors.New("boom"))

	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorMatches, "getting all operations from offset 0: .*boom.*")
}

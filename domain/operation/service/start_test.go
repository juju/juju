// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	context "context"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	internal "github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	uuid "github.com/juju/juju/internal/uuid"
)

type startSuite struct {
	state                 *MockState
	clock                 clock.Clock
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockLeadershipService *MockLeadershipService
}

func TestStartSuite(t *testing.T) {
	tc.Run(t, &startSuite{})
}

func (s *startSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.clock = clock.WallClock
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockLeadershipService = NewMockLeadershipService(ctrl)
	return ctrl
}

func (s *startSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter, s.mockLeadershipService)
}

func (s *startSuite) TestStartExecOperationWithMachinesAndUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		Machines: []machine.Name{"0", "1"},
		Units:    []unit.Name{"test-app/0", "test-app/1"},
	}
	args := operation.ExecArgs{
		Command:        "echo hello",
		Timeout:        time.Minute,
		Parallel:       true,
		ExecutionGroup: "test-group",
	}

	expectedStateTarget := internal.ReceiversWithResolvedLeaders{
		Machines: []machine.Name{"0", "1"},
		Units:    []unit.Name{"test-app/0", "test-app/1"},
	}

	expectedResult := operation.RunResult{
		OperationID: "42",
		Machines: []operation.MachineTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "43",
				},
				ReceiverName: "0",
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "44",
				},
				ReceiverName: "1",
			},
		},
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "45",
				},
				ReceiverName: "test-app/0",
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "46",
				},
				ReceiverName: "test-app/1",
			},
		},
	}

	s.state.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(expectedStateTarget), args).DoAndReturn(
		func(_ context.Context, _ uuid.UUID, actualTarget internal.ReceiversWithResolvedLeaders, actualArgs operation.ExecArgs) (operation.RunResult, error) {
			c.Assert(actualTarget.Applications, tc.DeepEquals, expectedStateTarget.Applications)
			c.Assert(actualTarget.Machines, tc.DeepEquals, expectedStateTarget.Machines)
			c.Assert(actualTarget.Units, tc.DeepEquals, expectedStateTarget.Units)
			c.Assert(actualTarget.LeaderUnits, tc.DeepEquals, expectedStateTarget.LeaderUnits)
			return expectedResult, nil
		})

	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "42")
	c.Check(result.Machines, tc.HasLen, 2)
	c.Assert(result.Units, tc.HasLen, 2)
	c.Check(result.Machines[0].ReceiverName, tc.Equals, machine.Name("0"))
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
}

func (s *startSuite) TestStartExecOperationWithLeaderUnitsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		LeaderUnit: []string{"test-app", "other-app"},
		// We manually provide a unit which happens to be the same as the first
		// target unit. In this case, we should still get 3 results back.
		Units: []unit.Name{unit.Name("test-app/0")},
	}
	args := operation.ExecArgs{
		Command: "leader-action",
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("test-app/0", nil)
	s.mockLeadershipService.EXPECT().ApplicationLeader("other-app").Return("other-app/1", nil)

	expectedStateTarget := internal.ReceiversWithResolvedLeaders{
		Units:       []unit.Name{"test-app/0"},
		LeaderUnits: []unit.Name{"test-app/0", "other-app/1"},
	}

	expectedResult := operation.RunResult{
		OperationID: "42",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "43",
				},
				ReceiverName: "test-app/0",
				IsLeader:     true,
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "44",
				},
				ReceiverName: "other-app/1",
				IsLeader:     true,
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "45",
				},
				ReceiverName: "test-app/0",
			},
		},
	}

	s.state.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(expectedStateTarget), args).DoAndReturn(
		func(_ context.Context, _ uuid.UUID, actualTarget internal.ReceiversWithResolvedLeaders, actualArgs operation.ExecArgs) (operation.RunResult, error) {
			c.Assert(actualTarget.Applications, tc.DeepEquals, expectedStateTarget.Applications)
			c.Assert(actualTarget.Machines, tc.DeepEquals, expectedStateTarget.Machines)
			c.Assert(actualTarget.Units, tc.DeepEquals, expectedStateTarget.Units)
			c.Assert(actualTarget.LeaderUnits, tc.DeepEquals, expectedStateTarget.LeaderUnits)
			return expectedResult, nil
		})

	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "42")
	c.Assert(result.Units, tc.HasLen, 3)
	c.Check(result.Units[0].ID, tc.Equals, "43")
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[1].ID, tc.Equals, "44")
	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("other-app/1"))
	c.Check(result.Units[1].IsLeader, tc.Equals, true)
	c.Check(result.Units[2].ID, tc.Equals, "45")
	c.Check(result.Units[2].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[2].IsLeader, tc.Equals, false)
}

func (s *startSuite) TestStartExecOperationConsolidationMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		LeaderUnit: []string{"test-app"},
	}
	args := operation.ExecArgs{
		Command: "leader-action",
	}

	// State layer returns different unit than expected.
	expectedResult := operation.RunResult{
		OperationID: "50",
		Units:       []operation.UnitTaskResult{},
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("test-app/0", nil)
	s.state.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(expectedResult, nil)

	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 1)
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "")
	c.Check(result.Units[0].TaskInfo.Error, tc.ErrorMatches, "missing result for unit test-app/0")
}

func (s *startSuite) TestStartExecOperationLeaderResolutionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		LeaderUnit: []string{"test-app"},
	}
	args := operation.ExecArgs{
		Command: "leader-action",
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("", errors.New("leadership error"))

	// Return early since there are no valid targets to pass to state layer.
	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "")
	c.Assert(result.Units, tc.HasLen, 1)
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[0].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for test-app:.*leadership error")
}

func (s *startSuite) TestStartExecOperationLeaderUnitNameParsingError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		LeaderUnit: []string{"test-app"},
	}
	args := operation.ExecArgs{
		Command: "leader-action",
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("invalid-unit-name", nil)

	// Return early since there are no valid targets to pass to state layer.
	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "")
	c.Assert(result.Units, tc.HasLen, 1)
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[0].TaskInfo.Error, tc.ErrorMatches, "parsing unit name for invalid-unit-name:.*")
}

func (s *startSuite) TestStartExecOperationMixedLeaderResults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		LeaderUnit: []string{"good-app", "bad-app"},
		Units:      []unit.Name{"regular-unit/0"},
	}
	args := operation.ExecArgs{
		Command: "mixed-action",
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("good-app").Return("good-app/0", nil)
	s.mockLeadershipService.EXPECT().ApplicationLeader("bad-app").Return("", errors.New("leadership error"))

	expectedStateTarget := internal.ReceiversWithResolvedLeaders{
		LeaderUnits: []unit.Name{"good-app/0"},
		Units:       []unit.Name{"regular-unit/0"},
	}

	expectedResult := operation.RunResult{
		OperationID: "42",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "44",
				},
				ReceiverName: "good-app/0",
				IsLeader:     true,
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "43",
				},
				ReceiverName: "regular-unit/0",
			},
		},
	}

	s.state.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(expectedStateTarget), args).DoAndReturn(
		func(_ context.Context, _ uuid.UUID, actualTarget internal.ReceiversWithResolvedLeaders, actualArgs operation.ExecArgs) (operation.RunResult, error) {
			c.Assert(actualTarget.Applications, tc.DeepEquals, expectedStateTarget.Applications)
			c.Assert(actualTarget.Machines, tc.DeepEquals, expectedStateTarget.Machines)
			c.Assert(actualTarget.Units, tc.DeepEquals, expectedStateTarget.Units)
			c.Assert(actualTarget.LeaderUnits, tc.DeepEquals, expectedStateTarget.LeaderUnits)
			return expectedResult, nil
		})

	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "42")
	c.Assert(result.Units, tc.HasLen, 3) // 2 leader units + 1 regular unit from state

	// Check that first leader unit has success from state layer,
	// second is in error.
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("good-app/0"))
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "44")

	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("bad-app/0"))
	c.Check(result.Units[1].IsLeader, tc.Equals, true)
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for bad-app:.*leadership error")

	// Regular unit from state layer (appended at the end).
	c.Check(result.Units[2].ReceiverName, tc.Equals, unit.Name("regular-unit/0"))
	c.Check(result.Units[2].IsLeader, tc.Equals, false)
	c.Check(result.Units[2].TaskInfo.ID, tc.Equals, "43")
}

func (s *startSuite) TestStartExecOperationStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		Machines: []machine.Name{"0"},
	}
	args := operation.ExecArgs{Command: "echo hello"}

	s.state.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.Any(), args).Return(operation.RunResult{}, errors.New("database error"))

	_, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.ErrorMatches, "starting exec operation: database error")
}

func (s *startSuite) TestStartExecOperationOnlyLeaderUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{
		LeaderUnit: []string{"test-app"},
	}
	args := operation.ExecArgs{Command: "leader-only"}

	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("test-app/0", nil)

	expectedStateTarget := internal.ReceiversWithResolvedLeaders{
		LeaderUnits: []unit.Name{"test-app/0"},
	}

	expectedResult := operation.RunResult{
		OperationID: "42",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "43",
				},
				ReceiverName: "test-app/0",
				IsLeader:     true,
			},
		},
	}

	s.state.EXPECT().AddExecOperation(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(expectedStateTarget), args).DoAndReturn(
		func(_ context.Context, _ uuid.UUID, actualTarget internal.ReceiversWithResolvedLeaders, actualArgs operation.ExecArgs) (operation.RunResult, error) {
			c.Assert(actualTarget.Applications, tc.DeepEquals, expectedStateTarget.Applications)
			c.Assert(actualTarget.Machines, tc.DeepEquals, expectedStateTarget.Machines)
			c.Assert(actualTarget.Units, tc.DeepEquals, expectedStateTarget.Units)
			c.Assert(actualTarget.LeaderUnits, tc.DeepEquals, expectedStateTarget.LeaderUnits)
			return expectedResult, nil
		})

	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 1)
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "43")
}

func (s *startSuite) TestStartExecOperationEmptyTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := operation.Receivers{}
	args := operation.ExecArgs{Command: "echo hello"}

	// Return early without calling state layer.
	result, err := s.service().AddExecOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "")
	c.Assert(result.Units, tc.HasLen, 0)
	c.Check(result.Machines, tc.HasLen, 0)
}

func (s *startSuite) TestStartExecOperationOnAllMachinesSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := operation.ExecArgs{
		Command:        "echo hello",
		Timeout:        time.Minute,
		Parallel:       false,
		ExecutionGroup: "maintenance",
	}

	expectedResult := operation.RunResult{
		OperationID: "47",
		Machines: []operation.MachineTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "48",
				},
				ReceiverName: "0",
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "49",
				},
				ReceiverName: "1",
			},
		},
	}

	s.state.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any(), args).Return(expectedResult, nil)

	result, err := s.service().AddExecOperationOnAllMachines(c.Context(), args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "47")
	c.Check(result.Machines, tc.HasLen, 2)
	c.Assert(result.Units, tc.HasLen, 0)
}

func (s *startSuite) TestStartExecOperationOnAllMachinesError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	args := operation.ExecArgs{Command: "echo hello"}

	s.state.EXPECT().AddExecOperationOnAllMachines(gomock.Any(), gomock.Any(), args).Return(operation.RunResult{}, errors.New("no machines found"))

	_, err := s.service().AddExecOperationOnAllMachines(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, "starting exec operation on all machines: no machines found")
}

func (s *startSuite) TestStartActionOperationWithRegularUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{Unit: "test-app/0"},
		{Unit: "test-app/1"},
	}
	args := operation.TaskArgs{
		ActionName:     "backup",
		ExecutionGroup: "maintenance",
		IsParallel:     false,
		Parameters: map[string]any{
			"filename": "backup.tar.gz",
			"compress": true,
		},
	}

	expectedTargetUnits := []unit.Name{"test-app/0", "test-app/1"}
	expectedResult := operation.RunResult{
		OperationID: "50",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID:         "51",
					ActionName: "backup",
					Enqueued:   time.Now().UTC(),
					Status:     corestatus.Pending,
				},
				ReceiverName: "test-app/0",
			},
			{
				TaskInfo: operation.TaskInfo{
					ID:         "52",
					ActionName: "backup",
					Enqueued:   time.Now().UTC(),
					Status:     corestatus.Pending,
				},
				ReceiverName: "test-app/1",
			},
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), expectedTargetUnits, args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "50")
	c.Assert(result.Units, tc.HasLen, 2)
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("test-app/1"))
}

func (s *startSuite) TestStartActionOperationWithLeaderUnits(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{LeaderUnit: "test-app"},
		{LeaderUnit: "other-app"},
		// We manually provide a unit which happens to be the same as the first
		// target unit. In this case, we should still get 3 results back.
		{Unit: unit.Name("test-app/0")},
	}
	args := operation.TaskArgs{
		ActionName: "leader-action",
		IsParallel: true,
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("test-app/0", nil)
	s.mockLeadershipService.EXPECT().ApplicationLeader("other-app").Return("other-app/1", nil)

	expectedTargetUnits := []unit.Name{"test-app/0", "other-app/1", "test-app/0"}
	expectedResult := operation.RunResult{
		OperationID: "50",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "51",
				},
				ReceiverName: "test-app/0",
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "52",
				},
				ReceiverName: "other-app/1",
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "53",
				},
				ReceiverName: "test-app/0",
			},
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), expectedTargetUnits, args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "50")
	c.Assert(result.Units, tc.HasLen, 3)
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[1].IsLeader, tc.Equals, true)
	c.Check(result.Units[2].IsLeader, tc.Equals, false)
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "51")
	c.Check(result.Units[1].TaskInfo.ID, tc.Equals, "52")
	c.Check(result.Units[2].TaskInfo.ID, tc.Equals, "53")
}

func (s *startSuite) TestStartActionOperationMixedLeaderResults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{
			LeaderUnit: "good-app",
		},
		{
			LeaderUnit: "bad-app",
		},
		{
			Unit: unit.Name("regular-unit/0"),
		},
	}
	args := operation.TaskArgs{
		ActionName: "leader-action",
		IsParallel: true,
	}
	s.mockLeadershipService.EXPECT().ApplicationLeader("good-app").Return("good-app/0", nil)
	s.mockLeadershipService.EXPECT().ApplicationLeader("bad-app").Return("", errors.New("leadership error"))

	expectedStateTarget := []unit.Name{"good-app/0", "regular-unit/0"}

	expectedResult := operation.RunResult{
		OperationID: "42",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "44",
				},
				ReceiverName: "good-app/0",
				IsLeader:     true,
			},
			{
				TaskInfo: operation.TaskInfo{
					ID: "43",
				},
				ReceiverName: "regular-unit/0",
			},
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), expectedStateTarget, args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "42")
	c.Assert(result.Units, tc.HasLen, 3) // 2 leader units + 1 regular unit from state

	// Check that first leader unit has success from state layer,
	// second is in error.
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("good-app/0"))
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "44")

	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("bad-app/0"))
	c.Check(result.Units[1].IsLeader, tc.Equals, true)
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for bad-app:.*leadership error")

	// Regular unit from state layer (appended at the end).
	c.Check(result.Units[2].ReceiverName, tc.Equals, unit.Name("regular-unit/0"))
	c.Check(result.Units[2].IsLeader, tc.Equals, false)
	c.Check(result.Units[2].TaskInfo.ID, tc.Equals, "43")
}

func (s *startSuite) TestStartActionOperationMixedFailedLeaderResults(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{
			LeaderUnit: "bad-app",
		},
		{
			LeaderUnit: "worse-app",
		},
		{
			Unit: unit.Name("regular-unit/0"),
		},
	}
	args := operation.TaskArgs{
		ActionName: "leader-action",
		IsParallel: true,
	}
	s.mockLeadershipService.EXPECT().ApplicationLeader("bad-app").Return("", errors.New("leadership error"))
	s.mockLeadershipService.EXPECT().ApplicationLeader("worse-app").Return("", errors.New("leadership error"))

	expectedStateTarget := []unit.Name{"regular-unit/0"}

	expectedResult := operation.RunResult{
		OperationID: "42",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo: operation.TaskInfo{
					ID: "43",
				},
				ReceiverName: "regular-unit/0",
			},
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), expectedStateTarget, args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Check(result.OperationID, tc.Equals, "42")
	c.Assert(result.Units, tc.HasLen, 3) // 2 leader units + 1 regular unit from state

	// Check that first leader unit has success from state layer,
	// second is in error.
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("bad-app/0"))
	c.Check(result.Units[0].IsLeader, tc.Equals, true)
	c.Check(result.Units[0].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for bad-app:.*leadership error")

	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("worse-app/0"))
	c.Check(result.Units[1].IsLeader, tc.Equals, true)
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for worse-app:.*leadership error")

	// Regular unit from state layer (appended at the end).
	c.Check(result.Units[2].ReceiverName, tc.Equals, unit.Name("regular-unit/0"))
	c.Check(result.Units[2].IsLeader, tc.Equals, false)
	c.Check(result.Units[2].TaskInfo.ID, tc.Equals, "43")
}

func (s *startSuite) TestStartActionOperationMixedTargets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{Unit: "test-app/0"},
		{LeaderUnit: "test-app"},
		{Unit: "other-app/0"},
	}
	args := operation.TaskArgs{
		ActionName: "mixed-action",
	}

	// Mock leadership service call
	s.mockLeadershipService.EXPECT().ApplicationLeader("test-app").Return("test-app/1", nil)

	expectedTargetUnits := []unit.Name{"test-app/0", "test-app/1", "other-app/0"}
	expectedResult := operation.RunResult{
		OperationID: "50",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo:     operation.TaskInfo{ID: "51", Status: corestatus.Pending},
				ReceiverName: "test-app/0",
			},
			{
				TaskInfo:     operation.TaskInfo{ID: "52", Status: corestatus.Pending},
				ReceiverName: "test-app/1",
			},
			{
				TaskInfo:     operation.TaskInfo{ID: "53", Status: corestatus.Pending},
				ReceiverName: "other-app/0",
			},
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), expectedTargetUnits, args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 3)
	c.Check(result.Units[0].IsLeader, tc.Equals, false) // Regular unit
	c.Check(result.Units[1].IsLeader, tc.Equals, true)  // Leader unit
	c.Check(result.Units[2].IsLeader, tc.Equals, false) // Regular unit
}

func (s *startSuite) TestStartActionOperationLeaderResolutionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{Unit: "test-app/0"},
		{LeaderUnit: "bad-app"},
	}
	args := operation.TaskArgs{
		ActionName: "test-action",
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("bad-app").Return("", errors.New("leadership error"))

	// Only the regular unit should be passed to state layer.
	expectedTargetUnits := []unit.Name{"test-app/0"}
	expectedResult := operation.RunResult{
		OperationID: "50",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo:     operation.TaskInfo{ID: "51", Status: corestatus.Pending},
				ReceiverName: "test-app/0",
			},
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), expectedTargetUnits, args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 2)

	// First unit should have state layer result.
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "51")
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)

	// Second unit should have error from leader resolution.
	c.Check(result.Units[1].ReceiverName, tc.Equals, unit.Name("bad-app/0"))
	c.Check(result.Units[1].IsLeader, tc.Equals, true)
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for bad-app:.*leadership error")
}

func (s *startSuite) TestStartActionOperationAllLeaderResolutionErrors(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{LeaderUnit: "bad-app-1"},
		{LeaderUnit: "bad-app-2"},
	}
	args := operation.TaskArgs{
		ActionName: "test-action",
	}

	s.mockLeadershipService.EXPECT().ApplicationLeader("bad-app-1").Return("", errors.New("error 1"))
	s.mockLeadershipService.EXPECT().ApplicationLeader("bad-app-2").Return("", errors.New("error 2"))

	// No units should be passed to state layer, so it should return early
	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 2)

	// Both units should have errors.
	c.Check(result.Units[0].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for bad-app-1:.*error 1")
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, "getting leader unit for bad-app-2:.*error 2")
}

func (s *startSuite) TestStartActionOperationStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{Unit: "test-app/0"},
	}
	args := operation.TaskArgs{ActionName: "backup"}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any(), args).Return(operation.RunResult{}, errors.New("action not found"))

	_, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.ErrorMatches, "adding action operation: action not found")
}

func (s *startSuite) TestStartActionOperationEmptyTarget(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{}
	args := operation.TaskArgs{ActionName: "backup"}

	// Should return early without calling state layer.
	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 0)
	c.Check(result.OperationID, tc.Equals, "")
}

func (s *startSuite) TestStartActionOperationConsolidationMismatch(c *tc.C) {
	defer s.setupMocks(c).Finish()

	target := []operation.ActionReceiver{
		{Unit: "test-app/0"},
		{Unit: "test-app/1"},
	}
	args := operation.TaskArgs{ActionName: "backup"}

	// State layer returns different unit than expected.
	expectedResult := operation.RunResult{
		OperationID: "50",
		Units: []operation.UnitTaskResult{
			{
				TaskInfo:     operation.TaskInfo{ID: "51", Status: corestatus.Pending},
				ReceiverName: "test-app/0",
			},
			// test-app/1 missing.
		},
	}

	s.state.EXPECT().AddActionOperation(gomock.Any(), gomock.Any(), gomock.Any(), args).Return(expectedResult, nil)

	result, err := s.service().AddActionOperation(c.Context(), target, args)
	c.Assert(err, tc.IsNil)
	c.Assert(result.Units, tc.HasLen, 2)
	c.Check(result.Units[0].ReceiverName, tc.Equals, unit.Name("test-app/0"))
	c.Check(result.Units[0].TaskInfo.ID, tc.Equals, "51")
	c.Check(result.Units[0].TaskInfo.Error, tc.IsNil)
	c.Check(result.Units[1].TaskInfo.Error, tc.ErrorMatches, "missing result for unit test-app/1")
}

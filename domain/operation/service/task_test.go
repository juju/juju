// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"io"
	"strings"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreerrors "github.com/juju/juju/core/errors"
	objectstoretesting "github.com/juju/juju/core/objectstore/testing"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	state                 *MockState
	clock                 clock.Clock
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
	mockLeadershipService *MockLeadershipService
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.clock = clock.WallClock
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockLeadershipService = NewMockLeadershipService(ctrl)
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter, s.mockLeadershipService)
}

func (s *serviceSuite) TestGetTaskSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedAction := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID: taskID,
		},
		Receiver: "test-app/0",
	}

	s.state.EXPECT().GetTask(gomock.Any(), taskID).Return(expectedAction, nil, nil)

	task, err := s.service().GetTask(c.Context(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(task.Receiver, tc.Equals, "test-app/0")
}

func (s *serviceSuite) TestGetTaskError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedError := errors.New("task not found")

	s.state.EXPECT().GetTask(gomock.Any(), gomock.Any()).Return(operation.Task{}, nil, expectedError)

	_, err := s.service().GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorMatches, `retrieving task ".*": task not found`)
}

func (s *serviceSuite) TestGetPendingTaskByTaskID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	result := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID:         taskID,
			ActionName: "fortune",
			Parameters: map[string]interface{}{
				"key": "value",
			},
			Status: corestatus.Pending,
		},
		Receiver: "test-app/0",
	}
	s.state.EXPECT().GetTask(gomock.Any(), taskID).Return(result, nil, nil)

	expectedTaskArgs := operation.TaskArgs{
		ActionName: "fortune",
		Parameters: map[string]interface{}{
			"key": "value",
		},
	}

	// Act
	task, err := s.service().GetPendingTaskByTaskID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(task, tc.DeepEquals, expectedTaskArgs)
}

func (s *serviceSuite) TestGetPendingTaskByTaskIDFailNotPending(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	result := operation.Task{
		TaskInfo: operation.TaskInfo{
			Status: corestatus.Running,
		},
	}
	s.state.EXPECT().GetTask(gomock.Any(), taskID).Return(result, nil, nil)

	// Act
	_, err := s.service().GetPendingTaskByTaskID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorIs, operationerrors.TaskNotPending)
}

func (s *serviceSuite) TestGetPendingTaskByTaskIDFail(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	result := operation.Task{
		TaskInfo: operation.TaskInfo{
			Status: corestatus.Running,
		},
	}
	s.state.EXPECT().GetTask(gomock.Any(), taskID).Return(result, nil, errors.New("boom"))

	// Act
	_, err := s.service().GetPendingTaskByTaskID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `getting pending task "42": boom`)
}

func (s *serviceSuite) TestCancelTaskSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedAction := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID: taskID,
		},
		Receiver: "test-app/0",
	}

	s.state.EXPECT().CancelTask(gomock.Any(), taskID).Return(expectedAction, nil)

	task, err := s.service().CancelTask(c.Context(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(task.Receiver, tc.Equals, "test-app/0")
}

func (s *serviceSuite) TestCancelTaskError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedError := errors.New("task not found")

	s.state.EXPECT().CancelTask(gomock.Any(), gomock.Any()).Return(operation.Task{}, expectedError)

	_, err := s.service().CancelTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorMatches, `cancelling task ".*": task not found`)
}

func (s *serviceSuite) TestGetTaskWithOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedTask := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID: taskID,
		},
		Receiver: "test-app/0",
	}

	outputPath := "task-output/test-output.json"
	outputJSON := `{"result": "success", "message": "Task completed successfully"}`

	s.state.EXPECT().GetTask(gomock.Any(), taskID).Return(expectedTask, &outputPath, nil)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	s.mockObjectStore.EXPECT().Get(gomock.Any(), outputPath).Return(
		io.NopCloser(strings.NewReader(outputJSON)), int64(len(outputJSON)), nil)

	task, err := s.service().GetTask(c.Context(), taskID)
	c.Assert(err, tc.IsNil)
	c.Check(task.Receiver, tc.Equals, "test-app/0")
	c.Assert(task.Output, tc.HasLen, 2)
	c.Check(task.Output["result"], tc.Equals, "success")
	c.Check(task.Output["message"], tc.Equals, "Task completed successfully")
}

func (s *serviceSuite) TestGetReceiverFromTaskID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	expectedReceiver := "app/0"
	s.state.EXPECT().GetReceiverFromTaskID(gomock.Any(), taskID).Return(expectedReceiver, nil)

	// Act
	receiver, err := s.service().GetReceiverFromTaskID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(receiver, tc.Equals, expectedReceiver)
}

func (s *serviceSuite) TestGetReceiverFromTaskIDFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	s.state.EXPECT().GetReceiverFromTaskID(gomock.Any(), taskID).Return("", errors.New("task start fail"))

	// Act
	_, err := s.service().GetReceiverFromTaskID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `task start fail`)
}

func (s *serviceSuite) TestStartTask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	s.state.EXPECT().StartTask(gomock.Any(), taskID).Return(nil)

	// Act
	err := s.service().StartTask(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *serviceSuite) TestStartTaskFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	s.state.EXPECT().StartTask(gomock.Any(), taskID).Return(errors.New("task start fail"))

	// Act
	err := s.service().StartTask(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `task start fail`)
}

func (s *serviceSuite) TestFinishTask(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	taskUUID := uuid.MustNewUUID().String()
	s.state.EXPECT().GetTaskUUIDByID(gomock.Any(), taskID).Return(taskUUID, nil)

	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	storeUUID := objectstoretesting.GenObjectStoreUUID(c)
	inputJSON := `{"foo":"bar"}`
	reader := strings.NewReader(inputJSON)
	s.mockObjectStore.EXPECT().Put(gomock.Any(), taskUUID, reader, int64(len(inputJSON))).Return(storeUUID, nil)

	input := internal.CompletedTask{
		TaskUUID:  taskUUID,
		StoreUUID: storeUUID.String(),
		Status:    corestatus.Completed.String(),
		Message:   "done",
	}
	s.state.EXPECT().FinishTask(gomock.Any(), input).Return(nil)

	arg := operation.CompletedTaskResult{
		TaskID:  taskID,
		Message: "done",
		Status:  corestatus.Completed.String(),
		Results: map[string]interface{}{"foo": "bar"},
	}

	// Act
	err := s.service().FinishTask(c.Context(), arg)
	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *serviceSuite) TestLogTaskMessage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	msg := "log message"
	s.state.EXPECT().LogTaskMessage(gomock.Any(), taskID, msg).Return(nil)

	// Act
	err := s.service().LogTaskMessage(c.Context(), taskID, msg)

	// Assert
	c.Assert(err, tc.IsNil)
}

func (s *serviceSuite) TestGetTaskStatusByID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	expectedStatus := corestatus.Aborting.String()
	s.state.EXPECT().GetTaskStatusByID(gomock.Any(), taskID).Return(expectedStatus, nil)

	// Act
	status, err := s.service().GetTaskStatusByID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(status, tc.Equals, expectedStatus)
}

func (s *serviceSuite) TestGetTaskStatusByIDFails(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	s.state.EXPECT().GetTaskStatusByID(gomock.Any(), taskID).Return("", errors.New("task start fail"))

	// Act
	_, err := s.service().GetTaskStatusByID(c.Context(), taskID)

	// Assert
	c.Assert(err, tc.ErrorMatches, `retrieving task status "42": task start fail`)
}

func (s *serviceSuite) TestFinishTaskError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"

	s.state.EXPECT().GetTaskUUIDByID(gomock.Any(), taskID).Return("", errors.New("boom"))

	arg := operation.CompletedTaskResult{
		TaskID:  taskID,
		Message: "done",
		Status:  corestatus.Completed.String(),
		Results: map[string]interface{}{"foo": "bar"},
	}

	// Act
	err := s.service().FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestFinishTaskInputStatusNotValid(c *tc.C) {
	// Arrange
	arg := operation.CompletedTaskResult{
		TaskID:  "42",
		Message: "done",
		Status:  corestatus.Pending.String(),
		Results: map[string]interface{}{"foo": "bar"},
	}

	// Act
	err := s.service().FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestFinishTaskInputIDNotValid(c *tc.C) {
	// Arrange
	arg := operation.CompletedTaskResult{}

	// Act
	err := s.service().FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestFinishTaskFailStorePut(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	taskUUID := uuid.MustNewUUID().String()
	s.state.EXPECT().GetTaskUUIDByID(gomock.Any(), taskID).Return(taskUUID, nil)

	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	inputJSON := `{"foo":"bar"}`
	s.mockObjectStore.EXPECT().Put(gomock.Any(), taskUUID, gomock.Any(), int64(len(inputJSON))).Return("", errors.New("store put error"))

	arg := operation.CompletedTaskResult{
		TaskID:  taskID,
		Message: "done",
		Status:  corestatus.Completed.String(),
		Results: map[string]interface{}{"foo": "bar"},
	}

	// Act
	err := s.service().FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, "putting task result \"42\" in store: failed to store results: store put error")
}

func (s *serviceSuite) TestFinishTaskFailState(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	taskUUID := uuid.MustNewUUID().String()
	s.state.EXPECT().GetTaskUUIDByID(gomock.Any(), taskID).Return(taskUUID, nil)

	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil)
	storeUUID := objectstoretesting.GenObjectStoreUUID(c)
	inputJSON := `{"foo":"bar"}`
	reader := strings.NewReader(inputJSON)
	s.mockObjectStore.EXPECT().Put(gomock.Any(), taskUUID, reader, int64(len(inputJSON))).Return(storeUUID, nil)

	input := internal.CompletedTask{
		TaskUUID:  taskUUID,
		StoreUUID: storeUUID.String(),
		Status:    corestatus.Completed.String(),
		Message:   "done",
	}
	s.state.EXPECT().FinishTask(gomock.Any(), input).Return(errors.New("boom"))

	s.mockObjectStore.EXPECT().Remove(c.Context(), taskUUID).Return(nil)

	arg := operation.CompletedTaskResult{
		TaskID:  taskID,
		Message: "done",
		Status:  corestatus.Completed.String(),
		Results: map[string]interface{}{"foo": "bar"},
	}

	// Act
	err := s.service().FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *serviceSuite) TestFinishTaskNoStore(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	taskID := "42"
	taskUUID := uuid.MustNewUUID().String()
	s.state.EXPECT().GetTaskUUIDByID(gomock.Any(), taskID).Return(taskUUID, nil)

	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(nil, errors.Errorf("boom"))

	arg := operation.CompletedTaskResult{
		TaskID:  taskID,
		Message: "done",
		Status:  corestatus.Completed.String(),
		Results: map[string]interface{}{"foo": "bar"},
	}

	// Act
	err := s.service().FinishTask(c.Context(), arg)

	// Assert
	c.Assert(err, tc.ErrorMatches, "putting task result \"42\" in store: getting object store: boom")
}

func (s *serviceSuite) TestLogTaskMessageError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Assert
	taskID := "42"
	msg := "log message"
	expectedError := errors.New("boom")
	s.state.EXPECT().LogTaskMessage(gomock.Any(), taskID, msg).Return(expectedError)

	// Act
	err := s.service().LogTaskMessage(c.Context(), taskID, msg)

	// Assert
	c.Assert(err, tc.ErrorMatches, `boom`)
}

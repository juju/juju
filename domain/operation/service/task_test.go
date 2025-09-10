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

	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type serviceSuite struct {
	state                 *MockState
	clock                 clock.Clock
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
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
	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter)
}

func (s *serviceSuite) TestGetTaskSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedAction := operation.TaskInfo{
		ID:       taskID,
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

	s.state.EXPECT().GetTask(gomock.Any(), gomock.Any()).Return(operation.TaskInfo{}, nil, expectedError)

	_, err := s.service().GetTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorMatches, `retrieving task ".*": task not found`)
}

func (s *serviceSuite) TestCancelTaskSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedAction := operation.TaskInfo{
		ID:       taskID,
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

	s.state.EXPECT().CancelTask(gomock.Any(), gomock.Any()).Return(operation.TaskInfo{}, expectedError)

	_, err := s.service().CancelTask(c.Context(), taskID)
	c.Assert(err, tc.ErrorMatches, `cancelling task ".*": task not found`)
}

func (s *serviceSuite) TestGetTaskWithOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	taskID := "42"
	expectedAction := operation.TaskInfo{
		ID:       taskID,
		Receiver: "test-app/0",
	}

	outputPath := "task-output/test-output.json"
	outputJSON := `{"result": "success", "message": "Task completed successfully"}`

	s.state.EXPECT().GetTask(gomock.Any(), taskID).Return(expectedAction, &outputPath, nil)
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

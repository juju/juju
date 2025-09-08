// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreoperation "github.com/juju/juju/core/operation"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ModelObjectStoreGetter,ObjectStore

type serviceSuite struct {
	state                 *MockState
	clock                 clock.Clock
	mockObjectStoreGetter *MockModelObjectStoreGetter
	mockObjectStore       *MockObjectStore
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) SetUpTest(c *tc.C) {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.clock = clock.WallClock
	s.mockObjectStore = NewMockObjectStore(ctrl)
	s.mockObjectStoreGetter = NewMockModelObjectStoreGetter(ctrl)
	s.mockObjectStoreGetter.EXPECT().GetObjectStore(gomock.Any()).Return(s.mockObjectStore, nil).AnyTimes()
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil), s.mockObjectStoreGetter)
}

func (s *serviceSuite) TestGetActionSuccess(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	operationID := coreoperation.ID("42")
	expectedAction := operation.Action{
		OperationID: operationID,
		UUID:        actionUUID,
		Receiver:    "test-app/0",
	}

	s.state.EXPECT().GetAction(gomock.Any(), operationID.String()).Return(expectedAction, "", nil)

	action, err := s.service().GetAction(context.Background(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, actionUUID)
	c.Check(action.Receiver, tc.Equals, "test-app/0")
}

func (s *serviceSuite) TestGetActionError(c *tc.C) {
	operationID := coreoperation.ID("42")
	expectedError := errors.New("action not found")

	s.state.EXPECT().GetAction(gomock.Any(), operationID.String()).Return(operation.Action{}, "", expectedError)

	_, err := s.service().GetAction(context.Background(), operationID)
	c.Assert(err, tc.ErrorMatches, `retrieving action ".*": action not found`)
}

func (s *serviceSuite) TestCancelActionSuccess(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	operationID := coreoperation.ID("42")
	expectedAction := operation.Action{
		OperationID: operationID,
		UUID:        actionUUID,
		Receiver:    "test-app/0",
	}

	s.state.EXPECT().CancelAction(gomock.Any(), operationID.String()).Return(expectedAction, nil)

	action, err := s.service().CancelAction(context.Background(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, actionUUID)
	c.Check(action.Receiver, tc.Equals, "test-app/0")
}

func (s *serviceSuite) TestCancelActionError(c *tc.C) {
	operationID := coreoperation.ID("42")
	expectedError := errors.New("action not found")

	s.state.EXPECT().CancelAction(gomock.Any(), operationID.String()).Return(operation.Action{}, expectedError)

	_, err := s.service().CancelAction(context.Background(), operationID)
	c.Assert(err, tc.ErrorMatches, `cancelling action ".*": action not found`)
}

func (s *serviceSuite) TestGetActionWithOutput(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	operationID := coreoperation.ID("42")
	expectedAction := operation.Action{
		OperationID: operationID,
		UUID:        actionUUID,
		Receiver:    "test-app/0",
	}

	outputPath := "task-output/test-output.json"
	outputJSON := `{"result": "success", "message": "Task completed successfully"}`

	s.state.EXPECT().GetAction(gomock.Any(), operationID.String()).Return(expectedAction, outputPath, nil)
	s.mockObjectStore.EXPECT().Get(gomock.Any(), outputPath).Return(
		io.NopCloser(strings.NewReader(outputJSON)), int64(len(outputJSON)), nil)

	action, err := s.service().GetAction(context.Background(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, actionUUID)
	c.Check(action.Receiver, tc.Equals, "test-app/0")
	c.Assert(action.Output, tc.HasLen, 2)
	c.Check(action.Output["result"], tc.Equals, "success")
	c.Check(action.Output["message"], tc.Equals, "Task completed successfully")
}

func (s *serviceSuite) TestGetActionWithEmptyOutput(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	operationID := coreoperation.ID("42")
	expectedAction := operation.Action{
		OperationID: operationID,
		UUID:        actionUUID,
		Receiver:    "test-app/0",
	}

	s.state.EXPECT().GetAction(gomock.Any(), operationID.String()).Return(expectedAction, "", nil)

	action, err := s.service().GetAction(context.Background(), operationID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, actionUUID)
	c.Check(action.Receiver, tc.Equals, "test-app/0")
	c.Check(action.Output, tc.HasLen, 0)
}

// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package service -destination service_mock_test.go github.com/juju/juju/domain/operation/service State

type serviceSuite struct {
	state *MockState
	clock clock.Clock
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) SetUpTest(c *tc.C) {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.clock = clock.WallClock
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state, s.clock, loggertesting.WrapCheckLog(nil))
}

func (s *serviceSuite) TestGetActionSuccess(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	expectedAction := operation.Action{
		UUID:     actionUUID,
		Receiver: "test-app/0",
	}

	s.state.EXPECT().GetAction(gomock.Any(), actionUUID.String()).Return(expectedAction, nil)

	action, err := s.service().GetAction(context.Background(), actionUUID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, actionUUID)
	c.Check(action.Receiver, tc.Equals, "test-app/0")
}

func (s *serviceSuite) TestGetActionError(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	expectedError := errors.New("action not found")

	s.state.EXPECT().GetAction(gomock.Any(), actionUUID.String()).Return(operation.Action{}, expectedError)

	_, err := s.service().GetAction(context.Background(), actionUUID)
	c.Assert(err, tc.ErrorMatches, `retrieving action ".*": action not found`)
}

func (s *serviceSuite) TestCancelActionSuccess(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	expectedAction := operation.Action{
		UUID:     actionUUID,
		Receiver: "test-app/0",
	}

	s.state.EXPECT().CancelAction(gomock.Any(), actionUUID.String()).Return(expectedAction, nil)

	action, err := s.service().CancelAction(context.Background(), actionUUID)
	c.Assert(err, tc.IsNil)
	c.Check(action.UUID, tc.Equals, actionUUID)
	c.Check(action.Receiver, tc.Equals, "test-app/0")
}

func (s *serviceSuite) TestCancelActionError(c *tc.C) {
	actionUUID := uuid.MustNewUUID()
	expectedError := errors.New("action not found")

	s.state.EXPECT().CancelAction(gomock.Any(), actionUUID.String()).Return(operation.Action{}, expectedError)

	_, err := s.service().CancelAction(context.Background(), actionUUID)
	c.Assert(err, tc.ErrorMatches, `cancelling action ".*": action not found`)
}

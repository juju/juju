// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct {
	authTag          names.MachineTag
	operationService *MockOperationService
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) SetUpTest(c *tc.C) {
	s.authTag = names.NewMachineTag("5")
}

func (*facadeSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:

- Test returns the running actions for the machine agent`)
}

func (s *facadeSuite) TestBeginActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return(s.authTag.Id(), nil)
	s.operationService.EXPECT().StartTask(gomock.Any(), "42").Return(nil)
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "47").Return(s.authTag.Id(), nil)
	s.operationService.EXPECT().StartTask(gomock.Any(), "47").Return(nil)

	// Act
	results := s.getFacade().BeginActions(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: "action-42"},
		{Tag: "action-47"},
	}})

	// Assert
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})
}

func (s *facadeSuite) TestFinishActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return(s.authTag.Id(), nil)
	taskOne := operation.CompletedTaskResult{
		TaskID: "42",
		Status: status.Completed.String(),
	}
	s.operationService.EXPECT().FinishTask(gomock.Any(), taskOne).Return(nil)
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "47").Return(s.authTag.Id(), nil)
	taskTwo := operation.CompletedTaskResult{
		TaskID: "47",
		Status: status.Cancelled.String(),
	}
	s.operationService.EXPECT().FinishTask(gomock.Any(), taskTwo).Return(nil)

	// Act
	results := s.getFacade().FinishActions(c.Context(), params.ActionExecutionResults{Results: []params.ActionExecutionResult{
		{ActionTag: "action-42", Status: status.Completed.String()},
		{ActionTag: "action-47", Status: status.Cancelled.String()},
	}})

	// Assert
	c.Assert(results, tc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})
}

func (s *facadeSuite) TestAuthTaskID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return(s.authTag.Id(), nil)

	// Act
	id, err := s.getFacade().authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(id, tc.Equals, "42")
}

func (s *facadeSuite) TestAuthTaskIDWrongMachineErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return("16", nil)

	// Act
	_, err := s.getFacade().authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *facadeSuite) TestAuthTaskIDUnitErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return("ubuntu/7", nil)

	// Act
	_, err := s.getFacade().authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *facadeSuite) TestActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	tagOne := names.NewActionTag("42")
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), tagOne.Id()).Return(s.authTag.Id(), nil)
	taskOne := operation.TaskArgs{
		ActionName: "one",
		Parameters: map[string]interface{}{"foo": "bar"},
	}
	s.operationService.EXPECT().GetPendingTaskByTaskID(gomock.Any(), tagOne.Id()).Return(taskOne, nil)

	tagTwo := names.NewActionTag("47")
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), tagTwo.Id()).Return(s.authTag.Id(), nil)
	taskTwo := operation.TaskArgs{
		ActionName: "two",
		Parameters: map[string]interface{}{"baz": "bar"},
	}
	s.operationService.EXPECT().GetPendingTaskByTaskID(gomock.Any(), tagTwo.Id()).Return(taskTwo, nil)

	// Act
	results := s.getFacade().Actions(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: tagOne.String()},
		{Tag: tagTwo.String()},
	}})

	// Assert
	c.Assert(results.Results, tc.HasLen, 2)
	c.Check(results.Results[0].Action, tc.DeepEquals, &params.Action{
		Name:       "one",
		Parameters: map[string]interface{}{"foo": "bar"},
	})
	c.Check(results.Results[1].Action, tc.DeepEquals, &params.Action{
		Name:       "two",
		Parameters: map[string]interface{}{"baz": "bar"},
	})
}

func (s *facadeSuite) TestActionsTaskNotPending(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	tagOne := names.NewActionTag("42")
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), tagOne.Id()).Return(s.authTag.Id(), nil)

	s.operationService.EXPECT().GetPendingTaskByTaskID(gomock.Any(), tagOne.Id()).Return(operation.TaskArgs{}, operationerrors.TaskNotPending)

	// Act
	results := s.getFacade().Actions(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: tagOne.String()},
	}})

	// Assert
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "action no longer available")
}

func (s *facadeSuite) getFacade() *Facade {
	return &Facade{
		operationService: s.operationService,
		accessMachine: func(tag names.Tag) bool {
			if tag.Id() == s.authTag.Id() {
				return true
			}
			return false
		},
	}
}

func (s *facadeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.operationService = NewMockOperationService(ctrl)
	c.Cleanup(func() {
		s.operationService = nil
	})
	return ctrl
}

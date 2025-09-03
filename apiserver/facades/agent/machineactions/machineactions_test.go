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
	results := s.getFacade(c).BeginActions(c.Context(), params.Entities{Entities: []params.Entity{
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
	results := s.getFacade(c).FinishActions(c.Context(), params.ActionExecutionResults{Results: []params.ActionExecutionResult{
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
	id, err := s.getFacade(c).authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(id, tc.Equals, "42")
}

func (s *facadeSuite) TestAuthTaskIDWrongMachineErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return("16", nil)

	// Act
	_, err := s.getFacade(c).authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *facadeSuite) TestAuthTaskIDUnitErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().ReceiverFromTask(gomock.Any(), "42").Return("ubuntu/7", nil)

	// Act
	_, err := s.getFacade(c).authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *facadeSuite) getFacade(c *tc.C) *Facade {
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

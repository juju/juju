// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiServerErrors "github.com/juju/juju/apiserver/errors"
	coremachine "github.com/juju/juju/core/machine"
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

func (s *facadeSuite) TestBeginActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "42").Return(s.authTag.Id(), nil)
	s.operationService.EXPECT().StartTask(gomock.Any(), "42").Return(nil)
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "47").Return(s.authTag.Id(), nil)
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
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "42").Return(s.authTag.Id(), nil)
	taskOne := operation.CompletedTaskResult{
		TaskID: "42",
		Status: status.Completed.String(),
	}
	s.operationService.EXPECT().FinishTask(gomock.Any(), taskOne).Return(nil)
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "47").Return(s.authTag.Id(), nil)
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
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "42").Return(s.authTag.Id(), nil)

	// Act
	id, err := s.getFacade().authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.IsNil)
	c.Assert(id, tc.Equals, "42")
}

func (s *facadeSuite) TestAuthTaskIDWrongMachineErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "42").Return("16", nil)

	// Act
	_, err := s.getFacade().authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *facadeSuite) TestAuthTaskIDUnitErrPerm(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), "42").Return("ubuntu/7", nil)

	// Act
	_, err := s.getFacade().authTaskID(c.Context(), "action-42")

	// Assert
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *facadeSuite) TestActions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	tagOne := names.NewActionTag("42")
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), tagOne.Id()).Return(s.authTag.Id(), nil)
	taskOne := operation.TaskArgs{
		ActionName: "one",
		Parameters: map[string]interface{}{"foo": "bar"},
	}
	s.operationService.EXPECT().GetPendingTaskByTaskID(gomock.Any(), tagOne.Id()).Return(taskOne, nil)

	tagTwo := names.NewActionTag("47")
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), tagTwo.Id()).Return(s.authTag.Id(), nil)
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
	s.operationService.EXPECT().GetReceiverFromTaskID(gomock.Any(), tagOne.Id()).Return(s.authTag.Id(), nil)

	s.operationService.EXPECT().GetPendingTaskByTaskID(gomock.Any(), tagOne.Id()).Return(operation.TaskArgs{}, operationerrors.TaskNotPending)

	// Act
	results := s.getFacade().Actions(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: tagOne.String()},
	}})

	// Assert
	c.Assert(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.ErrorMatches, "action no longer available")
}

func (s *facadeSuite) TestRunningActionsReturnsActionIDsForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	machineTag := names.NewMachineTag(s.authTag.Id())
	s.operationService.EXPECT().GetMachineTaskIDsWithStatus(
		gomock.Any(), coremachine.Name(s.authTag.Id()), status.Running,
	).Return([]string{"42", "47"}, nil)

	// Act
	results := s.getFacade().RunningActions(c.Context(), params.Entities{Entities: []params.Entity{{Tag: machineTag.String()}}})

	// Assert
	c.Assert(results.Actions, tc.HasLen, 1)
	c.Check(results.Actions[0].Receiver, tc.Equals, machineTag.String())
	c.Check(results.Actions[0].Error, tc.IsNil)
	c.Check(results.Actions[0].Actions, tc.HasLen, 2)
	c.Check(results.Actions[0].Actions[0].Action.Tag, tc.Equals, names.NewActionTag("42").String())
	c.Check(results.Actions[0].Actions[1].Action.Tag, tc.Equals, names.NewActionTag("47").String())
}

func (s *facadeSuite) TestRunningActionsReturnsActionIDsForMachineEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Act
	results := s.getFacade().RunningActions(c.Context(), params.Entities{Entities: []params.Entity{}})

	// Assert
	c.Assert(results.Actions, tc.HasLen, 0)
}

func (s *facadeSuite) TestRunningActionsReturnsActionIDsForMachineSeveralEntitiesWithUnexpectedTags(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.operationService.EXPECT().GetMachineTaskIDsWithStatus(
		gomock.Any(), coremachine.Name(s.authTag.Id()), status.Running,
	).Return([]string{"42", "47"}, nil)

	// Act
	results := s.getFacade().RunningActions(c.Context(), params.Entities{Entities: []params.Entity{
		{
			Tag: names.NewMachineTag("1/lxd/0").String(),
		},
		{
			Tag: s.authTag.String(),
		},
		{
			Tag: names.NewUnitTag("app/6").String(),
		},
	}})

	// Assert
	c.Assert(results.Actions, tc.HasLen, 3)
	c.Check(results.Actions[0].Receiver, tc.Equals, "machine-1-lxd-0")
	c.Check(results.Actions[0].Error, tc.ErrorMatches, apiServerErrors.ErrPerm.Error())
	c.Check(results.Actions[1].Receiver, tc.Equals, s.authTag.String())
	c.Check(results.Actions[1].Error, tc.IsNil)
	c.Check(results.Actions[1].Actions, tc.HasLen, 2)
	c.Check(results.Actions[1].Actions[0].Action.Tag, tc.Equals, names.NewActionTag("42").String())
	c.Check(results.Actions[1].Actions[1].Action.Tag, tc.Equals, names.NewActionTag("47").String())
	c.Check(results.Actions[2].Error, tc.ErrorMatches, apiServerErrors.ErrBadId.Error())
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

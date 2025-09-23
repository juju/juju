// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/leadership"
	modeltesting "github.com/juju/juju/core/model/testing"
	corestatus "github.com/juju/juju/core/status"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	operation "github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

func TestActionSuite(t *testing.T) {
	tc.Run(t, &actionSuite{})
}

func TestActionWatchSuite(t *testing.T) {
	tc.Run(t, &actionWatcherSuite{})
}

type actionSuite struct {
	applicationService *MockApplicationService
	operationService   *MockOperationService

	adminTag names.UserTag
	client   *ActionAPI
}

func (s *actionSuite) SetUpTest(c *tc.C) {
	s.adminTag = names.NewUserTag("admin")
}

func (s *actionSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = NewMockApplicationService(ctrl)
	s.operationService = NewMockOperationService(ctrl)

	c.Cleanup(func() {
		s.applicationService = nil
		s.operationService = nil
	})

	return ctrl
}

func (s *actionSuite) setupAPI(c *tc.C, authTag names.UserTag) {
	var err error

	auth := apiservertesting.FakeAuthorizer{
		Tag:      authTag,
		AdminTag: s.adminTag,
	}
	modelUUID := modeltesting.GenModelUUID(c)

	leadershipFunc := func() (leadership.Reader, error) {
		return FakeLeadership{}, nil
	}
	s.client, err = newActionAPI(
		auth,
		leadershipFunc,
		s.applicationService,
		nil,
		nil,
		s.operationService,
		modelUUID,
		nil,
	)
	c.Assert(err, tc.ErrorIsNil)

	c.Cleanup(func() {
		s.client = nil
	})
}
func (s *actionSuite) TestActionsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	resAction := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID:             "42",
			ActionName:     "charm-action-0",
			ExecutionGroup: ptr("group-0"),
			IsParallel:     true,
			Parameters: map[string]any{
				"arg-0": "value-0",
			},
			Status: corestatus.Completed,
		},
		Receiver: "app/0", // Unit receiver.
	}
	taskID := "42"

	s.operationService.EXPECT().GetTask(
		gomock.Any(),
		taskID,
	).Return(resAction, nil)

	result, err := s.client.Actions(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Check(result.Results[0].Action.Tag, tc.Equals, "42")
	c.Check(result.Results[0].Action.Receiver, tc.Equals, "unit-app-0") // ActionReceiverTag applies the conversion.
	c.Check(result.Results[0].Action.Name, tc.Equals, "charm-action-0")
	c.Check(*result.Results[0].Action.ExecutionGroup, tc.Equals, "group-0")
}

func (s *actionSuite) TestActionsPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Use a non-admin user tag to fail permission check.
	nonAdminUser := names.NewUserTag("unauthorized")
	s.setupAPI(c, nonAdminUser)
	taskID := "42"

	_, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *actionSuite) TestActionsInvalidActionTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "invalid-tag"},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestActionsActionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	taskID := "42"

	s.operationService.EXPECT().GetTask(
		gomock.Any(),
		taskID,
	).Return(operation.Task{}, operationerrors.TaskNotFound)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestActionsServerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	taskID := "42"

	s.operationService.EXPECT().GetTask(
		gomock.Any(),
		taskID,
	).Return(operation.Task{}, errors.New("boom"))

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	// This error was already (wrongly) black-holed into a ErrBadId.
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestActionsMultipleEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	taskID0 := "42"
	taskID1 := "43"
	resAction0 := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID: "42",
		},
		Receiver: "3", // Machine receiver.
	}

	s.operationService.EXPECT().GetTask(
		gomock.Any(),
		taskID0,
	).Return(resAction0, nil)
	s.operationService.EXPECT().GetTask(
		gomock.Any(),
		taskID1,
	).Return(operation.Task{}, operationerrors.TaskNotFound)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID0).String()},
			{Tag: names.NewActionTag(taskID1).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 2)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[1].Error, tc.NotNil)
	c.Assert(result.Results[1].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestActionsEmptyEntityList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func (s *actionSuite) TestCancelSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	cancelledAction := operation.Task{
		TaskInfo: operation.TaskInfo{
			ID: "42",
		},
		Receiver: "app/0", // Unit receiver.
	}
	taskID := "42"

	s.operationService.EXPECT().CancelTask(
		gomock.Any(),
		taskID,
	).Return(cancelledAction, nil)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Results, tc.HasLen, 1)
	c.Check(result.Results[0].Error, tc.IsNil)
	c.Check(result.Results[0].Action.Tag, tc.Equals, "42")
	c.Check(result.Results[0].Action.Receiver, tc.Equals, "unit-app-0") // ActionReceiverTag applies the conversion.
}

func (s *actionSuite) TestCancelPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Use a non-admin user tag to fail permission check.
	nonAdminUser := names.NewUserTag("unauthorized")
	s.setupAPI(c, nonAdminUser)
	taskID := "42"

	_, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *actionSuite) TestCancelInvalidActionTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "invalid-tag"},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestCancelActionNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	taskID := "42"

	s.operationService.EXPECT().CancelTask(
		gomock.Any(),
		taskID,
	).Return(operation.Task{}, operationerrors.TaskNotFound)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestCancelServerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	taskID := "42"

	s.operationService.EXPECT().CancelTask(
		gomock.Any(),
		taskID,
	).Return(operation.Task{}, errors.New("boom"))

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(taskID).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error.Message, tc.Equals, "boom")
}

func (s *actionSuite) TestCancelEmptyEntityList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func (s *actionSuite) TestApplicationsCharmsActionsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	appName := "postgresql"
	locator := applicationcharm.CharmLocator{
		Name:     "postgresql",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}
	actions := internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"backup": {
				Description: "Create a backup",
				Params: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"target": map[string]interface{}{
							"type": "string",
						},
					},
				},
			},
			"restore": {
				Description: "Restore from backup",
				Params: map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(
		gomock.Any(),
		appName,
	).Return(locator, nil)
	s.applicationService.EXPECT().GetCharmActions(
		gomock.Any(),
		locator,
	).Return(actions, nil)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(appName).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].ApplicationTag, tc.Equals, names.NewApplicationTag(appName).String())
	c.Assert(result.Results[0].Actions, tc.HasLen, 2)
	c.Assert(result.Results[0].Actions["backup"].Description, tc.Equals, "Create a backup")
	c.Assert(result.Results[0].Actions["restore"].Description, tc.Equals, "Restore from backup")
}

func (s *actionSuite) TestApplicationsCharmsActionsPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Use a non-admin user tag to fail permission check.
	nonAdminUser := names.NewUserTag("unauthorized")
	s.setupAPI(c, nonAdminUser)

	_, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag("postgresql").String()},
		},
	})

	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *actionSuite) TestApplicationsCharmsActionsInvalidApplicationTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: "invalid-tag"},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestApplicationsCharmsActionsApplicationNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	appName := "postgresql"

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(
		gomock.Any(),
		appName,
	).Return(applicationcharm.CharmLocator{}, applicationerrors.ApplicationNotFound)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(appName).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestApplicationsCharmsActionsCharmNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	appName := "postgresql"
	locator := applicationcharm.CharmLocator{
		Name:     "postgresql",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(
		gomock.Any(),
		appName,
	).Return(locator, nil)
	s.applicationService.EXPECT().GetCharmActions(
		gomock.Any(),
		locator,
	).Return(internalcharm.Actions{}, applicationerrors.CharmNotFound)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(appName).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestApplicationsCharmsActionsServerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	appName := "postgresql"

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(
		gomock.Any(),
		appName,
	).Return(applicationcharm.CharmLocator{}, errors.New("boom"))

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(appName).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error.Message, tc.Equals, "boom")
}

func (s *actionSuite) TestApplicationsCharmsActionsMultipleEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)
	appName1 := "postgresql"
	appName2 := "mysql"
	locator1 := applicationcharm.CharmLocator{
		Name:     "postgresql",
		Revision: 42,
		Source:   applicationcharm.CharmHubSource,
	}
	actions1 := internalcharm.Actions{
		ActionSpecs: map[string]internalcharm.ActionSpec{
			"backup": {
				Description: "Create a backup",
				Params: map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(
		gomock.Any(),
		appName1,
	).Return(locator1, nil)
	s.applicationService.EXPECT().GetCharmActions(
		gomock.Any(),
		locator1,
	).Return(actions1, nil)
	s.applicationService.EXPECT().GetCharmLocatorByApplicationName(
		gomock.Any(),
		appName2,
	).Return(applicationcharm.CharmLocator{}, applicationerrors.ApplicationNotFound)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewApplicationTag(appName1).String()},
			{Tag: names.NewApplicationTag(appName2).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 2)
	c.Assert(result.Results[0].Error, tc.IsNil)
	c.Assert(result.Results[0].ApplicationTag, tc.Equals, names.NewApplicationTag(appName1).String())
	c.Assert(result.Results[0].Actions, tc.HasLen, 1)
	c.Assert(result.Results[1].Error, tc.NotNil)
	c.Assert(result.Results[1].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestApplicationsCharmsActionsEmptyEntityList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, s.adminTag)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

type actionWatcherSuite struct {
	MockBaseSuite

	watcher *MockStringsWatcher
}

func (s *actionWatcherSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.MockBaseSuite.setupMocks(c)
	s.watcher = NewMockStringsWatcher(ctrl)

	c.Cleanup(func() { s.watcher = nil })

	return ctrl
}

func (s *actionWatcherSuite) TestWatchLogProgress(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	// First action
	chOne := make(chan []string, 1)
	watcherMsgOne := "test task log"
	chOne <- []string{
		watcherMsgOne,
	}
	s.watcher.EXPECT().Changes().Return(chOne)

	actionTagOne := names.NewActionTag("7")
	s.OperationService.EXPECT().WatchTaskLogs(gomock.Any(), actionTagOne.Id()).Return(s.watcher, nil)
	watcherIDOne := "42"
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return(watcherIDOne, nil)

	// Second action
	chTwo := make(chan []string, 1)
	watcherMsgTwo := "second test task log"
	chTwo <- []string{
		watcherMsgTwo,
	}
	s.watcher.EXPECT().Changes().Return(chTwo)

	actionTagTwo := names.NewActionTag("7")
	s.OperationService.EXPECT().WatchTaskLogs(gomock.Any(), actionTagTwo.Id()).Return(s.watcher, nil)
	watcherIDTwo := "42"
	s.watcherRegistry.EXPECT().Register(gomock.Any(), gomock.Any()).Return(watcherIDTwo, nil)

	// Act
	results, err := s.newActionAPI(c).WatchActionsProgress(c.Context(), params.Entities{Entities: []params.Entity{
		{Tag: actionTagOne.String()},
		{Tag: actionTagTwo.String()},
	}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.StringsWatchResults{
		Results: []params.StringsWatchResult{
			{StringsWatcherId: watcherIDOne, Changes: []string{watcherMsgOne}},
			{StringsWatcherId: watcherIDTwo, Changes: []string{watcherMsgTwo}},
		},
	})
}

func (s *actionWatcherSuite) TestWatchLogProgressNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	actionTag := names.NewActionTag("7")
	s.OperationService.EXPECT().WatchTaskLogs(gomock.Any(), actionTag.Id()).Return(nil, operationerrors.TaskNotFound)
	api := s.newActionAPI(c)

	// Act
	results, err := api.WatchActionsProgress(c.Context(), params.Entities{Entities: []params.Entity{{Tag: actionTag.String()}}})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, fmt.Sprintf("action %q not found", actionTag.Id()))
}

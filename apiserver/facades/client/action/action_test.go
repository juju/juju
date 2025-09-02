// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/action"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coreaction "github.com/juju/juju/core/action"
	modeltesting "github.com/juju/juju/core/model/testing"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	operation "github.com/juju/juju/domain/operation"
	operationerrors "github.com/juju/juju/domain/operation/errors"
	internalcharm "github.com/juju/juju/internal/charm"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

type actionSuite struct {
	jujutesting.ApiServerSuite

	applicationService *action.MockApplicationService
	operationService   *action.MockOperationService

	modelTag names.ModelTag
	client   *action.ActionAPI
}

func (s *actionSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.applicationService = action.NewMockApplicationService(ctrl)
	s.operationService = action.NewMockOperationService(ctrl)

	return ctrl
}

func (s *actionSuite) setupAPI(c *tc.C, authTag names.Tag) {
	var err error
	auth := apiservertesting.FakeAuthorizer{
		Tag:      authTag,
		AdminTag: jujutesting.AdminUser,
	}
	modelUUID := modeltesting.GenModelUUID(c)
	s.modelTag = names.NewModelTag(modelUUID.String())
	s.client, err = action.NewActionAPI(auth, action.FakeLeadership{}, s.applicationService, nil, nil, s.operationService, modelUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func TestActionSuite(t *testing.T) {
	tc.Run(t, &actionSuite{})
}

func (s *actionSuite) TestActionsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID := modeltesting.GenModelUUID(c)
	resAction := operation.Action{}

	s.operationService.EXPECT().GetAction(
		gomock.Any(),
		coreaction.UUID(actionUUID.String()),
	).Return(resAction, nil)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *actionSuite) TestActionsPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	actionUUID := modeltesting.GenModelUUID(c)
	// Use a non-admin user tag to fail permission check.
	nonAdminUser := names.NewUserTag("unauthorized")
	s.setupAPI(c, nonAdminUser)

	_, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *actionSuite) TestActionsInvalidActionTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)

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

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID := modeltesting.GenModelUUID(c)

	s.operationService.EXPECT().GetAction(
		gomock.Any(),
		coreaction.UUID(actionUUID.String()),
	).Return(operation.Action{}, operationerrors.ActionNotFound)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestActionsServerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID := modeltesting.GenModelUUID(c)

	s.operationService.EXPECT().GetAction(
		gomock.Any(),
		coreaction.UUID(actionUUID.String()),
	).Return(operation.Action{}, errors.New("boom"))

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	// This error was already (wrongly) black-holed into a ErrBadId.
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestActionsMultipleEntities(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID0 := modeltesting.GenModelUUID(c)
	actionUUID1 := modeltesting.GenModelUUID(c)
	resAction := operation.Action{}

	s.operationService.EXPECT().GetAction(
		gomock.Any(),
		coreaction.UUID(actionUUID0.String()),
	).Return(resAction, nil)
	s.operationService.EXPECT().GetAction(
		gomock.Any(),
		coreaction.UUID(actionUUID1.String()),
	).Return(operation.Action{}, operationerrors.ActionNotFound)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID0.String()).String()},
			{Tag: names.NewActionTag(actionUUID1.String()).String()},
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

	s.setupAPI(c, jujutesting.AdminUser)

	result, err := s.client.Actions(context.Background(), params.Entities{
		Entities: []params.Entity{},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func (s *actionSuite) TestCancelSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID := modeltesting.GenModelUUID(c)
	cancelledAction := operation.Action{}

	s.operationService.EXPECT().CancelAction(
		gomock.Any(),
		coreaction.UUID(actionUUID.String()),
	).Return(cancelledAction, nil)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.IsNil)
}

func (s *actionSuite) TestCancelPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	actionUUID := modeltesting.GenModelUUID(c)
	// Use a non-admin user tag to fail permission check.
	nonAdminUser := names.NewUserTag("unauthorized")
	s.setupAPI(c, nonAdminUser)

	_, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *actionSuite) TestCancelInvalidActionTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)

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

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID := modeltesting.GenModelUUID(c)

	s.operationService.EXPECT().CancelAction(
		gomock.Any(),
		coreaction.UUID(actionUUID.String()),
	).Return(operation.Action{}, operationerrors.ActionNotFound)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error, tc.NotNil)
	c.Assert(result.Results[0].Error.Code, tc.Equals, params.CodeNotFound)
}

func (s *actionSuite) TestCancelServerError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)
	actionUUID := modeltesting.GenModelUUID(c)

	s.operationService.EXPECT().CancelAction(
		gomock.Any(),
		coreaction.UUID(actionUUID.String()),
	).Return(operation.Action{}, errors.New("boom"))

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionUUID.String()).String()},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 1)
	c.Assert(result.Results[0].Error.Message, tc.Equals, "boom")
}

func (s *actionSuite) TestCancelEmptyEntityList(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)

	result, err := s.client.Cancel(context.Background(), params.Entities{
		Entities: []params.Entity{},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func (s *actionSuite) TestApplicationsCharmsActionsSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setupAPI(c, jujutesting.AdminUser)
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

	s.setupAPI(c, jujutesting.AdminUser)

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

	s.setupAPI(c, jujutesting.AdminUser)
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

	s.setupAPI(c, jujutesting.AdminUser)

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

	s.setupAPI(c, jujutesting.AdminUser)
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

	s.setupAPI(c, jujutesting.AdminUser)
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

	s.setupAPI(c, jujutesting.AdminUser)

	result, err := s.client.ApplicationsCharmsActions(context.Background(), params.Entities{
		Entities: []params.Entity{},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

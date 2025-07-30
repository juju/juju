// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelStatusSuite struct {
	controllerUUID string
	modelUUID      string

	authorizer apiservertesting.FakeAuthorizer

	machineService *MockMachineService
	statusService  *MockStatusService
	modelService   *MockModelService
}

func TestModelStatusSuite(t *stdtesting.T) {
	tc.Run(t, &modelStatusSuite{})
}

func (s *modelStatusSuite) SetUpTest(c *tc.C) {

	owner := names.NewLocalUserTag("test-admin")
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      owner,
		AdminTag: owner,
	}

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = uuid.MustNewUUID().String()
}

func (s *modelStatusSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Full multi-model status success case for IAAS.
- Full multi-model status success case for CAAS.
- Full multi-model status fails permission denied for non-model owner.
- Full multi-model status success for model owner.
`)
}

func (s *modelStatusSuite) TestModelStatusRunsForAllModels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetApplicationAndUnitModelStatuses(gomock.Any()).Return(
		map[string]int{}, nil,
	)

	modelTag := names.NewModelTag(s.modelUUID).String()
	req := params.Entities{
		Entities: []params.Entity{
			{Tag: "fail.me"},
			{Tag: modelTag},
		},
	}
	expected := params.ModelStatusResults{
		Results: []params.ModelStatus{
			{
				Error: apiservererrors.ServerError(errors.New(`"fail.me" is not a valid tag`))},
			{
				ModelTag:  modelTag,
				Qualifier: "foobar",
				Type:      string(state.ModelTypeIAAS),
			},
		},
	}

	s.statusService.EXPECT().GetModelStatusInfo(gomock.Any()).Return(domainstatus.ModelStatusInfo{
		Type: coremodel.IAAS,
	}, nil)

	s.machineService.EXPECT().AllMachineNames(gomock.Any()).Return([]machine.Name{}, nil)
	s.statusService.EXPECT().GetAllMachineStatuses(gomock.Any()).Return(map[machine.Name]status.StatusInfo{}, nil)

	modelStatusAPI := model.NewModelStatusAPI(
		s.controllerUUID,
		s.modelUUID,
		s.modelService,
		s.machineServiceGetter,
		s.statusServiceGetter,
		s.authorizer,
		s.authorizer.GetAuthTag().(names.UserTag),
	)
	result, err := modelStatusAPI.ModelStatus(c.Context(), req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, expected)
}

func (s *modelStatusSuite) machineServiceGetter(ctx context.Context, uuid coremodel.UUID) (model.MachineService, error) {
	return s.machineService, nil
}

func (s *modelStatusSuite) statusServiceGetter(ctx context.Context, uuid coremodel.UUID) (model.StatusService, error) {
	return s.statusService, nil
}

func (s *modelStatusSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.machineService = NewMockMachineService(ctrl)
	s.statusService = NewMockStatusService(ctrl)
	s.modelService = NewMockModelService(ctrl)

	return ctrl
}

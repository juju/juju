// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/storage/provider"
	dummystorage "github.com/juju/juju/internal/storage/provider/dummy"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type modelStatusSuite struct {
	statetesting.StateSuite

	resources  *common.Resources
	authorizer apiservertesting.FakeAuthorizer

	machineService *MockMachineService
	statusService  *MockStatusService
}

var _ = tc.Suite(&modelStatusSuite{})

func (s *modelStatusSuite) SetUpTest(c *tc.C) {
	// Initial config needs to be set before the StateSuite SetUpTest.
	s.InitialConfig = testing.CustomModelConfig(c, testing.Attrs{
		"name": "controller",
	})
	s.NewPolicy = func(*state.State) state.Policy {
		return statePolicy{}
	}

	s.StateSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *tc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      s.Owner,
		AdminTag: s.Owner,
	}
}

func (s *modelStatusSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Full multi-model status success case for IAAS.
- Full multi-model status success case for CAAS.
`)
}

func (s *modelStatusSuite) TestModelStatusNonAuth(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Set up the user making the call.
	user := names.NewUserTag("username")
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: user,
	}

	api := model.NewModelStatusAPI(
		model.NewModelManagerBackend(s.Model, s.StatePool),
		s.machineServiceGetter,
		s.statusServiceGetter,
		anAuthoriser,
		anAuthoriser.GetAuthTag().(names.UserTag),
	)
	controllerModelTag := s.Model.ModelTag().String()

	req := params.Entities{
		Entities: []params.Entity{{Tag: controllerModelTag}},
	}
	result, err := api.ModelStatus(context.Background(), req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results[0].Error, tc.ErrorMatches, "permission denied")
}

func (s *modelStatusSuite) TestModelStatusOwnerAllowed(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Set up the user making the call.
	owner := names.NewUserTag("owner")
	anAuthoriser := apiservertesting.FakeAuthorizer{
		Tag: owner,
	}
	st := s.Factory.MakeModel(c, &factory.ModelParams{Owner: owner})
	defer st.Close()
	api := model.NewModelStatusAPI(
		model.NewModelManagerBackend(s.Model, s.StatePool),
		s.machineServiceGetter,
		s.statusServiceGetter,
		anAuthoriser,
		anAuthoriser.GetAuthTag().(names.UserTag),
	)

	model, err := st.Model()
	c.Assert(err, tc.ErrorIsNil)
	req := params.Entities{
		Entities: []params.Entity{{Tag: model.ModelTag().String()}},
	}
	_, err = api.ModelStatus(context.Background(), req)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelStatusSuite) TestModelStatusRunsForAllModels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.statusService.EXPECT().GetApplicationAndUnitModelStatuses(gomock.Any()).Return(
		map[string]int{}, nil,
	)

	req := params.Entities{
		Entities: []params.Entity{
			{Tag: "fail.me"},
			{Tag: s.Model.ModelTag().String()},
		},
	}
	expected := params.ModelStatusResults{
		Results: []params.ModelStatus{
			{
				Error: apiservererrors.ServerError(errors.New(`"fail.me" is not a valid tag`))},
			{
				ModelTag: s.Model.ModelTag().String(),
				Life:     life.Value(s.Model.Life().String()),
				OwnerTag: s.Model.Owner().String(),
				Type:     string(state.ModelTypeIAAS),
			},
		},
	}
	modelStatusAPI := model.NewModelStatusAPI(
		model.NewModelManagerBackend(s.Model, s.StatePool),
		s.machineServiceGetter,
		s.statusServiceGetter,
		s.authorizer,
		s.authorizer.GetAuthTag().(names.UserTag),
	)
	result, err := modelStatusAPI.ModelStatus(context.Background(), req)
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

	return ctrl
}

type noopStoragePoolGetter struct{}

func (noopStoragePoolGetter) GetStorageRegistry(_ context.Context) (storage.ProviderRegistry, error) {
	return storage.ChainedProviderRegistry{
		dummystorage.StorageProviders(),
		provider.CommonStorageProviders(),
	}, nil
}

func (noopStoragePoolGetter) GetStoragePoolByName(_ context.Context, name string) (*storage.Config, error) {
	return nil, fmt.Errorf("storage pool %q not found%w", name, errors.Hide(storageerrors.PoolNotFoundError))
}

type statePolicy struct{}

func (statePolicy) ConstraintsValidator(context.Context) (constraints.Validator, error) {
	return nil, errors.NotImplementedf("ConstraintsValidator")
}

func (statePolicy) StorageServices() (state.StoragePoolGetter, error) {
	return noopStoragePoolGetter{}, nil
}

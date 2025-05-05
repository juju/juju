// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/credential"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/internal/provider/azure"
	_ "github.com/juju/juju/internal/provider/ec2"
	_ "github.com/juju/juju/internal/provider/maas"
	_ "github.com/juju/juju/internal/provider/openstack"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	jtesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type ListModelsWithInfoSuite struct {
	jujutesting.IsolationSuite

	st   *mockState
	cred cloud.Credential

	authoriser              apiservertesting.FakeAuthorizer
	adminUser               names.UserTag
	mockAccessService       *mocks.MockAccessService
	mockModelService        *mocks.MockModelService
	mockBlockCommandService *mocks.MockBlockCommandService

	api *modelmanager.ModelManagerAPI

	controllerUUID uuid.UUID
}

var _ = gc.Suite(&ListModelsWithInfoSuite{})

func (s *ListModelsWithInfoSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	var err error
	s.controllerUUID, err = uuid.UUIDFromString(testing.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	adminUser := "admin"
	s.adminUser = names.NewUserTag(adminUser)

	s.st = &mockState{
		model: s.createModel(c, s.adminUser),
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: s.adminUser,
	}

	s.cred = cloud.NewEmptyCredential()

	s.mockAccessService = mocks.NewMockAccessService(ctrl)
	s.mockModelService = mocks.NewMockModelService(ctrl)
	s.mockBlockCommandService = mocks.NewMockBlockCommandService(ctrl)

	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, nil, &mockState{},
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jtesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&s.cred),
			ModelService:         s.mockModelService,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil,
		common.NewBlockChecker(s.mockBlockCommandService), s.authoriser,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	return ctrl
}

func (s *ListModelsWithInfoSuite) createModel(c *gc.C, user names.UserTag) *mockModel {
	attrs := testing.FakeConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return &mockModel{
		owner:               user,
		cfg:                 cfg,
		setCloudCredentialF: func(tag names.CloudCredentialTag) (bool, error) { return false, nil },
	}
}

func (s *ListModelsWithInfoSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	modelmanager, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, nil, &mockState{},
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jtesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&s.cred),
			ModelService:         s.mockModelService,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil,
		common.NewBlockChecker(s.mockBlockCommandService), s.authoriser,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = modelmanager
}

func (s *ListModelsWithInfoSuite) TestListModelSummaries(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	lastLoginTime := time.Now()
	s.mockModelService.EXPECT().ListModelSummariesForUser(gomock.Any(), coreuser.AdminUserName).Return([]coremodel.UserModelSummary{{
		UserLastConnection: &lastLoginTime,
		UserAccess:         permission.AdminAccess,
		ModelSummary: coremodel.ModelSummary{
			Name:           "testmodel",
			OwnerName:      coreuser.AdminUserName,
			UUID:           modelUUID,
			ModelType:      coremodel.IAAS,
			CloudType:      "ec2",
			ControllerUUID: s.controllerUUID.String(),
			IsController:   true,
			CloudName:      "dummy",
			CloudRegion:    "dummy-region",
			CloudCredentialKey: credential.Key{
				Cloud: "dummy",
				Owner: usertesting.GenNewName(c, "bob"),
				Name:  "some-credential",
			},
			Life:         corelife.Alive,
			AgentVersion: jujuversion.Current,
			MachineCount: 10,
			CoreCount:    42,
			UnitCount:    10,
		},
	}}, nil)

	result, err := s.api.ListModelSummaries(context.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{
				Result: &params.ModelSummary{
					Name:               "testmodel",
					OwnerTag:           s.adminUser.String(),
					UUID:               modelUUID.String(),
					Type:               string(state.ModelTypeIAAS),
					ProviderType:       "ec2",
					ControllerUUID:     s.controllerUUID.String(),
					IsController:       true,
					CloudTag:           "cloud-dummy",
					CloudRegion:        "dummy-region",
					CloudCredentialTag: "cloudcred-dummy_bob_some-credential",
					Life:               "alive",
					UserLastConnection: &lastLoginTime,
					UserAccess:         "admin",
					AgentVersion:       &jujuversion.Current,
					Status:             params.EntityStatus{},
					Counts: []params.ModelEntityCount{
						{Entity: params.Machines, Count: 10},
						{Entity: params.Cores, Count: 42},
						{Entity: params.Units, Count: 10},
					},
					Migration: nil,
				},
			},
		},
	})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesAll(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockModelService.EXPECT().ListAllModelSummaries(gomock.Any()).Return([]coremodel.ModelSummary{{
		Name:           "testmodel",
		OwnerName:      coreuser.AdminUserName,
		UUID:           modelUUID,
		ModelType:      coremodel.IAAS,
		CloudType:      "ec2",
		ControllerUUID: s.controllerUUID.String(),
		IsController:   true,
		CloudName:      "dummy",
		CloudRegion:    "dummy-region",
		CloudCredentialKey: credential.Key{
			Cloud: "dummy",
			Owner: usertesting.GenNewName(c, "bob"),
			Name:  "some-credential",
		},
		Life:         corelife.Alive,
		AgentVersion: jujuversion.Current,
		MachineCount: 10,
		CoreCount:    42,
		UnitCount:    10,
	}}, nil)

	result, err := s.api.ListModelSummaries(context.Background(), params.ModelSummariesRequest{
		UserTag: s.adminUser.String(),
		All:     true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, params.ModelSummaryResults{
		Results: []params.ModelSummaryResult{
			{
				Result: &params.ModelSummary{
					Name:               "testmodel",
					OwnerTag:           s.adminUser.String(),
					UUID:               s.st.ModelUUID(),
					Type:               modelUUID.String(),
					ProviderType:       "ec2",
					ControllerUUID:     s.controllerUUID.String(),
					IsController:       true,
					CloudTag:           "cloud-dummy",
					CloudRegion:        "dummy-region",
					CloudCredentialTag: "cloudcred-dummy_bob_some-credential",
					Life:               "alive",
					UserLastConnection: nil,
					UserAccess:         "",
					AgentVersion:       &jujuversion.Current,
					Status:             params.EntityStatus{},
					Counts: []params.ModelEntityCount{
						{Entity: params.Machines, Count: 10},
						{Entity: params.Cores, Count: 42},
						{Entity: params.Units, Count: 10},
					},
					Migration: nil,
				},
			},
		},
	})
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesDenied(c *gc.C) {
	defer s.setupMocks(c).Finish()

	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	other := names.NewUserTag("other@remote")
	_, err := s.api.ListModelSummaries(context.Background(), params.ModelSummariesRequest{UserTag: other.String()})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesInvalidUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.api.ListModelSummaries(context.Background(), params.ModelSummariesRequest{UserTag: "invalid"})
	c.Assert(err, gc.ErrorMatches, `"invalid" is not a valid tag`)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesDomainError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	errMsg := "captain error for ModelSummariesForUser"
	s.mockModelService.EXPECT().ListModelSummariesForUser(gomock.Any(), coreuser.AdminUserName).Return(nil, errors.New(errMsg))
	_, err := s.api.ListModelSummaries(context.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, gc.ErrorMatches, errMsg)
}

func (s *ListModelsWithInfoSuite) TestListModelSummariesNoModelsForUser(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockModelService.EXPECT().ListModelSummariesForUser(gomock.Any(), coreuser.AdminUserName).Return(nil, nil)
	results, err := s.api.ListModelSummaries(context.Background(), params.ModelSummariesRequest{UserTag: s.adminUser.String()})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

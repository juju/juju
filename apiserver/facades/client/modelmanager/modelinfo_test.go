// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	"time"

	"github.com/juju/collections/set"
	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/access"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelInfoSuite struct {
	coretesting.BaseSuite
	authorizer               apiservertesting.FakeAuthorizer
	st                       *mockState
	ctlrSt                   *mockState
	controllerUserInfo       []access.ModelUserInfo
	modelUserInfo            []access.ModelUserInfo
	mockSecretBackendService *mocks.MockSecretBackendService
	controllerUUID           uuid.UUID
	mockAccessService        *mocks.MockAccessService
}

func pUint64(v uint64) *uint64 {
	return &v
}

var _ = gc.Suite(&modelInfoSuite{})

func (s *modelInfoSuite) SetUpTest(c *gc.C) {
	var err error
	s.controllerUUID, err = uuid.UUIDFromString(coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	s.BaseSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin@local"),
	}
	s.st = &mockState{
		controllerUUID: coretesting.ControllerTag.Id(),
		cfgDefaults: config.ModelDefaultAttributes{
			"attr": config.AttributeDefaultValues{
				Default:    "",
				Controller: "val",
				Regions: []config.RegionDefaultValue{{
					Name:  "dummy",
					Value: "val++"}}},
			"attr2": config.AttributeDefaultValues{
				Controller: "val3",
				Default:    "val2",
				Regions: []config.RegionDefaultValue{{
					Name:  "left",
					Value: "spam"}}},
		},
	}

	s.controllerUserInfo = []access.ModelUserInfo{{
		Name:   "admin",
		Access: permission.AdminAccess,
	}, {
		Name:   "otheruser",
		Access: permission.AdminAccess,
	}}

	controllerModel := &mockModel{
		owner: names.NewUserTag("admin@local"),
		life:  state.Alive,
		cfg:   coretesting.ModelConfig(c),
		// This feels kind of wrong as both controller model and
		// default model will end up with the same model tag.
		tag:            coretesting.ModelTag,
		controllerUUID: s.st.controllerUUID,
		isController:   true,
		status: status.StatusInfo{
			Status: status.Available,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   permission.AdminAccess,
		}, {
			userName: "otheruser",
			access:   permission.AdminAccess,
		}},
	}
	s.st.controllerModel = controllerModel

	s.ctlrSt = &mockState{
		model:           controllerModel,
		controllerModel: controllerModel,
	}

	s.modelUserInfo = []access.ModelUserInfo{{
		Name:   "admin",
		Access: permission.AdminAccess,
	}, {
		Name:        "bob",
		DisplayName: "Bob",
		Access:      permission.ReadAccess,
	}, {
		Name:        "charlotte",
		DisplayName: "Charlotte",
		Access:      permission.ReadAccess,
	}, {
		Name:        "mary",
		DisplayName: "Mary",
		Access:      permission.WriteAccess,
	}}

	s.st.model = &mockModel{
		owner:          names.NewUserTag("bob@local"),
		cfg:            coretesting.ModelConfig(c),
		tag:            coretesting.ModelTag,
		controllerUUID: s.st.controllerUUID,
		isController:   false,
		life:           state.Dying,
		status: status.StatusInfo{
			Status: status.Destroying,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   permission.AdminAccess,
		}, {
			userName:    "bob",
			displayName: "Bob",
			access:      permission.ReadAccess,
		}, {
			userName:    "charlotte",
			displayName: "Charlotte",
			access:      permission.ReadAccess,
		}, {
			userName:    "mary",
			displayName: "Mary",
			access:      permission.WriteAccess,
		}},
	}
	s.st.machines = []common.Machine{
		&mockMachine{
			id:            "1",
			containerType: "none",
			life:          state.Alive,
			hw:            &instance.HardwareCharacteristics{CpuCores: pUint64(1)},
		},
		&mockMachine{
			id:            "2",
			life:          state.Alive,
			containerType: "lxc",
		},
		&mockMachine{
			id:   "3",
			life: state.Dead,
		},
	}
	s.st.controllerNodes = []common.ControllerNode{
		&mockControllerNode{
			id:        "1",
			hasVote:   true,
			wantsVote: true,
		},
		&mockControllerNode{
			id:        "2",
			hasVote:   false,
			wantsVote: true,
		},
	}
}

func (s *modelInfoSuite) TearDownTest(c *gc.C) {
	modelmanager.ResetSupportedFeaturesGetter()
}

func (s *modelInfoSuite) getAPI(c *gc.C) (*modelmanager.ModelManagerAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.mockAccessService = mocks.NewMockAccessService(ctrl)
	cred := cloud.NewEmptyCredential()
	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, nil, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": testing.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
			SecretBackendService: s.mockSecretBackendService,
		},
		state.NoopConfigSchemaSource,
		nil, nil, common.NewBlockChecker(s.st),
		&s.authorizer, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})
	modelmanager.MockSupportedFeatures(fs)
	return api, ctrl
}

func (s *modelInfoSuite) getAPIWithUser(c *gc.C, user names.UserTag) (*modelmanager.ModelManagerAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.mockAccessService = mocks.NewMockAccessService(ctrl)
	s.authorizer.Tag = user
	cred := cloud.NewEmptyCredential()
	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, nil, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": testing.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ObjectStore:          &mockObjectStore{},
			SecretBackendService: s.mockSecretBackendService,
		},
		state.NoopConfigSchemaSource,
		nil, nil,
		common.NewBlockChecker(s.st), s.authorizer, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	return api, ctrl
}

func (s *modelInfoSuite) expectedModelInfo(c *gc.C, credentialValidity *bool) params.ModelInfo {
	expectedAgentVersion, exists := s.st.model.cfg.AgentVersion()
	c.Assert(exists, jc.IsTrue)
	info := params.ModelInfo{
		Name:               "testmodel",
		UUID:               s.st.model.cfg.UUID(),
		Type:               string(s.st.model.Type()),
		ControllerUUID:     "deadbeef-1bad-500d-9000-4b1d0d06f00d",
		IsController:       false,
		OwnerTag:           "user-bob",
		ProviderType:       "dummy",
		CloudTag:           "cloud-dummy",
		CloudRegion:        "dummy-region",
		CloudCredentialTag: "cloudcred-dummy_bob_some-credential",
		Life:               life.Dying,
		Status: params.EntityStatus{
			Status: status.Destroying,
			Since:  &time.Time{},
		},
		Users: []params.ModelUserInfo{{
			UserName:       "admin",
			LastConnection: nil,
			Access:         params.ModelAdminAccess,
		}, {
			UserName:       "bob",
			DisplayName:    "Bob",
			LastConnection: nil,
			Access:         params.ModelReadAccess,
		}, {
			UserName:       "charlotte",
			DisplayName:    "Charlotte",
			LastConnection: nil,
			Access:         params.ModelReadAccess,
		}, {
			UserName:       "mary",
			DisplayName:    "Mary",
			LastConnection: nil,
			Access:         params.ModelWriteAccess,
		}},
		Machines: []params.ModelMachineInfo{{
			Id:        "1",
			Hardware:  &params.MachineHardware{Cores: pUint64(1)},
			HasVote:   true,
			WantsVote: true,
		}, {
			Id:        "2",
			WantsVote: true,
		}},
		SecretBackends: []params.SecretBackendResult{{
			Result: params.SecretBackend{
				Name:        "myvault",
				BackendType: "vault",
				Config: map[string]interface{}{
					"endpoint": "http://vault",
				},
			},
			Status:     "active",
			NumSecrets: 2,
		}},
		AgentVersion: &expectedAgentVersion,
		SupportedFeatures: []params.SupportedFeature{
			{Name: "example"},
		},
	}
	info.CloudCredentialValidity = credentialValidity
	return info
}

func (s *modelInfoSuite) TestModelInfo(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return([]*secretbackendservice.SecretBackendInfo{
		{
			SecretBackend: secrets.SecretBackend{
				Name:        "myvault",
				BackendType: "vault",
				Config: map[string]interface{}{
					"endpoint": "http://vault",
				},
			},
			Status:     "active",
			NumSecrets: 2,
		},
	}, nil)

	info := s.getModelInfo(c, api, s.st.model.cfg.UUID())
	_true := true
	s.assertModelInfo(c, info, s.expectedModelInfo(c, &_true))
	s.st.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "ControllerTag", Args: nil},
		{FuncName: "GetBackend", Args: []interface{}{s.st.model.cfg.UUID()}},
		{FuncName: "Model", Args: nil},
		{FuncName: "IsController", Args: nil},
		{FuncName: "AllMachines", Args: nil},
		{FuncName: "ControllerNodes", Args: nil},
		{FuncName: "HAPrimaryMachine", Args: nil},
		{FuncName: "LatestMigration", Args: nil},
	})
}

func (s *modelInfoSuite) assertModelInfo(c *gc.C, got, expected params.ModelInfo) {
	c.Assert(got, jc.DeepEquals, expected)
	s.st.model.CheckCalls(c, []jujutesting.StubCall{
		{"Name", nil},
		{"Type", nil},
		{"UUID", nil},
		{"UUID", nil},
		{"Owner", nil},
		{"Life", nil},
		{"CloudName", nil},
		{"CloudRegion", nil},
		{"CloudCredentialTag", nil},
		{"Life", nil},
		{"Config", nil},
		{"Status", nil},
		{"UUID", nil},
		{"Type", nil},
	})
}

func (s *modelInfoSuite) TestModelInfoWriteAccess(c *gc.C) {
	mary := names.NewUserTag("mary@local")
	s.authorizer.HasWriteTag = mary
	api, ctrl := s.getAPIWithUser(c, mary)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "mary", coremodel.UUID(s.st.model.cfg.UUID())).Return(
		[]access.ModelUserInfo{{
			Name:        "mary",
			DisplayName: "Mary",
			Access:      permission.WriteAccess,
		}}, nil,
	)

	info := s.getModelInfo(c, api, s.st.model.cfg.UUID())
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "mary")
	c.Assert(info.Machines, gc.HasLen, 2)
}

func (s *modelInfoSuite) TestModelInfoNonOwner(c *gc.C) {
	api, ctrl := s.getAPIWithUser(c, names.NewUserTag("charlotte@local"))
	defer ctrl.Finish()
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "charlotte", coremodel.UUID(s.st.model.cfg.UUID())).Return(
		[]access.ModelUserInfo{{
			Name:        "charlotte",
			DisplayName: "Charlotte",
			Access:      permission.ReadAccess,
		}}, nil,
	)
	info := s.getModelInfo(c, api, s.st.model.cfg.UUID())
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "charlotte")
	c.Assert(info.Machines, gc.HasLen, 0)
}

type modelInfo interface {
	ModelInfo(context.Context, params.Entities) (params.ModelInfoResults, error)
}

func (s *modelInfoSuite) getModelInfo(c *gc.C, modelInfo modelInfo, modelUUID string) params.ModelInfo {
	results, err := modelInfo.ModelInfo(context.Background(), params.Entities{
		Entities: []params.Entity{{
			names.NewModelTag(modelUUID).String(),
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.NotNil)
	return *results.Results[0].Result
}

func (s *modelInfoSuite) TestModelInfoErrorInvalidTag(c *gc.C) {
	api, _ := s.getAPI(c)
	s.testModelInfoError(c, api, "user-bob", `"user-bob" is not a valid model tag`)
}

func (s *modelInfoSuite) TestModelInfoErrorGetModelNotFound(c *gc.C) {
	api, _ := s.getAPI(c)
	s.st.SetErrors(errors.NotFoundf("model"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestModelInfoErrorModelConfig(c *gc.C) {
	api, _ := s.getAPI(c)
	s.st.model.SetErrors(errors.Errorf("no config for you"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `no config for you`)
}

func (s *modelInfoSuite) TestModelInfoErrorModelUsers(c *gc.C) {
	api, _ := s.getAPI(c)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(coretesting.ModelTag.Id())).Return(nil, errors.Errorf("no users for you"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `getting model user info: no users for you`)
}

func (s *modelInfoSuite) TestModelInfoErrorNoModelUsers(c *gc.C) {
	c.Skip("This error check in modelInfo is disabled until all permissions have been moved to domain")
	api, _ := s.getAPI(c)
	s.st.model.users = nil
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestModelInfoErrorNoAccess(c *gc.C) {
	c.Skip("This error check in modelInfo is disabled until all permissions have been moved to domain")
	api, _ := s.getAPIWithUser(c, names.NewUserTag("nemo@local"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestRunningMigration(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(coretesting.ModelTag.Id())).Return(s.modelUserInfo, nil)
	start := time.Now().Add(-20 * time.Minute)
	s.st.migration = &mockMigration{
		status: "computing optimal bin packing",
		start:  start,
	}

	results, err := api.ModelInfo(context.Background(), params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})

	c.Assert(err, jc.ErrorIsNil)
	migrationResult := results.Results[0].Result.Migration
	c.Assert(migrationResult.Status, gc.Equals, "computing optimal bin packing")
	c.Assert(*migrationResult.Start, gc.Equals, start)
	c.Assert(migrationResult.End, gc.IsNil)
}

func (s *modelInfoSuite) TestFailedMigration(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(coretesting.ModelTag.Id())).Return(s.modelUserInfo, nil)
	start := time.Now().Add(-20 * time.Minute)
	end := time.Now().Add(-10 * time.Minute)
	s.st.migration = &mockMigration{
		status: "couldn't realign alternate time frames",
		start:  start,
		end:    end,
	}

	results, err := api.ModelInfo(context.Background(), params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})

	c.Assert(err, jc.ErrorIsNil)
	migrationResult := results.Results[0].Result.Migration
	c.Assert(migrationResult.Status, gc.Equals, "couldn't realign alternate time frames")
	c.Assert(*migrationResult.Start, gc.Equals, start)
	c.Assert(*migrationResult.End, gc.Equals, end)
}

func (s *modelInfoSuite) TestNoMigration(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(coretesting.ModelTag.Id())).Return(s.modelUserInfo, nil)
	results, err := api.ModelInfo(context.Background(), params.Entities{
		Entities: []params.Entity{{coretesting.ModelTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Result.Migration, gc.IsNil)
}

func (s *modelInfoSuite) TestAliveModelGetsAllInfo(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Alive, life.Alive)
}

func (s *modelInfoSuite) TestAliveModelWithConfigFailure(c *gc.C) {
	api, _ := s.getAPI(c)
	s.st.model.life = state.Alive
	s.setModelConfigError()
	s.testModelInfoError(c, api, s.st.model.tag.String(), "config not found")
}

func (s *modelInfoSuite) TestAliveModelWithStatusFailure(c *gc.C) {
	api, _ := s.getAPI(c)
	s.st.model.life = state.Alive
	s.setModelStatusError()
	s.testModelInfoError(c, api, s.st.model.tag.String(), "status not found")
}

func (s *modelInfoSuite) TestAliveModelWithUsersFailure(c *gc.C) {
	api, _ := s.getAPI(c)
	s.st.model.life = state.Alive
	s.setModelUsersError()
	s.testModelInfoError(c, api, s.st.model.tag.String(), "getting model user info: model not found")
}

func (s *modelInfoSuite) TestDeadModelGetsAllInfo(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Dead, life.Dead)
}

func (s *modelInfoSuite) TestDeadModelWithConfigFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	testData := incompleteModelInfoTest{
		failModel:    s.setModelConfigError,
		desiredLife:  state.Dead,
		expectedLife: life.Dead,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestDeadModelWithStatusFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Dead,
		expectedLife: life.Dead,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestDeadModelWithUsersFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Dead,
		expectedLife: life.Dead,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestDyingModelWithConfigFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	testData := incompleteModelInfoTest{
		failModel:    s.setModelConfigError,
		desiredLife:  state.Dying,
		expectedLife: life.Dying,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestDyingModelWithStatusFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Dying,
		expectedLife: life.Dying,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestDyingModelWithUsersFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Dying,
		expectedLife: life.Dying,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestImportingModelGetsAllInfo(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.model.migrationStatus = state.MigrationModeImporting
	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Alive, life.Alive)
}

func (s *modelInfoSuite) TestImportingModelWithConfigFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.model.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelConfigError,
		desiredLife:  state.Alive,
		expectedLife: life.Alive,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestImportingModelWithStatusFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockAccessService.EXPECT().GetModelUsers(gomock.Any(), "admin", coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.model.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Alive,
		expectedLife: life.Alive,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestImportingModelWithUsersFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.st.model.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Alive,
		expectedLife: life.Alive,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

type incompleteModelInfoTest struct {
	failModel    func()
	desiredLife  state.Life
	expectedLife life.Value
}

func (s *modelInfoSuite) setModelConfigError() {
	s.st.model.SetErrors(errors.NotFoundf("config"))
}

func (s *modelInfoSuite) setModelStatusError() {
	s.st.model.SetErrors(
		nil,                        // Config
		errors.NotFoundf("status"), // Status
	)
}

func (s *modelInfoSuite) setModelUsersError() {
	s.mockAccessService.EXPECT().GetModelUsers(
		gomock.Any(),
		"admin",
		gomock.Any(),
	).Return(
		nil,
		modelerrors.NotFound,
	)
}

func (s *modelInfoSuite) assertSuccessWithMissingData(c *gc.C, api *modelmanager.ModelManagerAPI, test incompleteModelInfoTest) {
	test.failModel()
	// We do not expect any errors to surface and still want to get basic model info.
	s.assertSuccess(c, api, s.st.model.cfg.UUID(), test.desiredLife, test.expectedLife)
}

func (s *modelInfoSuite) assertSuccess(c *gc.C, api *modelmanager.ModelManagerAPI, modelUUID string, desiredLife state.Life, expectedLife life.Value) {
	s.st.model.life = desiredLife
	// should get no errors
	info := s.getModelInfo(c, api, modelUUID)
	c.Assert(info.UUID, gc.Equals, modelUUID)
	c.Assert(info.Life, gc.Equals, expectedLife)
}

func (s *modelInfoSuite) testModelInfoError(c *gc.C, api *modelmanager.ModelManagerAPI, modelTag, expectedErr string) {
	results, err := api.ModelInfo(context.Background(), params.Entities{
		Entities: []params.Entity{{modelTag}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Result, gc.IsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, expectedErr)
}

type unitRetriever interface {
	Unit(name string) (*state.Unit, error)
}

type mockCaasBroker struct {
	jujutesting.Stub
	caas.Broker

	namespace string
}

func (m *mockCaasBroker) Create(envcontext.ProviderCallContext, environs.CreateParams) error {
	m.MethodCall(m, "Create")
	if m.namespace == "existing-ns" {
		return errors.AlreadyExistsf("namespace %q", m.namespace)
	}
	return nil
}

type mockState struct {
	jujutesting.Stub

	environs.EnvironConfigGetter
	common.APIHostPortsForAgentsGetter
	common.ToolsStorageGetter
	common.BlockGetter
	unitRetriever

	controllerUUID  string
	cloudUsers      map[string]permission.Access
	model           *mockModel
	controllerModel *mockModel
	users           []permission.UserAccess
	machines        []common.Machine
	controllerNodes []common.ControllerNode
	cfgDefaults     config.ModelDefaultAttributes
	blockMsg        string
	block           state.BlockType
	migration       *mockMigration
	modelConfig     *config.Config

	modelDetailsForUser func() ([]state.ModelSummary, error)
}

type fakeModelDescription struct {
	description.Model `yaml:"-"`

	UUID string `yaml:"model-uuid"`
}

func (st *mockState) ModelUUID() string {
	st.MethodCall(st, "ModelUUID")
	return st.model.UUID()
}

func (st *mockState) Name() string {
	st.MethodCall(st, "Name")
	return "test-model"
}

func (st *mockState) ControllerModelUUID() string {
	st.MethodCall(st, "ControllerModelUUID")
	return st.controllerModel.tag.Id()
}

func (st *mockState) ControllerModelTag() names.ModelTag {
	st.MethodCall(st, "ControllerModelTag")
	return st.controllerModel.tag
}

func (st *mockState) Export(leaders map[string]string, store objectstore.ObjectStore) (description.Model, error) {
	st.MethodCall(st, "Export", leaders)
	return &fakeModelDescription{UUID: st.model.UUID()}, nil
}

func (st *mockState) ExportPartial(cfg state.ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	st.MethodCall(st, "ExportPartial", cfg)
	if !cfg.IgnoreIncompleteModel {
		return nil, errors.New("expected IgnoreIncompleteModel=true")
	}
	return &fakeModelDescription{UUID: st.model.UUID()}, nil
}

func (st *mockState) AllModelUUIDs() ([]string, error) {
	st.MethodCall(st, "AllModelUUIDs")
	return []string{st.model.UUID()}, st.NextErr()
}

func (st *mockState) GetBackend(modelUUID string) (common.ModelManagerBackend, func() bool, error) {
	st.MethodCall(st, "GetBackend", modelUUID)
	return st, func() bool { return true }, st.NextErr()
}

func (st *mockState) GetModel(modelUUID string) (common.Model, func() bool, error) {
	st.MethodCall(st, "GetModel", modelUUID)
	return st.model, func() bool { return true }, st.NextErr()
}

func (st *mockState) ModelUUIDsForUser(user names.UserTag) ([]string, error) {
	st.MethodCall(st, "ModelUUIDsForUser", user)
	return nil, st.NextErr()
}

func (st *mockState) AllApplications() ([]common.Application, error) {
	st.MethodCall(st, "AllApplications")
	return nil, st.NextErr()
}

func (st *mockState) AllVolumes() ([]state.Volume, error) {
	st.MethodCall(st, "AllVolumes")
	return nil, st.NextErr()
}

func (st *mockState) AllFilesystems() ([]state.Filesystem, error) {
	st.MethodCall(st, "AllFilesystems")
	return nil, st.NextErr()
}

func (st *mockState) IsControllerAdmin(user names.UserTag) (bool, error) {
	st.MethodCall(st, "IsControllerAdmin", user)
	if st.controllerModel == nil {
		return user.Id() == "admin", st.NextErr()
	}
	if st.controllerModel.users == nil {
		return user.Id() == "admin", st.NextErr()
	}

	for _, u := range st.controllerModel.users {
		if user.Name() == u.userName && u.access == permission.AdminAccess {
			nextErr := st.NextErr()
			if user.Name() != "admin" {
				panic(user.Name())
			}
			return true, nextErr
		}
	}
	return false, st.NextErr()
}

func (st *mockState) NewModel(args state.ModelArgs) (common.Model, common.ModelManagerBackend, error) {
	st.MethodCall(st, "NewModel", args)
	st.model.tag = names.NewModelTag(args.Config.UUID())
	return st.model, st, st.NextErr()
}

func (st *mockState) ControllerModel() (common.Model, error) {
	st.MethodCall(st, "ControllerModel")
	return st.controllerModel, st.NextErr()
}

func (st *mockState) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag(st.controllerUUID)
}

func (st *mockState) ComposeNewModelConfig(_ config.ConfigSchemaSourceGetter, modelAttr map[string]interface{}, regionSpec *environscloudspec.CloudRegionSpec) (map[string]interface{}, error) {
	st.MethodCall(st, "ComposeNewModelConfig")
	attr := make(map[string]interface{})
	for attrName, val := range modelAttr {
		attr[attrName] = val
	}
	attr["something"] = "value"
	return attr, st.NextErr()
}

func (st *mockState) ControllerUUID() string {
	st.MethodCall(st, "ControllerUUID")
	return st.controllerUUID
}

func (st *mockState) IsController() bool {
	st.MethodCall(st, "IsController")
	return st.controllerUUID == st.model.UUID()
}

func (st *mockState) ControllerNodes() ([]common.ControllerNode, error) {
	st.MethodCall(st, "ControllerNodes")
	return st.controllerNodes, st.NextErr()
}

func (st *mockState) Model() (common.Model, error) {
	st.MethodCall(st, "Model")
	return st.model, st.NextErr()
}

func (st *mockState) ModelTag() names.ModelTag {
	st.MethodCall(st, "ModelTag")
	return st.model.ModelTag()
}

func (st *mockState) AllMachines() ([]common.Machine, error) {
	st.MethodCall(st, "AllMachines")
	return st.machines, st.NextErr()
}

func (st *mockState) SetModelAgentVersion(newVersion version.Number, stream *string, ignoreAgentVersions bool) error {
	return errors.NotImplementedf("SetModelAgentVersion")
}

func (st *mockState) AbortCurrentUpgrade() error {
	return errors.NotImplementedf("AbortCurrentUpgrade")
}

func (st *mockState) Close() error {
	st.MethodCall(st, "Close")
	return st.NextErr()
}

func (st *mockState) AddControllerUser(spec state.UserAccessSpec) (permission.UserAccess, error) {
	st.MethodCall(st, "AddControllerUser", spec)
	return permission.UserAccess{}, st.NextErr()
}

func (st *mockState) UserAccess(tag names.UserTag, target names.Tag) (permission.UserAccess, error) {
	st.MethodCall(st, "ModelUser", tag, target)
	for _, user := range st.users {
		if user.UserTag != tag {
			continue
		}
		nextErr := st.NextErr()
		if nextErr != nil {
			return permission.UserAccess{}, nextErr
		}
		return user, nil
	}
	return permission.UserAccess{}, st.NextErr()
}

func (st *mockState) ModelSummariesForUser(user names.UserTag, isSuperuser bool) ([]state.ModelSummary, error) {
	st.MethodCall(st, "ModelSummariesForUser", user, isSuperuser)
	return st.modelDetailsForUser()
}

func (st *mockState) RemoveUserAccess(subject names.UserTag, target names.Tag) error {
	st.MethodCall(st, "RemoveUserAccess", subject, target)
	return st.NextErr()
}

func (st *mockState) SetUserAccess(subject names.UserTag, target names.Tag, access permission.Access) (permission.UserAccess, error) {
	st.MethodCall(st, "SetUserAccess", subject, target, access)
	return permission.UserAccess{}, st.NextErr()
}

func (st *mockState) ModelConfigDefaultValues(cloud string) (config.ModelDefaultAttributes, error) {
	st.MethodCall(st, "ModelConfigDefaultValues", cloud)
	return st.cfgDefaults, nil
}

func (st *mockState) UpdateModelConfigDefaultValues(update map[string]interface{}, remove []string, rspec *environscloudspec.CloudRegionSpec) error {
	st.MethodCall(st, "UpdateModelConfigDefaultValues", update, remove, rspec)
	for k, v := range update {
		if rspec != nil {
			adv := st.cfgDefaults[k]
			adv.Regions = append(adv.Regions, config.RegionDefaultValue{
				Name:  rspec.Region,
				Value: v})

		} else {
			st.cfgDefaults[k] = config.AttributeDefaultValues{Controller: v}
		}
	}
	for _, n := range remove {
		if rspec != nil {
			for i, r := range st.cfgDefaults[n].Regions {
				if r.Name == rspec.Region {
					adv := st.cfgDefaults[n]
					adv.Regions = append(adv.Regions[:i], adv.Regions[i+1:]...)
					st.cfgDefaults[n] = adv
				}
			}
		} else {
			if len(st.cfgDefaults[n].Regions) == 0 {
				delete(st.cfgDefaults, n)
			} else {

				st.cfgDefaults[n] = config.AttributeDefaultValues{
					Regions: st.cfgDefaults[n].Regions}
			}
		}
	}
	return nil
}

func (st *mockState) GetBlockForType(t state.BlockType) (state.Block, bool, error) {
	st.MethodCall(st, "GetBlockForType", t)
	if st.block == t {
		return &mockBlock{t: t, m: st.blockMsg}, true, nil
	} else {
		return nil, false, nil
	}
}

func (st *mockState) SaveProviderSubnets(subnets []network.SubnetInfo, spaceID string) error {
	st.MethodCall(st, "SaveProviderSubnets", subnets, spaceID)
	return st.NextErr()
}

func (st *mockState) DumpAll() (map[string]interface{}, error) {
	st.MethodCall(st, "DumpAll")
	return map[string]interface{}{
		"models": "lots of data",
	}, st.NextErr()
}

func (st *mockState) LatestMigration() (state.ModelMigration, error) {
	st.MethodCall(st, "LatestMigration")
	if st.migration == nil {
		// Handle nil->notfound directly here rather than having to
		// count errors.
		return nil, errors.NotFoundf("")
	}
	return st.migration, st.NextErr()
}

func (st *mockState) ModelConfig(context.Context) (*config.Config, error) {
	st.MethodCall(st, "ModelConfig")
	return st.modelConfig, st.NextErr()
}

func (st *mockState) HAPrimaryMachine() (names.MachineTag, error) {
	st.MethodCall(st, "HAPrimaryMachine")
	return names.MachineTag{}, nil
}

func (st *mockState) DefaultEndpointBindingSpace() (string, error) {
	st.MethodCall(st, "DefaultEndpointBindingSpace")
	return "alpha", nil
}

func (st *mockState) ConstraintsBySpaceName(spaceName string) ([]*state.Constraints, error) {
	st.MethodCall(st, "ConstraintsBySpaceName", spaceName)
	return nil, st.NextErr()
}

func (st *mockState) ListModelSecrets(all bool) (map[string]set.Strings, error) {
	return map[string]set.Strings{
		"backend-id": set.NewStrings("a", "b"),
	}, nil
}

func (st *mockState) GetSecretBackendByID(id string) (*secrets.SecretBackend, error) {
	if id != "backend-id" {
		return nil, errors.NotFoundf("backend %q", id)
	}
	return &secrets.SecretBackend{
		ID:          "backend-id",
		Name:        "myvault",
		BackendType: "vault",
		Config: map[string]interface{}{
			"endpoint": "http://vault",
			"token":    "secret",
		},
	}, nil
}

func (st *mockState) ListSecretBackends() ([]*secrets.SecretBackend, error) {
	return []*secrets.SecretBackend{{
		ID:          "backend-id",
		Name:        "myvault",
		BackendType: "vault",
		Config: map[string]interface{}{
			"endpoint": "http://vault",
			"token":    "secret",
		},
	}}, nil
}

func (st *mockState) InvalidateModelCredential(reason string) error {
	st.MethodCall(st, "InvalidateModelCredential", reason)
	return nil
}

type mockBlock struct {
	state.Block
	t state.BlockType
	m string
}

func (m mockBlock) Id() string { return "" }

func (m mockBlock) Tag() (names.Tag, error) { return names.NewModelTag("mocktesting"), nil }

func (m mockBlock) Type() state.BlockType { return m.t }

func (m mockBlock) Message() string { return m.m }

func (m mockBlock) ModelUUID() string { return "" }

type mockControllerNode struct {
	id        string
	hasVote   bool
	wantsVote bool
}

func (m *mockControllerNode) Id() string {
	return m.id
}

func (m *mockControllerNode) WantsVote() bool {
	return m.wantsVote
}

func (m *mockControllerNode) HasVote() bool {
	return m.hasVote
}

type mockMachine struct {
	common.Machine
	id            string
	life          state.Life
	containerType instance.ContainerType
	hw            *instance.HardwareCharacteristics
}

func (m *mockMachine) Id() string {
	return m.id
}

func (m *mockMachine) Life() state.Life {
	return m.life
}

func (m *mockMachine) ContainerType() instance.ContainerType {
	return m.containerType
}

func (m *mockMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	return m.hw, nil
}

func (m *mockMachine) InstanceId() (instance.Id, error) {
	return "", nil
}

func (m *mockMachine) InstanceNames() (instance.Id, string, error) {
	return "", "", nil
}

func (m *mockMachine) HasVote() bool {
	return false
}

func (m *mockMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{}, nil
}

type mockModel struct {
	jujutesting.Stub
	owner               names.UserTag
	life                state.Life
	tag                 names.ModelTag
	status              status.StatusInfo
	cfg                 *config.Config
	users               []*mockModelUser
	migrationStatus     state.MigrationMode
	controllerUUID      string
	isController        bool
	setCloudCredentialF func(tag names.CloudCredentialTag) (bool, error)
}

func (m *mockModel) Config() (*config.Config, error) {
	m.MethodCall(m, "Config")
	return m.cfg, m.NextErr()
}

func (m *mockModel) Owner() names.UserTag {
	m.MethodCall(m, "Owner")
	return m.owner
}

func (m *mockModel) ModelTag() names.ModelTag {
	m.MethodCall(m, "ModelTag")
	return m.tag
}

func (m *mockModel) Type() state.ModelType {
	m.MethodCall(m, "Type")
	return state.ModelTypeIAAS
}

func (m *mockModel) Life() state.Life {
	m.MethodCall(m, "Life")
	return m.life
}

func (m *mockModel) Status() (status.StatusInfo, error) {
	m.MethodCall(m, "Status")
	return m.status, m.NextErr()
}

func (m *mockModel) CloudName() string {
	m.MethodCall(m, "CloudName")
	return "dummy"
}

func (m *mockModel) CloudRegion() string {
	m.MethodCall(m, "CloudRegion")
	return "dummy-region"
}

func (m *mockModel) CloudCredentialTag() (names.CloudCredentialTag, bool) {
	m.MethodCall(m, "CloudCredentialTag")
	return names.NewCloudCredentialTag("dummy/bob/some-credential"), true
}

func (m *mockModel) Destroy(args state.DestroyModelParams) error {
	m.MethodCall(m, "Destroy", args)
	return m.NextErr()
}

func (m *mockModel) ControllerUUID() string {
	m.MethodCall(m, "ControllerUUID")
	return m.controllerUUID
}

func (m *mockModel) UUID() string {
	m.MethodCall(m, "UUID")
	return m.cfg.UUID()
}

func (m *mockModel) Name() string {
	m.MethodCall(m, "Name")
	return m.cfg.Name()
}

func (m *mockModel) MigrationMode() state.MigrationMode {
	m.MethodCall(m, "MigrationMode")
	return m.migrationStatus
}

func (m *mockModel) AddUser(spec state.UserAccessSpec) (permission.UserAccess, error) {
	m.MethodCall(m, "AddUser", spec)
	return permission.UserAccess{}, m.NextErr()
}

func (m *mockModel) getModelDetails() state.ModelSummary {
	cred, _ := m.CloudCredentialTag()
	return state.ModelSummary{
		Name:               m.Name(),
		UUID:               m.UUID(),
		Type:               m.Type(),
		Life:               m.Life(),
		Owner:              m.Owner().Id(),
		ControllerUUID:     m.ControllerUUID(),
		CloudTag:           m.CloudName(),
		CloudRegion:        m.CloudRegion(),
		CloudCredentialTag: cred.String(),
	}
}

func (m *mockModel) SetCloudCredential(tag names.CloudCredentialTag) (bool, error) {
	m.MethodCall(m, "SetCloudCredential", tag)
	return m.setCloudCredentialF(tag)
}

type mockModelUser struct {
	jujutesting.Stub
	userName    string
	displayName string
	access      permission.Access
}

type mockMigration struct {
	state.ModelMigration

	status string
	start  time.Time
	end    time.Time
}

func (m *mockMigration) StatusMessage() string {
	return m.status
}

func (m *mockMigration) StartTime() time.Time {
	return m.start
}

func (m *mockMigration) EndTime() time.Time {
	return m.end
}

type mockCloudService struct {
	clouds map[string]cloud.Cloud
}

func (m *mockCloudService) WatchCloud(ctx context.Context, name string) (watcher.NotifyWatcher, error) {
	return nil, errors.NotSupported
}

func (m *mockCloudService) Cloud(ctx context.Context, name string) (*cloud.Cloud, error) {
	cld, ok := m.clouds[name]
	if !ok {
		return nil, errors.NotFoundf("cloud %q", name)
	}
	return &cld, nil
}

func (m *mockCloudService) ListAll(ctx context.Context) ([]cloud.Cloud, error) {
	var result []cloud.Cloud
	for _, cld := range m.clouds {
		result = append(result, cld)
	}
	return result, nil
}

type mockCredentialShim struct {
	common.ModelManagerBackend
}

func (s mockCredentialShim) InvalidateModelCredential(reason string) error {
	return nil
}

type mockObjectStore struct {
	objectstore.ObjectStore
}

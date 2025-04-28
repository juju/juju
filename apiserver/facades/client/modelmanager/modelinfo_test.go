// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/user"
	coreusertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelInfoSuite struct {
	coretesting.BaseSuite
	authorizer         apiservertesting.FakeAuthorizer
	st                 *mockState
	ctlrSt             *mockState
	controllerUserInfo []coremodel.ModelUserInfo
	modelUserInfo      []coremodel.ModelUserInfo
	controllerUUID     uuid.UUID

	mockAccessService        *mocks.MockAccessService
	mockApplicationService   *mocks.MockApplicationService
	mockDomainServicesGetter *mocks.MockDomainServicesGetter
	mockMachineService       *mocks.MockMachineService
	mockModelDomainServices  *mocks.MockModelDomainServices
	mockModelService         *mocks.MockModelService
	mockSecretBackendService *mocks.MockSecretBackendService
	mockBlockCommandService  *mocks.MockBlockCommandService
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
	}

	s.controllerUserInfo = []coremodel.ModelUserInfo{{
		Name:   user.AdminUserName,
		Access: permission.AdminAccess,
	}, {
		Name:   coreusertesting.GenNewName(c, "otheruser"),
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

	s.modelUserInfo = []coremodel.ModelUserInfo{{
		Name:   user.AdminUserName,
		Access: permission.AdminAccess,
	}, {
		Name:        coreusertesting.GenNewName(c, "bob"),
		DisplayName: "Bob",
		Access:      permission.ReadAccess,
	}, {
		Name:        coreusertesting.GenNewName(c, "charlotte"),
		DisplayName: "Charlotte",
		Access:      permission.ReadAccess,
	}, {
		Name:        coreusertesting.GenNewName(c, "mary"),
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
	s.st.machines = []commonmodel.Machine{
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
	s.st.controllerNodes = []commonmodel.ControllerNode{
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

func (s *modelInfoSuite) getAPI(c *gc.C) (*modelmanager.ModelManagerAPI, *gomock.Controller) {
	api, ctrl := s.getAPIWithoutModelInfo(c)

	mockModelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(mockModelDomainServices, nil).AnyTimes()

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	mockModelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()

	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	mockModelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)

	mockModelDomainServices.EXPECT().Machine().Return(s.mockMachineService).AnyTimes()

	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(model.StatusInfo{
		Status: status.Active,
		Since:  time.Now(),
	}, nil)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		AgentVersion:   semversion.MustParse("1.99.9"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)

	return api, ctrl
}

func (s *modelInfoSuite) getAPIWithoutModelInfo(c *gc.C) (*modelmanager.ModelManagerAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.mockAccessService = mocks.NewMockAccessService(ctrl)
	s.mockModelService = mocks.NewMockModelService(ctrl)
	s.mockApplicationService = mocks.NewMockApplicationService(ctrl)
	s.mockMachineService = mocks.NewMockMachineService(ctrl)
	s.mockDomainServicesGetter = mocks.NewMockDomainServicesGetter(ctrl)

	s.mockBlockCommandService = mocks.NewMockBlockCommandService(ctrl)
	cred := cloud.NewEmptyCredential()
	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, nil, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.mockDomainServicesGetter,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": testing.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         s.mockModelService,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ApplicationService:   s.mockApplicationService,
			ObjectStore:          &mockObjectStore{},
			SecretBackendService: s.mockSecretBackendService,
			MachineService:       s.mockMachineService,
		},
		nil, common.NewBlockChecker(s.mockBlockCommandService),
		&s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})

	s.mockApplicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil).AnyTimes()
	return api, ctrl
}

func (s *modelInfoSuite) getAPIWithUser(c *gc.C, user names.UserTag) (*modelmanager.ModelManagerAPI, *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.mockSecretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.mockAccessService = mocks.NewMockAccessService(ctrl)
	s.mockModelService = mocks.NewMockModelService(ctrl)
	s.mockApplicationService = mocks.NewMockApplicationService(ctrl)
	s.mockModelDomainServices = mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter = mocks.NewMockDomainServicesGetter(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(s.mockModelDomainServices, nil).AnyTimes()
	s.mockBlockCommandService = mocks.NewMockBlockCommandService(ctrl)
	s.authorizer.Tag = user
	cred := cloud.NewEmptyCredential()
	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, nil, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.mockDomainServicesGetter,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": testing.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         s.mockModelService,
			ModelDefaultsService: nil,
			AccessService:        s.mockAccessService,
			ApplicationService:   s.mockApplicationService,
			ObjectStore:          &mockObjectStore{},
			SecretBackendService: s.mockSecretBackendService,
			MachineService:       s.mockMachineService,
		},
		nil,
		common.NewBlockChecker(s.mockBlockCommandService), s.authorizer,
	)
	c.Assert(err, jc.ErrorIsNil)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})
	s.mockApplicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil).AnyTimes()
	return api, ctrl
}

func (s *modelInfoSuite) expectedModelInfo(c *gc.C, credentialValidity *bool) params.ModelInfo {
	expectedAgentVersion := jujuversion.Current
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
			Id:         "1",
			InstanceId: "inst-deadbeef1",
			Hardware:   &params.MachineHardware{Cores: pUint64(1)},
			HasVote:    true,
			WantsVote:  true,
		}, {
			Id:         "2",
			InstanceId: "inst-deadbeef2",
			WantsVote:  true,
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
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
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
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	cores := uint64(1)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{CpuCores: &cores}, nil)

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
		{FuncName: "UUID", Args: nil},
		{FuncName: "Name", Args: nil},
		{FuncName: "Type", Args: nil},
		{FuncName: "UUID", Args: nil},
		{FuncName: "Owner", Args: nil},
		{FuncName: "Life", Args: nil},
		{FuncName: "CloudName", Args: nil},
		{FuncName: "CloudRegion", Args: nil},
		{FuncName: "CloudCredentialTag", Args: nil},
		{FuncName: "Life", Args: nil},
		{FuncName: "Status", Args: nil},
		{FuncName: "Type", Args: nil},
	})
}

func (s *modelInfoSuite) TestModelInfoWriteAccess(c *gc.C) {
	mary := names.NewUserTag("mary@local")
	s.authorizer.HasWriteTag = mary
	api, ctrl := s.getAPIWithUser(c, mary)
	defer ctrl.Finish()
	maryName := coreusertesting.GenNewName(c, "mary")

	s.mockMachineService = mocks.NewMockMachineService(ctrl)
	s.mockModelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUser(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID()), maryName).Return(
		coremodel.ModelUserInfo{
			Name:        maryName,
			DisplayName: "Mary",
			Access:      permission.WriteAccess,
		}, nil,
	)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(model.StatusInfo{
		Status: status.Active,
		Since:  time.Now(),
	}, nil)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		AgentVersion:   semversion.MustParse("1.99.9"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)
	s.mockModelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	s.mockModelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()

	info := s.getModelInfo(c, api, s.st.model.cfg.UUID())
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "mary")
	c.Assert(info.Machines, gc.HasLen, 2)
}

func (s *modelInfoSuite) TestModelInfoReadAccess(c *gc.C) {
	mary := names.NewUserTag("mary@local")
	s.authorizer.HasReadTag = mary
	api, ctrl := s.getAPIWithUser(c, mary)
	defer ctrl.Finish()
	maryName := coreusertesting.GenNewName(c, "mary")

	s.mockMachineService = mocks.NewMockMachineService(ctrl)
	s.mockModelService.EXPECT().GetModelUser(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID()), maryName).Return(
		coremodel.ModelUserInfo{
			Name:        maryName,
			DisplayName: "Mary",
			Access:      permission.ReadAccess,
		}, nil,
	)
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(model.StatusInfo{
		Status: status.Active,
		Since:  time.Now(),
	}, nil)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		AgentVersion:   semversion.MustParse("1.99.9"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)
	s.mockModelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	s.mockModelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()

	info := s.getModelInfo(c, api, s.st.model.cfg.UUID())
	c.Assert(info.Users, gc.HasLen, 1)
	c.Assert(info.Users[0].UserName, gc.Equals, "mary")
	c.Assert(info.Machines, gc.HasLen, 0)
}

func (s *modelInfoSuite) TestModelInfoNonOwner(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithUser(c, names.NewUserTag("charlotte@local"))
	defer ctrl.Finish()

	charlotteName := coreusertesting.GenNewName(c, "charlotte")
	s.mockAccessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), user.NameFromTag(charlotteName), permission.ID{
		ObjectType: permission.Model,
		Key:        s.st.model.cfg.UUID(),
	}).Return(permission.ReadAccess, nil)
	s.mockModelService.EXPECT().GetModelUser(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID()), charlotteName).Return(
		coremodel.ModelUserInfo{
			Name:        charlotteName,
			DisplayName: "Charlotte",
			Access:      permission.ReadAccess,
		}, nil,
	)
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		AgentVersion:   semversion.MustParse("1.99.9"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)
	s.mockModelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	s.mockModelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()
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
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.testModelInfoError(c, api, "user-bob", `"user-bob" is not a valid model tag`)
}

func (s *modelInfoSuite) TestModelInfoErrorGetModelNotFound(c *gc.C) {
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.st.SetErrors(errors.NotFoundf("model"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestModelInfoErrorModelConfig(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.st.model.SetErrors(errors.Errorf("no config for you"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `no config for you`)
}

func (s *modelInfoSuite) TestModelInfoErrorModelUsers(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(nil, errors.Errorf("no users for you"))
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `getting model user info: no users for you`)
}

func (s *modelInfoSuite) TestModelInfoErrorNoModelUsers(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(nil, modelerrors.UserNotFoundOnModel)
	s.st.model.users = nil
	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `getting model user info: user not found on model`)
}

func (s *modelInfoSuite) TestModelInfoErrorNoAccess(c *gc.C) {
	noAccessUser := names.NewUserTag("nemo@local")
	api, ctrl := s.getAPIWithUser(c, noAccessUser)
	defer ctrl.Finish()

	s.testModelInfoError(c, api, coretesting.ModelTag.String(), `permission denied`)
}

func (s *modelInfoSuite) TestRunningMigration(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()

	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(s.modelUserInfo, nil)
	start := time.Now().Add(-20 * time.Minute)
	s.st.migration = &mockMigration{
		status: "computing optimal bin packing",
		start:  start,
	}
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

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
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(s.modelUserInfo, nil)
	start := time.Now().Add(-20 * time.Minute)
	end := time.Now().Add(-10 * time.Minute)
	s.st.migration = &mockMigration{
		status: "couldn't realign alternate time frames",
		start:  start,
		end:    end,
	}
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

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
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(coretesting.ModelTag.Id())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	results, err := api.ModelInfo(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: coretesting.ModelTag.String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Result.Migration, gc.IsNil)
}

func (s *modelInfoSuite) TestAliveModelGetsAllInfo(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Alive, life.Alive)
}

func (s *modelInfoSuite) TestAliveModelWithGetModelInfoFailure(c *gc.C) {
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, errors.NotFoundf("model info"))

	s.st.model.life = state.Alive
	s.testModelInfoError(c, api, s.st.model.tag.String(), "model info not found")
}

func (s *modelInfoSuite) TestAliveModelWithGetModelTargetAgentVersionFailure(c *gc.C) {
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, nil)
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, errors.NotFoundf("model agent version"))

	s.st.model.life = state.Alive
	s.testModelInfoError(c, api, s.st.model.tag.String(), "model agent version not found")
}

func (s *modelInfoSuite) TestAliveModelWithStatusFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.st.model.life = state.Alive
	s.setModelStatusError(c)
	s.testModelInfoError(c, api, s.st.model.tag.String(), "status not found")
}

func (s *modelInfoSuite) TestAliveModelWithUsersFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.st.model.life = state.Alive
	s.setModelUsersError(c)
	s.testModelInfoError(c, api, s.st.model.tag.String(), "getting model user info: model not found")
}

func (s *modelInfoSuite) TestDeadModelGetsAllInfo(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Dead, life.Dead)
}

func (s *modelInfoSuite) TestDeadModelWithGetModelInfoFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, errors.NotFoundf("model info"))

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)

	modelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Dead, life.Dead)
}

func (s *modelInfoSuite) TestDeadModelWithGetModelTargetAgentVersionFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, nil)

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, errors.NotFoundf("model agent version"))

	modelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Dead, life.Dead)
}

func (s *modelInfoSuite) TestDeadModelWithStatusFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

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
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Dead,
		expectedLife: life.Dead,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestDyingModelWithGetModelInfoFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, errors.NotFoundf("model info"))

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)

	modelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Dying, life.Dying)
}

func (s *modelInfoSuite) TestDyingModelWithGetModelTargetAgentVersionFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, nil)

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, errors.NotFoundf("model agent version"))

	modelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Dying, life.Dying)
}

func (s *modelInfoSuite) TestDyingModelWithStatusFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

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
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

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
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.migrationStatus = state.MigrationModeImporting
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Alive, life.Alive)
}

func (s *modelInfoSuite) TestImportingModelWithGetModelInfoFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.migrationStatus = state.MigrationModeImporting
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, errors.NotFoundf("model info"))

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)

	modelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Alive, life.Alive)
}

func (s *modelInfoSuite) TestImportingModelWithGetModelTargetAgentVersionFailure(c *gc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	api, ctrl := s.getAPIWithoutModelInfo(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.migrationStatus = state.MigrationModeImporting
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.mockDomainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{}, nil)

	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().Agent().Return(modelAgentService)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.Zero, errors.NotFoundf("model agent version"))

	modelDomainServices.EXPECT().Machine().Return(s.mockMachineService)

	s.assertSuccess(c, api, s.st.model.cfg.UUID(), state.Alive, life.Alive)
}

func (s *modelInfoSuite) TestImportingModelWithStatusFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.mockModelService.EXPECT().GetModelUsers(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(s.modelUserInfo, nil)
	s.st.migrationStatus = state.MigrationModeImporting
	testData := incompleteModelInfoTest{
		failModel:    s.setModelStatusError,
		desiredLife:  state.Alive,
		expectedLife: life.Alive,
	}
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)
	s.assertSuccessWithMissingData(c, api, testData)
}

func (s *modelInfoSuite) TestImportingModelWithUsersFailure(c *gc.C) {
	api, ctrl := s.getAPI(c)
	defer ctrl.Finish()
	s.mockSecretBackendService.EXPECT().BackendSummaryInfoForModel(gomock.Any(), coremodel.UUID(s.st.model.cfg.UUID())).Return(nil, nil)
	s.st.migrationStatus = state.MigrationModeImporting
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("1")).Return("deadbeef1", nil)
	s.mockMachineService.EXPECT().GetMachineUUID(gomock.Any(), machine.Name("2")).Return("deadbeef2", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef1").Return("inst-deadbeef1", "", nil)
	s.mockMachineService.EXPECT().InstanceIDAndName(gomock.Any(), "deadbeef2").Return("inst-deadbeef2", "", nil)
	s.mockMachineService.EXPECT().HardwareCharacteristics(gomock.Any(), "deadbeef1").Return(&instance.HardwareCharacteristics{}, nil)

	testData := incompleteModelInfoTest{
		failModel:    s.setModelUsersError,
		desiredLife:  state.Alive,
		expectedLife: life.Alive,
	}
	s.assertSuccessWithMissingData(c, api, testData)
}

type incompleteModelInfoTest struct {
	failModel    func(*gc.C)
	desiredLife  state.Life
	expectedLife life.Value
}

func (s *modelInfoSuite) setModelStatusError(*gc.C) {
	s.st.model.SetErrors(
		errors.NotFoundf("status"), // Status
	)
}

func (s *modelInfoSuite) setModelUsersError(c *gc.C) {
	s.mockModelService.EXPECT().GetModelUsers(
		gomock.Any(),
		gomock.Any(),
	).Return(
		nil,
		modelerrors.NotFound,
	)
}

func (s *modelInfoSuite) assertSuccessWithMissingData(c *gc.C, api *modelmanager.ModelManagerAPI, test incompleteModelInfoTest) {
	test.failModel(c)
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

type mockState struct {
	jujutesting.Stub

	environs.EnvironConfigGetter
	common.APIHostPortsForAgentsGetter
	common.ToolsStorageGetter
	unitRetriever

	controllerUUID  string
	cloudUsers      map[string]permission.Access
	model           *mockModel
	controllerModel *mockModel
	machines        []commonmodel.Machine
	controllerNodes []commonmodel.ControllerNode
	migration       *mockMigration
	migrationStatus state.MigrationMode
	modelConfig     *config.Config
}

type fakeModelDescription struct {
	description.Model `yaml:"-"`

	ModelUUID string `yaml:"model-uuid"`
}

func (st *mockState) ModelUUID() string {
	st.MethodCall(st, "ModelUUID")
	return st.model.UUID()
}

func (st *mockState) Name() string {
	st.MethodCall(st, "Name")
	return "test-model"
}

func (st *mockState) ControllerModelTag() names.ModelTag {
	st.MethodCall(st, "ControllerModelTag")
	return st.controllerModel.tag
}

func (st *mockState) Export(store objectstore.ObjectStore) (description.Model, error) {
	st.MethodCall(st, "Export")
	return &fakeModelDescription{ModelUUID: st.model.UUID()}, nil
}

func (st *mockState) ExportPartial(cfg state.ExportConfig, store objectstore.ObjectStore) (description.Model, error) {
	st.MethodCall(st, "ExportPartial", cfg)
	if !cfg.IgnoreIncompleteModel {
		return nil, errors.New("expected IgnoreIncompleteModel=true")
	}
	return &fakeModelDescription{ModelUUID: st.model.UUID()}, nil
}

func (st *mockState) AllModelUUIDs() ([]string, error) {
	st.MethodCall(st, "AllModelUUIDs")
	return []string{st.model.UUID()}, st.NextErr()
}

func (st *mockState) GetBackend(modelUUID string) (commonmodel.ModelManagerBackend, func() bool, error) {
	st.MethodCall(st, "GetBackend", modelUUID)
	err := st.NextErr()
	return st, func() bool { return true }, err
}

func (st *mockState) GetModel(modelUUID string) (commonmodel.Model, func() bool, error) {
	st.MethodCall(st, "GetModel", modelUUID)
	return st.model, func() bool { return true }, st.NextErr()
}

func (st *mockState) AllApplications() ([]commonmodel.Application, error) {
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

func (st *mockState) NewModel(args state.ModelArgs) (commonmodel.Model, commonmodel.ModelManagerBackend, error) {
	st.MethodCall(st, "NewModel", args)
	st.model.tag = names.NewModelTag(args.Config.UUID())
	err := st.NextErr()
	return st.model, st, err
}

func (st *mockState) ControllerTag() names.ControllerTag {
	st.MethodCall(st, "ControllerTag")
	return names.NewControllerTag(st.controllerUUID)
}

func (st *mockState) IsController() bool {
	st.MethodCall(st, "IsController")
	return st.controllerUUID == st.model.UUID()
}

func (st *mockState) ControllerNodes() ([]commonmodel.ControllerNode, error) {
	st.MethodCall(st, "ControllerNodes")
	return st.controllerNodes, st.NextErr()
}

func (st *mockState) Model() (commonmodel.Model, error) {
	st.MethodCall(st, "Model")
	return st.model, st.NextErr()
}

func (st *mockState) ModelTag() names.ModelTag {
	st.MethodCall(st, "ModelTag")
	return st.model.ModelTag()
}

func (st *mockState) AllMachines() ([]commonmodel.Machine, error) {
	st.MethodCall(st, "AllMachines")
	return st.machines, st.NextErr()
}

func (st *mockState) Close() error {
	st.MethodCall(st, "Close")
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

func (st *mockState) HAPrimaryMachine() (names.MachineTag, error) {
	st.MethodCall(st, "HAPrimaryMachine")
	return names.MachineTag{}, nil
}

func (st *mockState) ConstraintsBySpaceName(spaceName string) ([]*state.Constraints, error) {
	st.MethodCall(st, "ConstraintsBySpaceName", spaceName)
	return nil, st.NextErr()
}

func (st *mockState) InvalidateModelCredential(reason string) error {
	st.MethodCall(st, "InvalidateModelCredential", reason)
	return nil
}

func (st *mockState) MigrationMode() (state.MigrationMode, error) {
	st.MethodCall(st, "MigrationMode")
	return st.migrationStatus, nil
}

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
	commonmodel.Machine
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
	controllerUUID      string
	isController        bool
	setCloudCredentialF func(tag names.CloudCredentialTag) (bool, error)
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
	commonmodel.ModelManagerBackend
}

func (s mockCredentialShim) InvalidateModelCredential(reason string) error {
	return nil
}

type mockObjectStore struct {
	objectstore.ObjectStore
}

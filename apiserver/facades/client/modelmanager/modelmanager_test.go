// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/migration"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	domainmodel "github.com/juju/juju/domain/model"
	"github.com/juju/juju/domain/modeldefaults"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/internal/provider/azure"
	_ "github.com/juju/juju/internal/provider/ec2"
	_ "github.com/juju/juju/internal/provider/maas"
	_ "github.com/juju/juju/internal/provider/openstack"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/version"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
)

func createArgs(owner names.UserTag) params.ModelCreateArgs {
	return params.ModelCreateArgs{
		Name:     "test-model",
		OwnerTag: owner.String(),
		Config: map[string]interface{}{
			"authorized-keys": "ssh-key",
			// And to make it a valid dummy config
			"somebool": false,
		},
	}
}

type modelManagerSuite struct {
	jtesting.IsolationSuite

	st                   *mockState
	ctlrSt               *mockState
	caasSt               *mockState
	caasBroker           *mockCaasBroker
	cloudService         *mockCloudService
	accessService        *mocks.MockAccessService
	modelService         *mocks.MockModelService
	modelDefaultService  *mocks.MockModelDefaultsService
	modelExporter        *mocks.MockModelExporter
	domainServicesGetter *mocks.MockDomainServicesGetter
	domainServices       *mocks.MockModelDomainServices
	applicationService   *mocks.MockApplicationService
	blockCommandService  *mocks.MockBlockCommandService
	authoriser           apiservertesting.FakeAuthorizer
	api                  *modelmanager.ModelManagerAPI
	caasApi              *modelmanager.ModelManagerAPI
	controllerUUID       uuid.UUID
	modelConfigService   *mocks.MockModelConfigService
	machineService       *mocks.MockMachineService
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelExporter = mocks.NewMockModelExporter(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.modelDefaultService = mocks.NewMockModelDefaultsService(ctrl)
	s.accessService = mocks.NewMockAccessService(ctrl)
	s.domainServicesGetter = mocks.NewMockDomainServicesGetter(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.blockCommandService = mocks.NewMockBlockCommandService(ctrl)
	s.machineService = mocks.NewMockMachineService(ctrl)
	s.domainServices = mocks.NewMockModelDomainServices(ctrl)

	return ctrl
}

func (s *modelManagerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.controllerUUID, err = uuid.UUIDFromString(coretesting.ControllerTag.Id())
	c.Assert(err, jc.ErrorIsNil)

	attrs := coretesting.FakeConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	controllerModel := &mockModel{
		owner: names.NewUserTag("admin"),
		life:  state.Alive,
		cfg:   cfg,
		status: status.StatusInfo{
			Status: status.Available,
			Since:  &time.Time{},
		},
		users: []*mockModelUser{{
			userName: "admin",
			access:   permission.AdminAccess,
		}, {
			userName: "add-model",
			access:   permission.AdminAccess,
		}, {
			userName: "otheruser",
			access:   permission.WriteAccess,
		}},
	}

	s.st = &mockState{
		controllerModel: controllerModel,
		model: &mockModel{
			owner: names.NewUserTag("admin"),
			life:  state.Alive,
			tag:   coretesting.ModelTag,
			cfg:   cfg,
			status: status.StatusInfo{
				Status: status.Available,
				Since:  &time.Time{},
			},
			users: []*mockModelUser{{
				userName: "admin",
				access:   permission.AdminAccess,
			}, {
				userName: "add-model",
				access:   permission.AdminAccess,
			}, {
				userName: "otheruser",
				access:   permission.WriteAccess,
			}},
		},
		modelConfig: coretesting.ModelConfig(c),
	}
	s.ctlrSt = &mockState{
		model:           s.st.model,
		controllerModel: controllerModel,
		cloudUsers:      map[string]permission.Access{},
	}

	s.caasSt = &mockState{
		controllerModel: controllerModel,
		model: &mockModel{
			owner: names.NewUserTag("admin"),
			life:  state.Alive,
			tag:   coretesting.ModelTag,
			cfg:   cfg,
			status: status.StatusInfo{
				Status: status.Available,
				Since:  &time.Time{},
			},
			users: []*mockModelUser{{
				userName: "admin",
				access:   permission.AdminAccess,
			}, {
				userName: "add-model",
				access:   permission.AdminAccess,
			}},
		},
		modelConfig: coretesting.ModelConfig(c),
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}

}

func (s *modelManagerSuite) setUpAPI(c *gc.C) *gomock.Controller {
	ctrl := s.setUpMocks(c)

	dummyCloud := cloud.Cloud{
		Name:      "dummy",
		Type:      "dummy",
		AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
		Regions: []cloud.Region{
			{Name: "dummy-region"},
			{Name: "qux"},
		},
	}

	mockK8sCloud := cloud.Cloud{
		Name:      "k8s-cloud",
		Type:      "kubernetes",
		AuthTypes: []cloud.AuthType{cloud.UserPassAuthType},
	}

	newBroker := func(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
		s.caasBroker = &mockCaasBroker{}
		return s.caasBroker, nil
	}

	s.cloudService = &mockCloudService{
		clouds: map[string]cloud.Cloud{
			"dummy": dummyCloud,
		},
	}
	cred := cloud.NewEmptyCredential()
	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, modelExporter(s.modelExporter), s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         s.cloudService,
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         s.modelService,
			ModelDefaultsService: s.modelDefaultService,
			ApplicationService:   s.applicationService,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, newBroker, common.NewBlockChecker(s.blockCommandService),
		s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	caasCred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	caasApi, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.caasSt, modelExporter(s.modelExporter), s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{
					"k8s-cloud": mockK8sCloud,
				},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&caasCred),
			ModelService:         s.modelService,
			ModelDefaultsService: s.modelDefaultService,
			AccessService:        s.accessService,
			ApplicationService:   s.applicationService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, newBroker, common.NewBlockChecker(s.blockCommandService),
		s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.caasApi = caasApi

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})

	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil).AnyTimes()
	return ctrl
}

func (s *modelManagerSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	newBroker := func(_ context.Context, args environs.OpenParams, _ environs.CredentialInvalidator) (caas.Broker, error) {
		return s.caasBroker, nil
	}
	mm, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, modelExporter(s.modelExporter), s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jujutesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(nil),
			ModelService:         s.modelService,
			ModelDefaultsService: s.modelDefaultService,
			AccessService:        s.accessService,
			ApplicationService:   s.applicationService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, newBroker, common.NewBlockChecker(s.blockCommandService),
		s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = mm
}

// expectCreateModel expects all the calls to the services made during model
// creation. It generates the calls based off the modelCreateArgs.
func (s *modelManagerSuite) expectCreateModel(
	c *gc.C,
	ctrl *gomock.Controller,
	modelCreateArgs params.ModelCreateArgs,
	expectedCloudCredential credential.Key,
	expectedCloudName string,
	expectedCloudRegion string,
) coremodel.UUID {
	modelUUID := modeltesting.GenModelUUID(c)
	userTag, err := names.ParseUserTag(modelCreateArgs.OwnerTag)
	c.Assert(err, gc.IsNil)
	ownerName := user.NameFromTag(userTag)
	ownerUUID := usertesting.GenUserUUID(c)

	// Get the default cloud name and credential.
	s.modelService.EXPECT().DefaultModelCloudNameAndCredential(
		gomock.Any()).Return("dummy", credential.Key{}, nil)
	// Get the uuid of the model owner.
	s.accessService.EXPECT().GetUserByName(
		gomock.Any(), ownerName,
	).Return(user.User{UUID: ownerUUID}, nil)

	// Create model in controller database.
	s.modelService.EXPECT().CreateModel(gomock.Any(), domainmodel.GlobalModelCreationArgs{
		Name:        modelCreateArgs.Name,
		Owner:       ownerUUID,
		Cloud:       expectedCloudName,
		CloudRegion: expectedCloudRegion,
		Credential:  expectedCloudCredential,
	}).Return(
		modelUUID,
		func(context.Context) error { return nil },
		nil,
	)

	// Create and setup model in model database.
	s.expectCreateModelOnModelDB(ctrl, modelCreateArgs.Config)

	modelConfig := map[string]any{}
	for k, v := range modelCreateArgs.Config {
		modelConfig[k] = v
	}

	modelConfig["uuid"] = modelUUID
	modelConfig["name"] = modelCreateArgs.Name
	modelConfig["type"] = expectedCloudName

	cfg, err := config.New(config.NoDefaults, modelConfig)
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil)

	// Called as part of getModelInfo which returns information to the user
	// about the newly created model.
	s.modelService.EXPECT().GetModelUsers(gomock.Any(), gomock.Any()).AnyTimes()

	return modelUUID
}

// expectCreateModelOnModelDB expects all the service calls to the new model's
// own database.
func (s *modelManagerSuite) expectCreateModelOnModelDB(
	ctrl *gomock.Controller,
	modelConfig map[string]any,
) {
	// Expect call to get the model domain services.
	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.domainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()

	// Expect calls to get various model services.
	modelInfoService := mocks.NewMockModelInfoService(ctrl)
	networkService := mocks.NewMockNetworkService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)

	s.modelConfigService = mocks.NewMockModelConfigService(ctrl)
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(modelInfoService).AnyTimes()
	modelDomainServices.EXPECT().Network().Return(networkService)
	modelDomainServices.EXPECT().Config().Return(s.modelConfigService).AnyTimes()
	modelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()
	modelDomainServices.EXPECT().Machine().Return(machineService)

	// Expect calls to functions of the model services.
	modelInfoService.EXPECT().CreateModel(gomock.Any(), s.controllerUUID)
	modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{
		Status: status.Available,
		Since:  time.Now(),
	}, nil)
	modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		// Use a version we shouldn't have now to ensure we're using the
		// ModelAgentService rather than the ModelInfo data.
		AgentVersion:   version.MustParse("2.6.5"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	s.modelConfigService.EXPECT().SetModelConfig(gomock.Any(), modelConfig)
	networkService.EXPECT().ReloadSpaces(gomock.Any())
}

func (s *modelManagerSuite) getModelArgs(c *gc.C) state.ModelArgs {
	return getModelArgsFor(c, s.st)
}

func getModelArgsFor(c *gc.C, mockState *mockState) state.ModelArgs {
	for _, v := range mockState.Calls() {
		if v.Args == nil {
			continue
		}
		if newModelArgs, ok := v.Args[0].(state.ModelArgs); ok {
			return newModelArgs
		}
	}
	c.Fatal("failed to find state.ModelArgs")
	panic("unreachable")
}

func (s *modelManagerSuite) TestCreateModelArgsWithCloud(c *gc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	cloudCredental := credential.Key{
		Cloud: "dummy",
		Owner: user.AdminUserName,
		Name:  "some-credential",
	}
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudTag:           "cloud-dummy",
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-dummy_admin_some-credential",
	}
	s.expectCreateModel(c, ctrl, args, cloudCredental, "dummy", "qux")
	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudName, gc.Equals, "dummy")
}

func (s *modelManagerSuite) TestCreateModelArgsWithCloudNotFound(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		CloudTag: "cloud-some-unknown-cloud",
	}
	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, `cloud "some-unknown-cloud" not found, expected one of \["dummy"\]`)
}

func (s *modelManagerSuite) TestCreateModelDefaultRegion(c *gc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
	}
	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")
	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudRegion, gc.Equals, "dummy-region")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdmin(c *gc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
	}
	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")
	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, gc.Equals, names.NewCloudCredentialTag(
		"dummy/bob/some-credential",
	))
}

func (s *modelManagerSuite) TestCreateModelEmptyCredentialNonAdmin(c *gc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob",
	}
	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")

	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, gc.Equals, names.CloudCredentialTag{})
}

func (s *modelManagerSuite) TestCreateModelNoDefaultCredentialNonAdmin(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	cld := s.cloudService.clouds["dummy"]
	cld.AuthTypes = nil
	s.cloudService.clouds["dummy"] = cld
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob",
	}
	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, gc.ErrorMatches, "no credential specified")
}

// TODO (tlm): Have disabled the below test as it is almost impossible to mock
// correctly while this facade is in flux. We want to move this logic back down
// into the services layer so it doesn't make a lot of sense for namespace in
// kubernetes to be created at the facade. Keep this test commented out here as
// a reminder to assert the logic when this facade is fully swapped over dqlite.

//func (s *modelManagerSuite) TestCreateCAASModelNamespaceClash(c *gc.C) {
//	ctrl := s.setUpAPI(c)
//	defer ctrl.Finish()
//
//	args := params.ModelCreateArgs{
//		Name:               "existing-ns",
//		OwnerTag:           "user-admin",
//		Config:             map[string]interface{}{},
//		CloudTag:           "cloud-k8s-cloud",
//		CloudCredentialTag: "cloudcred-k8s-cloud_admin_some-credential",
//	}
//
//	s.expectCreateModel(
//		c,
//		ctrl,
//		args,
//		credential.Key{
//			Cloud: "k8s-cloud",
//			Owner: user.AdminUserName,
//			Name:  "some-credential",
//		},
//		"k8s-cloud",
//		"",
//	)
//
//	// Expect calls to create model in domain, this has to be done before the
//	// caasBroker is called and returns the error this test looks for.
//	//modelUUID := modeltesting.GenModelUUID(c)
//	//userTag, err := names.ParseUserTag("user-admin")
//	//c.Assert(err, gc.IsNil)
//	//ownerName := user.NameFromTag(userTag)
//	//ownerUUID := usertesting.GenUserUUID(c)
//	//s.modelService.EXPECT().DefaultModelCloudNameAndCredential(
//	//	gomock.Any()).Return("dummy", credential.Key{}, nil)
//	//s.accessService.EXPECT().GetUserByName(
//	//	gomock.Any(), ownerName,
//	//).Return(user.User{UUID: ownerUUID}, nil)
//	//s.modelService.EXPECT().CreateModel(gomock.Any(), model.GlobalModelCreationArgs{
//	//	Name:        "existing-ns",
//	//	Owner:       ownerUUID,
//	//	Cloud:       "k8s-cloud",
//	//	CloudRegion: "",
//	//	Credential: credential.Key{
//	//		Cloud: "k8s-cloud",
//	//		Owner: user.AdminUserName,
//	//		Name:  "some-credential",
//	//	},
//	//}).Return(
//	//	modelUUID,
//	//	func(context.Context) error { return nil },
//	//	nil,
//	//)
//	//s.expectCreateModelOnModelDB(ctrl, map[string]any{})
//
//	_, err := s.caasApi.CreateModel(context.Background(), args)
//	s.caasBroker.CheckCallNames(c, "Create")
//	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
//}

func (s *modelManagerSuite) TestModelDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.modelDefaultService.EXPECT().CloudDefaults(gomock.Any(), "dummy").Return(modeldefaults.ModelDefaultAttributes{
		"attr": {
			Controller: "val",
			Regions: []modeldefaults.RegionDefaultValue{{
				Name:  "dummy",
				Value: "val++",
			}},
		},
		"attr2": {
			Default:    "val2",
			Controller: "val3",
			Regions: []modeldefaults.RegionDefaultValue{{
				Name:  "left",
				Value: "spam",
			}},
		},
	}, nil)

	results, err := s.api.ModelDefaultsForClouds(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := map[string]params.ModelDefaults{
		"attr": {
			Controller: "val",
			Regions: []params.RegionDefaults{{
				RegionName: "dummy",
				Value:      "val++"}}},
		"attr2": {
			Controller: "val3",
			Default:    "val2",
			Regions: []params.RegionDefaults{{
				RegionName: "left",
				Value:      "spam"}}},
	}
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Config, jc.DeepEquals, expectedValues)
}

func (s *modelManagerSuite) TestSetModelCloudDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	defaults := map[string]interface{}{
		"attr3": "val3",
		"attr4": "val4",
	}
	s.modelDefaultService.EXPECT().UpdateCloudDefaults(gomock.Any(), "test", defaults)
	params := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{CloudTag: "cloud-test", Config: defaults}},
	}
	result, err := s.api.SetModelDefaults(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestSetModelRegionDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	defaults := map[string]interface{}{
		"attr3": "val3",
		"attr4": "val4",
	}
	s.modelDefaultService.EXPECT().UpdateCloudRegionDefaults(gomock.Any(), "test", "east", defaults)
	params := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{CloudTag: "cloud-test", CloudRegion: "east", Config: defaults}},
	}
	result, err := s.api.SetModelDefaults(context.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) blockAllChanges(c *gc.C, msg string) {
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)
}

func (s *modelManagerSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelManagerSuite) TestBlockChangesSetModelDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockAllChanges(c, "TestBlockChangesSetModelDefaults")
	_, err := s.api.SetModelDefaults(context.Background(), params.SetModelDefaults{})
	s.assertBlocked(c, err, "TestBlockChangesSetModelDefaults")
}

func (s *modelManagerSuite) TestUnsetModelCloudDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	s.modelDefaultService.EXPECT().RemoveCloudDefaults(gomock.Any(), "test", []string{"attr"})
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			CloudTag: "cloud-test",
			Keys:     []string{"attr"},
		}}}
	result, err := s.api.UnsetModelDefaults(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestUnsetModelRegionDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	s.modelDefaultService.EXPECT().RemoveCloudRegionDefaults(gomock.Any(), "test", "east", []string{"attr"})
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			CloudTag:    "cloud-test",
			CloudRegion: "east",
			Keys:        []string{"attr"},
		}}}
	result, err := s.api.UnsetModelDefaults(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestBlockUnsetModelDefaults(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockAllChanges(c, "TestBlockUnsetModelDefaults")
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"abc"},
		}}}
	_, err := s.api.UnsetModelDefaults(context.Background(), args)
	s.assertBlocked(c, err, "TestBlockUnsetModelDefaults")
}

func (s *modelManagerSuite) TestModelDefaultsAsNormalUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.ModelDefaultsForClouds(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(got, gc.DeepEquals, params.ModelDefaultsResults{})
}

func (s *modelManagerSuite) TestSetModelDefaultsAsNormalUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.SetModelDefaults(context.Background(), params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config: map[string]interface{}{
				"ftp-proxy": "http://charlie",
			}}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(got, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *modelManagerSuite) TestUnsetModelDefaultsAsNormalUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.UnsetModelDefaults(context.Background(), params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr2"}}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(got, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *modelManagerSuite) TestDumpModel(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, modelExporter(s.modelExporter), s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jujutesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(nil),
			ModelService:         s.modelService,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, nil, common.NewBlockChecker(s.blockCommandService),
		s.authoriser, s.st.model,
	)
	c.Check(err, jc.ErrorIsNil)

	s.modelExporter.EXPECT().ExportModelPartial(
		gomock.Any(),
		state.ExportConfig{IgnoreIncompleteModel: true},
		gomock.Any(),
	).Times(1).Return(
		&fakeModelDescription{ModelUUID: s.st.model.UUID()},
		nil)
	results := api.DumpModels(context.Background(), params.DumpModelRequest{
		Entities: []params.Entity{{
			Tag: "bad-tag",
		}, {
			Tag: "application-foo",
		}, {
			Tag: s.st.ModelTag().String(),
		}}})

	c.Assert(results.Results, gc.HasLen, 3)
	bad, notApp, good := results.Results[0], results.Results[1], results.Results[2]
	c.Check(bad.Result, gc.Equals, "")
	c.Check(bad.Error.Message, gc.Equals, `"bad-tag" is not a valid tag`)

	c.Check(notApp.Result, gc.Equals, "")
	c.Check(notApp.Error.Message, gc.Equals, `"application-foo" is not a valid model tag`)

	c.Check(good.Error, gc.IsNil)
	c.Check(good.Result, jc.DeepEquals, "model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d\n")
}

func (s *modelManagerSuite) TestDumpModelMissingModel(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.st.SetErrors(errors.NotFoundf("boom"))
	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f000")
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: tag.String()}}}
	results := s.api.DumpModels(context.Background(), models)
	s.st.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "ControllerTag", Args: nil},
		{FuncName: "GetBackend", Args: []interface{}{tag.Id()}},
	})
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Result, gc.Equals, "")
	c.Assert(result.Error, gc.NotNil)
	c.Check(result.Error.Code, gc.Equals, `not found`)
	c.Check(result.Error.Message, gc.Equals, `id not found`)
}

func (s *modelManagerSuite) TestDumpModelUsers(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: s.st.ModelTag().String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModels(context.Background(), models)
		c.Assert(results.Results, gc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, gc.Equals, "")
		c.Assert(result.Error, gc.NotNil)
		c.Check(result.Error.Message, gc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestDumpModelsDB(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	results := s.api.DumpModelsDB(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: "application-foo",
	}, {
		Tag: s.st.ModelTag().String(),
	}}})

	c.Assert(results.Results, gc.HasLen, 3)
	bad, notApp, good := results.Results[0], results.Results[1], results.Results[2]
	c.Check(bad.Result, gc.IsNil)
	c.Check(bad.Error.Message, gc.Equals, `"bad-tag" is not a valid tag`)

	c.Check(notApp.Result, gc.IsNil)
	c.Check(notApp.Error.Message, gc.Equals, `"application-foo" is not a valid model tag`)

	c.Check(good.Error, gc.IsNil)
	c.Check(good.Result, jc.DeepEquals, map[string]interface{}{
		"models": "lots of data",
	})
}

func (s *modelManagerSuite) TestDumpModelsDBMissingModel(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.st.SetErrors(errors.NotFoundf("boom"))
	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f000")
	models := params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
	results := s.api.DumpModelsDB(context.Background(), models)

	s.st.CheckCalls(c, []jtesting.StubCall{
		{FuncName: "ControllerTag", Args: nil},
		{FuncName: "ModelTag", Args: nil},
		{FuncName: "GetBackend", Args: []interface{}{tag.Id()}},
	})
	c.Assert(results.Results, gc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Result, gc.IsNil)
	c.Assert(result.Error, gc.NotNil)
	c.Check(result.Error.Code, gc.Equals, `not found`)
	c.Check(result.Error.Message, gc.Equals, `id not found`)
}

func (s *modelManagerSuite) TestDumpModelsDBUsers(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	models := params.Entities{Entities: []params.Entity{{Tag: s.st.ModelTag().String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModelsDB(context.Background(), models)
		c.Assert(results.Results, gc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, gc.IsNil)
		c.Assert(result.Error, gc.NotNil)
		c.Check(result.Error.Message, gc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestAddModelCantCreateModelForSomeoneElse(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	addModelUser := names.NewUserTag("add-model")

	s.setAPIUser(c, addModelUser)
	nonAdminUser := names.NewUserTag("non-admin")
	_, err := s.api.CreateModel(context.Background(), createArgs(nonAdminUser))
	c.Assert(err, gc.ErrorMatches, "\"add-model\" permission does not permit creation of models for different owners: permission denied")
}

func (s *modelManagerSuite) TestUpdatedModel(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	as := s.accessService.EXPECT()
	modelUUID := modeltesting.GenModelUUID(c).String()
	testUser := names.NewUserTag("foobar")
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        modelUUID,
			},
			Access: permission.WriteAccess,
		},
		Change:  permission.Grant,
		Subject: user.NameFromTag(testUser),
	}
	as.UpdatePermission(gomock.Any(), updateArgs).Return(nil)

	s.setAPIUser(c, jujutesting.AdminUser)

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{
			{
				UserTag:  testUser.String(),
				Action:   params.GrantModelAccess,
				Access:   params.ModelWriteAccess,
				ModelTag: names.NewModelTag(modelUUID).String(),
			},
		}}

	results, err := s.api.ModifyModelAccess(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results.Results, gc.HasLen, 1)
	c.Check(results.OneError(), jc.ErrorIsNil)
}

// modelManagerStateSuite contains end-to-end tests.
// Prefer adding tests to modelManagerSuite above.
type modelManagerStateSuite struct {
	jujutesting.ApiServerSuite

	modelmanager *modelmanager.ModelManagerAPI
	authoriser   apiservertesting.FakeAuthorizer

	controllerConfigService *mocks.MockControllerConfigService
	accessService           *mocks.MockAccessService
	modelService            *mocks.MockModelService
	modelInfoService        *mocks.MockModelInfoService
	applicationService      *mocks.MockApplicationService
	domainServicesGetter    *mocks.MockDomainServicesGetter
	blockCommandService     *mocks.MockBlockCommandService

	store objectstore.ObjectStore

	controllerUUID uuid.UUID
}

var _ = gc.Suite(&modelManagerStateSuite{})

func (s *modelManagerStateSuite) SetUpSuite(c *gc.C) {
	coretesting.SkipUnlessControllerOS(c)
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *modelManagerStateSuite) SetUpTest(c *gc.C) {
	s.controllerUUID = uuid.MustNewUUID()

	s.ControllerModelConfigAttrs = map[string]interface{}{
		"agent-version": jujuversion.Current.String(),
	}
	s.ApiServerSuite.SetUpTest(c)
	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: jujutesting.AdminUser,
	}

	s.store = jujutesting.NewObjectStore(c, s.ControllerModelUUID())

	loggo.GetLogger("juju.apiserver.modelmanager").SetLogLevel(loggo.TRACE)
}

func (s *modelManagerStateSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.accessService = mocks.NewMockAccessService(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.modelInfoService = mocks.NewMockModelInfoService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.domainServicesGetter = mocks.NewMockDomainServicesGetter(ctrl)
	s.blockCommandService = mocks.NewMockBlockCommandService(ctrl)

	var fs assumes.FeatureSet
	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).AnyTimes().Return(fs, nil)

	return ctrl
}

func (s *modelManagerStateSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	st := commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool())
	ctlrSt := commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool())

	domainServices := s.ControllerDomainServices(c)

	urlGetter := common.NewToolsURLGetter(st.ModelUUID(), ctlrSt)
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	configGetter := stateenvirons.EnvironConfigGetter{
		Model:              s.ControllerModel(c),
		ModelConfigService: domainServices.Config(),
		CloudService:       domainServices.Cloud(),
		CredentialService:  domainServices.Credential(),
	}
	newEnviron := common.EnvironFuncForModel(model, domainServices.Cloud(), domainServices.Credential(), configGetter)
	toolsFinder := common.NewToolsFinder(s.controllerConfigService, st, urlGetter, newEnviron, s.store)
	modelmanager, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{ModelManagerBackend: st}, nil, ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelService:         s.modelService,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
			ApplicationService:   s.applicationService,
		},
		toolsFinder,
		nil,
		common.NewBlockChecker(s.blockCommandService),
		s.authoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.modelmanager = modelmanager
}

// expectCreateModelStateSuite expects all the calls to the services made during
// model creation. Since this is the state suite, we are not explicitly
// testing the services calls here so these expectations are quite permissive.
func (s *modelManagerStateSuite) expectCreateModelStateSuite(
	c *gc.C,
	ctrl *gomock.Controller,
	modelCreateArgs params.ModelCreateArgs,
) {
	modelUUID := modeltesting.GenModelUUID(c)
	userTag, err := names.ParseUserTag(modelCreateArgs.OwnerTag)
	c.Assert(err, gc.IsNil)
	ownerName := user.NameFromTag(userTag)
	ownerUUID := usertesting.GenUserUUID(c)

	// Get the default cloud name and credential.
	s.modelService.EXPECT().DefaultModelCloudNameAndCredential(
		gomock.Any()).Return("dummy", credential.Key{}, nil)
	// Get the uuid of the model owner.
	s.accessService.EXPECT().GetUserByName(
		gomock.Any(), ownerName,
	).Return(user.User{UUID: ownerUUID}, nil)

	// Create model in controller database.
	s.modelService.EXPECT().CreateModel(gomock.Any(), domainmodel.GlobalModelCreationArgs{
		Name:        modelCreateArgs.Name,
		Owner:       ownerUUID,
		Cloud:       "dummy",
		CloudRegion: "dummy-region",
		Credential:  credential.Key{},
	}).Return(
		modelUUID,
		func(context.Context) error { return nil },
		nil,
	)

	modelConfig := map[string]any{}
	for k, v := range modelCreateArgs.Config {
		modelConfig[k] = v
	}

	modelConfig["uuid"] = modelUUID
	modelConfig["name"] = modelCreateArgs.Name
	modelConfig["type"] = "dummy"

	cfg, err := config.New(config.NoDefaults, modelConfig)
	c.Assert(err, jc.ErrorIsNil)

	// Expect call to get the model domain services
	modelDomainServices := mocks.NewMockModelDomainServices(ctrl)
	s.domainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()

	// Expect calls to get various model services.
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelConfigService := mocks.NewMockModelConfigService(ctrl)
	networkService := mocks.NewMockNetworkService(ctrl)
	machineService := mocks.NewMockMachineService(ctrl)

	modelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()
	modelDomainServices.EXPECT().Config().Return(modelConfigService).AnyTimes()
	modelDomainServices.EXPECT().ModelInfo().Return(s.modelInfoService).AnyTimes()
	modelDomainServices.EXPECT().Network().Return(networkService)
	modelDomainServices.EXPECT().Machine().Return(machineService)

	blockCommandService := mocks.NewMockBlockCommandService(ctrl)
	modelDomainServices.EXPECT().BlockCommand().Return(blockCommandService).AnyTimes()

	// Expect calls to functions of the model services.
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	modelConfigService.EXPECT().SetModelConfig(gomock.Any(), gomock.Any())
	modelConfigService.EXPECT().ModelConfig(gomock.Any()).Return(cfg, nil).AnyTimes()
	s.modelInfoService.EXPECT().CreateModel(gomock.Any(), s.controllerUUID)
	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{
		Status: status.Active,
		Since:  time.Now(),
	}, nil)
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
		// Use a version we shouldn't have now to ensure we're using the
		// ModelAgentService rather than the ModelInfo data.
		AgentVersion:   version.MustParse("2.6.5"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)
	networkService.EXPECT().ReloadSpaces(gomock.Any())

	blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return("", blockcommanderrors.NotFound).AnyTimes()

	// Called as part of getModelInfo which returns information to the user
	// about the newly created model.
	s.modelService.EXPECT().GetModelUsers(gomock.Any(), gomock.Any()).AnyTimes()
}

func (s *modelManagerStateSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	st := commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool())
	domainServices := s.ControllerDomainServices(c)

	endPoint, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{st},
		nil,
		commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelService:         s.modelService,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, nil, common.NewBlockChecker(s.blockCommandService), anAuthoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *modelManagerStateSuite) createArgsForVersion(c *gc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := createArgs(owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerStateSuite) TestUserCanCreateModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)
	model, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
	c.Assert(model.Type, gc.Equals, "iaas")
}

func (s *modelManagerStateSuite) TestAdminCanCreateModelForSomeoneElse(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.setAPIUser(c, jujutesting.AdminUser)
	owner := names.NewUserTag("external@remote")
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)

	model, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
	c.Assert(model.Type, gc.Equals, "iaas")

	newState, err := s.StatePool().Get(model.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer newState.Release()

	newModel, err := newState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newModel.Owner(), gc.Equals, owner)
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *gc.C) {
	defer s.setupMocks(c).Finish()

	userTag := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, userTag)

	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(context.Background(), createArgs(owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSelf(c *gc.C) {
	defer s.setupMocks(c).Finish()

	owner := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, owner)

	_, err := s.modelmanager.CreateModel(context.Background(), createArgs(owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	admin := jujutesting.AdminUser
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, jujuversion.Current.String())
	s.expectCreateModelStateSuite(c, ctrl, args)
	_, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

// TODO (tlm): Re-implement under DQlite
//func (s *modelManagerStateSuite) TestCreateModelBadAgentVersion(c *gc.C) {
//	ctrl := s.setupMocks(c)
//	defer ctrl.Finish()
//	err := s.ControllerModel(c).State().SetModelAgentVersion(coretesting.FakeVersionNumber, nil, false, stubUpgrader{})
//	c.Assert(err, jc.ErrorIsNil)
//
//	admin := jujutesting.AdminUser
//	s.setAPIUser(c, admin)
//
//	bigger := coretesting.FakeVersionNumber
//	bigger.Minor += 1
//
//	smaller := coretesting.FakeVersionNumber
//	smaller.Minor -= 1
//
//	for i, test := range []struct {
//		value    interface{}
//		errMatch string
//	}{
//		{
//			value:    42,
//			errMatch: `failed to create config: agent-version must be a string but has type 'int'`,
//		}, {
//			value:    "not a number",
//			errMatch: `failed to create config: invalid version \"not a number\"`,
//		}, {
//			value:    bigger.String(),
//			errMatch: "failed to create config: agent-version .* cannot be greater than the controller .*",
//		}, {
//			value:    smaller.String(),
//			errMatch: "failed to create config: no agent binaries found for version .*",
//		},
//	} {
//		c.Logf("test %d", i)
//		args := s.createArgsForVersion(c, admin, test.value)
//		s.expectCreateModelStateSuite(c, ctrl, args)
//		_, err := s.modelmanager.CreateModel(context.Background(), args)
//		c.Check(err, gc.ErrorMatches, test.errMatch)
//	}
//}

// TODO (tlm): Re-implement under DQlite
//func (s *modelManagerStateSuite) TestListModelsAdminSelf(c *gc.C) {
//	defer s.setupMocks(c).Finish()
//
//	userUUID := usertesting.GenUserUUID(c)
//	userTag := jujutesting.AdminUser
//	user := coreuser.User{
//		UUID: userUUID,
//	}
//	s.setAPIUser(c, userTag)
//	s.accessService.EXPECT().GetUserByName(gomock.Any(), userTag.Name()).Return(user, nil)
//	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return([]coremodel.Model{}, nil)
//	result, err := s.modelmanager.ListModels(context.Background(), params.Entity{Tag: userTag.String()})
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(result.UserModels, gc.HasLen, 1)
//	//expected, err := s.ControllerModel(c).State().Model()
//	//c.Assert(err, jc.ErrorIsNil)
//	//s.checkModelMatches(c, result.UserModels[0].Model, expected)
//}
//
//func (s *modelManagerStateSuite) TestListModelsAdminListsOther(c *gc.C) {
//	defer s.setupMocks(c).Finish()
//
//	user := jujutesting.AdminUser
//	s.setAPIUser(c, user)
//	other := names.NewUserTag("admin")
//	result, err := s.modelmanager.ListModels(context.Background(), params.Entity{Tag: other.String()})
//	c.Assert(err, jc.ErrorIsNil)
//	c.Assert(result.UserModels, gc.HasLen, 1)
//}
//
//func (s *modelManagerStateSuite) TestListModelsDenied(c *gc.C) {
//	defer s.setupMocks(c).Finish()
//
//	user := names.NewUserTag("external@remote")
//	s.setAPIUser(c, user)
//	other := names.NewUserTag("other@remote")
//	_, err := s.modelmanager.ListModels(context.Background(), params.Entity{Tag: other.String()})
//	c.Assert(err, gc.ErrorMatches, "permission denied")
//}

func (s *modelManagerStateSuite) TestAdminModelManager(c *gc.C) {
	defer s.setupMocks(c).Finish()

	user := jujutesting.AdminUser
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), jc.IsTrue)
}

func (s *modelManagerStateSuite) TestNonAdminModelManager(c *gc.C) {
	defer s.setupMocks(c).Finish()

	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), jc.IsFalse)
}

func (s *modelManagerStateSuite) TestDestroyOwnModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	domainServices := s.ControllerDomainServices(c)

	// TODO(perrito666) this test is not valid until we have
	// proper controller permission since the only users that
	// can create models are controller admins.
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)
	m, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.StatePool().Get(m.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	backend := commonmodel.NewModelManagerBackend(model, s.StatePool())

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{ModelManagerBackend: backend},
		nil,
		commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, nil, common.NewBlockChecker(s.blockCommandService), s.authoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	force := true
	timeout := time.Minute
	results, err := s.modelmanager.DestroyModels(context.Background(), params.DestroyModelsParams{
		Models: []params.DestroyModelParams{{
			ModelTag: "model-" + m.UUID,
			Force:    &force,
			Timeout:  &timeout,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	model, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Not(gc.Equals), state.Alive)
	gotTimeout := model.DestroyTimeout()
	c.Assert(gotTimeout, gc.NotNil)
	c.Assert(*gotTimeout, gc.Equals, timeout)
	gotForce := model.ForceDestroyed()
	c.Assert(gotForce, jc.IsTrue)
}

func (s *modelManagerStateSuite) TestAdminDestroysOtherModel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// TODO(perrito666) Both users are admins in this case, this tesst is of dubious
	// usefulness until proper controller permissions are in place.
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)
	m, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.StatePool().Get(m.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.authoriser.Tag = jujutesting.AdminUser
	backend := commonmodel.NewModelManagerBackend(model, s.StatePool())

	domainServices := s.ControllerDomainServices(c)

	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{Status: status.Available}, nil)

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{backend},
		nil,
		commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, nil, common.NewBlockChecker(s.blockCommandService), s.authoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.modelmanager.DestroyModels(context.Background(), params.DestroyModelsParams{
		Models: []params.DestroyModelParams{{
			ModelTag: "model-" + m.UUID,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	s.authoriser.Tag = owner
	model, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Not(gc.Equals), state.Alive)
}

func (s *modelManagerStateSuite) TestDestroyModelErrors(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	owner := names.NewUserTag(user.AdminUserName.Name())
	s.setAPIUser(c, owner)
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)
	m, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.StatePool().Get(m.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	domainServices := s.ControllerDomainServices(c)

	backend := commonmodel.NewModelManagerBackend(model, s.StatePool())
	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{backend},
		nil,
		commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, nil, common.NewBlockChecker(s.blockCommandService), s.authoriser, s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	user := names.NewUserTag("other@remote")
	s.setAPIUser(c, user)

	results, err := s.modelmanager.DestroyModels(context.Background(), params.DestroyModelsParams{
		Models: []params.DestroyModelParams{
			{ModelTag: "model-" + m.UUID},
			{ModelTag: "model-9f484882-2f18-4fd2-967d-db9663db7bea"},
			{ModelTag: "machine-42"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, jc.DeepEquals, []params.ErrorResult{{
		// we don't have admin access to the model
		Error: &params.Error{
			Message: "permission denied",
			Code:    params.CodeUnauthorized,
		},
	}, {
		Error: &params.Error{
			Message: `model "9f484882-2f18-4fd2-967d-db9663db7bea" not found`,
			Code:    params.CodeNotFound,
		},
	}, {
		Error: &params.Error{
			Message: `"machine-42" is not a valid model tag`,
		},
	}})

	s.setAPIUser(c, owner)
	model, err = st.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Alive)
}

func (s *modelManagerStateSuite) TestModifyModelAccessEmptyArgs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.setAPIUser(c, jujutesting.AdminUser)
	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{{}}}

	result, err := s.modelmanager.ModifyModelAccess(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	expectedErr := `could not modify model access: "" is not a valid tag`
	c.Assert(result.OneError(), gc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestModelInfoForMigratedModel(c *gc.C) {
	user := names.NewUserTag("admin")

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()

	modelState := f.MakeModel(c, &factory.ModelParams{
		Owner: user,
	})
	defer modelState.Close()
	model, err := modelState.Model()
	c.Assert(err, jc.ErrorIsNil)

	// Migrate the model and delete it from the state
	mig, err := modelState.CreateMigration(state.MigrationSpec{
		InitiatedBy: user,
		TargetInfo: migration.TargetInfo{
			ControllerTag:   names.NewControllerTag(uuid.MustNewUUID().String()),
			ControllerAlias: "target",
			Addrs:           []string{"1.2.3.4:5555"},
			CACert:          coretesting.CACert,
			AuthTag:         names.NewUserTag("user2"),
			Password:        "secret",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	for _, phase := range migration.SuccessfulMigrationPhases() {
		c.Assert(mig.SetPhase(phase), jc.ErrorIsNil)
	}
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(modelState.RemoveDyingModel(), jc.ErrorIsNil)

	domainServices := s.ControllerDomainServices(c)

	anAuthoriser := s.authoriser
	anAuthoriser.Tag = user
	st := commonmodel.NewUserAwareModelManagerBackend(model, s.StatePool(), user)
	endPoint, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{st},
		nil,
		commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CloudService:         domainServices.Cloud(),
			CredentialService:    domainServices.Credential(),
			ModelService:         s.modelService,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		nil, nil, common.NewBlockChecker(s.blockCommandService), anAuthoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)

	res, err := endPoint.ModelInfo(
		context.Background(),
		params.Entities{
			Entities: []params.Entity{
				{Tag: model.ModelTag().String()},
			},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res.Results, gc.HasLen, 1)
	resErr0 := errors.Cause(res.Results[0].Error)
	c.Assert(params.IsRedirect(resErr0), gc.Equals, true)

	pErr, ok := resErr0.(*params.Error)
	c.Assert(ok, gc.Equals, true)

	var info params.RedirectErrorInfo
	c.Assert(pErr.UnmarshalInfo(&info), jc.ErrorIsNil)

	nhp := params.HostPort{
		Address: params.Address{
			Value: "1.2.3.4",
			Type:  string(network.IPv4Address),
			Scope: string(network.ScopePublic),
		},
		Port: 5555,
	}
	c.Assert(info.Servers, jc.DeepEquals, [][]params.HostPort{{nhp}})
	c.Assert(info.CACert, gc.Equals, coretesting.CACert)
	c.Assert(info.ControllerAlias, gc.Equals, "target")
}

func (s *modelManagerSuite) TestModelStatus(c *gc.C) {
	defer s.setUpAPI(c).Finish()

	s.domainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(s.domainServices, nil).AnyTimes()
	s.domainServices.EXPECT().Machine().Return(s.machineService).AnyTimes()

	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ModelStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: s.st.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.api.ModelStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.st.ModelTag().String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we return successfully if no errors.
	results, err = s.api.ModelStatus(context.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.st.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *modelManagerSuite) TestChangeModelCredential(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return true, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredentialBulkUninterrupted(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return true, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: "bad-model-tag"},
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `"bad-model-tag" is not a valid tag`)
	c.Assert(results.Results[1].Error, gc.IsNil)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String()},
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: "bad-credential-tag"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, `"bad-credential-tag" is not a valid tag`)
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorisedUser(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `permission denied`)
}

func (s *modelManagerSuite) TestChangeModelCredentialGetModelFail(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	s.st.SetErrors(errors.New("getting model"))
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()

	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `getting model`)
	s.st.CheckCallNames(c, "ControllerTag", "ModelTag", "ControllerTag", "GetModel")
}

func (s *modelManagerSuite) TestChangeModelCredentialNotUpdated(c *gc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return false, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `model deadbeef-0bad-400d-8000-4b1d0d06f00d already uses credential foo/bob/bar`)
}

func modelExporter(exporter *mocks.MockModelExporter) func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (modelmanager.ModelExporter, error) {
	return func(context.Context, coremodel.UUID, facade.LegacyStateExporter) (modelmanager.ModelExporter, error) {
		return exporter, nil
	}
}

type fakeProvider struct {
	environs.CloudEnvironProvider
}

func (*fakeProvider) Validate(_ context.Context, cfg, old *config.Config) (*config.Config, error) {
	return cfg, nil
}

func (*fakeProvider) PrepareForCreateEnvironment(controllerUUID string, cfg *config.Config) (*config.Config, error) {
	return cfg, nil
}

func init() {
	environs.RegisterProvider("fake", &fakeProvider{})
}

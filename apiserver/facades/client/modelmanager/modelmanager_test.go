// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	commonmodel "github.com/juju/juju/apiserver/common/model"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
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
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
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
	testhelpers.IsolationSuite

	st                   *mockState
	ctlrSt               *mockState
	caasSt               *mockState
	accessService        *mocks.MockAccessService
	modelService         *mocks.MockModelService
	modelDefaultService  *mocks.MockModelDefaultsService
	modelExporter        *mocks.MockModelExporter
	domainServicesGetter *mocks.MockDomainServicesGetter
	domainServices       *mocks.MockModelDomainServices
	applicationService   *mocks.MockApplicationService
	blockCommandService  *mocks.MockBlockCommandService
	modelInfoService     *mocks.MockModelInfoService
	authoriser           apiservertesting.FakeAuthorizer
	api                  *modelmanager.ModelManagerAPI
	caasApi              *modelmanager.ModelManagerAPI
	controllerUUID       uuid.UUID
	modelConfigService   *mocks.MockModelConfigService
	machineService       *mocks.MockMachineService

	modelStatusAPI *mocks.MockModelStatusAPI
}

var _ = tc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) setUpMocks(c *tc.C) *gomock.Controller {
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
	s.modelStatusAPI = mocks.NewMockModelStatusAPI(ctrl)

	return ctrl
}

func (s *modelManagerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.controllerUUID, err = uuid.UUIDFromString(coretesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	attrs := coretesting.FakeConfig()
	attrs["agent-version"] = jujuversion.Current.String()
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)

	controllerModel := &mockModel{
		owner: names.NewUserTag("admin"),
		life:  state.Alive,
		cfg:   cfg,
		status: status.StatusInfo{
			Status: status.Available,
			Since:  &time.Time{},
		},
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
		},
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
		},
	}

	s.authoriser = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}

}

func (s *modelManagerSuite) setUpAPI(c *tc.C) *gomock.Controller {
	ctrl := s.setUpMocks(c)

	cred := cloud.NewEmptyCredential()
	apiUser, _ := s.authoriser.GetAuthTag().(names.UserTag)
	s.api = modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st,
		true,
		apiUser,
		s.modelStatusAPI,
		modelExporter(s.modelExporter),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         s.modelService,
			ModelDefaultsService: s.modelDefaultService,
			ApplicationService:   s.applicationService,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		common.NewBlockChecker(s.blockCommandService),
		s.authoriser,
	)
	caasCred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	s.caasApi = modelmanager.NewModelManagerAPI(
		context.Background(),
		s.caasSt,
		true,
		apiUser,
		s.modelStatusAPI,
		modelExporter(s.modelExporter),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CredentialService:    apiservertesting.ConstCredentialGetter(&caasCred),
			ModelService:         s.modelService,
			ModelDefaultsService: s.modelDefaultService,
			AccessService:        s.accessService,
			ApplicationService:   s.applicationService,
			ObjectStore:          &mockObjectStore{},
		},
		common.NewBlockChecker(s.blockCommandService),
		s.authoriser,
	)

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})

	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil).AnyTimes()
	return ctrl
}

func (s *modelManagerSuite) setAPIUser(c *tc.C, user names.UserTag) {
	s.authoriser.Tag = user
	s.api = modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st,
		false,
		user,
		s.modelStatusAPI,
		modelExporter(s.modelExporter),
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CredentialService:    apiservertesting.ConstCredentialGetter(nil),
			ModelService:         s.modelService,
			ModelDefaultsService: s.modelDefaultService,
			AccessService:        s.accessService,
			ApplicationService:   s.applicationService,
			ObjectStore:          &mockObjectStore{},
		},
		common.NewBlockChecker(s.blockCommandService),
		s.authoriser,
	)
}

// generateModelUUIDAndTag generates a model UUID and tag for testing. This is
// a simple convenience function to avoid having to first generate a model uuid
// then cast it into a tag. This function does not setup any preconditions in
// testing states.
func generateModelUUIDAndTag(c *tc.C) (coremodel.UUID, names.ModelTag) {
	modelUUID := modeltesting.GenModelUUID(c)
	return modelUUID, names.NewModelTag(modelUUID.String())
}

// expectCreateModel expects all the calls to the services made during model
// creation. It generates the calls based off the modelCreateArgs.
func (s *modelManagerSuite) expectCreateModel(
	c *tc.C,
	ctrl *gomock.Controller,
	modelCreateArgs params.ModelCreateArgs,
	expectedCloudCredential credential.Key,
	expectedCloudName string,
	expectedCloudRegion string,
) coremodel.UUID {
	modelUUID := modeltesting.GenModelUUID(c)
	userTag, err := names.ParseUserTag(modelCreateArgs.OwnerTag)
	c.Assert(err, tc.IsNil)
	ownerName := user.NameFromTag(userTag)
	ownerUUID := usertesting.GenUserUUID(c)

	defaultCred := credential.Key{
		Cloud: "dummy",
		Owner: user.AdminUserName,
		Name:  "some-credential",
	}

	// Get the default cloud name and credential.
	s.modelService.EXPECT().DefaultModelCloudInfo(
		gomock.Any()).Return("dummy", "dummy-region", nil)
	// Get the uuid of the model owner.
	s.accessService.EXPECT().GetUserUUIDByName(
		gomock.Any(), ownerName,
	).Return(ownerUUID, nil)

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

	expectedModelInfo := coremodel.Model{
		Name:        "foo",
		UUID:        modelUUID,
		Owner:       ownerUUID,
		OwnerName:   ownerName,
		Cloud:       expectedCloudName,
		CloudRegion: expectedCloudRegion,
	}
	if expectedCloudCredential.IsZero() {
		expectedModelInfo.Credential = defaultCred
	} else {
		expectedModelInfo.Credential = expectedCloudCredential
	}
	s.modelService.EXPECT().Model(gomock.Any(), modelUUID).Return(expectedModelInfo, nil)

	// Create and setup model in model database.
	s.expectCreateModelOnModelDB(ctrl, modelCreateArgs.Config)

	modelConfig := map[string]any{}
	for k, v := range modelCreateArgs.Config {
		modelConfig[k] = v
	}

	modelConfig["uuid"] = modelUUID
	modelConfig["name"] = modelCreateArgs.Name
	modelConfig["type"] = expectedCloudName

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
	s.modelInfoService = mocks.NewMockModelInfoService(ctrl)
	networkService := mocks.NewMockNetworkService(ctrl)

	s.modelConfigService = mocks.NewMockModelConfigService(ctrl)
	modelAgentService := mocks.NewMockModelAgentService(ctrl)
	modelDomainServices.EXPECT().ModelInfo().Return(s.modelInfoService).AnyTimes()
	modelDomainServices.EXPECT().Network().Return(networkService)
	modelDomainServices.EXPECT().Config().Return(s.modelConfigService).AnyTimes()
	modelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()

	// Expect calls to functions of the model services.
	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{
		Status: status.Available,
		Since:  time.Now(),
	}, nil)
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		// Use a version we shouldn't have now to ensure we're using the
		// ModelAgentService rather than the ModelInfo data.
		AgentVersion:   semversion.MustParse("2.6.5"),
		ControllerUUID: s.controllerUUID,
		Cloud:          "dummy",
		CloudType:      "dummy",
	}, nil)
	modelAgentService.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(jujuversion.Current, nil)
	s.modelConfigService.EXPECT().SetModelConfig(gomock.Any(), modelConfig)
	networkService.EXPECT().ReloadSpaces(gomock.Any())
}

func (s *modelManagerSuite) getModelArgs(c *tc.C) state.ModelArgs {
	return getModelArgsFor(c, s.st)
}

func getModelArgsFor(c *tc.C, mockState *mockState) state.ModelArgs {
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

func (s *modelManagerSuite) TestCreateModelArgsWithCloud(c *tc.C) {
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
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)

	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudName, tc.Equals, "dummy")
}

func (s *modelManagerSuite) TestCreateModelDefaultRegion(c *tc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
	}

	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)

	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudRegion, tc.Equals, "dummy-region")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdmin(c *tc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
	}

	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)

	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, tc.Equals, names.NewCloudCredentialTag(
		"dummy/admin/some-credential",
	))
}

func (s *modelManagerSuite) TestCreateModelArgsWithAgentVersion(c *tc.C) {
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
			"bar":                  "baz",
			config.AgentVersionKey: jujuversion.Current.String(),
		},
		CloudTag:           "cloud-dummy",
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-dummy_admin_some-credential",
	}

	s.expectCreateModel(c, ctrl, args, cloudCredental, "dummy", "qux")
	s.modelInfoService.EXPECT().CreateModelWithAgentVersion(gomock.Any(), jujuversion.Current).Return(nil)

	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudName, tc.Equals, "dummy")
}

func (s *modelManagerSuite) TestCreateModelArgsWithAgentVersionAndStream(c *tc.C) {
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
			"bar":                  "baz",
			config.AgentVersionKey: jujuversion.Current.String(),
			config.AgentStreamKey:  "released",
		},
		CloudTag:           "cloud-dummy",
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-dummy_admin_some-credential",
	}

	s.expectCreateModel(c, ctrl, args, cloudCredental, "dummy", "qux")
	s.modelInfoService.EXPECT().CreateModelWithAgentVersionStream(gomock.Any(), jujuversion.Current, coreagentbinary.AgentStreamReleased).Return(nil)

	_, err := s.api.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudName, tc.Equals, "dummy")
}

// TODO (tlm): Have disabled the below test as it is almost impossible to mock
// correctly while this facade is in flux. We want to move this logic back down
// into the services layer so it doesn't make a lot of sense for namespace in
// kubernetes to be created at the facade. Keep this test commented out here as
// a reminder to assert the logic when this facade is fully swapped over dqlite.

//func (s *modelManagerSuite) TestCreateCAASModelNamespaceClash(c *tc.C) {
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
//	//c.Assert(err, tc.IsNil)
//	//ownerName := user.NameFromTag(userTag)
//	//ownerUUID := usertesting.GenUserUUID(c)
//	//s.modelService.EXPECT().DefaultModelCloudInfoAndCredential(
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
//}

func (s *modelManagerSuite) TestModelDefaults(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
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
	c.Assert(results.Results, tc.HasLen, 1)
	c.Assert(results.Results[0].Error, tc.IsNil)
	c.Assert(results.Results[0].Config, tc.DeepEquals, expectedValues)
}

func (s *modelManagerSuite) TestSetModelCloudDefaults(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestSetModelRegionDefaults(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *modelManagerSuite) blockAllChanges(c *tc.C, msg string) {
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), gomock.Any()).Return(msg, nil)
}

func (s *modelManagerSuite) assertBlocked(c *tc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), tc.IsTrue, tc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), tc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelManagerSuite) TestBlockChangesSetModelDefaults(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockAllChanges(c, "TestBlockChangesSetModelDefaults")
	_, err := s.api.SetModelDefaults(context.Background(), params.SetModelDefaults{})
	s.assertBlocked(c, err, "TestBlockChangesSetModelDefaults")
}

func (s *modelManagerSuite) TestUnsetModelCloudDefaults(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestUnsetModelRegionDefaults(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestBlockUnsetModelDefaults(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.blockAllChanges(c, "TestBlockUnsetModelDefaults")
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"abc"},
		}}}
	_, err := s.api.UnsetModelDefaults(context.Background(), args)
	s.assertBlocked(c, err, "TestBlockUnsetModelDefaults")
}

func (s *modelManagerSuite) TestModelDefaultsAsNormalUser(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.ModelDefaultsForClouds(context.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ModelDefaultsResults{})
}

func (s *modelManagerSuite) TestSetModelDefaultsAsNormalUser(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.SetModelDefaults(context.Background(), params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config: map[string]interface{}{
				"ftp-proxy": "http://charlie",
			}}}})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *modelManagerSuite) TestUnsetModelDefaultsAsNormalUser(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.UnsetModelDefaults(context.Background(), params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr2"}}}})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *modelManagerSuite) TestDumpModel(c *tc.C) {
	c.Skip("TODO: Fix when refactoring the api into the domain services layer")
	// 	defer s.setUpAPI(c).Finish()

	// 	api, err := modelmanager.NewModelManagerAPI(
	// 		context.Background(),
	// 		s.st, modelExporter(s.modelExporter),
	// 		s.controllerUUID,
	// 		modelmanager.Services{
	// 			DomainServicesGetter: s.domainServicesGetter,
	// 			CloudService: &mockCloudService{
	// 				clouds: map[string]cloud.Cloud{"dummy": jujutesting.DefaultCloud},
	// 			},
	// 			CredentialService:    apiservertesting.ConstCredentialGetter(nil),
	// 			ModelService:         s.modelService,
	// 			ModelDefaultsService: nil,
	// 			AccessService:        s.accessService,
	// 			ObjectStore:          &mockObjectStore{},
	// 		},
	// 		nil, common.NewBlockChecker(s.blockCommandService),
	// 		s.authoriser,
	// 	)
	// 	c.Check(err, tc.ErrorIsNil)

	// 	s.modelExporter.EXPECT().ExportModelPartial(
	// 		gomock.Any(),
	// 		state.ExportConfig{IgnoreIncompleteModel: true},
	// 		gomock.Any(),
	// 	).Times(1).Return(
	// 		&fakeModelDescription{ModelUUID: s.st.model.UUID()},
	// 		nil)
	// 	results := api.DumpModels(context.Background(), params.DumpModelRequest{
	// 		Entities: []params.Entity{{
	// 			Tag: "bad-tag",
	// 		}, {
	// 			Tag: "application-foo",
	// 		}, {
	// 			Tag: s.st.ModelTag().String(),
	// 		}}})

	// 	c.Assert(results.Results, tc.HasLen, 3)
	// 	bad, notApp, good := results.Results[0], results.Results[1], results.Results[2]
	// 	c.Check(bad.Result, tc.Equals, "")
	// 	c.Check(bad.Error.Message, tc.Equals, `"bad-tag" is not a valid tag`)

	// 	c.Check(notApp.Result, tc.Equals, "")
	// 	c.Check(notApp.Error.Message, tc.Equals, `"application-foo" is not a valid model tag`)

	// c.Check(good.Error, tc.IsNil)
	// c.Check(good.Result, tc.DeepEquals, "model-uuid: deadbeef-0bad-400d-8000-4b1d0d06f00d\n")
}

func (s *modelManagerSuite) TestDumpModelMissingModel(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.st.SetErrors(errors.NotFoundf("boom"))
	_, modelTag := generateModelUUIDAndTag(c)
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: modelTag.String()}}}
	results := s.api.DumpModels(context.Background(), models)
	s.st.CheckCalls(c, []testhelpers.StubCall{
		{FuncName: "GetBackend", Args: []interface{}{modelTag.Id()}},
	})
	c.Assert(results.Results, tc.HasLen, 1)
	result := results.Results[0]
	c.Assert(result.Result, tc.Equals, "")
	c.Assert(result.Error, tc.NotNil)
	c.Check(result.Error.Code, tc.Equals, `not found`)
	c.Check(result.Error.Message, tc.Equals, `id not found`)
}

func (s *modelManagerSuite) TestDumpModelUsers(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	_, modelTag := generateModelUUIDAndTag(c)
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: modelTag.String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModels(context.Background(), models)
		c.Assert(results.Results, tc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, tc.Equals, "")
		c.Assert(result.Error, tc.NotNil)
		c.Check(result.Error.Message, tc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestAddModelCantCreateModelForSomeoneElse(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	s.modelService.EXPECT().DefaultModelCloudInfo(
		gomock.Any()).Return("dummy", "dummy-region", nil)

	addModelUser := names.NewUserTag("add-model")

	s.setAPIUser(c, addModelUser)
	nonAdminUser := names.NewUserTag("non-admin")
	_, err := s.api.CreateModel(context.Background(), createArgs(nonAdminUser))
	c.Assert(err, tc.ErrorMatches, "\"add-model\" permission does not permit creation of models for different owners")
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

func (s *modelManagerSuite) TestUpdatedModel(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	as := s.accessService.EXPECT()
	modelUUID, modelTag := generateModelUUIDAndTag(c)
	testUser := names.NewUserTag("foobar")
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        modelUUID.String(),
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
				ModelTag: modelTag.String(),
			},
		}}

	results, err := s.api.ModifyModelAccess(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Check(results.OneError(), tc.ErrorIsNil)
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

	modelStatusAPI *mocks.MockModelStatusAPI

	store objectstore.ObjectStore

	controllerUUID uuid.UUID
}

var _ = tc.Suite(&modelManagerStateSuite{})

func (s *modelManagerStateSuite) SetUpSuite(c *tc.C) {
	coretesting.SkipUnlessControllerOS(c)
	s.ApiServerSuite.SetUpSuite(c)
}

func (s *modelManagerStateSuite) SetUpTest(c *tc.C) {
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

func (s *modelManagerStateSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = mocks.NewMockControllerConfigService(ctrl)
	s.accessService = mocks.NewMockAccessService(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.modelInfoService = mocks.NewMockModelInfoService(ctrl)
	s.applicationService = mocks.NewMockApplicationService(ctrl)
	s.domainServicesGetter = mocks.NewMockDomainServicesGetter(ctrl)
	s.blockCommandService = mocks.NewMockBlockCommandService(ctrl)
	s.modelStatusAPI = mocks.NewMockModelStatusAPI(ctrl)

	var fs assumes.FeatureSet
	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).AnyTimes().Return(fs, nil)

	return ctrl
}

func (s *modelManagerStateSuite) setAPIUser(c *tc.C, user names.UserTag) {
	s.authoriser.Tag = user
	st := commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool())

	domainServices := s.ControllerDomainServices(c)

	s.modelmanager = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{ModelManagerBackend: st},
		true,
		user,
		s.modelStatusAPI,
		nil,
		s.controllerUUID,
		modelmanager.Services{
			DomainServicesGetter: s.domainServicesGetter,
			CredentialService:    domainServices.Credential(),
			ModelService:         s.modelService,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
			ApplicationService:   s.applicationService,
		},
		common.NewBlockChecker(s.blockCommandService),
		s.authoriser,
	)
}

// expectCreateModelStateSuite expects all the calls to the services made during
// model creation. Since this is the state suite, we are not explicitly
// testing the services calls here so these expectations are quite permissive.
func (s *modelManagerStateSuite) expectCreateModelStateSuite(
	c *tc.C,
	ctrl *gomock.Controller,
	modelCreateArgs params.ModelCreateArgs,
) {
	modelUUID := modeltesting.GenModelUUID(c)
	userTag, err := names.ParseUserTag(modelCreateArgs.OwnerTag)
	c.Assert(err, tc.IsNil)
	ownerName := user.NameFromTag(userTag)
	ownerUUID := usertesting.GenUserUUID(c)

	// Get the default cloud name and credential.
	s.modelService.EXPECT().DefaultModelCloudInfo(
		gomock.Any()).Return("dummy", "dummy-region", nil)
	// Get the uuid of the model owner.
	s.accessService.EXPECT().GetUserUUIDByName(
		gomock.Any(), ownerName,
	).Return(ownerUUID, nil)

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

	c.Assert(err, tc.ErrorIsNil)

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
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)
	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{
		Status: status.Active,
		Since:  time.Now(),
	}, nil)
	s.modelInfoService.EXPECT().GetModelInfo(gomock.Any()).Return(coremodel.ModelInfo{
		UUID: modelUUID,
		// Use a version we shouldn't have now to ensure we're using the
		// ModelAgentService rather than the ModelInfo data.
		AgentVersion:   semversion.MustParse("2.6.5"),
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

func (s *modelManagerStateSuite) TestNewAPIAcceptsClient(c *tc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	// 	anAuthoriser := s.authoriser
	// 	anAuthoriser.Tag = names.NewUserTag("external@remote")
	// 	st := commonmodel.NewModelManagerBackend(s.ControllerModel(c), s.StatePool())
	// 	domainServices := s.ControllerDomainServices(c)

	// endPoint, err := modelmanager.NewModelManagerAPI(
	//
	//	context.Background(),
	//	mockCredentialShim{st},
	//	nil,
	//	s.controllerUUID,
	//	modelmanager.Services{
	//		DomainServicesGetter: s.domainServicesGetter,
	//		CloudService:         domainServices.Cloud(),
	//		CredentialService:    domainServices.Credential(),
	//		ModelService:         s.modelService,
	//		ModelDefaultsService: nil,
	//		AccessService:        s.accessService,
	//		ObjectStore:          &mockObjectStore{},
	//	},
	//	nil, common.NewBlockChecker(s.blockCommandService), anAuthoriser,
	//
	// )
	// c.Assert(err, tc.ErrorIsNil)
	// c.Assert(endPoint, tc.NotNil)
}

func (s *modelManagerStateSuite) createArgsForVersion(c *tc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := createArgs(owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerStateSuite) TestUserCanCreateModel(c *tc.C) {
	c.Skip("skip for now because all state code will be removed")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)
	model, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.OwnerTag, tc.Equals, owner.String())
	c.Assert(model.Name, tc.Equals, "test-model")
	c.Assert(model.Type, tc.Equals, "iaas")
}

func (s *modelManagerStateSuite) TestAdminCanCreateModelForSomeoneElse(c *tc.C) {
	c.Skip("skip for now because all state code will be removed")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.setAPIUser(c, jujutesting.AdminUser)
	owner := names.NewUserTag("external@remote")
	args := createArgs(owner)
	s.expectCreateModelStateSuite(c, ctrl, args)

	model, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(model.OwnerTag, tc.Equals, owner.String())
	c.Assert(model.Name, tc.Equals, "test-model")
	c.Assert(model.Type, tc.Equals, "iaas")

	newState, err := s.StatePool().Get(model.UUID)
	c.Assert(err, tc.ErrorIsNil)
	defer newState.Release()

	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *tc.C) {
	c.Skip("skip for now because all state code will be removed")

	defer s.setupMocks(c).Finish()

	userTag := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, userTag)

	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(context.Background(), createArgs(owner))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSelf(c *tc.C) {
	c.Skip("skip for now because all state code will be removed")

	defer s.setupMocks(c).Finish()

	owner := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, owner)

	_, err := s.modelmanager.CreateModel(context.Background(), createArgs(owner))
	c.Assert(err, tc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestCreateModelSameAgentVersion(c *tc.C) {
	c.Skip("skip for now because all state code will be removed")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	admin := jujutesting.AdminUser
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, jujuversion.Current.String())
	s.expectCreateModelStateSuite(c, ctrl, args)
	_, err := s.modelmanager.CreateModel(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
}

// TODO (tlm): Re-implement under DQlite
//func (s *modelManagerStateSuite) TestCreateModelBadAgentVersion(c *tc.C) {
//	ctrl := s.setupMocks(c)
//	defer ctrl.Finish()
//	err := s.ControllerModel(c).State().SetModelAgentVersion(coretesting.FakeVersionNumber, nil, false, stubUpgrader{})
//	c.Assert(err, tc.ErrorIsNil)
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
//		c.Check(err, tc.ErrorMatches, test.errMatch)
//	}
//}

// TODO (tlm): Re-implement under DQlite
//func (s *modelManagerStateSuite) TestListModelsAdminSelf(c *tc.C) {
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
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(result.UserModels, tc.HasLen, 1)
//	//expected, err := s.ControllerModel(c).State().Model()
//	//c.Assert(err, tc.ErrorIsNil)
//	//s.checkModelMatches(c, result.UserModels[0].Model, expected)
//}
//
//func (s *modelManagerStateSuite) TestListModelsAdminListsOther(c *tc.C) {
//	defer s.setupMocks(c).Finish()
//
//	user := jujutesting.AdminUser
//	s.setAPIUser(c, user)
//	other := names.NewUserTag("admin")
//	result, err := s.modelmanager.ListModels(context.Background(), params.Entity{Tag: other.String()})
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(result.UserModels, tc.HasLen, 1)
//}
//
//func (s *modelManagerStateSuite) TestListModelsDenied(c *tc.C) {
//	defer s.setupMocks(c).Finish()
//
//	user := names.NewUserTag("external@remote")
//	s.setAPIUser(c, user)
//	other := names.NewUserTag("other@remote")
//	_, err := s.modelmanager.ListModels(context.Background(), params.Entity{Tag: other.String()})
//	c.Assert(err, tc.ErrorMatches, "permission denied")
//}

func (s *modelManagerStateSuite) TestAdminModelManager(c *tc.C) {
	defer s.setupMocks(c).Finish()

	user := jujutesting.AdminUser
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), tc.IsTrue)
}

func (s *modelManagerStateSuite) TestNonAdminModelManager(c *tc.C) {
	c.Skip("skip for now because all state code will be removed")

	defer s.setupMocks(c).Finish()

	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), tc.IsFalse)
}

func (s *modelManagerStateSuite) TestDestroyOwnModel(c *tc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	// 	ctrl := s.setupMocks(c)
	// 	defer ctrl.Finish()

	// 	domainServices := s.ControllerDomainServices(c)

	// 	// TODO(perrito666) this test is not valid until we have
	// 	// proper controller permission since the only users that
	// 	// can create models are controller admins.
	// 	owner := names.NewUserTag("admin")
	// 	s.setAPIUser(c, owner)
	// 	args := createArgs(owner)
	// 	s.expectCreateModelStateSuite(c, ctrl, args)
	// 	m, err := s.modelmanager.CreateModel(context.Background(), args)
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	st, err := s.StatePool().Get(m.UUID)
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	defer st.Release()
	// 	model, err := st.Model()
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	backend := commonmodel.NewModelManagerBackend(model, s.StatePool())

	// 	s.modelmanager, err = modelmanager.NewModelManagerAPI(
	// 		context.Background(),
	// 		mockCredentialShim{ModelManagerBackend: backend},
	// 		nil,
	// 		s.controllerUUID,
	// 		modelmanager.Services{
	// 			DomainServicesGetter: s.domainServicesGetter,
	// 			CloudService:         domainServices.Cloud(),
	// 			CredentialService:    domainServices.Credential(),
	// 			ModelDefaultsService: nil,
	// 			AccessService:        s.accessService,
	// 			ObjectStore:          &mockObjectStore{},
	// 		},
	// 		nil, common.NewBlockChecker(s.blockCommandService), s.authoriser,
	// 	)
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	force := true
	// 	timeout := time.Minute
	// 	results, err := s.modelmanager.DestroyModels(context.Background(), params.DestroyModelsParams{
	// 		Models: []params.DestroyModelParams{{
	// 			ModelTag: "model-" + m.UUID,
	// 			Force:    &force,
	// 			Timeout:  &timeout,
	// 		}},
	// 	})
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(results.Results, tc.HasLen, 1)
	// 	c.Assert(results.Results[0].Error, tc.IsNil)

	// 	model, err = st.Model()
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(model.Life(), tc.Not(tc.Equals), state.Alive)
	// 	gotTimeout := model.DestroyTimeout()
	// 	c.Assert(gotTimeout, tc.NotNil)
	// 	c.Assert(*gotTimeout, tc.Equals, timeout)
	// 	gotForce := model.ForceDestroyed()
	// 	c.Assert(gotForce, tc.IsTrue)
	// }

	// func (s *modelManagerStateSuite) TestAdminDestroysOtherModel(c *tc.C) {
	// 	ctrl := s.setupMocks(c)
	// 	defer ctrl.Finish()

	// 	// TODO(perrito666) Both users are admins in this case, this tesst is of dubious
	// 	// usefulness until proper controller permissions are in place.
	// 	owner := names.NewUserTag("admin")
	// 	s.setAPIUser(c, owner)
	// 	args := createArgs(owner)
	// 	s.expectCreateModelStateSuite(c, ctrl, args)
	// 	m, err := s.modelmanager.CreateModel(context.Background(), args)
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	st, err := s.StatePool().Get(m.UUID)
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	defer st.Release()
	// 	model, err := st.Model()
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	s.authoriser.Tag = jujutesting.AdminUser
	// 	backend := commonmodel.NewModelManagerBackend(model, s.StatePool())

	// 	domainServices := s.ControllerDomainServices(c)

	// 	s.modelInfoService.EXPECT().GetStatus(gomock.Any()).Return(domainmodel.StatusInfo{Status: status.Available}, nil)

	// 	s.modelmanager, err = modelmanager.NewModelManagerAPI(
	// 		context.Background(),
	// 		mockCredentialShim{backend},
	// 		nil,
	// 		s.controllerUUID,
	// 		modelmanager.Services{
	// 			DomainServicesGetter: s.domainServicesGetter,
	// 			CloudService:         domainServices.Cloud(),
	// 			CredentialService:    domainServices.Credential(),
	// 			ModelService:         nil,
	// 			ModelDefaultsService: nil,
	// 			AccessService:        s.accessService,
	// 			ObjectStore:          &mockObjectStore{},
	// 		},
	// 		nil, common.NewBlockChecker(s.blockCommandService), s.authoriser,
	// 	)
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	results, err := s.modelmanager.DestroyModels(context.Background(), params.DestroyModelsParams{
	// 		Models: []params.DestroyModelParams{{
	// 			ModelTag: "model-" + m.UUID,
	// 		}},
	// 	})
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(results.Results, tc.HasLen, 1)
	// 	c.Assert(results.Results[0].Error, tc.IsNil)

	// 	s.authoriser.Tag = owner
	// 	model, err = st.Model()
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(model.Life(), tc.Not(tc.Equals), state.Alive)
	// }

	// func (s *modelManagerStateSuite) TestDestroyModelErrors(c *tc.C) {
	// 	ctrl := s.setupMocks(c)
	// 	defer ctrl.Finish()

	// 	owner := names.NewUserTag(user.AdminUserName.Name())
	// 	s.setAPIUser(c, owner)
	// 	args := createArgs(owner)
	// 	s.expectCreateModelStateSuite(c, ctrl, args)
	// 	m, err := s.modelmanager.CreateModel(context.Background(), args)
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	st, err := s.StatePool().Get(m.UUID)
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	defer st.Release()
	// 	model, err := st.Model()
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	domainServices := s.ControllerDomainServices(c)

	// 	backend := commonmodel.NewModelManagerBackend(model, s.StatePool())
	// 	s.modelmanager, err = modelmanager.NewModelManagerAPI(
	// 		context.Background(),
	// 		mockCredentialShim{backend},
	// 		nil,
	// 		s.controllerUUID,
	// 		modelmanager.Services{
	// 			DomainServicesGetter: s.domainServicesGetter,
	// 			CloudService:         domainServices.Cloud(),
	// 			CredentialService:    domainServices.Credential(),
	// 			ModelService:         nil,
	// 			ModelDefaultsService: nil,
	// 			AccessService:        s.accessService,
	// 			ObjectStore:          &mockObjectStore{},
	// 		},
	// 		nil, common.NewBlockChecker(s.blockCommandService), s.authoriser,
	// 	)
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	user := names.NewUserTag("other@remote")
	// 	s.setAPIUser(c, user)

	// 	results, err := s.modelmanager.DestroyModels(context.Background(), params.DestroyModelsParams{
	// 		Models: []params.DestroyModelParams{
	// 			{ModelTag: "model-" + m.UUID},
	// 			{ModelTag: "model-9f484882-2f18-4fd2-967d-db9663db7bea"},
	// 			{ModelTag: "machine-42"},
	// 		},
	// 	})
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(results.Results, tc.DeepEquals, []params.ErrorResult{{
	// 		// we don't have admin access to the model
	// 		Error: &params.Error{
	// 			Message: "permission denied",
	// 			Code:    params.CodeUnauthorized,
	// 		},
	// 	}, {
	// 		Error: &params.Error{
	// 			Message: `model "9f484882-2f18-4fd2-967d-db9663db7bea" not found`,
	// 			Code:    params.CodeNotFound,
	// 		},
	// 	}, {
	// 		Error: &params.Error{
	// 			Message: `"machine-42" is not a valid model tag`,
	// 		},
	// 	}})

	// s.setAPIUser(c, owner)
	// model, err = st.Model()
	// c.Assert(err, tc.ErrorIsNil)
	// c.Assert(model.Life(), tc.Equals, state.Alive)
}

func (s *modelManagerStateSuite) TestModifyModelAccessEmptyArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setAPIUser(c, jujutesting.AdminUser)
	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{{}}}

	result, err := s.modelmanager.ModifyModelAccess(context.Background(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := `could not modify model access: "" is not a valid tag`
	c.Assert(result.OneError(), tc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestModelInfoForMigratedModel(c *tc.C) {
	c.Skip("TODO tlm: Fix when refactoring the api into the domain services layer")
	// 	user := names.NewUserTag("admin")

	// 	f, release := s.NewFactory(c, s.ControllerModelUUID())
	// 	defer release()

	// 	modelState := f.MakeModel(c, &factory.ModelParams{
	// 		Owner: user,
	// 	})
	// 	defer modelState.Close()
	// 	model, err := modelState.Model()
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	// Migrate the model and delete it from the state
	// 	mig, err := modelState.CreateMigration(state.MigrationSpec{
	// 		InitiatedBy: user,
	// 		TargetInfo: migration.TargetInfo{
	// 			ControllerTag:   names.NewControllerTag(uuid.MustNewUUID().String()),
	// 			ControllerAlias: "target",
	// 			Addrs:           []string{"1.2.3.4:5555"},
	// 			CACert:          coretesting.CACert,
	// 			AuthTag:         names.NewUserTag("user2"),
	// 			Password:        "secret",
	// 		},
	// 	})
	// 	c.Assert(err, tc.ErrorIsNil)

	// 	for _, phase := range migration.SuccessfulMigrationPhases() {
	// 		c.Assert(mig.SetPhase(phase), tc.ErrorIsNil)
	// 	}
	// 	c.Assert(model.Destroy(state.DestroyModelParams{}), tc.ErrorIsNil)
	// 	c.Assert(modelState.RemoveDyingModel(), tc.ErrorIsNil)

	// 	domainServices := s.ControllerDomainServices(c)

	// 	anAuthoriser := s.authoriser
	// 	anAuthoriser.Tag = user
	// 	st := commonmodel.NewUserAwareModelManagerBackend(model, s.StatePool(), user)
	// 	endPoint, err := modelmanager.NewModelManagerAPI(
	// 		context.Background(),
	// 		mockCredentialShim{st},
	// 		nil,
	// 		s.controllerUUID,
	// 		modelmanager.Services{
	// 			DomainServicesGetter: s.domainServicesGetter,
	// 			CloudService:         domainServices.Cloud(),
	// 			CredentialService:    domainServices.Credential(),
	// 			ModelService:         s.modelService,
	// 			ModelDefaultsService: nil,
	// 			AccessService:        s.accessService,
	// 			ObjectStore:          &mockObjectStore{},
	// 		},
	// 		nil, common.NewBlockChecker(s.blockCommandService), anAuthoriser,
	// 	)
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(endPoint, tc.NotNil)

	// 	res, err := endPoint.ModelInfo(
	// 		context.Background(),
	// 		params.Entities{
	// 			Entities: []params.Entity{
	// 				{Tag: model.ModelTag().String()},
	// 			},
	// 		},
	// 	)
	// 	c.Assert(err, tc.ErrorIsNil)
	// 	c.Assert(res.Results, tc.HasLen, 1)
	// 	resErr0 := errors.Cause(res.Results[0].Error)
	// 	c.Assert(params.IsRedirect(resErr0), tc.Equals, true)

	// 	pErr, ok := resErr0.(*params.Error)
	// 	c.Assert(ok, tc.Equals, true)

	// 	var info params.RedirectErrorInfo
	// 	c.Assert(pErr.UnmarshalInfo(&info), tc.ErrorIsNil)

	//	nhp := params.HostPort{
	//		Address: params.Address{
	//			Value: "1.2.3.4",
	//			Type:  string(network.IPv4Address),
	//			Scope: string(network.ScopePublic),
	//		},
	//		Port: 5555,
	//	}
	//
	// c.Assert(info.Servers, tc.DeepEquals, [][]params.HostPort{{nhp}})
	// c.Assert(info.CACert, tc.Equals, coretesting.CACert)
	// c.Assert(info.ControllerAlias, tc.Equals, "target")
}

func (s *modelManagerSuite) TestChangeModelCredential(c *tc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar")
	modelUUID, modelTag := generateModelUUIDAndTag(c)
	s.modelService.EXPECT().UpdateCredential(
		gomock.Any(),
		modelUUID,
		credential.KeyFromTag(credentialTag),
	).Return(nil)
	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{
				ModelTag:           modelTag.String(),
				CloudCredentialTag: credentialTag.String(),
			},
		},
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Check(results.Results[0].Error, tc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredentialBulkUninterrupted(c *tc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).
		Return("", blockcommanderrors.NotFound).AnyTimes()

	credentialTag := names.NewCloudCredentialTag("foo/bob/bar")
	modelUUID, modelTag := generateModelUUIDAndTag(c)
	s.modelService.EXPECT().UpdateCredential(
		gomock.Any(),
		modelUUID,
		credential.KeyFromTag(credentialTag),
	).Return(nil)
	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: "bad-model-tag"},
			{
				ModelTag:           modelTag.String(),
				CloudCredentialTag: credentialTag.String(),
			},
		},
	})

	c.Check(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 2)
	c.Check(results.Results[0].Error, tc.ErrorMatches, `"bad-model-tag" is not a valid tag`)
	c.Check(results.Results[1].Error, tc.IsNil)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: modelTag.String()},
			{ModelTag: modelTag.String(), CloudCredentialTag: "bad-credential-tag"},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 2)
	c.Assert(results.Results[1].Error, tc.ErrorMatches, `"bad-credential-tag" is not a valid tag`)
}

func (s *modelManagerSuite) TestChangeModelCredentialUnauthorisedUser(c *tc.C) {
	defer s.setUpAPI(c).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	_, modelTag := generateModelUUIDAndTag(c)
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	results, err := s.api.ChangeModelCredential(context.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: modelTag.String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `permission denied`)
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

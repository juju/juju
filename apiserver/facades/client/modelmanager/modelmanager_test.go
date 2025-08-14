// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/blockcommand"
	blockcommanderrors "github.com/juju/juju/domain/blockcommand/errors"
	domainmodel "github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
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
)

type modelManagerSuite struct {
	testhelpers.IsolationSuite

	accessService        *MockAccessService
	modelService         *MockModelService
	modelDefaultService  *MockModelDefaultsService
	domainServicesGetter *MockDomainServicesGetter
	domainServices       *MockModelDomainServices
	applicationService   *MockApplicationService
	blockCommandService  *MockBlockCommandService
	modelInfoService     *MockModelInfoService
	authoriser           apiservertesting.FakeAuthorizer
	api                  *modelmanager.ModelManagerAPI
	controllerUUID       uuid.UUID
	modelConfigService   *MockModelConfigService
	machineService       *MockMachineService

	modelStatusAPI *MockModelStatusAPI
}

func TestModelManagerSuite(t *testing.T) {
	tc.Run(t, &modelManagerSuite{})
}

func (s *modelManagerSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelService = NewMockModelService(ctrl)
	s.modelDefaultService = NewMockModelDefaultsService(ctrl)
	s.accessService = NewMockAccessService(ctrl)
	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.machineService = NewMockMachineService(ctrl)
	s.domainServices = NewMockModelDomainServices(ctrl)
	s.modelStatusAPI = NewMockModelStatusAPI(ctrl)

	c.Cleanup(func() {
		s.modelService = nil
		s.modelDefaultService = nil
		s.accessService = nil
		s.domainServicesGetter = nil
		s.applicationService = nil
		s.blockCommandService = nil
		s.machineService = nil
		s.domainServices = nil
		s.modelStatusAPI = nil
	})

	return ctrl
}

func (s *modelManagerSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	var err error
	s.controllerUUID, err = uuid.UUIDFromString(coretesting.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)

	attrs := coretesting.FakeConfig()
	attrs["agent-version"] = jujuversion.Current.String()

	s.authoriser = apiservertesting.FakeAuthorizer{Tag: jujutesting.AdminUser}
}

func (s *modelManagerSuite) setUpAPI(c *tc.C) *gomock.Controller {
	return s.setUpAPIWithUser(c, jujutesting.AdminUser)
}

func (s *modelManagerSuite) setUpAPIWithUser(c *tc.C, user names.UserTag) *gomock.Controller {
	ctrl := s.setUpMocks(c)

	s.authoriser.Tag = user
	user, _ = s.authoriser.GetAuthTag().(names.UserTag)

	cred := cloud.NewEmptyCredential()
	s.api = modelmanager.NewModelManagerAPI(
		c.Context(),
		user.Name() == "admin",
		user,
		s.modelStatusAPI,
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

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})

	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).Return(fs, nil).AnyTimes()
	return ctrl
}

// generateModelUUIDAndTag generates a model UUID and tag for testing. This is
// a simple convenience function to avoid having to first generate a model uuid
// then cast it into a tag. This function does not setup any preconditions in
// testing states.
func generateModelUUIDAndTag(c *tc.C) (coremodel.UUID, names.ModelTag) {
	modelUUID := coremodel.GenUUID(c)
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
	modelUUID := coremodel.GenUUID(c)
	adminName := usertesting.GenNewName(c, "admin")
	adminUUID := usertesting.GenUserUUID(c)

	defaultCred := credential.Key{
		Cloud: "dummy",
		Owner: user.AdminUserName,
		Name:  "some-credential",
	}

	// Get the default cloud name and credential.
	s.modelService.EXPECT().DefaultModelCloudInfo(
		gomock.Any()).Return("dummy", "dummy-region", nil)
	// Get the uuid of the model admin.
	s.accessService.EXPECT().GetUserUUIDByName(
		gomock.Any(), adminName,
	).Return(adminUUID, nil)

	// Create model in controller database.
	s.modelService.EXPECT().CreateModel(gomock.Any(), domainmodel.GlobalModelCreationArgs{
		Name:        modelCreateArgs.Name,
		Qualifier:   "admin",
		AdminUsers:  []user.UUID{adminUUID},
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
		Qualifier:   coremodel.Qualifier(modelCreateArgs.Qualifier),
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
	modelDomainServices := NewMockModelDomainServices(ctrl)
	s.domainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(modelDomainServices, nil).AnyTimes()

	// Expect calls to get various model services.
	s.modelInfoService = NewMockModelInfoService(ctrl)
	networkService := NewMockNetworkService(ctrl)

	s.modelConfigService = NewMockModelConfigService(ctrl)
	modelAgentService := NewMockModelAgentService(ctrl)

	statusService := NewMockStatusService(ctrl)

	modelDomainServices.EXPECT().ModelInfo().Return(s.modelInfoService).AnyTimes()
	modelDomainServices.EXPECT().Network().Return(networkService)
	modelDomainServices.EXPECT().Config().Return(s.modelConfigService).AnyTimes()
	modelDomainServices.EXPECT().Agent().Return(modelAgentService).AnyTimes()
	modelDomainServices.EXPECT().Status().Return(statusService).AnyTimes()

	// Expect calls to functions of the model services.
	t := time.Now()
	statusService.EXPECT().GetModelStatus(gomock.Any()).Return(corestatus.StatusInfo{
		Status: corestatus.Available,
		Since:  &t,
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

func (s *modelManagerSuite) TestCreateModelQualifierMismatch(c *tc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	s.modelService.EXPECT().DefaultModelCloudInfo(
		gomock.Any()).Return("dummy", "dummy-region", nil)

	args := params.ModelCreateArgs{
		Name:      "foo",
		Qualifier: "prod",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudTag:           "cloud-dummy",
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-dummy_admin_some-credential",
	}

	_, err := s.api.CreateModel(c.Context(), args)
	c.Assert(err, tc.ErrorMatches, `cannot create model with qualifier "prod"`)
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
		Name:      "foo",
		Qualifier: "admin",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudTag:           "cloud-dummy",
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-dummy_admin_some-credential",
	}

	s.expectCreateModel(c, ctrl, args, cloudCredental, "dummy", "qux")
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)

	_, err := s.api.CreateModel(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestCreateModelDefaultRegion(c *tc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:      "foo",
		Qualifier: "admin",
	}

	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)

	_, err := s.api.CreateModel(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdmin(c *tc.C) {
	ctrl := s.setUpAPI(c)
	defer ctrl.Finish()

	args := params.ModelCreateArgs{
		Name:      "foo",
		Qualifier: "admin",
	}

	s.expectCreateModel(c, ctrl, args, credential.Key{}, "dummy", "dummy-region")
	s.modelInfoService.EXPECT().CreateModel(gomock.Any()).Return(nil)

	_, err := s.api.CreateModel(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
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
		Name:      "foo",
		Qualifier: "admin",
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

	_, err := s.api.CreateModel(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
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
		Name:      "foo",
		Qualifier: "admin",
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

	_, err := s.api.CreateModel(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
}

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

	results, err := s.api.ModelDefaultsForClouds(c.Context(), params.Entities{
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
	result, err := s.api.SetModelDefaults(c.Context(), params)
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
	result, err := s.api.SetModelDefaults(c.Context(), params)
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
	_, err := s.api.SetModelDefaults(c.Context(), params.SetModelDefaults{})
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
	result, err := s.api.UnsetModelDefaults(c.Context(), args)
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
	result, err := s.api.UnsetModelDefaults(c.Context(), args)
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
	_, err := s.api.UnsetModelDefaults(c.Context(), args)
	s.assertBlocked(c, err, "TestBlockUnsetModelDefaults")
}

func (s *modelManagerSuite) TestModelDefaultsAsNormalUser(c *tc.C) {
	defer s.setUpAPIWithUser(c, names.NewUserTag("charlie")).Finish()

	got, err := s.api.ModelDefaultsForClouds(c.Context(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ModelDefaultsResults{})
}

func (s *modelManagerSuite) TestSetModelDefaultsAsNormalUser(c *tc.C) {
	defer s.setUpAPIWithUser(c, names.NewUserTag("charlie")).Finish()

	got, err := s.api.SetModelDefaults(c.Context(), params.SetModelDefaults{
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
	defer s.setUpAPIWithUser(c, names.NewUserTag("charlie")).Finish()

	got, err := s.api.UnsetModelDefaults(c.Context(), params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr2"}}}})
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(got, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})
}

func (s *modelManagerSuite) TestDumpModel(c *tc.C) {
	c.Skip("re-implement dump model")
}

func (s *modelManagerSuite) TestDumpModelMissingModel(c *tc.C) {
	c.Skip("re-implement dump model")
}

func (s *modelManagerSuite) TestDumpModelUsers(c *tc.C) {
	c.Skip("re-implement dump model")
}

func (s *modelManagerSuite) TestUpdatedModel(c *tc.C) {
	defer s.setUpAPIWithUser(c, jujutesting.AdminUser).Finish()

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

	args := params.ModifyModelAccessRequest{
		Changes: []params.ModifyModelAccess{
			{
				UserTag:  testUser.String(),
				Action:   params.GrantModelAccess,
				Access:   params.ModelWriteAccess,
				ModelTag: modelTag.String(),
			},
		}}

	results, err := s.api.ModifyModelAccess(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(results.Results, tc.HasLen, 1)
	c.Check(results.OneError(), tc.ErrorIsNil)
}

func (s *modelManagerSuite) TestModelStatus(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	_, modelTag := generateModelUUIDAndTag(c)

	s.domainServicesGetter.EXPECT().DomainServicesForModel(gomock.Any(), gomock.Any()).Return(s.domainServices, nil).AnyTimes()
	s.domainServices.EXPECT().Machine().Return(s.machineService).AnyTimes()
	s.modelStatusAPI.EXPECT().ModelStatus(gomock.Any(), params.Entities{
		Entities: []params.Entity{
			{Tag: modelTag.String()},
		},
	}).Return(params.ModelStatusResults{
		Results: []params.ModelStatus{
			{ModelTag: modelTag.String()},
		},
	}, nil)

	results, err := s.api.ModelStatus(c.Context(), params.Entities{
		Entities: []params.Entity{
			{Tag: modelTag.String()},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.ModelStatusResults{
		Results: []params.ModelStatus{
			{ModelTag: modelTag.String()},
		},
	})
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
	results, err := s.api.ChangeModelCredential(c.Context(), params.ChangeModelCredentialsParams{
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
	results, err := s.api.ChangeModelCredential(c.Context(), params.ChangeModelCredentialsParams{
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
	results, err = s.api.ChangeModelCredential(c.Context(), params.ChangeModelCredentialsParams{
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
	defer s.setUpAPIWithUser(c, names.NewUserTag("bob@remote")).Finish()
	s.blockCommandService.EXPECT().GetBlockSwitchedOn(gomock.Any(), blockcommand.ChangeBlock).Return("", blockcommanderrors.NotFound)

	_, modelTag := generateModelUUIDAndTag(c)
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()

	results, err := s.api.ChangeModelCredential(c.Context(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: modelTag.String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results[0].Error, tc.ErrorMatches, `permission denied`)
}

func (s *modelManagerSuite) TestListModelsAdminSelf(c *tc.C) {
	defer s.setUpAPI(c).Finish()

	userUUID := usertesting.GenUserUUID(c)
	userTag := names.NewUserTag("non-admin")

	modelUUID := coremodel.GenUUID(c)
	modelUUIDNeverAccessed := coremodel.GenUUID(c)
	modelUUIDNotExist := coremodel.GenUUID(c)

	now := time.Now()
	s.accessService.EXPECT().GetUserUUIDByName(gomock.Any(), user.NameFromTag(userTag)).Return(userUUID, nil)
	s.modelService.EXPECT().ListAllModels(gomock.Any()).Return([]coremodel.Model{
		{UUID: modelUUID, Qualifier: "prod"},
		{UUID: modelUUIDNeverAccessed, Qualifier: "prod"},
		{UUID: modelUUIDNotExist},
	}, nil)

	s.accessService.EXPECT().LastModelLogin(
		gomock.Any(), user.NameFromTag(userTag), modelUUID).Return(now, nil)
	s.accessService.EXPECT().LastModelLogin(
		gomock.Any(), user.NameFromTag(userTag), modelUUIDNeverAccessed).Return(time.Time{}, accesserrors.UserNeverAccessedModel)
	s.accessService.EXPECT().LastModelLogin(
		gomock.Any(), user.NameFromTag(userTag), modelUUIDNotExist).Return(time.Time{}, modelerrors.NotFound)

	results, err := s.api.ListModels(
		c.Context(),
		params.Entity{Tag: userTag.String()},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UserModelList{
		UserModels: []params.UserModel{
			{
				Model: params.Model{
					UUID:      modelUUID.String(),
					Qualifier: "prod",
				},
				LastConnection: &now,
			},
			{
				Model: params.Model{
					UUID:      modelUUIDNeverAccessed.String(),
					Qualifier: "prod",
				},
			},
		},
	})
}

func (s *modelManagerSuite) TestListModelsNonAdminSelf(c *tc.C) {
	userUUID := usertesting.GenUserUUID(c)
	userTag := names.NewUserTag("non-admin")

	defer s.setUpAPIWithUser(c, userTag).Finish()

	modelUUID := coremodel.GenUUID(c)
	modelUUIDNeverAccessed := coremodel.GenUUID(c)
	modelUUIDNotExist := coremodel.GenUUID(c)

	now := time.Now()
	s.accessService.EXPECT().GetUserUUIDByName(gomock.Any(), user.NameFromTag(userTag)).Return(userUUID, nil)
	s.modelService.EXPECT().ListModelsForUser(gomock.Any(), userUUID).Return([]coremodel.Model{
		{UUID: modelUUID, Qualifier: "prod"},
		{UUID: modelUUIDNeverAccessed, Qualifier: "prod"},
		{UUID: modelUUIDNotExist},
	}, nil)

	s.accessService.EXPECT().LastModelLogin(
		gomock.Any(), user.NameFromTag(userTag), modelUUID).Return(now, nil)
	s.accessService.EXPECT().LastModelLogin(
		gomock.Any(), user.NameFromTag(userTag), modelUUIDNeverAccessed).Return(time.Time{}, accesserrors.UserNeverAccessedModel)
	s.accessService.EXPECT().LastModelLogin(
		gomock.Any(), user.NameFromTag(userTag), modelUUIDNotExist).Return(time.Time{}, modelerrors.NotFound)

	results, err := s.api.ListModels(
		c.Context(),
		params.Entity{Tag: userTag.String()},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.UserModelList{
		UserModels: []params.UserModel{
			{
				Model: params.Model{
					UUID:      modelUUID.String(),
					Qualifier: "prod",
				},
				LastConnection: &now,
			},
			{
				Model: params.Model{
					UUID:      modelUUIDNeverAccessed.String(),
					Qualifier: "prod",
				},
			},
		},
	})
}

func (s *modelManagerSuite) TestListModelsDenied(c *tc.C) {
	userTag := names.NewUserTag("non-admin@remote")
	anotherUserTag := names.NewUserTag("another-non-admin@remote")

	defer s.setUpAPIWithUser(c, userTag).Finish()

	_, err := s.api.ListModels(
		c.Context(),
		params.Entity{Tag: anotherUserTag.String()},
	)
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
}

// modelManagerStateSuite contains end-to-end tests.
// Prefer adding tests to modelManagerSuite above.
type modelManagerStateSuite struct {
	jujutesting.ApiServerSuite

	modelmanager *modelmanager.ModelManagerAPI
	authoriser   apiservertesting.FakeAuthorizer

	controllerConfigService *MockControllerConfigService
	accessService           *MockAccessService
	modelService            *MockModelService
	modelInfoService        *MockModelInfoService
	applicationService      *MockApplicationService
	domainServicesGetter    *MockDomainServicesGetter
	blockCommandService     *MockBlockCommandService

	modelStatusAPI *MockModelStatusAPI

	store objectstore.ObjectStore

	controllerUUID uuid.UUID
}

func TestModelManagerStateSuite(t *testing.T) {
	tc.Run(t, &modelManagerStateSuite{})
}

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

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.accessService = NewMockAccessService(ctrl)
	s.modelService = NewMockModelService(ctrl)
	s.modelInfoService = NewMockModelInfoService(ctrl)
	s.applicationService = NewMockApplicationService(ctrl)
	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.blockCommandService = NewMockBlockCommandService(ctrl)
	s.modelStatusAPI = NewMockModelStatusAPI(ctrl)

	var fs assumes.FeatureSet
	s.applicationService.EXPECT().GetSupportedFeatures(gomock.Any()).AnyTimes().Return(fs, nil)

	return ctrl
}

func (s *modelManagerStateSuite) setAPIUser(c *tc.C, user names.UserTag) {
	s.authoriser.Tag = user

	domainServices := s.ControllerDomainServices(c)

	s.modelmanager = modelmanager.NewModelManagerAPI(
		c.Context(),
		user.Name() == "admin",
		user,
		s.modelStatusAPI,
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

func (s *modelManagerStateSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Test admin can create model for someone else;
- Test create a model by an admin user for other user;
- Test create a model by a non admin user for other user will fail;
- Test create a model by a non admin user for self;
- Test create a model with agent version;
- Test create a model with agent version and stream;
- Test destroy an owned model by the logged in user;
- Test destroy models by an admin user;
- Test destroy models failed - invalid model tag;
- Test destroy models failed - permission denied;
`)
}

func (s *modelManagerStateSuite) TestAdminModelManager(c *tc.C) {
	defer s.setupMocks(c).Finish()

	user := jujutesting.AdminUser
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), tc.IsTrue)
}

func (s *modelManagerStateSuite) TestNonAdminModelManager(c *tc.C) {
	defer s.setupMocks(c).Finish()

	user := names.NewUserTag("external@remote")
	s.setAPIUser(c, user)
	c.Assert(modelmanager.AuthCheck(c, s.modelmanager, user), tc.IsFalse)
}

func (s *modelManagerStateSuite) TestModifyModelAccessFailedInvalidModelTag(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.setAPIUser(c, jujutesting.AdminUser)
	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{{}}}

	result, err := s.modelmanager.ModifyModelAccess(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	expectedErr := `could not modify model access: "" is not a valid tag`
	c.Assert(result.OneError(), tc.ErrorMatches, expectedErr)
}

func (s *modelManagerStateSuite) TestModifyModelAccessFailedPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userTag := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, userTag)
	modelUUID := coremodel.GenUUID(c)
	modelTag := names.NewModelTag(modelUUID.String())

	args := params.ModifyModelAccessRequest{Changes: []params.ModifyModelAccess{
		{ModelTag: modelTag.String()},
	}}

	result, err := s.modelmanager.ModifyModelAccess(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.OneError(), tc.ErrorMatches, `permission denied`)
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

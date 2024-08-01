// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager_test

import (
	"context"
	stdcontext "context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	jtesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	// Register the providers for the field check test
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/modelmanager"
	"github.com/juju/juju/apiserver/facades/client/modelmanager/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/core/migration"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/access"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	_ "github.com/juju/juju/internal/provider/azure"
	_ "github.com/juju/juju/internal/provider/ec2"
	_ "github.com/juju/juju/internal/provider/maas"
	_ "github.com/juju/juju/internal/provider/openstack"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/internal/uuid"
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

	st             *mockState
	ctlrSt         *mockState
	caasSt         *mockState
	caasBroker     *mockCaasBroker
	cloudService   *mockCloudService
	accessService  *mocks.MockAccessService
	modelService   *mocks.MockModelService
	modelExporter  *mocks.MockModelExporter
	authoriser     apiservertesting.FakeAuthorizer
	api            *modelmanager.ModelManagerAPI
	caasApi        *modelmanager.ModelManagerAPI
	controllerUUID uuid.UUID
}

var _ = gc.Suite(&modelManagerSuite{})

func (s *modelManagerSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelExporter = mocks.NewMockModelExporter(ctrl)
	s.modelService = mocks.NewMockModelService(ctrl)
	s.accessService = mocks.NewMockAccessService(ctrl)
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
		block:           -1,
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

func (s *modelManagerSuite) setUpAPI(c *gc.C) {
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

	newBroker := func(_ stdcontext.Context, args environs.OpenParams) (caas.Broker, error) {
		s.caasBroker = &mockCaasBroker{namespace: args.Config.Name()}
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
		s.st, s.modelExporter, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         s.cloudService,
			CredentialService:    apiservertesting.ConstCredentialGetter(&cred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, newBroker, common.NewBlockChecker(s.st),
		s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
	caasCred := cloud.NewCredential(cloud.UserPassAuthType, nil)
	caasApi, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.caasSt, s.modelExporter, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{
					"k8s-cloud": mockK8sCloud,
				},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(&caasCred),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, newBroker, common.NewBlockChecker(s.caasSt),
		s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.caasApi = caasApi

	var fs assumes.FeatureSet
	fs.Add(assumes.Feature{Name: "example"})
	modelmanager.MockSupportedFeatures(fs)
}

func (s *modelManagerSuite) TearDownTest(c *gc.C) {
	modelmanager.ResetSupportedFeaturesGetter()
}

func (s *modelManagerSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	newBroker := func(_ stdcontext.Context, args environs.OpenParams) (caas.Broker, error) {
		return s.caasBroker, nil
	}
	mm, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, s.modelExporter, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jujutesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(nil),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, newBroker, common.NewBlockChecker(s.st),
		s.authoriser, s.st.model,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = mm
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

func (s *modelManagerSuite) TestCreateModelArgs(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "user-admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		Config: map[string]interface{}{
			"bar": "baz",
		},
		CloudRegion:        "qux",
		CloudCredentialTag: "cloudcred-dummy_admin_some-credential",
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	s.st.CheckCallNames(c,
		"ControllerTag",
		"ControllerTag",
		"ComposeNewModelConfig",
		"NewModel",
		"Close",
		"GetBackend",
		"Model",
		"IsController",
		"AllMachines",
		"ControllerNodes",
		"HAPrimaryMachine",
		"LatestMigration",
	)

	// We cannot predict the UUID, because it's generated,
	// so we just extract it and ensure that it's not the
	// same as the controller UUID.
	newModelArgs := s.getModelArgs(c)
	uuid := newModelArgs.Config.UUID()
	c.Assert(uuid, gc.Not(gc.Equals), s.st.controllerModel.cfg.UUID())

	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":          "foo",
		"type":          "dummy",
		"uuid":          uuid,
		"agent-version": jujuversion.Current.String(),
		"bar":           "baz",
		"somebool":      false,
		"broken":        "",
		"secret":        "pork",
		"something":     "value",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newModelArgs.StorageProviderRegistry, gc.NotNil)
	newModelArgs.StorageProviderRegistry = nil

	c.Assert(newModelArgs, jc.DeepEquals, state.ModelArgs{
		Type:        state.ModelTypeIAAS,
		Owner:       names.NewUserTag("admin"),
		CloudName:   "dummy",
		CloudRegion: "qux",
		CloudCredential: names.NewCloudCredentialTag(
			"dummy/admin/some-credential",
		),
		Config: cfg,
	})
}

func (s *modelManagerSuite) TestCreateModelArgsWithCloud(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "user-admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)
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
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudName, gc.Equals, "dummy")
}

func (s *modelManagerSuite) TestCreateModelArgsWithCloudNotFound(c *gc.C) {
	s.setUpAPI(c)
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
		CloudTag: "cloud-some-unknown-cloud",
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, `cloud "some-unknown-cloud" not found, expected one of \["dummy"\]`)
}

func (s *modelManagerSuite) TestCreateModelDefaultRegion(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "user-admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-admin",
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudRegion, gc.Equals, "dummy-region")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdmin(c *gc.C) {
	s.testCreateModelDefaultCredentialAdmin(c, "user-admin")
}

func (s *modelManagerSuite) TestCreateModelDefaultCredentialAdminNoDomain(c *gc.C) {
	s.testCreateModelDefaultCredentialAdmin(c, "user-admin")
}

func (s *modelManagerSuite) testCreateModelDefaultCredentialAdmin(c *gc.C, ownerTag string) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        ownerTag,
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: ownerTag,
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, gc.Equals, names.NewCloudCredentialTag(
		"dummy/bob/some-credential",
	))
}

func (s *modelManagerSuite) TestCreateModelEmptyCredentialNonAdmin(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "user-bob",
			DisplayName: "Bob",
			Access:      permission.ReadAccess,
		}}, nil,
	)
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob",
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	newModelArgs := s.getModelArgs(c)
	c.Assert(newModelArgs.CloudCredential, gc.Equals, names.CloudCredentialTag{})
}

func (s *modelManagerSuite) TestCreateModelNoDefaultCredentialNonAdmin(c *gc.C) {
	s.setUpAPI(c)
	cld := s.cloudService.clouds["dummy"]
	cld.AuthTypes = nil
	s.cloudService.clouds["dummy"] = cld
	args := params.ModelCreateArgs{
		Name:     "foo",
		OwnerTag: "user-bob",
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, "no credential specified")
}

func (s *modelManagerSuite) TestCreateModelUnknownCredential(c *gc.C) {
	s.setUpAPI(c)
	s.st.SetErrors(nil, errors.NotFoundf("credential"))
	args := params.ModelCreateArgs{
		Name:               "foo",
		OwnerTag:           "user-admin",
		CloudCredentialTag: "cloudcred-dummy_admin_bar",
	}
	_, err := s.api.CreateModel(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches, `.*credential not found`)
}

func (s *modelManagerSuite) TestCreateCAASModelArgs(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "user-admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)
	args := params.ModelCreateArgs{
		Name:               "foo",
		OwnerTag:           "user-admin",
		Config:             map[string]interface{}{},
		CloudTag:           "cloud-k8s-cloud",
		CloudCredentialTag: "cloudcred-k8s-cloud_admin_some-credential",
	}
	_, err := s.caasApi.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	s.caasSt.CheckCallNames(c,
		"ControllerTag",
		"ControllerTag",
		"ComposeNewModelConfig",
		"NewModel",
		"Close",
		"GetBackend",
		"Model",
		"IsController",
		"AllMachines",
		"ControllerNodes",
		"HAPrimaryMachine",
		"LatestMigration",
	)
	s.caasBroker.CheckCallNames(c, "Create")

	// We cannot predict the UUID, because it's generated,
	// so we just extract it and ensure that it's not the
	// same as the controller UUID.
	newModelArgs := getModelArgsFor(c, s.caasSt)
	uuid := newModelArgs.Config.UUID()
	c.Assert(uuid, gc.Not(gc.Equals), s.caasSt.controllerModel.cfg.UUID())

	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"name":                              "foo",
		"type":                              "kubernetes",
		"uuid":                              uuid,
		"agent-version":                     jujuversion.Current.String(),
		"storage-default-block-source":      "kubernetes",
		"storage-default-filesystem-source": "kubernetes",
		"something":                         "value",
		"operator-storage":                  "",
		"workload-storage":                  "",
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(newModelArgs.StorageProviderRegistry, gc.NotNil)
	newModelArgs.StorageProviderRegistry = nil

	c.Assert(newModelArgs, jc.DeepEquals, state.ModelArgs{
		Type:      state.ModelTypeCAAS,
		Owner:     names.NewUserTag("admin"),
		CloudName: "k8s-cloud",
		CloudCredential: names.NewCloudCredentialTag(
			"k8s-cloud/admin/some-credential",
		),
		Config: cfg,
	})
}

func (s *modelManagerSuite) TestCreateCAASModelNamespaceClash(c *gc.C) {
	s.setUpAPI(c)
	args := params.ModelCreateArgs{
		Name:               "existing-ns",
		OwnerTag:           "user-admin",
		Config:             map[string]interface{}{},
		CloudTag:           "cloud-k8s-cloud",
		CloudCredentialTag: "cloudcred-k8s-cloud_admin_some-credential",
	}
	_, err := s.caasApi.CreateModel(stdcontext.Background(), args)
	s.caasBroker.CheckCallNames(c, "Create")
	c.Assert(err, jc.ErrorIs, errors.AlreadyExists)
}

func (s *modelManagerSuite) TestModelDefaults(c *gc.C) {
	s.setUpAPI(c)
	results, err := s.api.ModelDefaultsForClouds(stdcontext.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	expectedValues := map[string]params.ModelDefaults{
		"attr": {
			Controller: "val",
			Default:    "",
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

func (s *modelManagerSuite) TestSetModelDefaults(c *gc.C) {
	s.setUpAPI(c)
	params := params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config: map[string]interface{}{
				"attr3": "val3",
				"attr4": "val4"},
		}}}
	result, err := s.api.SetModelDefaults(stdcontext.Background(), params)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	c.Assert(s.ctlrSt.cfgDefaults, jc.DeepEquals, config.ModelDefaultAttributes{
		"attr": {
			Controller: "val",
			Default:    "",
			Regions: []config.RegionDefaultValue{{
				Name:  "dummy",
				Value: "val++"}}},
		"attr2": {
			Controller: "val3",
			Default:    "val2",
			Regions: []config.RegionDefaultValue{{
				Name:  "left",
				Value: "spam"}}},
		"attr3": {Controller: "val3"},
		"attr4": {Controller: "val4"},
	})
}

func (s *modelManagerSuite) blockAllChanges(c *gc.C, msg string) {
	s.st.blockMsg = msg
	s.st.block = state.ChangeBlock
}

func (s *modelManagerSuite) assertBlocked(c *gc.C, err error, msg string) {
	c.Assert(params.IsCodeOperationBlocked(err), jc.IsTrue, gc.Commentf("error: %#v", err))
	c.Assert(errors.Cause(err), jc.DeepEquals, &params.Error{
		Message: msg,
		Code:    "operation is blocked",
	})
}

func (s *modelManagerSuite) TestBlockChangesSetModelDefaults(c *gc.C) {
	s.setUpAPI(c)
	s.blockAllChanges(c, "TestBlockChangesSetModelDefaults")
	_, err := s.api.SetModelDefaults(stdcontext.Background(), params.SetModelDefaults{})
	s.assertBlocked(c, err, "TestBlockChangesSetModelDefaults")
}

func (s *modelManagerSuite) TestUnsetModelDefaults(c *gc.C) {
	s.setUpAPI(c)
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr"},
		}}}
	result, err := s.api.UnsetModelDefaults(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	want := config.ModelDefaultAttributes{
		"attr": config.AttributeDefaultValues{
			Regions: []config.RegionDefaultValue{
				{Name: "dummy", Value: "val++"},
			},
		},
		"attr2": config.AttributeDefaultValues{
			Default:    "val2",
			Controller: "val3",
			Regions: []config.RegionDefaultValue{
				{Name: "left", Value: "spam"},
			},
		},
	}
	c.Assert(s.ctlrSt.cfgDefaults, jc.DeepEquals, want)
}

func (s *modelManagerSuite) TestBlockUnsetModelDefaults(c *gc.C) {
	s.setUpAPI(c)
	s.blockAllChanges(c, "TestBlockUnsetModelDefaults")
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"abc"},
		}}}
	_, err := s.api.UnsetModelDefaults(stdcontext.Background(), args)
	s.assertBlocked(c, err, "TestBlockUnsetModelDefaults")
}

func (s *modelManagerSuite) TestUnsetModelDefaultsMissing(c *gc.C) {
	s.setUpAPI(c)
	// It's okay to unset a non-existent attribute.
	args := params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"not there"},
		}}}
	result, err := s.api.UnsetModelDefaults(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestModelDefaultsAsNormalUser(c *gc.C) {
	s.setUpAPI(c)
	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.ModelDefaultsForClouds(stdcontext.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(got, gc.DeepEquals, params.ModelDefaultsResults{})
}

func (s *modelManagerSuite) TestSetModelDefaultsAsNormalUser(c *gc.C) {
	s.setUpAPI(c)
	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.SetModelDefaults(stdcontext.Background(), params.SetModelDefaults{
		Config: []params.ModelDefaultValues{{
			Config: map[string]interface{}{
				"ftp-proxy": "http://charlie",
			}}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{
				Error: &params.Error{
					Message: "permission denied",
					Code:    "unauthorized access"}}}})

	// Make sure it didn't change.
	s.setAPIUser(c, names.NewUserTag("admin"))
	results, err := s.api.ModelDefaultsForClouds(stdcontext.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Config["ftp-proxy"].Controller, gc.IsNil)
}

func (s *modelManagerSuite) TestUnsetModelDefaultsAsNormalUser(c *gc.C) {
	s.setUpAPI(c)
	s.setAPIUser(c, names.NewUserTag("charlie"))
	got, err := s.api.UnsetModelDefaults(stdcontext.Background(), params.UnsetModelDefaults{
		Keys: []params.ModelUnsetKeys{{
			Keys: []string{"attr2"}}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	c.Assert(got, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}},
	})

	// Make sure it didn't change.
	s.setAPIUser(c, names.NewUserTag("admin"))
	results, err := s.api.ModelDefaultsForClouds(stdcontext.Background(), params.Entities{
		Entities: []params.Entity{{Tag: names.NewCloudTag("dummy").String()}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[0].Config["attr2"].Controller.(string), gc.Equals, "val3")
}

func (s *modelManagerSuite) TestDumpModel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	api, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		s.st, s.modelExporter, s.ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService: &mockCloudService{
				clouds: map[string]cloud.Cloud{"dummy": jujutesting.DefaultCloud},
			},
			CredentialService:    apiservertesting.ConstCredentialGetter(nil),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		state.NoopConfigSchemaSource,
		nil, nil, common.NewBlockChecker(s.st),
		s.authoriser, s.st.model,
	)
	c.Check(err, jc.ErrorIsNil)

	s.modelExporter.EXPECT().ExportModelPartial(
		gomock.Any(),
		state.ExportConfig{IgnoreIncompleteModel: true},
		gomock.Any(),
	).Times(1).Return(
		&fakeModelDescription{UUID: s.st.model.UUID()},
		nil)
	results := api.DumpModels(stdcontext.Background(), params.DumpModelRequest{
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
	s.setUpAPI(c)
	s.st.SetErrors(errors.NotFoundf("boom"))
	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f000")
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: tag.String()}}}
	results := s.api.DumpModels(stdcontext.Background(), models)
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
	s.setUpAPI(c)
	models := params.DumpModelRequest{Entities: []params.Entity{{Tag: s.st.ModelTag().String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModels(stdcontext.Background(), models)
		c.Assert(results.Results, gc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, gc.Equals, "")
		c.Assert(result.Error, gc.NotNil)
		c.Check(result.Error.Message, gc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestDumpModelsDB(c *gc.C) {
	s.setUpAPI(c)
	results := s.api.DumpModelsDB(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
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
	s.setUpAPI(c)
	s.st.SetErrors(errors.NotFoundf("boom"))
	tag := names.NewModelTag("deadbeef-0bad-400d-8000-4b1d0d06f000")
	models := params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
	results := s.api.DumpModelsDB(stdcontext.Background(), models)

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
	s.setUpAPI(c)
	models := params.Entities{Entities: []params.Entity{{Tag: s.st.ModelTag().String()}}}
	for _, user := range []names.UserTag{
		names.NewUserTag("otheruser"),
		names.NewUserTag("unknown"),
	} {
		s.setAPIUser(c, user)
		results := s.api.DumpModelsDB(stdcontext.Background(), models)
		c.Assert(results.Results, gc.HasLen, 1)
		result := results.Results[0]
		c.Assert(result.Result, gc.IsNil)
		c.Assert(result.Error, gc.NotNil)
		c.Check(result.Error.Message, gc.Equals, `permission denied`)
	}
}

func (s *modelManagerSuite) TestAddModelCanCreateModel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	addModelUser := names.NewUserTag("add-model")

	as := s.accessService.EXPECT()
	as.ReadUserAccessLevelForTarget(gomock.Any(), addModelUser.Id(), gomock.AssignableToTypeOf(permission.ID{})).Return(permission.AddModelAccess, nil)
	as.GetModelUsers(gomock.Any(), "add-model", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "add-model",
			DisplayName: "Addy McModel",
			Access:      permission.AdminAccess,
		}}, nil,
	)

	s.setAPIUser(c, addModelUser)
	_, err := s.api.CreateModel(stdcontext.Background(), createArgs(addModelUser))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerSuite) TestAddModelCantCreateModelForSomeoneElse(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	addModelUser := names.NewUserTag("add-model")

	s.accessService.EXPECT().ReadUserAccessLevelForTarget(gomock.Any(), addModelUser.Id(), gomock.AssignableToTypeOf(permission.ID{})).Return(permission.AddModelAccess, nil)

	s.setAPIUser(c, addModelUser)
	nonAdminUser := names.NewUserTag("non-admin")
	_, err := s.api.CreateModel(stdcontext.Background(), createArgs(nonAdminUser))
	c.Assert(err, gc.ErrorMatches, "\"add-model\" permission does not permit creation of models for different owners: permission denied")
}

func (s *modelManagerSuite) TestUpdatedModel(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.setUpAPI(c)
	as := s.accessService.EXPECT()
	modelUUID := modeltesting.GenModelUUID(c).String()
	testUser := names.NewUserTag("foobar")
	external := false
	updateArgs := access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        modelUUID,
			},
			Access: permission.WriteAccess,
		},
		AddUser:  true,
		External: &external,
		ApiUser:  jujutesting.AdminUser.Id(),
		Change:   permission.Grant,
		Subject:  testUser.Id(),
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

	results, err := s.api.ModifyModelAccess(stdcontext.Background(), args)
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

	return ctrl
}

func (s *modelManagerStateSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authoriser.Tag = user
	st := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool())
	ctlrSt := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool())

	serviceFactory := s.ControllerServiceFactory(c)

	urlGetter := common.NewToolsURLGetter(st.ModelUUID(), ctlrSt)
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	configGetter := stateenvirons.EnvironConfigGetter{Model: s.ControllerModel(c), CloudService: serviceFactory.Cloud(), CredentialService: serviceFactory.Credential()}
	newEnviron := common.EnvironFuncForModel(model, serviceFactory.Cloud(), serviceFactory.Credential(), configGetter)
	toolsFinder := common.NewToolsFinder(s.controllerConfigService, st, urlGetter, newEnviron, s.store)
	modelmanager, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{st}, nil, ctlrSt,
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		toolsFinder,
		nil,
		common.NewBlockChecker(st),
		s.authoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.modelmanager = modelmanager
}

func (s *modelManagerStateSuite) TestNewAPIAcceptsClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUserTag("external@remote")
	st := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool())
	serviceFactory := s.ControllerServiceFactory(c)

	endPoint, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{st},
		nil,
		common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		nil, nil, common.NewBlockChecker(st), anAuthoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *modelManagerStateSuite) TestNewAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authoriser
	anAuthoriser.Tag = names.NewUnitTag("mysql/0")
	st := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool())
	serviceFactory := s.ControllerServiceFactory(c)

	endPoint, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{st},
		nil,
		common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		nil, nil, common.NewBlockChecker(st), anAuthoriser, s.ControllerModel(c),
	)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) createArgsForVersion(c *gc.C, owner names.UserTag, ver interface{}) params.ModelCreateArgs {
	params := createArgs(owner)
	params.Config["agent-version"] = ver
	return params
}

func (s *modelManagerStateSuite) TestUserCanCreateModel(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)

	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	model, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
	c.Assert(model.Type, gc.Equals, "iaas")
}

func (s *modelManagerStateSuite) TestAdminCanCreateModelForSomeoneElse(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)

	s.setAPIUser(c, jujutesting.AdminUser)
	owner := names.NewUserTag("external@remote")

	model, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.OwnerTag, gc.Equals, owner.String())
	c.Assert(model.Name, gc.Equals, "test-model")
	c.Assert(model.Type, gc.Equals, "iaas")
	// Make sure that the environment created does actually have the correct
	// owner, and that owner is actually allowed to use the environment.
	newState, err := s.StatePool().Get(model.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer newState.Release()

	newModel, err := newState.Model()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(newModel.Owner(), gc.Equals, owner)
	_, err = newState.UserAccess(owner, newModel.ModelTag())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSomeoneElse(c *gc.C) {
	defer s.setupMocks(c).Finish()

	userTag := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, userTag)
	as := s.accessService.EXPECT()
	id := permission.ID{
		ObjectType: permission.Cloud,
		Key:        "dummy",
	}
	as.ReadUserAccessLevelForTarget(gomock.Any(), userTag.Id(), id).Return(permission.WriteAccess, nil)
	owner := names.NewUserTag("external@remote")
	_, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestNonAdminCannotCreateModelForSelf(c *gc.C) {
	defer s.setupMocks(c).Finish()

	owner := names.NewUserTag("non-admin@remote")
	s.setAPIUser(c, owner)
	as := s.accessService.EXPECT()
	id := permission.ID{
		ObjectType: permission.Cloud,
		Key:        "dummy",
	}
	as.ReadUserAccessLevelForTarget(gomock.Any(), owner.Id(), id).Return(permission.WriteAccess, nil)

	_, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *modelManagerStateSuite) TestCreateModelValidatesConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	admin := jujutesting.AdminUser
	s.setAPIUser(c, admin)
	args := createArgs(admin)
	args.Config["somebool"] = "maybe"
	_, err := s.modelmanager.CreateModel(stdcontext.Background(), args)
	c.Assert(err, gc.ErrorMatches,
		"failed to create config: provider config preparation failed: somebool: expected bool, got string\\(\"maybe\"\\)",
	)
}

func (s *modelManagerStateSuite) TestCreateModelSameAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "add-model",
			DisplayName: "Addy McModel",
			Access:      permission.AdminAccess,
		}}, nil,
	)

	admin := jujutesting.AdminUser
	s.setAPIUser(c, admin)
	args := s.createArgsForVersion(c, admin, jujuversion.Current.String())
	_, err := s.modelmanager.CreateModel(stdcontext.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *modelManagerStateSuite) TestCreateModelBadAgentVersion(c *gc.C) {
	defer s.setupMocks(c).Finish()
	err := s.ControllerModel(c).State().SetModelAgentVersion(coretesting.FakeVersionNumber, nil, false, stubUpgrader{})
	c.Assert(err, jc.ErrorIsNil)

	admin := jujutesting.AdminUser
	s.setAPIUser(c, admin)

	bigger := coretesting.FakeVersionNumber
	bigger.Minor += 1

	smaller := coretesting.FakeVersionNumber
	smaller.Minor -= 1

	for i, test := range []struct {
		value    interface{}
		errMatch string
	}{
		{
			value:    42,
			errMatch: `failed to create config: agent-version must be a string but has type 'int'`,
		}, {
			value:    "not a number",
			errMatch: `failed to create config: invalid version \"not a number\"`,
		}, {
			value:    bigger.String(),
			errMatch: "failed to create config: agent-version .* cannot be greater than the controller .*",
		}, {
			value:    smaller.String(),
			errMatch: "failed to create config: no agent binaries found for version .*",
		},
	} {
		c.Logf("test %d", i)
		args := s.createArgsForVersion(c, admin, test.value)
		_, err := s.modelmanager.CreateModel(stdcontext.Background(), args)
		c.Check(err, gc.ErrorMatches, test.errMatch)
	}
}

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
//	result, err := s.modelmanager.ListModels(stdcontext.Background(), params.Entity{Tag: userTag.String()})
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
//	result, err := s.modelmanager.ListModels(stdcontext.Background(), params.Entity{Tag: other.String()})
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
//	_, err := s.modelmanager.ListModels(stdcontext.Background(), params.Entity{Tag: other.String()})
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
	defer s.setupMocks(c).Finish()
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)

	// TODO(perrito666) this test is not valid until we have
	// proper controller permission since the only users that
	// can create models are controller admins.
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.StatePool().Get(m.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)
	backend := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), model, s.StatePool())

	serviceFactory := s.ControllerServiceFactory(c)

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{backend},
		nil,
		common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		nil, nil, common.NewBlockChecker(backend), s.authoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	force := true
	timeout := time.Minute
	results, err := s.modelmanager.DestroyModels(stdcontext.Background(), params.DestroyModelsParams{
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
	defer s.setupMocks(c).Finish()

	// TODO(perrito666) Both users are admins in this case, this tesst is of dubious
	// usefulness until proper controller permissions are in place.
	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)
	m, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.StatePool().Get(m.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	s.authoriser.Tag = jujutesting.AdminUser
	backend := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), model, s.StatePool())

	serviceFactory := s.ControllerServiceFactory(c)

	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{backend},
		nil,
		common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		nil, nil, common.NewBlockChecker(backend), s.authoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	results, err := s.modelmanager.DestroyModels(stdcontext.Background(), params.DestroyModelsParams{
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
	defer s.setupMocks(c).Finish()
	s.accessService.EXPECT().GetModelUsers(gomock.Any(), "admin", gomock.Any()).Return(
		[]access.ModelUserInfo{{
			Name:        "admin",
			DisplayName: "Admin",
			Access:      permission.AdminAccess,
		}}, nil,
	)

	owner := names.NewUserTag("admin")
	s.setAPIUser(c, owner)
	m, err := s.modelmanager.CreateModel(stdcontext.Background(), createArgs(owner))
	c.Assert(err, jc.ErrorIsNil)

	st, err := s.StatePool().Get(m.UUID)
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()
	model, err := st.Model()
	c.Assert(err, jc.ErrorIsNil)

	serviceFactory := s.ControllerServiceFactory(c)

	backend := common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), model, s.StatePool())
	s.modelmanager, err = modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{backend},
		nil,
		common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		nil, nil, common.NewBlockChecker(backend), s.authoriser, s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)

	user := names.NewUserTag("other@remote")
	s.setAPIUser(c, user)

	results, err := s.modelmanager.DestroyModels(stdcontext.Background(), params.DestroyModelsParams{
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

	result, err := s.modelmanager.ModifyModelAccess(stdcontext.Background(), args)
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

	serviceFactory := s.ControllerServiceFactory(c)

	anAuthoriser := s.authoriser
	anAuthoriser.Tag = user
	st := common.NewUserAwareModelManagerBackend(s.ConfigSchemaSourceGetter(c), model, s.StatePool(), user)
	endPoint, err := modelmanager.NewModelManagerAPI(
		context.Background(),
		mockCredentialShim{st},
		nil,
		common.NewModelManagerBackend(s.ConfigSchemaSourceGetter(c), s.ControllerModel(c), s.StatePool()),
		s.controllerUUID,
		modelmanager.Services{
			ServiceFactoryGetter: nil,
			CloudService:         serviceFactory.Cloud(),
			CredentialService:    serviceFactory.Credential(),
			ModelService:         nil,
			ModelDefaultsService: nil,
			AccessService:        s.accessService,
			ObjectStore:          &mockObjectStore{},
		},
		s.ConfigSchemaSourceGetter(c),
		nil, nil, common.NewBlockChecker(st), anAuthoriser,
		s.ControllerModel(c),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(endPoint, gc.NotNil)

	res, err := endPoint.ModelInfo(
		stdcontext.Background(),
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
	s.setUpAPI(c)
	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ModelStatus(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: "bad-tag",
	}, {
		Tag: s.st.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we don't err out if a model errs even if some firsts in collection pass.
	results, err = s.api.ModelStatus(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.st.ModelTag().String(),
	}, {
		Tag: "bad-tag",
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, `"bad-tag" is not a valid tag`)

	// Check that we return successfully if no errors.
	results, err = s.api.ModelStatus(stdcontext.Background(), params.Entities{Entities: []params.Entity{{
		Tag: s.st.ModelTag().String(),
	}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
}

func (s *modelManagerSuite) TestChangeModelCredential(c *gc.C) {
	s.setUpAPI(c)
	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return true, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	results, err := s.api.ChangeModelCredential(stdcontext.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)
}

func (s *modelManagerSuite) TestChangeModelCredentialBulkUninterrupted(c *gc.C) {
	s.setUpAPI(c)
	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return true, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	// Check that we don't err out immediately if a model errs.
	results, err := s.api.ChangeModelCredential(stdcontext.Background(), params.ChangeModelCredentialsParams{
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
	results, err = s.api.ChangeModelCredential(stdcontext.Background(), params.ChangeModelCredentialsParams{
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
	s.setUpAPI(c)
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	apiUser := names.NewUserTag("bob@remote")
	s.setAPIUser(c, apiUser)

	results, err := s.api.ChangeModelCredential(stdcontext.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `permission denied`)
}

func (s *modelManagerSuite) TestChangeModelCredentialGetModelFail(c *gc.C) {
	s.setUpAPI(c)
	s.st.SetErrors(errors.New("getting model"))
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()

	results, err := s.api.ChangeModelCredential(stdcontext.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `getting model`)
	s.st.CheckCallNames(c, "ControllerTag", "ModelTag", "GetBlockForType", "ControllerTag", "GetModel")
}

func (s *modelManagerSuite) TestChangeModelCredentialNotUpdated(c *gc.C) {
	s.setUpAPI(c)
	s.st.model.setCloudCredentialF = func(tag names.CloudCredentialTag) (bool, error) { return false, nil }
	credentialTag := names.NewCloudCredentialTag("foo/bob/bar").String()
	results, err := s.api.ChangeModelCredential(stdcontext.Background(), params.ChangeModelCredentialsParams{
		Models: []params.ChangeModelCredentialParams{
			{ModelTag: s.st.ModelTag().String(), CloudCredentialTag: credentialTag},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.ErrorMatches, `model deadbeef-0bad-400d-8000-4b1d0d06f00d already uses credential foo/bob/bar`)
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

type stubUpgrader struct{}

func (stubUpgrader) IsUpgrading() (bool, error) {
	return false, nil
}

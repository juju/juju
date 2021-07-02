// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"regexp"
	"strings"
	"time"

	"github.com/juju/charm/v8"
	csparams "github.com/juju/charmrepo/v6/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/schema"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type ApplicationSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
	backend            mockBackend
	model              mockModel
	leadership         mockLeadership
	endpoints          []state.Endpoint
	relation           mockRelation
	application        mockApplication
	storagePoolManager *mockStoragePoolManager
	registry           *mockStorageRegistry
	updateSeries       *mockUpdateSeries

	caasBroker   *mockCaasBroker
	env          environs.Environ
	blockChecker mockBlockChecker
	authorizer   apiservertesting.FakeAuthorizer
	api          *application.APIv13
	deployParams map[string]application.DeployApplicationParams
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	s.storagePoolManager = &mockStoragePoolManager{storageType: k8sconstants.StorageProviderType}
	s.registry = &mockStorageRegistry{}
	s.caasBroker = &mockCaasBroker{}
	api, err := application.NewAPIBase(
		&s.backend,
		&s.backend,
		s.authorizer,
		s.updateSeries,
		&s.blockChecker,
		&s.model,
		&s.leadership,
		func(application.Charm) *state.Charm {
			return &state.Charm{}
		},
		func(_ application.ApplicationDeployer, p application.DeployApplicationParams) (application.Application, error) {
			s.deployParams[p.ApplicationName] = p
			return nil, nil
		},
		s.storagePoolManager,
		s.registry,
		common.NewResources(),
		s.caasBroker,
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = &application.APIv13{api}
}

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	agentTools := &tools.Tools{
		Version: version.Binary{
			Number:  version.Number{Major: 2, Minor: 6, Patch: 0},
			Release: "ubuntu",
			Arch:    "x86",
		},
	}
	olderAgentTools := &tools.Tools{
		Version: version.Binary{
			Number:  version.Number{Major: 2, Minor: 5, Patch: 1},
			Release: "ubuntu",
			Arch:    "x86",
		},
	}
	lxdProfile := &charm.LXDProfile{
		Config: map[string]string{
			"security.nested": "false",
		},
	}
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}
	s.updateSeries = &mockUpdateSeries{}
	s.deployParams = make(map[string]application.DeployApplicationParams)
	s.env = &mockEnviron{}
	s.endpoints = []state.Endpoint{
		{ApplicationName: "postgresql"},
		{ApplicationName: "bar"},
	}
	s.relation = mockRelation{tag: names.NewRelationTag("wordpress:db mysql:db")}
	s.model = newMockModel()
	s.backend = mockBackend{
		controllers: make(map[string]crossmodel.ControllerInfo),
		applications: map[string]*mockApplication{
			"postgresql": {
				name:        "postgresql",
				series:      "quantal",
				subordinate: false,
				curl:        charm.MustParseURL("cs:postgresql-42"),
				charm: &mockCharm{
					config: &charm.Config{
						Options: map[string]charm.Option{
							"stringOption": {Type: "string"},
							"intOption":    {Type: "int", Default: int(123)},
						},
					},
					meta:       &charm.Meta{Name: "charm-postgresql"},
					lxdProfile: lxdProfile,
				},
				units: []*mockUnit{
					{
						name:       "postgresql/0",
						tag:        names.NewUnitTag("postgresql/0"),
						machineId:  "0",
						agentTools: agentTools,
					},
					{
						name:       "postgresql/1",
						tag:        names.NewUnitTag("postgresql/1"),
						machineId:  "1",
						agentTools: agentTools,
					},
				},
				addedUnit: mockUnit{
					tag: names.NewUnitTag("postgresql/99"),
				},
				constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
				channel:     csparams.DevelopmentChannel,
				bindings: map[string]string{
					"juju-info": "myspace",
				},
				agentTools: agentTools,
			},
			"postgresql-subordinate": {
				name:        "postgresql-subordinate",
				series:      "quantal",
				subordinate: true,
				charm: &mockCharm{
					config: &charm.Config{
						Options: map[string]charm.Option{
							"stringOption": {Type: "string"},
							"intOption":    {Type: "int", Default: int(123)},
						},
					},
					meta:       &charm.Meta{Name: "charm-postgresql-subordinate"},
					lxdProfile: lxdProfile,
				},
				units: []*mockUnit{
					{
						tag:        names.NewUnitTag("postgresql-subordinate/0"),
						agentTools: agentTools,
					},
					{
						tag:        names.NewUnitTag("postgresql-subordinate/1"),
						agentTools: agentTools,
					},
				},
				addedUnit: mockUnit{
					tag: names.NewUnitTag("postgresql-subordinate/99"),
				},
				constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
				channel:     csparams.DevelopmentChannel,
				agentTools:  agentTools,
			},
			"redis": {
				name:        "redis",
				series:      "quantal",
				subordinate: false,
				charm: &mockCharm{
					config: &charm.Config{
						Options: map[string]charm.Option{
							"stringOption": {Type: "string"},
							"intOption":    {Type: "int", Default: int(123)},
						},
					},
					meta:       &charm.Meta{Name: "charm-redis"},
					lxdProfile: lxdProfile,
				},
				units: []*mockUnit{
					{
						name:       "redis/0",
						tag:        names.NewUnitTag("redis/0"),
						machineId:  "0",
						agentTools: olderAgentTools,
					},
					{
						name:       "redis/1",
						tag:        names.NewUnitTag("redis/1"),
						machineId:  "1",
						agentTools: olderAgentTools,
					},
				},
				addedUnit: mockUnit{
					tag: names.NewUnitTag("redis/99"),
				},
				constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
				channel:     csparams.DevelopmentChannel,
				bindings: map[string]string{
					"juju-info": "myspace",
				},
				agentTools: agentTools,
			},
			"test-app-info": {
				name:        "test-app-info",
				series:      "quantal",
				subordinate: false,
				charm: &mockCharm{
					config: &charm.Config{
						Options: map[string]charm.Option{
							"stringOption": {Type: "string"},
							"intOption":    {Type: "int", Default: int(123)},
						},
					},
					meta:       &charm.Meta{Name: "charm-test-app-info"},
					lxdProfile: lxdProfile,
				},
				constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
				channel:     csparams.DevelopmentChannel,
				charmOrigin: &state.CharmOrigin{Channel: &state.Channel{
					Track: "2.0",
					Risk:  "candidate",
				}},
				bindings: map[string]string{
					"juju-info": "myspace",
				},
			},
		},
		remoteApplications: map[string]application.RemoteApplication{
			"hosted-db2": &mockRemoteApplication{},
		},
		charm: &mockCharm{
			meta: &charm.Meta{},
			config: &charm.Config{
				Options: map[string]charm.Option{
					"stringOption": {Type: "string"},
					"intOption":    {Type: "int", Default: int(123)}},
			},
			lxdProfile: lxdProfile,
		},
		endpoints: &s.endpoints,
		relations: map[int]*mockRelation{
			123: &s.relation,
		},
		offerConnections: make(map[string]application.OfferConnection),
		unitStorageAttachments: map[string][]state.StorageAttachment{
			"postgresql/0": {
				&mockStorageAttachment{
					unit:    names.NewUnitTag("postgresql/0"),
					storage: names.NewStorageTag("pgdata/0"),
				},
				&mockStorageAttachment{
					unit:    names.NewUnitTag("foo/0"),
					storage: names.NewStorageTag("pgdata/1"),
				},
			},
		},
		storageInstances: map[string]*mockStorage{
			"pgdata/0": {
				tag:   names.NewStorageTag("pgdata/0"),
				owner: names.NewUnitTag("postgresql/0"),
			},
			"pgdata/1": {
				tag:   names.NewStorageTag("pgdata/1"),
				owner: names.NewUnitTag("foo/0"),
			},
		},
		storageInstanceFilesystems: map[string]*mockFilesystem{
			"pgdata/0": {detachable: true},
			"pgdata/1": {detachable: false},
		},
	}
	s.blockChecker = mockBlockChecker{}
	s.setAPIUser(c, names.NewUserTag("admin"))
}

func (s *ApplicationSuite) TearDownTest(c *gc.C) {
	s.JujuOSEnvSuite.TearDownTest(c)
	s.IsolationSuite.TearDownTest(c)
}

func (s *ApplicationSuite) TestSetCharmStorageConstraints(c *gc.C) {
	toUint64Ptr := func(v uint64) *uint64 {
		return &v
	}
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		StorageConstraints: map[string]params.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: toUint64Ptr(123)},
			"d": {Count: toUint64Ptr(456)},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application", "Charm")
	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 2, "SetCharm", state.SetCharmConfig{
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{},
		},
		StorageConstraints: map[string]state.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: 123},
			"d": {Count: 456},
		},
	})
}

func (s *ApplicationSuite) TestSetCAASCharmInvalid(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{},
		},
	}
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
	})
	c.Assert(err, gc.NotNil)
	msg := strings.Replace(err.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, "Juju on containers does not support updating deployment info.*")
}

func (s *ApplicationSuite) TestUpdateCAASApplicationSettings(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeOperator,
			},
		},
	}

	// Update settings for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "postgresql",
		SettingsYAML:    "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}
	api := &application.APIv12{s.api}
	err := api.Update(args)
	c.Assert(err, jc.ErrorIsNil)

	pgApp := s.backend.applications["postgresql"]
	pgApp.CheckCall(c, 2, "UpdateCharmConfig", "master", charm.Settings{
		"stringOption": "bar",
	})

	appDefaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	appCfgSchema, appDefaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, appDefaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "foo",
	}, appCfgSchema, appDefaults)
	c.Assert(err, jc.ErrorIsNil)

	pgApp.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
}

func (s *ApplicationSuite) TestSetCAASConfigSettings(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeOperator,
			},
		},
	}

	// Update settings for the application.
	args := params.ConfigSetArgs{Args: []params.ConfigSet{{
		ApplicationName: "postgresql",
		ConfigYAML:      "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}}}
	results, err := s.api.SetConfigs(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{}})

	pgApp := s.backend.applications["postgresql"]
	pgApp.CheckCall(c, 2, "UpdateCharmConfig", "master", charm.Settings{
		"stringOption": "bar",
	})

	appDefaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	appCfgSchema, appDefaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, appDefaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "foo",
	}, appCfgSchema, appDefaults)
	c.Assert(err, jc.ErrorIsNil)

	pgApp.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
}

func (s *ApplicationSuite) TestUpdateCAASApplicationSettingsInIAASModelTriggersError(c *gc.C) {
	s.model.modelType = state.ModelTypeIAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeOperator,
			},
		},
	}

	// Update settings for the application.
	args := params.ApplicationUpdate{
		ApplicationName: "postgresql",
		SettingsYAML:    "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}
	api := &application.APIv12{s.api}
	err := api.Update(args)
	c.Assert(err, gc.ErrorMatches, `.*unknown option "juju-external-hostname"`, gc.Commentf("expected to get an error when attempting to set CAAS-specific app setting in IAAS model"))
}

func (s *ApplicationSuite) TestSetCAASConfigSettingsInIAASModelTriggersError(c *gc.C) {
	s.model.modelType = state.ModelTypeIAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeOperator,
			},
		},
	}

	// Update settings for the application.
	args := params.ConfigSetArgs{Args: []params.ConfigSet{{
		ApplicationName: "postgresql",
		ConfigYAML:      "postgresql:\n  stringOption: bar\n  juju-external-hostname: foo",
	}}}

	results, err := s.api.SetConfigs(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.DeepEquals, []params.ErrorResult{{
		Error: &params.Error{
			Message: "parsing settings for application: unknown option \"juju-external-hostname\"",
		},
	}}, gc.Commentf("expected to get an error when attempting to set CAAS-specific app setting in IAAS model"))
}

func (s *ApplicationSuite) TestSetCharmConfigSettings(c *gc.C) {
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application", "Charm")
	s.backend.charm.CheckCallNames(c, "Config")
	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 2, "SetCharm", state.SetCharmConfig{
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	})
}

func (s *ApplicationSuite) TestSetCharmConfigSettingsYAML(c *gc.C) {
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettingsYAML: `
postgresql:
  stringOption: value
`,
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application", "Charm")
	s.backend.charm.CheckCallNames(c, "Config")
	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 2, "SetCharm", state.SetCharmConfig{
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	})
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithNewerAgentVersion(c *gc.C) {
	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application", "Charm")
	s.backend.charm.CheckCallNames(c, "Config")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "AgentTools", "SetCharm")
	app.CheckCall(c, 2, "SetCharm", state.SetCharmConfig{
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	})
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithOldAgentVersion(c *gc.C) {
	// Patch the mock model to always be behind the epoch.
	s.model.cfg["agent-version"] = "2.5.0"

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "redis",
		CharmURL:        "cs:redis",
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, gc.ErrorMatches, "Unable to upgrade LXDProfile charms with the current model version. "+
		"Please run juju upgrade-juju to upgrade the current model to match your controller.")

	s.backend.CheckCallNames(c, "Application", "Charm")
	app := s.backend.applications["redis"]
	app.CheckCallNames(c, "Charm", "AgentTools")
}

func (s *ApplicationSuite) TestLXDProfileSetCharmWithEmptyProfile(c *gc.C) {
	// Patch the mock backend charm profile to have an empty value, so that it
	// shows how SetCharm profile works with empty profiles.
	s.backend.charm.lxdProfile = &charm.LXDProfile{}

	err := s.api.SetCharm(params.ApplicationSetCharm{
		ApplicationName: "postgresql",
		CharmURL:        "cs:postgresql",
		ConfigSettings:  map[string]string{"stringOption": "value"},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application", "Charm")
	s.backend.charm.CheckCallNames(c, "Config")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "AgentTools", "SetCharm")
	app.CheckCall(c, 2, "SetCharm", state.SetCharmConfig{
		Charm: &state.Charm{},
		CharmOrigin: &state.CharmOrigin{
			Source:   "charm-store",
			Platform: &state.Platform{},
		},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	})
}

func (s *ApplicationSuite) TestDestroyRelation(c *gc.C) {
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, jc.ErrorIsNil)
	s.blockChecker.CheckCallNames(c, "RemoveAllowed")
	s.backend.CheckCallNames(c, "InferEndpoints", "EndpointsRelation")
	s.backend.CheckCall(c, 0, "InferEndpoints", []string{"a", "b"})
	s.relation.CheckCallNames(c, "DestroyWithForce")
}

func (s *ApplicationSuite) TestDestroyRelationNoRelationsFound(c *gc.C) {
	s.backend.SetErrors(nil, errors.New("no relations found"))
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *ApplicationSuite) TestDestroyRelationRelationNotFound(c *gc.C) {
	s.backend.SetErrors(nil, errors.NotFoundf(`relation "a:b c:d"`))
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a:b", "c:d"}})
	c.Assert(err, gc.ErrorMatches, `relation "a:b c:d" not found`)
}

func (s *ApplicationSuite) TestBlockRemoveDestroyRelation(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("postgresql"))
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "postgresql")
	s.blockChecker.CheckCallNames(c, "RemoveAllowed")
	s.backend.CheckNoCalls(c)
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestDestroyRelationId(c *gc.C) {
	err := s.api.DestroyRelation(params.DestroyRelation{RelationId: 123})
	c.Assert(err, jc.ErrorIsNil)
	s.blockChecker.CheckCallNames(c, "RemoveAllowed")
	s.backend.CheckCallNames(c, "Relation")
	s.backend.CheckCall(c, 0, "Relation", 123)
	s.relation.CheckCallNames(c, "DestroyWithForce")
}

func (s *ApplicationSuite) TestDestroyRelationIdRelationNotFound(c *gc.C) {
	s.backend.SetErrors(errors.NotFoundf(`relation "123"`))
	err := s.api.DestroyRelation(params.DestroyRelation{RelationId: 123})
	c.Assert(err, gc.ErrorMatches, `relation "123" not found`)
}

func (s *ApplicationSuite) TestDestroyApplication(c *gc.C) {
	s.assertDestroyApplication(c, false, nil)
}

func (s *ApplicationSuite) TestForceDestroyApplication(c *gc.C) {
	zero := time.Duration(0)
	s.assertDestroyApplication(c, true, &zero)
}

func (s *ApplicationSuite) assertDestroyApplication(c *gc.C, force bool, maxWait *time.Duration) {
	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
			Force:          force,
			MaxWait:        maxWait,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Info: &params.DestroyApplicationInfo{
			DestroyedUnits: []params.Entity{
				{Tag: "unit-postgresql-0"},
				{Tag: "unit-postgresql-1"},
			},
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	})

	s.backend.CheckCallNames(c,
		"Application",
		"UnitStorageAttachments",
		"StorageInstance",
		"StorageInstance",
		"StorageInstanceFilesystem",
		"StorageInstanceFilesystem",
		"UnitStorageAttachments",
		"ApplyOperation",
	)
	expectedOp := &state.DestroyApplicationOperation{ForcedOperation: state.ForcedOperation{Force: force}}
	if force {
		expectedOp.MaxWait = common.MaxWait(maxWait)
	}
	s.backend.CheckCall(c, 7, "ApplyOperation", expectedOp)
}

func (s *ApplicationSuite) TestDestroyApplicationDestroyStorage(c *gc.C) {
	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
			DestroyStorage: true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Info: &params.DestroyApplicationInfo{
			DestroyedUnits: []params.Entity{
				{Tag: "unit-postgresql-0"},
				{Tag: "unit-postgresql-1"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
				{Tag: "storage-pgdata-1"},
			},
		},
	})

	s.backend.CheckCallNames(c,
		"Application",
		"UnitStorageAttachments",
		"StorageInstance",
		"StorageInstance",
		"UnitStorageAttachments",
		"ApplyOperation",
	)
	s.backend.CheckCall(c, 5, "ApplyOperation", &state.DestroyApplicationOperation{
		DestroyStorage: true,
	})
}

func (s *ApplicationSuite) TestDestroyApplicationNotFound(c *gc.C) {
	delete(s.backend.applications, "postgresql")
	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{
			{ApplicationTag: "application-postgresql"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `application "postgresql" not found`,
		},
	})
}

func (s *ApplicationSuite) TestDestroyConsumedApplication(c *gc.C) {
	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{ApplicationTag: "application-hosted-db2"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{})

	s.backend.CheckCallNames(c, "RemoteApplication", "ApplyOperation")
	app := s.backend.remoteApplications["hosted-db2"]
	app.(*mockRemoteApplication).CheckCallNames(c, "DestroyOperation")
}

func (s *ApplicationSuite) TestForceDestroyConsumedApplication(c *gc.C) {
	force := true
	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{
			ApplicationTag: "application-hosted-db2",
			Force:          &force,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{})

	s.backend.CheckCallNames(c, "RemoteApplication", "ApplyOperation")
	app := s.backend.remoteApplications["hosted-db2"]
	app.(*mockRemoteApplication).CheckCallNames(c, "DestroyOperation")
}

func (s *ApplicationSuite) TestForceDestroyConsumedApplicationNoWait(c *gc.C) {
	force := true
	noWait := 0 * time.Minute
	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{
			ApplicationTag: "application-hosted-db2",
			Force:          &force,
			MaxWait:        &noWait,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{})

	s.backend.CheckCallNames(c, "RemoteApplication", "ApplyOperation")
	app := s.backend.remoteApplications["hosted-db2"]
	app.(*mockRemoteApplication).CheckCallNames(c, "DestroyOperation")
}

func (s *ApplicationSuite) TestDestroyConsumedApplicationNotFound(c *gc.C) {
	delete(s.backend.remoteApplications, "hosted-db2")
	results, err := s.api.DestroyConsumedApplications(params.DestroyConsumedApplicationsParams{
		Applications: []params.DestroyConsumedApplicationParams{{ApplicationTag: "application-hosted-db2"}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `remote application "hosted-db2" not found`,
		},
	})
}

func (s *ApplicationSuite) TestDestroyUnit(c *gc.C) {
	s.assertDestroyUnit(c, false, nil)
}

func (s *ApplicationSuite) TestForceDestroyUnit(c *gc.C) {
	zero := time.Second * 0
	s.assertDestroyUnit(c, true, &zero)
}

func (s *ApplicationSuite) assertDestroyUnit(c *gc.C, force bool, maxWait *time.Duration) {
	results, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{
				UnitTag: "unit-postgresql-0",
				Force:   force,
				MaxWait: maxWait,
			}, {
				UnitTag:        "unit-postgresql-1",
				DestroyStorage: true,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results, jc.DeepEquals, []params.DestroyUnitResult{{
		Info: &params.DestroyUnitInfo{
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	}, {
		Info: &params.DestroyUnitInfo{},
	}})

	s.backend.CheckCallNames(c,
		"Unit",
		"UnitStorageAttachments",
		"StorageInstance",
		"StorageInstance",
		"StorageInstanceFilesystem",
		"StorageInstanceFilesystem",
		"ApplyOperation",

		"Unit",
		"UnitStorageAttachments",
		"ApplyOperation",
	)
	expectedOp := &state.DestroyUnitOperation{ForcedOperation: state.ForcedOperation{Force: force}}
	if force {
		expectedOp.MaxWait = common.MaxWait(maxWait)
	}
	s.backend.CheckCall(c, 6, "ApplyOperation", expectedOp)
	s.backend.CheckCall(c, 9, "ApplyOperation", &state.DestroyUnitOperation{
		DestroyStorage: true,
	})
}

func (s *ApplicationSuite) TestDeployAttachStorage(c *gc.C) {
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			AttachStorage:   []string{"storage-foo-0"},
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-1",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        2,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-2",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			AttachStorage:   []string{"volume-baz-0"},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, "AttachStorage is non-empty, but NumUnits is 2")
	c.Assert(results.Results[2].Error, gc.ErrorMatches, `"volume-baz-0" is not a valid volume tag`)
}

func (s *ApplicationSuite) TestDeployCharmOrigin(c *gc.C) {
	track := "latest"
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}, {
			ApplicationName: "bar",
			CharmURL:        "cs:bar-0",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-store",
				Risk:   "stable",
				Track:  &track,
			},
			NumUnits: 1,
		}, {
			ApplicationName: "hub",
			CharmURL:        "hub-0",
			CharmOrigin: &params.CharmOrigin{
				Source: "charm-hub",
				Risk:   "stable",
			},
			NumUnits: 1,
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error, gc.IsNil)

	c.Assert(s.deployParams["foo"].CharmOrigin.Source, gc.Equals, corecharm.Source("local"))
	c.Assert(s.deployParams["bar"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-store"))
	c.Assert(s.deployParams["hub"].CharmOrigin.Source, gc.Equals, corecharm.Source("charm-hub"))
}

func (s *ApplicationSuite) TestDeployMinDeploymentVersionTooHigh(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				MinVersion: "1.99.0",
			},
		},
		config: &charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	}
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-annotations": "a=b c="},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(
		results.Results[0].Error, gc.ErrorMatches,
		regexp.QuoteMeta(`charm requires a minimum k8s version of 1.99.0 but the cluster only runs version 1.15.0`),
	)
}

func (s *ApplicationSuite) TestDeployCAASModel(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{},
		config: &charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	}
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-annotations": "a=b c="},
			ConfigYAML:      "foo:\n  stringOption: fred\n  kubernetes-service-type: loadbalancer",
		}, {
			ApplicationName: "foobar",
			CharmURL:        "local:foobar-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-type": "cluster", "intOption": "2"},
			ConfigYAML:      "foobar:\n  intOption: 1\n  kubernetes-service-type: loadbalancer\n  kubernetes-ingress-ssl-redirect: true",
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			Placement:       []*instance.Placement{{}, {}},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 4)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.IsNil)
	c.Assert(results.Results[2].Error, gc.ErrorMatches, "AttachStorage may not be specified for container models")
	c.Assert(results.Results[3].Error, gc.ErrorMatches, "only 1 placement directive is supported for container models, got 2")

	c.Assert(s.deployParams["foo"].ApplicationConfig.Attributes()["kubernetes-service-type"], gc.Equals, "loadbalancer")
	// Check parsing of k8s service annotations.
	c.Assert(s.deployParams["foo"].ApplicationConfig.Attributes()["kubernetes-service-annotations"], jc.DeepEquals, map[string]string{"a": "b", "c": ""})
	c.Assert(s.deployParams["foobar"].ApplicationConfig.Attributes()["kubernetes-service-type"], gc.Equals, "cluster")
	c.Assert(s.deployParams["foobar"].ApplicationConfig.Attributes()["kubernetes-ingress-ssl-redirect"], gc.Equals, true)
	c.Assert(s.deployParams["foobar"].CharmConfig, jc.DeepEquals, charm.Settings{"intOption": int64(2)})
}

func (s *ApplicationSuite) TestDeployCAASInvalidServiceType(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{},
		config: &charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	}

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
			Config:          map[string]string{"kubernetes-service-type": "ClusterIP", "intOption": "2"},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.OneError(), gc.ErrorMatches, `service type "ClusterIP" not valid`)
}

func (s *ApplicationSuite) TestDeployCAASBlockStorageRejected(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			Storage: map[string]charm.Storage{"block": {Name: "block", Type: charm.StorageBlock}},
		},
	}

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.OneError(), gc.ErrorMatches, `block storage "block" is not supported for container charms`)
}

func (s *ApplicationSuite) TestDeployCAASModelNoOperatorStorage(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	delete(s.model.cfg, "operator-storage")
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `deploying this Kubernetes application requires a suitable storage class.*`)
}

func (s *ApplicationSuite) TestDeployCAASModelCharmNeedsNoOperatorStorage(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	delete(s.model.cfg, "operator-storage")
	s.PatchValue(&jujuversion.Current, version.MustParse("2.8-beta1"))
	s.backend.charm = &mockCharm{
		meta: &charm.Meta{
			MinJujuVersion: version.MustParse("2.8.0"),
		},
	}

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelSidecarCharmNeedsNoOperatorStorage(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	delete(s.model.cfg, "operator-storage")
	s.backend.charm = &mockCharm{
		meta:     &charm.Meta{},
		manifest: &charm.Manifest{Bases: []charm.Base{{}}},
	}

	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelDefaultOperatorStorageClass(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.storagePoolManager.SetErrors(errors.NotFoundf("pool"))
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestDeployCAASModelWrongOperatorStorageType(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.storagePoolManager.storageType = provider.RootfsProviderType
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `the "k8s-operator-storage" storage pool requires a provider type of "kubernetes", not "rootfs"`)
}

func (s *ApplicationSuite) TestDeployCAASModelInvalidStorage(c *gc.C) {
	s.caasBroker.SetErrors(errors.NotFoundf("storage class"))
	s.model.modelType = state.ModelTypeCAAS
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			Storage: map[string]storage.Constraints{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	msg := result.OneError().Error()
	c.Assert(strings.Replace(msg, "\n", "", -1), gc.Matches, `storage class not found`)
}

func (s *ApplicationSuite) TestDeployCAASModelDefaultStorageClass(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			CharmOrigin:     &params.CharmOrigin{Source: "local"},
			NumUnits:        1,
			Storage: map[string]storage.Constraints{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results[0].Error, gc.IsNil)
}

func (s *ApplicationSuite) TestAddUnits(c *gc.C) {
	results, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.AddApplicationUnitsResults{
		Units: []string{"postgresql/99"},
	})
	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 0, "AddUnit", state.AddUnitParams{})
	app.addedUnit.CheckCall(c, 0, "AssignWithPolicy", state.AssignCleanEmpty)
}

func (s *ApplicationSuite) TestAddUnitsCAASModel(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
	})
	c.Assert(err, gc.ErrorMatches, "adding units to a container-based model not supported")
	app := s.backend.applications["postgresql"]
	app.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestDestroyUnitsCAASModel(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	_, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{UnitTag: "unit-postgresql-0"},
			{
				UnitTag:        "unit-postgresql-1",
				DestroyStorage: true,
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, "removing units on a non-container model not supported")
	app := s.backend.applications["postgresql"]
	app.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModel(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{{
			Info: &params.ScaleApplicationInfo{Scale: 5},
		}},
	})
	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 1, "Scale", 5)
}

func (s *ApplicationSuite) TestScaleApplicationsNotAllowedForOperator(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.applications["postgresql"].charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentMode: charm.ModeOperator,
			},
		},
	}
	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}},
	}
	result, err := s.api.ScaleApplications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	msg := strings.Replace(result.Results[0].Error.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, `scale an "operator" application not supported`)
}

func (s *ApplicationSuite) TestScaleApplicationsNotAllowedForDaemonSet(c *gc.C) {
	s.model.modelType = state.ModelTypeCAAS
	s.setAPIUser(c, names.NewUserTag("admin"))
	s.backend.applications["postgresql"].charm = &mockCharm{
		meta: &charm.Meta{
			Deployment: &charm.Deployment{
				DeploymentType: charm.DeploymentDaemon,
			},
		},
	}
	args := params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}},
	}
	result, err := s.api.ScaleApplications(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0].Error, gc.NotNil)
	msg := strings.Replace(result.Results[0].Error.Error(), "\n", "", -1)
	c.Assert(msg, gc.Matches, `scale a "daemon" application not supported`)
}

func (s *ApplicationSuite) TestScaleApplicationsBlocked(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	s.blockChecker.SetErrors(apiservererrors.ServerError(apiservererrors.OperationBlockedError("test block")))
	_, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}}})
	c.Assert(err, gc.ErrorMatches, "test block")
	c.Assert(err, jc.Satisfies, params.IsCodeOperationBlocked)
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModelScaleChange(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	s.backend.applications["postgresql"].scale = 2
	results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			ScaleChange:    5,
		}}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(results, jc.DeepEquals, params.ScaleApplicationResults{
		Results: []params.ScaleApplicationResult{{
			Info: &params.ScaleApplicationInfo{Scale: 7},
		}},
	})
	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 1, "ChangeScale", 5)
}

func (s *ApplicationSuite) TestScaleApplicationsCAASModelScaleArgCheck(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	s.backend.applications["postgresql"].scale = 2

	for i, test := range []struct {
		scale       int
		scaleChange int
		errorStr    string
	}{{
		scale:       5,
		scaleChange: 5,
		errorStr:    "requesting both scale and scale-change not valid",
	}, {
		scale:       -1,
		scaleChange: 0,
		errorStr:    "scale < 0 not valid",
	}} {
		c.Logf("test #%d", i)
		results, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
			Applications: []params.ScaleApplicationParams{{
				ApplicationTag: "application-postgresql",
				Scale:          test.scale,
				ScaleChange:    test.scaleChange,
			}}})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.Results, gc.HasLen, 1)
		c.Assert(results.Results[0].Error, gc.ErrorMatches, test.errorStr)
	}
}

func (s *ApplicationSuite) TestScaleApplicationsIAASModel(c *gc.C) {
	_, err := s.api.ScaleApplications(params.ScaleApplicationsParams{
		Applications: []params.ScaleApplicationParams{{
			ApplicationTag: "application-postgresql",
			Scale:          5,
		}}})
	c.Assert(err, gc.ErrorMatches, "scaling applications on a non-container model not supported")
	app := s.backend.applications["postgresql"]
	app.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestAddUnitsAttachStorage(c *gc.C) {
	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "postgresql",
		NumUnits:        1,
		AttachStorage:   []string{"storage-pgdata-0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 0, "AddUnit", state.AddUnitParams{
		AttachStorage: []names.StorageTag{names.NewStorageTag("pgdata/0")},
	})
}

func (s *ApplicationSuite) TestAddUnitsAttachStorageMultipleUnits(c *gc.C) {
	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        2,
		AttachStorage:   []string{"storage-foo-0"},
	})
	c.Assert(err, gc.ErrorMatches, "AttachStorage is non-empty, but NumUnits is 2")
}

func (s *ApplicationSuite) TestAddUnitsAttachStorageInvalidStorageTag(c *gc.C) {
	_, err := s.api.AddUnits(params.AddApplicationUnits{
		ApplicationName: "foo",
		NumUnits:        1,
		AttachStorage:   []string{"volume-0"},
	})
	c.Assert(err, gc.ErrorMatches, `"volume-0" is not a valid storage tag`)
}

func (s *ApplicationSuite) TestSetRelationSuspended(c *gc.C) {
	s.backend.offerConnections["wordpress:db mysql:db"] = &mockOfferConnection{}
	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
			Message:    "message",
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
	c.Assert(s.relation.suspended, jc.IsTrue)
	c.Assert(s.relation.suspendedReason, gc.Equals, "message")
	c.Assert(s.relation.status, gc.Equals, status.Suspending)
	c.Assert(s.relation.message, gc.Equals, "message")
}

func (s *ApplicationSuite) TestSetRelationSuspendedNoOp(c *gc.C) {
	s.backend.offerConnections["wordpress:db mysql:db"] = &mockOfferConnection{}
	s.relation.suspended = true
	s.relation.status = status.Error
	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
	c.Assert(s.relation.suspended, jc.IsTrue)
	c.Assert(s.relation.status, gc.Equals, status.Error)
}

func (s *ApplicationSuite) TestSetRelationSuspendedFalse(c *gc.C) {
	s.backend.offerConnections["wordpress:db mysql:db"] = &mockOfferConnection{}
	s.relation.suspended = true
	s.relation.suspendedReason = "reason"
	s.relation.status = status.Error
	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  false,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
	c.Assert(s.relation.suspended, jc.IsFalse)
	c.Assert(s.relation.suspendedReason, gc.Equals, "")
	c.Assert(s.relation.status, gc.Equals, status.Joining)
}

func (s *ApplicationSuite) TestSetNonOfferRelationStatus(c *gc.C) {
	s.backend.relations[123].tag = names.NewRelationTag("mediawiki:db mysql:db")
	results, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.ErrorMatches, `cannot set suspend status for "mediawiki:db mysql:db" which is not associated with an offer`)
}

func (s *ApplicationSuite) TestBlockSetRelationSuspended(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("blocked"))
	_, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, gc.ErrorMatches, "blocked")
	s.blockChecker.CheckCallNames(c, "ChangeAllowed")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestSetRelationSuspendedPermissionDenied(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("fred"))
	_, err := s.api.SetRelationsSuspended(params.RelationSuspendedArgs{
		Args: []params.RelationSuspendedArg{{
			RelationId: 123,
			Suspended:  true,
		}},
	})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestConsumeIdempotent(c *gc.C) {
	for i := 0; i < 2; i++ {
		results, err := s.api.Consume(params.ConsumeApplicationArgs{
			Args: []params.ConsumeApplicationArg{{
				ApplicationOfferDetails: params.ApplicationOfferDetails{
					SourceModelTag:         coretesting.ModelTag.String(),
					OfferName:              "hosted-mysql",
					OfferUUID:              "hosted-mysql-uuid",
					ApplicationDescription: "a database",
					Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
					OfferURL:               "othermodel.hosted-mysql",
				},
			}},
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(results.OneError(), gc.IsNil)
	}
	obtained, ok := s.backend.remoteApplications["hosted-mysql"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(obtained, jc.DeepEquals, &mockRemoteApplication{
		name:           "hosted-mysql",
		sourceModelTag: coretesting.ModelTag,
		offerUUID:      "hosted-mysql-uuid",
		offerURL:       "othermodel.hosted-mysql",
		endpoints: []state.Endpoint{
			{ApplicationName: "hosted-mysql", Relation: charm.Relation{Name: "database", Interface: "mysql", Role: "provider"}}},
	})
}

func (s *ApplicationSuite) TestConsumeFromExternalController(c *gc.C) {
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	controllerUUID := utils.MustNewUUID().String()
	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         coretesting.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
			},
			Macaroon: mac,
			ControllerInfo: &params.ExternalControllerInfo{
				ControllerTag: names.NewControllerTag(controllerUUID).String(),
				Alias:         "controller-alias",
				CACert:        coretesting.CACert,
				Addrs:         []string{"192.168.1.1:1234"},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
	obtained, ok := s.backend.remoteApplications["hosted-mysql"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(obtained, jc.DeepEquals, &mockRemoteApplication{
		name:           "hosted-mysql",
		sourceModelTag: coretesting.ModelTag,
		offerUUID:      "hosted-mysql-uuid",
		offerURL:       "othermodel.hosted-mysql",
		endpoints: []state.Endpoint{
			{ApplicationName: "hosted-mysql", Relation: charm.Relation{Name: "database", Interface: "mysql", Role: "provider"}}},
		mac: mac,
	})
	c.Assert(s.backend.controllers[coretesting.ModelTag.Id()], jc.DeepEquals, crossmodel.ControllerInfo{
		ControllerTag: names.NewControllerTag(controllerUUID),
		Alias:         "controller-alias",
		CACert:        coretesting.CACert,
		Addrs:         []string{"192.168.1.1:1234"},
	})
}

func (s *ApplicationSuite) TestConsumeFromSameController(c *gc.C) {
	mac, err := apitesting.NewMacaroon("test")
	c.Assert(err, jc.ErrorIsNil)
	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         coretesting.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
			},
			Macaroon: mac,
			ControllerInfo: &params.ExternalControllerInfo{
				ControllerTag: coretesting.ControllerTag.String(),
				Alias:         "controller-alias",
				CACert:        coretesting.CACert,
				Addrs:         []string{"192.168.1.1:1234"},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)
	_, ok := s.backend.remoteApplications["hosted-mysql"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(s.backend.controllers, gc.HasLen, 0)
}

func (s *ApplicationSuite) TestConsumeIncludesSpaceInfo(c *gc.C) {
	s.env.(*mockEnviron).spaceInfo = &environs.ProviderSpaceInfo{
		CloudType: "grandaddy",
		ProviderAttributes: map[string]interface{}{
			"thunderjaws": 1,
		},
		SpaceInfo: network.SpaceInfo{
			Name:       "yourspace",
			ProviderId: "juju-space-myspace",
			Subnets: []network.SubnetInfo{{
				CIDR:              "5.6.7.0/24",
				ProviderId:        "juju-subnet-1",
				AvailabilityZones: []string{"az1"},
			}},
		},
	}

	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationAlias: "beirut",
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         coretesting.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "server", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
				Bindings:               map[string]string{"server": "myspace"},
				Spaces: []params.RemoteSpace{
					{
						CloudType:  "grandaddy",
						Name:       "myspace",
						ProviderId: "juju-space-myspace",
						ProviderAttributes: map[string]interface{}{
							"thunderjaws": 1,
						},
						Subnets: []params.Subnet{{
							CIDR:       "5.6.7.0/24",
							ProviderId: "juju-subnet-1",
							Zones:      []string{"az1"},
						}},
					},
				},
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)

	obtained, ok := s.backend.remoteApplications["beirut"]
	c.Assert(ok, jc.IsTrue)
	endpoints, err := obtained.Endpoints()
	c.Assert(err, jc.ErrorIsNil)
	epNames := make([]string, len(endpoints))
	for i, ep := range endpoints {
		epNames[i] = ep.Name
	}
	c.Assert(epNames, jc.SameContents, []string{"server"})
	c.Assert(obtained.Bindings(), jc.DeepEquals, map[string]string{"server": "myspace"})
	c.Assert(obtained.Spaces(), jc.DeepEquals, []state.RemoteSpace{{
		CloudType:  "grandaddy",
		Name:       "myspace",
		ProviderId: "juju-space-myspace",
		ProviderAttributes: map[string]interface{}{
			"thunderjaws": 1,
		},
		Subnets: []state.RemoteSubnet{{
			CIDR:              "5.6.7.0/24",
			ProviderId:        "juju-subnet-1",
			AvailabilityZones: []string{"az1"},
		}},
	}})
}

func (s *ApplicationSuite) TestConsumeRemoteAppExistsDifferentSourceModel(c *gc.C) {
	arg := params.ConsumeApplicationArg{
		ApplicationOfferDetails: params.ApplicationOfferDetails{
			SourceModelTag:         coretesting.ModelTag.String(),
			OfferName:              "hosted-mysql",
			OfferUUID:              "hosted-mysql-uuid",
			ApplicationDescription: "a database",
			Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
			OfferURL:               "othermodel.hosted-mysql",
		},
	}
	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{arg},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0].Error, gc.IsNil)

	arg.SourceModelTag = names.NewModelTag(utils.MustNewUUID().String()).String()
	results, err = s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{arg},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.ErrorMatches, `remote application called "hosted-mysql" from a different model already exists`)
}

func (s *ApplicationSuite) assertConsumeWithNoSpacesInfoAvailable(c *gc.C) {
	results, err := s.api.Consume(params.ConsumeApplicationArgs{
		Args: []params.ConsumeApplicationArg{{
			ApplicationOfferDetails: params.ApplicationOfferDetails{
				SourceModelTag:         coretesting.ModelTag.String(),
				OfferName:              "hosted-mysql",
				OfferUUID:              "hosted-mysql-uuid",
				ApplicationDescription: "a database",
				Endpoints:              []params.RemoteEndpoint{{Name: "database", Interface: "mysql", Role: "provider"}},
				OfferURL:               "othermodel.hosted-mysql",
			},
		}},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.OneError(), gc.IsNil)

	// Successfully added, but with no bindings or spaces since the
	// environ doesn't support networking.
	obtained, ok := s.backend.remoteApplications["hosted-mysql"]
	c.Assert(ok, jc.IsTrue)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained.Bindings(), gc.IsNil)
	c.Assert(obtained.Spaces(), gc.IsNil)
}

func (s *ApplicationSuite) TestConsumeWithNonNetworkingEnviron(c *gc.C) {
	s.env = &mockNoNetworkEnviron{}
	s.assertConsumeWithNoSpacesInfoAvailable(c)
}

func (s *ApplicationSuite) TestConsumeProviderSpaceInfoNotSupported(c *gc.C) {
	s.env.(*mockEnviron).stub.SetErrors(errors.NotSupportedf("provider space info"))
	s.assertConsumeWithNoSpacesInfoAvailable(c)
}

func (s *ApplicationSuite) TestApplicationUpdateSeriesNoParams(c *gc.C) {
	results, err := s.api.UpdateApplicationSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{}})

	s.backend.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestApplicationUpdateSeriesPermissionDenied(c *gc.C) {
	user := names.NewUserTag("fred")
	s.setAPIUser(c, user)
	_, err := s.api.UpdateApplicationSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{
				Entity: params.Entity{Tag: names.NewApplicationTag("postgresql").String()},
				Series: "trusty",
			}},
		},
	)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ApplicationSuite) TestRemoteRelationBadCIDR(c *gc.C) {
	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"bad.cidr"}})
	c.Assert(err, gc.ErrorMatches, `invalid CIDR address: bad.cidr`)
}

func (s *ApplicationSuite) TestRemoteRelationDisAllowedCIDR(c *gc.C) {
	endpoints := []string{"wordpress", "hosted-mysql:nope"}
	_, err := s.api.AddRelation(params.AddRelation{Endpoints: endpoints, ViaCIDRs: []string{"0.0.0.0/0"}})
	c.Assert(err, gc.ErrorMatches, `CIDR "0.0.0.0/0" not allowed`)
}

func (s *ApplicationSuite) TestSetApplicationConfigExplicitMaster(c *gc.C) {
	s.testSetApplicationConfig(c, model.GenerationMaster)
}

func (s *ApplicationSuite) TestSetApplicationConfigEmptyUsesMaster(c *gc.C) {
	s.testSetApplicationConfig(c, "")
}

func (s *ApplicationSuite) testSetApplicationConfig(c *gc.C, branchName string) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	api := &application.APIv12{s.api}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "value",
				"stringOption":           "stringVal",
			},
			Generation: branchName,
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "Name", "UpdateCharmConfig", "UpdateApplicationConfig")

	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, defaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "value",
	}, appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
	app.CheckCall(c, 2, "UpdateCharmConfig", model.GenerationMaster, charm.Settings{"stringOption": "stringVal"})

	// We should never have accessed the generation.
	c.Check(s.backend.generation, gc.IsNil)
}

func (s *ApplicationSuite) TestSetApplicationConfigBranch(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	api := &application.APIv12{s.api}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "value",
				"stringOption":           "stringVal",
			},
			Generation: "new-branch",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "Name", "UpdateCharmConfig", "UpdateApplicationConfig", "Name")

	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, defaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "value",
	}, appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
	app.CheckCall(c, 2, "UpdateCharmConfig", "new-branch", charm.Settings{"stringOption": "stringVal"})

	s.backend.generation.CheckCall(c, 0, "AssignApplication", "postgresql")
}

func (s *ApplicationSuite) TestSetApplicationsEmptyConfigMasterBranch(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	api := &application.APIv12{s.api}
	result, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "value",
				"stringOption":           "",
			},
			Generation: "master",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "Name", "UpdateCharmConfig", "UpdateApplicationConfig")

	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, defaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "value",
	}, appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
	app.CheckCall(c, 2, "UpdateCharmConfig", "master", charm.Settings{"stringOption": ""})
}

func (s *ApplicationSuite) TestSetConfigBranch(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	result, err := s.api.SetConfigs(params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "value",
				"stringOption":           "stringVal",
			},
			Generation: "new-branch",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "Name", "UpdateCharmConfig", "UpdateApplicationConfig", "Name")

	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, defaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "value",
	}, appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
	app.CheckCall(c, 2, "UpdateCharmConfig", "new-branch", charm.Settings{"stringOption": "stringVal"})

	s.backend.generation.CheckCall(c, 0, "AssignApplication", "postgresql")
}

func (s *ApplicationSuite) TestSetEmptyConfigMasterBranch(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	result, err := s.api.SetConfigs(params.ConfigSetArgs{
		Args: []params.ConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "value",
				"stringOption":           "",
			},
			Generation: "master",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "Charm", "Name", "UpdateCharmConfig", "UpdateApplicationConfig")

	appCfgSchema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	appCfgSchema, defaults, err = application.AddTrustSchemaAndDefaults(appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	appCfg, err := coreapplication.NewConfig(map[string]interface{}{
		"juju-external-hostname": "value",
	}, appCfgSchema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 3, "UpdateApplicationConfig", appCfg.Attributes(), []string(nil), appCfgSchema, schema.Defaults(nil))
	app.CheckCall(c, 2, "UpdateCharmConfig", "master", charm.Settings{"stringOption": ""})
}

func (s *ApplicationSuite) TestBlockSetApplicationConfig(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("blocked"))
	api := &application.APIv12{s.api}
	_, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
	s.blockChecker.CheckCallNames(c, "ChangeAllowed")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestSetApplicationConfigPermissionDenied(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("fred"))
	api := &application.APIv12{s.api}
	_, err := api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
		}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.application.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestUnsetApplicationConfig(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	result, err := s.api.UnsetApplicationsConfig(params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "postgresql",
			Options:         []string{"juju-external-hostname", "stringVal"},
			BranchName:      "new-branch",
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	c.Assert(err, jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "UpdateApplicationConfig", "UpdateCharmConfig")

	schema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	schema, defaults, err = application.AddTrustSchemaAndDefaults(schema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 0, "UpdateApplicationConfig", coreapplication.ConfigAttributes(nil),
		[]string{"juju-external-hostname"}, schema, defaults)
	app.CheckCall(c, 1, "UpdateCharmConfig", "new-branch", charm.Settings{"stringVal": nil})
}

func (s *ApplicationSuite) TestBlockUnsetApplicationConfig(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("blocked"))
	_, err := s.api.UnsetApplicationsConfig(params.ApplicationConfigUnsetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
	s.blockChecker.CheckCallNames(c, "ChangeAllowed")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestUnsetApplicationConfigPermissionDenied(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("fred"))
	_, err := s.api.UnsetApplicationsConfig(params.ApplicationConfigUnsetArgs{
		Args: []params.ApplicationUnset{{
			ApplicationName: "postgresql",
			Options:         []string{"option"},
		}}})
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.application.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestResolveUnitErrors(c *gc.C) {
	entities := []params.Entity{{Tag: "unit-postgresql-0"}, {Tag: "unit-postgresql-1"}}
	p := params.UnitsResolved{
		Retry: true,
		Tags: params.Entities{
			Entities: entities,
		},
	}
	result, err := s.api.ResolveUnitErrors(p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{{}, {}}})

	for i := 0; i < 2; i++ {
		unit := s.backend.applications["postgresql"].units[i]
		unit.CheckCallNames(c, "Resolve")
		unit.CheckCall(c, 0, "Resolve", true)
	}
}

func (s *ApplicationSuite) TestResolveUnitErrorsAll(c *gc.C) {
	p := params.UnitsResolved{
		All:   true,
		Retry: true,
	}
	_, err := s.api.ResolveUnitErrors(p)
	c.Assert(err, jc.ErrorIsNil)

	unit := s.backend.applications["postgresql"].units[0]
	unit.CheckCallNames(c, "Resolve")
	unit.CheckCall(c, 0, "Resolve", true)
}

func (s *ApplicationSuite) TestBlockResolveUnitErrors(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("blocked"))
	_, err := s.api.ResolveUnitErrors(params.UnitsResolved{})
	c.Assert(err, gc.ErrorMatches, "blocked")
	s.blockChecker.CheckCallNames(c, "ChangeAllowed")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestResolveUnitErrorsPermissionDenied(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("fred"))

	entities := []params.Entity{{Tag: "unit-postgresql-0"}}
	p := params.UnitsResolved{
		Retry: true,
		Tags: params.Entities{
			Entities: entities,
		},
	}
	_, err := s.api.ResolveUnitErrors(p)
	c.Assert(err, gc.ErrorMatches, "permission denied")
	s.application.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestCAASExposeWithoutHostname(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	err := s.api.Expose(params.ApplicationExpose{
		ApplicationName: "postgresql",
	})
	c.Assert(err, gc.ErrorMatches,
		`cannot expose a container application without a "juju-external-hostname" value set, run\n`+
			`juju config postgresql juju-external-hostname=<value>`)
}

func (s *ApplicationSuite) TestCAASExposeWithHostname(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	app := s.backend.applications["postgresql"]
	app.config = coreapplication.ConfigAttributes{"juju-external-hostname": "exthost"}
	err := s.api.Expose(params.ApplicationExpose{
		ApplicationName: "postgresql",
	})
	c.Assert(err, jc.ErrorIsNil)
	app.CheckCallNames(c, "ApplicationConfig", "MergeExposeSettings")
}

func (s *ApplicationSuite) TestApplicationsInfoOne(c *gc.C) {
	entities := []params.Entity{{Tag: "application-test-app-info"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.ApplicationResult{
		Tag:         "application-test-app-info",
		Charm:       "charm-test-app-info",
		Series:      "quantal",
		Channel:     "2.0/candidate",
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
	})
	app := s.backend.applications["test-app-info"]
	app.CheckCallNames(c, "CharmConfig", "Charm", "ApplicationConfig", "IsPrincipal", "Constraints", "EndpointBindings", "Series", "Channel", "EndpointBindings", "ExposedEndpoints", "CharmOrigin", "IsPrincipal", "IsExposed", "IsRemote")
}

func (s *ApplicationSuite) TestApplicationsInfoOneWithExposedEndpoints(c *gc.C) {
	s.backend.spaceInfos = network.SpaceInfos{
		{
			ID:   "42",
			Name: "non-euclidean-geometry",
		},
	}
	app := s.backend.applications["postgresql"]
	app.exposedEndpoints = map[string]state.ExposedEndpoint{
		"server": {
			ExposeToSpaceIDs: []string{"42"},
			ExposeToCIDRs:    []string{"10.0.0.0/24", "192.168.0.0/24"},
		},
	}

	entities := []params.Entity{{Tag: "application-postgresql"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.ApplicationResult{
		Tag:         "application-postgresql",
		Charm:       "charm-postgresql",
		Series:      "quantal",
		Channel:     "development",
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
		ExposedEndpoints: map[string]params.ExposedEndpoint{
			"server": {
				ExposeToSpaces: []string{"non-euclidean-geometry"},
				ExposeToCIDRs:  []string{"10.0.0.0/24", "192.168.0.0/24"},
			},
		},
	})
	app.CheckCallNames(c, "CharmConfig", "Charm", "ApplicationConfig", "IsPrincipal", "Constraints", "EndpointBindings", "Series", "Channel", "EndpointBindings", "ExposedEndpoints", "CharmOrigin", "IsPrincipal", "IsExposed", "IsRemote")
}

func (s *ApplicationSuite) TestApplicationsInfoDetailsErr(c *gc.C) {
	entities := []params.Entity{{Tag: "application-postgresql"}}
	app := s.backend.applications["postgresql"]
	app.SetErrors(
		errors.Errorf("boom"), // a.CharmConfig() call
	)

	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	app.CheckCallNames(c, "CharmConfig")
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) TestApplicationsInfoBindingsErr(c *gc.C) {
	entities := []params.Entity{{Tag: "application-postgresql"}}
	app := s.backend.applications["postgresql"]
	app.SetErrors(
		nil,                   // a.CharmConfig() call
		errors.Errorf("boom"), // a.EndpointBindings() call
	)

	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	app.CheckCallNames(c, "CharmConfig", "Charm", "ApplicationConfig")
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) TestApplicationsInfoMany(c *gc.C) {
	entities := []params.Entity{{Tag: "application-postgresql"}, {Tag: "application-wordpress"}, {Tag: "unit-postgresql-0"}}
	result, err := s.api.ApplicationsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.ApplicationResult{
		Tag:         "application-postgresql",
		Charm:       "charm-postgresql",
		Series:      "quantal",
		Channel:     "development",
		Constraints: constraints.MustParse("arch=amd64 mem=4G cores=1 root-disk=8G"),
		Principal:   true,
		EndpointBindings: map[string]string{
			"juju-info": "myspace",
		},
	})
	c.Assert(result.Results[1].Error, gc.ErrorMatches, `application "wordpress" not found`)
	c.Assert(result.Results[2].Error, gc.ErrorMatches, `"unit-postgresql-0" is not a valid application tag`)
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "CharmConfig", "Charm", "ApplicationConfig", "IsPrincipal", "Constraints", "EndpointBindings", "Series", "Channel", "EndpointBindings", "ExposedEndpoints", "CharmOrigin", "IsPrincipal", "IsExposed", "IsRemote")
}

func (s *ApplicationSuite) TestApplicationMergeBindingsErr(c *gc.C) {
	req := params.ApplicationMergeBindingsArgs{
		Args: []params.ApplicationMergeBindings{
			{
				ApplicationTag: "application-postgresql",
			},
		},
	}
	app := s.backend.applications["postgresql"]
	app.SetErrors(
		errors.Errorf("boom"), // a.MergeBindings() call
	)

	result, err := s.api.MergeBindings(req)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(req.Args))
	app.CheckCallNames(c, "MergeBindings")
	c.Assert(*result.Results[0].Error, gc.ErrorMatches, "boom")
}

func (s *ApplicationSuite) TestUnitsInfo(c *gc.C) {
	s.backend.machines = map[string]*mockMachine{"0": {}}

	entities := []params.Entity{{Tag: "unit-postgresql-0"}, {Tag: "unit-mysql-0"}}
	result, err := s.api.UnitsInfo(params.Entities{Entities: entities})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Results, gc.HasLen, len(entities))
	c.Assert(result.Results[0].Error, gc.IsNil)
	c.Assert(*result.Results[0].Result, gc.DeepEquals, params.UnitResult{
		Tag:             "unit-postgresql-0",
		WorkloadVersion: "666",
		Machine:         "0",
		OpenedPorts:     []string{"100-102/tcp"},
		PublicAddress:   "10.0.0.1",
		Charm:           "cs:postgresql-42",
		Leader:          true,
		RelationData: []params.EndpointRelationData{{
			Endpoint:        "db",
			CrossModel:      true,
			RelatedEndpoint: "server",
			ApplicationData: map[string]interface{}{"app-postgresql": "setting"},
			UnitRelationData: map[string]params.RelationData{
				"gitlab/2": {
					InScope:  true,
					UnitData: map[string]interface{}{"gitlab/2": "gitlab/2-setting"},
				},
			},
		}},
		ProviderId: "provider-id",
		Address:    "192.168.1.1",
	})
	c.Assert(result.Results[1].Error, jc.DeepEquals, &params.Error{
		Code:    "not found",
		Message: `unit "mysql/0" not found`,
	})
}

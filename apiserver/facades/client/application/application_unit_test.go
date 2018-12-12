// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/testing"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/caas"
	k8s "github.com/juju/juju/caas/kubernetes/provider"
	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/storage/provider"
	coretesting "github.com/juju/juju/testing"
)

type ApplicationSuite struct {
	testing.IsolationSuite
	coretesting.JujuOSEnvSuite
	backend            mockBackend
	endpoints          []state.Endpoint
	relation           mockRelation
	application        mockApplication
	storagePoolManager *mockStoragePoolManager

	env          environs.Environ
	blockChecker mockBlockChecker
	authorizer   apiservertesting.FakeAuthorizer
	api          *application.APIv8
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) setAPIUser(c *gc.C, user names.UserTag) {
	s.authorizer.Tag = user
	s.storagePoolManager = &mockStoragePoolManager{storageType: k8s.K8s_ProviderType}
	api, err := application.NewAPIBase(
		&s.backend,
		&s.backend,
		s.authorizer,
		&s.blockChecker,
		names.NewModelTag(utils.MustNewUUID().String()),
		state.ModelTypeIAAS,
		func(application.Charm) *state.Charm {
			return &state.Charm{}
		},
		func(application.ApplicationDeployer, application.DeployApplicationParams) (application.Application, error) {
			return nil, nil
		},
		s.storagePoolManager,
		common.NewResources(),
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = &application.APIv8{api}
}

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.JujuOSEnvSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}
	s.env = &mockEnviron{}
	s.endpoints = []state.Endpoint{
		{ApplicationName: "postgresql"},
		{ApplicationName: "bar"},
	}
	s.relation = mockRelation{tag: names.NewRelationTag("wordpress:db mysql:db")}
	s.backend = mockBackend{
		controllers: make(map[string]crossmodel.ControllerInfo),
		applications: map[string]*mockApplication{
			"postgresql": {
				name:        "postgresql",
				series:      "quantal",
				subordinate: false,
				charm: &mockCharm{
					config: &charm.Config{
						Options: map[string]charm.Option{
							"stringOption": {Type: "string"},
							"intOption":    {Type: "int", Default: int(123)},
						},
					},
				},
				units: []*mockUnit{
					{
						name:      "postgresql/0",
						tag:       names.NewUnitTag("postgresql/0"),
						machineId: "machine-0",
					},
					{
						name:      "postgresql/1",
						tag:       names.NewUnitTag("postgresql/1"),
						machineId: "machine-1",
					},
				},
				addedUnit: mockUnit{
					tag: names.NewUnitTag("postgresql/99"),
				},
				lxdProfileUpgradeChanges: make(chan struct{}),
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
				},
				units: []*mockUnit{
					{tag: names.NewUnitTag("postgresql-subordinate/0")},
					{tag: names.NewUnitTag("postgresql-subordinate/1")},
				},
				addedUnit: mockUnit{
					tag: names.NewUnitTag("postgresql-subordinate/99"),
				},
				lxdProfileUpgradeChanges: make(chan struct{}),
			},
		},
		remoteApplications: map[string]application.RemoteApplication{
			"hosted-db2": &mockRemoteApplication{},
		},
		charm: &mockCharm{
			meta: &charm.Meta{}, config: &charm.Config{
				Options: map[string]charm.Option{
					"stringOption": {Type: "string"},
					"intOption":    {Type: "int", Default: int(123)}},
			},
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
		machines: map[string]*mockMachine{
			"machine-0": {id: "0", upgradeCharmProfileComplete: ""},
			"machine-1": {id: "1", upgradeCharmProfileComplete: "not required"},
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
	app.CheckCallNames(c, "SetCharm")
	app.CheckCall(c, 0, "SetCharm", state.SetCharmConfig{
		Charm: &state.Charm{},
		StorageConstraints: map[string]state.StorageConstraints{
			"a": {},
			"b": {Pool: "radiant"},
			"c": {Size: 123},
			"d": {Count: 456},
		},
	})
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
	app.CheckCallNames(c, "SetCharm")
	app.CheckCall(c, 0, "SetCharm", state.SetCharmConfig{
		Charm:          &state.Charm{},
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
	app.CheckCallNames(c, "SetCharm")
	app.CheckCall(c, 0, "SetCharm", state.SetCharmConfig{
		Charm:          &state.Charm{},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	})
}

func (s *ApplicationSuite) TestDestroyRelation(c *gc.C) {
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, jc.ErrorIsNil)
	s.blockChecker.CheckCallNames(c, "RemoveAllowed")
	s.backend.CheckCallNames(c, "InferEndpoints", "EndpointsRelation")
	s.backend.CheckCall(c, 0, "InferEndpoints", []string{"a", "b"})
	s.relation.CheckCallNames(c, "Destroy")
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
	s.relation.CheckCallNames(c, "Destroy")
}

func (s *ApplicationSuite) TestDestroyRelationIdRelationNotFound(c *gc.C) {
	s.backend.SetErrors(errors.NotFoundf(`relation "123"`))
	err := s.api.DestroyRelation(params.DestroyRelation{RelationId: 123})
	c.Assert(err, gc.ErrorMatches, `relation "123" not found`)
}

func (s *ApplicationSuite) TestDestroyApplication(c *gc.C) {
	results, err := s.api.DestroyApplication(params.DestroyApplicationsParams{
		Applications: []params.DestroyApplicationParams{{
			ApplicationTag: "application-postgresql",
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
	s.backend.CheckCall(c, 7, "ApplyOperation", &state.DestroyApplicationOperation{})
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

	s.backend.CheckCallNames(c, "RemoteApplication")
	app := s.backend.remoteApplications["hosted-db2"]
	app.(*mockRemoteApplication).CheckCallNames(c, "Destroy")
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
	results, err := s.api.DestroyUnit(params.DestroyUnitsParams{
		Units: []params.DestroyUnitParams{
			{UnitTag: "unit-postgresql-0"},
			{
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
	s.backend.CheckCall(c, 6, "ApplyOperation", &state.DestroyUnitOperation{})
	s.backend.CheckCall(c, 9, "ApplyOperation", &state.DestroyUnitOperation{
		DestroyStorage: true,
	})
}

func (s *ApplicationSuite) TestDeployAttachStorage(c *gc.C) {
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
			AttachStorage:   []string{"storage-foo-0"},
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-1",
			NumUnits:        2,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-2",
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

func (s *ApplicationSuite) TestDeployCAASModel(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
		}, {
			ApplicationName: "bar",
			CharmURL:        "local:bar-0",
			NumUnits:        1,
			AttachStorage:   []string{"storage-bar-0"},
		}, {
			ApplicationName: "baz",
			CharmURL:        "local:baz-0",
			NumUnits:        1,
			Placement:       []*instance.Placement{{}, {}},
		}},
	}
	results, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 3)
	c.Assert(results.Results[0].Error, gc.IsNil)
	c.Assert(results.Results[1].Error, gc.ErrorMatches, "AttachStorage may not be specified for caas models")
	c.Assert(results.Results[2].Error, gc.ErrorMatches, "only 1 placement directive is supported for caas models, got 2")
}

func (s *ApplicationSuite) TestDeployCAASModelNoOperatorStorage(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	s.storagePoolManager.SetErrors(errors.NotFoundf("pool"))
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
		}},
	}
	_, err := s.api.Deploy(args)
	c.Assert(err, gc.ErrorMatches, `deploying a Kubernetes application requires a storage pool called "operator-storage": .*`)
}

func (s *ApplicationSuite) TestDeployCAASModelWrongOperatorStorageType(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	s.storagePoolManager.storageType = provider.RootfsProviderType
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
		}},
	}
	_, err := s.api.Deploy(args)
	c.Assert(err, gc.ErrorMatches, `the "operator-storage" storage pool requires a provider type of "kubernetes", not "rootfs"`)
}

func (s *ApplicationSuite) TestDeployCAASModelNoStoragePool(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
			Storage: map[string]storage.Constraints{
				"database": {},
			},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `storage pool for "database" must be specified`)
}

func (s *ApplicationSuite) TestDeployCAASModelWrongStorageType(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	args := params.ApplicationsDeploy{
		Applications: []params.ApplicationDeploy{{
			ApplicationName: "foo",
			CharmURL:        "local:foo-0",
			NumUnits:        1,
			Storage: map[string]storage.Constraints{
				"database": {Pool: "db"},
			},
		}},
	}
	result, err := s.api.Deploy(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), gc.ErrorMatches, `invalid storage provider type "rootfs" for "database"`)
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
	c.Assert(err, gc.ErrorMatches, "adding units on a non-container model not supported")
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
	app.CheckCall(c, 0, "Scale", 5)
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
	app.CheckCall(c, 0, "ChangeScale", 5)
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
		scale:       0,
		scaleChange: 0,
		errorStr:    "scale of 0 not valid",
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

func (s *ApplicationSuite) TestApplicationUpdateSeries(c *gc.C) {
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewApplicationTag("postgresql").String()},
			Series: "trusty",
		}, {
			Entity: params.Entity{Tag: names.NewApplicationTag("postgresql").String()},
			Series: "quantal",
		}, {
			Entity: params.Entity{Tag: names.NewApplicationTag("name").String()},
			Series: "trusty",
		}, {
			Entity: params.Entity{Tag: names.NewUnitTag("mysql/0").String()},
			Series: "trusty",
		}},
	}
	results, err := s.api.UpdateApplicationSeries(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{}, {},
			{Error: &params.Error{Message: "application \"name\" not found", Code: "not found"}},
			{Error: &params.Error{Message: "\"unit-mysql-0\" is not a valid application tag", Code: ""}},
		}})
	s.backend.CheckCall(c, 0, "Application", "postgresql")
	s.backend.CheckCall(c, 1, "Application", "postgresql")

	app := s.backend.applications["postgresql"]
	app.CheckCall(c, 0, "IsPrincipal")
	app.CheckCall(c, 1, "Series")
	app.CheckCall(c, 2, "UpdateApplicationSeries", "trusty", false)
	app.CheckCall(c, 3, "IsPrincipal")
	app.CheckCall(c, 4, "Series")
	// ensure that app.UpdateApplicationSeries wasn't called a 2nd time.
	c.Assert(len(app.Calls()), gc.Equals, 5)
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

func (s *ApplicationSuite) TestApplicationUpdateSeriesNoSeries(c *gc.C) {
	results, err := s.api.UpdateApplicationSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{Entity: params.Entity{Tag: names.NewApplicationTag("postgresql").String()}}},
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeBadRequest,
			Message: `series missing from args`,
		},
	})

	s.backend.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestApplicationUpdateSeriesOfSubordinate(c *gc.C) {
	args := params.UpdateSeriesArgs{
		Args: []params.UpdateSeriesArg{{
			Entity: params.Entity{Tag: names.NewApplicationTag("postgresql-subordinate").String()},
			Series: "xenial",
		}},
	}
	results, err := s.api.UpdateApplicationSeries(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeNotSupported,
			Message: `"postgresql-subordinate" is a subordinate application, update-series not supported`,
		},
	})

	s.backend.CheckCall(c, 0, "Application", "postgresql-subordinate")

	app := s.backend.applications["postgresql-subordinate"]
	app.CheckCall(c, 0, "IsPrincipal")
}

func (s *ApplicationSuite) TestApplicationUpdateSeriesIncompatibleSeries(c *gc.C) {
	app := s.backend.applications["postgresql"]
	app.SetErrors(nil, nil, &state.ErrIncompatibleSeries{[]string{"yakkety", "zesty"}, "xenial", "testCharm"})
	results, err := s.api.UpdateApplicationSeries(
		params.UpdateSeriesArgs{
			Args: []params.UpdateSeriesArg{{
				Entity: params.Entity{Tag: names.NewApplicationTag("postgresql").String()},
				Series: "xenial",
			}},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(results.Results), gc.Equals, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.ErrorResult{
		Error: &params.Error{
			Code:    params.CodeIncompatibleSeries,
			Message: "series \"xenial\" not supported by charm \"testCharm\", supported series are: yakkety, zesty",
		},
	})
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

func (s *ApplicationSuite) TestSetApplicationConfig(c *gc.C) {
	application.SetModelType(s.api, state.ModelTypeCAAS)
	result, err := s.api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
		Args: []params.ApplicationConfigSet{{
			ApplicationName: "postgresql",
			Config: map[string]string{
				"juju-external-hostname": "value",
				"stringOption":           "stringVal"},
		}}})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.OneError(), jc.ErrorIsNil)
	s.backend.CheckCallNames(c, "Application")
	app := s.backend.applications["postgresql"]
	app.CheckCallNames(c, "UpdateApplicationConfig", "UpdateCharmConfig")

	schema, err := caas.ConfigSchema(k8s.ConfigSchema())
	c.Assert(err, jc.ErrorIsNil)
	defaults := caas.ConfigDefaults(k8s.ConfigDefaults())
	schema, defaults, err = application.AddTrustSchemaAndDefaults(schema, defaults)
	c.Assert(err, jc.ErrorIsNil)

	app.CheckCall(c, 0, "UpdateApplicationConfig", coreapplication.ConfigAttributes{
		"juju-external-hostname": "value",
	}, []string(nil), schema, defaults)
	app.CheckCall(c, 1, "UpdateCharmConfig", charm.Settings{"stringOption": "stringVal"})
}

func (s *ApplicationSuite) TestBlockSetApplicationConfig(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("blocked"))
	_, err := s.api.SetApplicationsConfig(params.ApplicationConfigSetArgs{})
	c.Assert(err, gc.ErrorMatches, "blocked")
	s.blockChecker.CheckCallNames(c, "ChangeAllowed")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestSetApplicationConfigPermissionDenied(c *gc.C) {
	s.setAPIUser(c, names.NewUserTag("fred"))
	_, err := s.api.SetApplicationsConfig(params.ApplicationConfigSetArgs{
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
	app.CheckCall(c, 1, "UpdateCharmConfig", charm.Settings{"stringVal": nil})
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
		`cannot expose a CAAS application without a "juju-external-hostname" value set, run\n`+
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
	app.CheckCallNames(c, "ApplicationConfig", "SetExposed")
}

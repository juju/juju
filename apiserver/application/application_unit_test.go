// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/application"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type ApplicationSuite struct {
	testing.IsolationSuite
	backend     mockBackend
	application mockApplication
	charm       mockCharm
	endpoints   []state.Endpoint
	relation    mockRelation

	blockChecker mockBlockChecker
	authorizer   apiservertesting.FakeAuthorizer
	api          *application.API
}

var _ = gc.Suite(&ApplicationSuite{})

func (s *ApplicationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: names.NewUserTag("admin"),
	}
	s.application = mockApplication{
		units: []mockUnit{{
			tag: names.NewUnitTag("foo/0"),
		}, {
			tag: names.NewUnitTag("foo/1"),
		}},
	}
	s.charm = mockCharm{
		meta: &charm.Meta{},
		config: &charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	}
	s.endpoints = []state.Endpoint{
		{ApplicationName: "foo"},
		{ApplicationName: "bar"},
	}
	s.relation = mockRelation{}
	s.backend = mockBackend{
		application: &s.application,
		charm:       &s.charm,
		endpoints:   &s.endpoints,
		relation:    &s.relation,
		unitStorageAttachments: map[string][]state.StorageAttachment{
			"foo/0": {
				&mockStorageAttachment{
					unit:    names.NewUnitTag("foo/0"),
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
				owner: names.NewUnitTag("foo/0"),
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
	resources := common.NewResources()
	resources.RegisterNamed("dataDir", common.StringResource(c.MkDir()))
	api, err := application.NewAPI(
		&s.backend,
		s.authorizer,
		resources,
		nil,
		&s.blockChecker,
		func(application.Charm) *state.Charm {
			return &state.Charm{}
		},
		func(application.ApplicationDeployer, application.DeployApplicationParams) (application.Application, error) {
			return nil, nil
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	s.api = api
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
	s.backend.CheckCallNames(c, "ModelTag", "Application", "Charm")
	s.application.CheckCallNames(c, "SetCharm")
	s.application.CheckCall(c, 0, "SetCharm", state.SetCharmConfig{
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
	s.backend.CheckCallNames(c, "ModelTag", "Application", "Charm")
	s.charm.CheckCallNames(c, "Config")
	s.application.CheckCallNames(c, "SetCharm")
	s.application.CheckCall(c, 0, "SetCharm", state.SetCharmConfig{
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
	s.backend.CheckCallNames(c, "ModelTag", "Application", "Charm")
	s.charm.CheckCallNames(c, "Config")
	s.application.CheckCallNames(c, "SetCharm")
	s.application.CheckCall(c, 0, "SetCharm", state.SetCharmConfig{
		Charm:          &state.Charm{},
		ConfigSettings: charm.Settings{"stringOption": "value"},
	})
}

func (s *ApplicationSuite) TestDestroyRelation(c *gc.C) {
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, jc.ErrorIsNil)
	s.blockChecker.CheckCallNames(c, "RemoveAllowed")
	s.backend.CheckCallNames(c, "ModelTag", "InferEndpoints", "EndpointsRelation")
	s.backend.CheckCall(c, 1, "InferEndpoints", []string{"a", "b"})
	s.relation.CheckCallNames(c, "Destroy")
}

func (s *ApplicationSuite) TestDestroyRelationNoRelationsFound(c *gc.C) {
	s.backend.SetErrors(nil, errors.New("no relations found"))
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "no relations found")
}

func (s *ApplicationSuite) TestDestroyRelationRelationNotFound(c *gc.C) {
	s.backend.SetErrors(nil, nil, errors.NotFoundf(`relation "a:b c:d"`))
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a:b", "c:d"}})
	c.Assert(err, gc.ErrorMatches, `relation "a:b c:d" not found`)
}

func (s *ApplicationSuite) TestBlockRemoveDestroyRelation(c *gc.C) {
	s.blockChecker.SetErrors(errors.New("foo"))
	err := s.api.DestroyRelation(params.DestroyRelation{Endpoints: []string{"a", "b"}})
	c.Assert(err, gc.ErrorMatches, "foo")
	s.blockChecker.CheckCallNames(c, "RemoveAllowed")
	s.backend.CheckCallNames(c, "ModelTag")
	s.relation.CheckNoCalls(c)
}

func (s *ApplicationSuite) TestDestroyApplication(c *gc.C) {
	results, err := s.api.DestroyApplication(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Info: &params.DestroyApplicationInfo{
			DestroyedUnits: []params.Entity{
				{Tag: "unit-foo-0"},
				{Tag: "unit-foo-1"},
			},
			DetachedStorage: []params.Entity{
				{Tag: "storage-pgdata-0"},
			},
			DestroyedStorage: []params.Entity{
				{Tag: "storage-pgdata-1"},
			},
		},
	})
}

func (s *ApplicationSuite) TestDestroyApplicationNotFound(c *gc.C) {
	s.backend.application = nil
	results, err := s.api.DestroyApplication(params.Entities{
		Entities: []params.Entity{
			{Tag: "application-foo"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], jc.DeepEquals, params.DestroyApplicationResult{
		Error: &params.Error{
			Code:    params.CodeNotFound,
			Message: `application "foo" not found`,
		},
	})
}

func (s *ApplicationSuite) TestDestroyUnit(c *gc.C) {
	results, err := s.api.DestroyUnit(params.Entities{
		Entities: []params.Entity{
			{Tag: "unit-foo-0"},
			{Tag: "unit-foo-1"},
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

type mockBackend struct {
	application.Backend
	testing.Stub
	application                *mockApplication
	charm                      *mockCharm
	endpoints                  *[]state.Endpoint
	relation                   *mockRelation
	unitStorageAttachments     map[string][]state.StorageAttachment
	storageInstances           map[string]*mockStorage
	storageInstanceFilesystems map[string]*mockFilesystem
}

func (b *mockBackend) ModelTag() names.ModelTag {
	b.MethodCall(b, "ModelTag")
	b.PopNoErr()
	return coretesting.ModelTag
}

func (b *mockBackend) RemoteApplication(name string) (*state.RemoteApplication, error) {
	b.MethodCall(b, "RemoteApplication", name)
	return nil, errors.NotFoundf("remote application %q", name)
}

func (b *mockBackend) Application(name string) (application.Application, error) {
	b.MethodCall(b, "Application", name)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	if b.application != nil {
		return b.application, nil
	}
	return nil, errors.NotFoundf("application %q", name)
}

func (b *mockBackend) Unit(name string) (application.Unit, error) {
	b.MethodCall(b, "Unit", name)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	if b.application != nil {
		for _, u := range b.application.units {
			if u.tag.Id() == name {
				return &u, nil
			}
		}
	}
	return nil, errors.NotFoundf("unit %q", name)
}

func (b *mockBackend) Charm(curl *charm.URL) (application.Charm, error) {
	b.MethodCall(b, "Charm", curl)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	if b.charm != nil {
		return b.charm, nil
	}
	return nil, errors.NotFoundf("charm %q", curl)
}

func (b *mockBackend) InferEndpoints(endpoints ...string) ([]state.Endpoint, error) {
	b.MethodCall(b, "InferEndpoints", endpoints)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	if b.endpoints != nil {
		return *b.endpoints, nil
	}
	return nil, errors.Errorf("no relations found")
}

func (b *mockBackend) EndpointsRelation(endpoints ...state.Endpoint) (application.Relation, error) {
	b.MethodCall(b, "EndpointsRelation", endpoints)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	if b.relation != nil {
		return b.relation, nil
	}
	return nil, errors.NotFoundf("relation")
}

func (b *mockBackend) UnitStorageAttachments(tag names.UnitTag) ([]state.StorageAttachment, error) {
	b.MethodCall(b, "UnitStorageAttachments", tag)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	return b.unitStorageAttachments[tag.Id()], nil
}

func (b *mockBackend) StorageInstance(tag names.StorageTag) (state.StorageInstance, error) {
	b.MethodCall(b, "StorageInstance", tag)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	s, ok := b.storageInstances[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("storage %s", tag.Id())
	}
	return s, nil
}

func (b *mockBackend) StorageInstanceFilesystem(tag names.StorageTag) (state.Filesystem, error) {
	b.MethodCall(b, "StorageInstanceFilesystem", tag)
	if err := b.NextErr(); err != nil {
		return nil, err
	}
	f, ok := b.storageInstanceFilesystems[tag.Id()]
	if !ok {
		return nil, errors.NotFoundf("filesystem for storage %s", tag.Id())
	}
	return f, nil
}

type mockApplication struct {
	application.Application
	testing.Stub
	units []mockUnit
}

func (a *mockApplication) AllUnits() ([]application.Unit, error) {
	a.MethodCall(a, "AllUnits")
	if err := a.NextErr(); err != nil {
		return nil, err
	}
	units := make([]application.Unit, len(a.units))
	for i := range a.units {
		units[i] = &a.units[i]
	}
	return units, nil
}

func (a *mockApplication) SetCharm(cfg state.SetCharmConfig) error {
	a.MethodCall(a, "SetCharm", cfg)
	return a.NextErr()
}

func (a *mockApplication) Destroy() error {
	a.MethodCall(a, "Destroy")
	return a.NextErr()
}

type mockCharm struct {
	application.Charm
	testing.Stub
	config *charm.Config
	meta   *charm.Meta
}

func (c *mockCharm) Config() *charm.Config {
	c.MethodCall(c, "Config")
	c.PopNoErr()
	return c.config
}

func (c *mockCharm) Meta() *charm.Meta {
	c.MethodCall(c, "Meta")
	c.PopNoErr()
	return c.meta
}

type mockBlockChecker struct {
	testing.Stub
}

func (c *mockBlockChecker) ChangeAllowed() error {
	c.MethodCall(c, "ChangeAllowed")
	return c.NextErr()
}

func (c *mockBlockChecker) RemoveAllowed() error {
	c.MethodCall(c, "RemoveAllowed")
	return c.NextErr()
}

type mockRelation struct {
	application.Relation
	testing.Stub
}

func (r *mockRelation) Destroy() error {
	r.MethodCall(r, "Destroy")
	return r.NextErr()
}

type mockUnit struct {
	application.Unit
	testing.Stub
	tag names.UnitTag
}

func (u *mockUnit) UnitTag() names.UnitTag {
	return u.tag
}

func (u *mockUnit) IsPrincipal() bool {
	u.MethodCall(u, "IsPrincipal")
	u.PopNoErr()
	return true
}

func (u *mockUnit) Destroy() error {
	u.MethodCall(u, "Destroy")
	return u.NextErr()
}

type mockStorageAttachment struct {
	state.StorageAttachment
	testing.Stub
	unit    names.UnitTag
	storage names.StorageTag
}

func (a *mockStorageAttachment) Unit() names.UnitTag {
	return a.unit
}

func (a *mockStorageAttachment) StorageInstance() names.StorageTag {
	return a.storage
}

type mockStorage struct {
	state.StorageInstance
	testing.Stub
	tag   names.StorageTag
	owner names.Tag
}

func (a *mockStorage) Kind() state.StorageKind {
	return state.StorageKindFilesystem
}

func (a *mockStorage) StorageTag() names.StorageTag {
	return a.tag
}

func (a *mockStorage) Owner() (names.Tag, bool) {
	return a.owner, a.owner != nil
}

type mockFilesystem struct {
	state.Filesystem
	detachable bool
}

func (f *mockFilesystem) Detachable() bool {
	return f.detachable
}

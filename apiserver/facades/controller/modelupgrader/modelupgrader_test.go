// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelupgrader_test

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facades/controller/modelupgrader"
	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

var (
	modelTag1 = names.NewModelTag("6e114b25-fc6d-448e-b58a-22fff690689e")
	modelTag2 = names.NewModelTag("631d2cbe-1085-4b74-ab76-41badfc73d9a")
)

type ModelUpgraderSuite struct {
	testing.IsolationSuite
	backend      mockBackend
	pool         mockPool
	providers    mockProviderRegistry
	watcher      mockWatcher
	statusSetter mockStatusSetter
	authorizer   apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&ModelUpgraderSuite{})

func (s *ModelUpgraderSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Controller: true,
		Tag:        names.NewMachineTag("0"),
	}
	s.backend = mockBackend{
		clouds: map[string]cloud.Cloud{
			"foo": {Type: "foo-provider"},
			"bar": {Type: "bar-provider"},
		},
	}
	s.pool = mockPool{
		models: map[string]*mockModel{
			modelTag1.Id(): {cloud: "foo", v: 0},
			modelTag2.Id(): {cloud: "bar", v: 1},
		},
	}
	s.providers = mockProviderRegistry{
		providers: map[string]*mockProvider{
			"foo-provider": {version: 123},
		},
	}
	s.watcher = mockWatcher{}
	s.statusSetter = mockStatusSetter{}
}

func (s *ModelUpgraderSuite) TestAuthController(c *gc.C) {
	_, err := modelupgrader.NewFacade(&s.backend, &s.pool, &s.providers, &s.watcher, &s.statusSetter, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ModelUpgraderSuite) TestAuthNonController(c *gc.C) {
	s.authorizer.Controller = false
	s.authorizer.Tag = names.NewUserTag("admin")
	_, err := modelupgrader.NewFacade(&s.backend, &s.pool, &s.providers, &s.watcher, &s.statusSetter, &s.authorizer)
	c.Assert(err, gc.Equals, common.ErrPerm)
}

func (s *ModelUpgraderSuite) TestModelEnvironVersion(c *gc.C) {
	facade, err := modelupgrader.NewFacade(&s.backend, &s.pool, &s.providers, &s.watcher, &s.statusSetter, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.ModelEnvironVersion(params.Entities{
		Entities: []params.Entity{
			{Tag: modelTag1.String()},
			{Tag: modelTag2.String()},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.IntResults{
		Results: []params.IntResult{{
			Result: 0,
		}, {
			Result: 1,
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid model tag`},
		}},
	})
	s.pool.CheckCalls(c, []testing.StubCall{
		{"GetModel", []interface{}{modelTag1.Id()}},
		{"GetModel", []interface{}{modelTag2.Id()}},
	})
	s.pool.models[modelTag1.Id()].CheckCallNames(c, "EnvironVersion")
	s.pool.models[modelTag2.Id()].CheckCallNames(c, "EnvironVersion")
}

func (s *ModelUpgraderSuite) TestModelTargetEnvironVersion(c *gc.C) {
	s.providers.SetErrors(nil, errors.New("blargh"))
	facade, err := modelupgrader.NewFacade(&s.backend, &s.pool, &s.providers, &s.watcher, &s.statusSetter, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.ModelTargetEnvironVersion(params.Entities{
		Entities: []params.Entity{
			{Tag: modelTag1.String()},
			{Tag: modelTag2.String()},
			{Tag: "machine-0"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.IntResults{
		Results: []params.IntResult{{
			Result: 123,
		}, {
			Error: &params.Error{Message: `blargh`},
		}, {
			Error: &params.Error{Message: `"machine-0" is not a valid model tag`},
		}},
	})
	s.backend.CheckCalls(c, []testing.StubCall{
		{"Cloud", []interface{}{"foo"}},
		{"Cloud", []interface{}{"bar"}},
	})
	s.pool.CheckCalls(c, []testing.StubCall{
		{"GetModel", []interface{}{modelTag1.Id()}},
		{"GetModel", []interface{}{modelTag2.Id()}},
	})
	s.pool.models[modelTag1.Id()].CheckCallNames(c, "CloudName")
	s.pool.models[modelTag2.Id()].CheckCallNames(c, "CloudName")
	s.providers.CheckCalls(c, []testing.StubCall{
		{"Provider", []interface{}{"foo-provider"}},
		{"Provider", []interface{}{"bar-provider"}},
	})
	s.providers.providers["foo-provider"].CheckCallNames(c, "Version")
}

func (s *ModelUpgraderSuite) TestSetModelEnvironVersion(c *gc.C) {
	facade, err := modelupgrader.NewFacade(&s.backend, &s.pool, &s.providers, &s.watcher, &s.statusSetter, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.SetModelEnvironVersion(params.SetModelEnvironVersions{
		Models: []params.SetModelEnvironVersion{
			{ModelTag: modelTag1.String(), Version: 1},
			{ModelTag: "machine-0", Version: 0},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			{},
			{&params.Error{Message: `"machine-0" is not a valid model tag`}},
		},
	})
	s.pool.CheckCalls(c, []testing.StubCall{
		{"GetModel", []interface{}{modelTag1.Id()}},
	})
	s.pool.models[modelTag1.Id()].CheckCalls(c, []testing.StubCall{
		{"SetEnvironVersion", []interface{}{int(1)}},
	})
}

func (s *ModelUpgraderSuite) TestSetModelStatus(c *gc.C) {
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{{
			Tag:    "machine-0",
			Status: "bar",
			Info:   "baz",
			Data: map[string]interface{}{
				"qux": "quux",
			},
		}},
	}
	s.statusSetter.results = params.ErrorResults{
		Results: []params.ErrorResult{
			{&params.Error{Message: `"machine-0" is not a valid model tag`}},
		},
	}

	facade, err := modelupgrader.NewFacade(&s.backend, &s.pool, &s.providers, &s.watcher, &s.statusSetter, &s.authorizer)
	c.Assert(err, jc.ErrorIsNil)
	results, err := facade.SetModelStatus(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, s.statusSetter.results)
	s.backend.CheckNoCalls(c)
	s.pool.models[modelTag1.Id()].CheckNoCalls(c)
	s.statusSetter.CheckCalls(c, []testing.StubCall{
		{"SetStatus", []interface{}{args}},
	})
}

type mockBackend struct {
	testing.Stub
	clouds map[string]cloud.Cloud
}

func (b *mockBackend) Cloud(name string) (cloud.Cloud, error) {
	b.MethodCall(b, "Cloud", name)
	return b.clouds[name], b.NextErr()
}

type mockPool struct {
	testing.Stub
	models map[string]*mockModel
}

func (p *mockPool) GetModel(uuid string) (modelupgrader.Model, func(), error) {
	p.MethodCall(p, "GetModel", uuid)
	return p.models[uuid], func() {}, p.NextErr()
}

type mockModel struct {
	testing.Stub
	cloud string
	v     int
}

func (m *mockModel) CloudName() string {
	m.MethodCall(m, "CloudName")
	m.PopNoErr()
	return m.cloud
}

func (m *mockModel) EnvironVersion() int {
	m.MethodCall(m, "EnvironVersion")
	m.PopNoErr()
	return m.v
}

func (m *mockModel) SetEnvironVersion(v int) error {
	m.MethodCall(m, "SetEnvironVersion", v)
	return m.NextErr()
}

type mockWatcher struct {
	testing.Stub
}

func (m *mockWatcher) Watch(args params.Entities) (params.NotifyWatchResults, error) {
	m.MethodCall(m, "Watch", args)
	if err := m.NextErr(); err != nil {
		return params.NotifyWatchResults{}, err
	}
	return params.NotifyWatchResults{}, errors.NotImplementedf("Watch")
}

type mockProviderRegistry struct {
	testing.Stub
	providers map[string]*mockProvider
}

func (m *mockProviderRegistry) Provider(name string) (environs.EnvironProvider, error) {
	m.MethodCall(m, "Provider", name)
	if err := m.NextErr(); err != nil {
		return nil, err
	}
	return m.providers[name], nil
}

type mockProvider struct {
	testing.Stub
	environs.EnvironProvider
	version int
}

func (m *mockProvider) Version() int {
	m.MethodCall(m, "Version")
	m.PopNoErr()
	return m.version
}

type mockStatusSetter struct {
	testing.Stub
	results params.ErrorResults
}

func (m *mockStatusSetter) SetStatus(args params.SetStatus) (params.ErrorResults, error) {
	m.MethodCall(m, "SetStatus", args)
	return m.results, m.NextErr()
}

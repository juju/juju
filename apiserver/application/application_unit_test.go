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
	s.application = mockApplication{}
	s.charm = mockCharm{
		config: &charm.Config{
			Options: map[string]charm.Option{
				"stringOption": {Type: "string"},
				"intOption":    {Type: "int", Default: int(123)},
			},
		},
	}
	s.backend = mockBackend{
		application: &s.application,
		charm:       &s.charm,
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

type mockBackend struct {
	application.Backend
	testing.Stub
	application *mockApplication
	charm       *mockCharm
}

func (b *mockBackend) ModelTag() names.ModelTag {
	b.MethodCall(b, "ModelTag")
	b.PopNoErr()
	return coretesting.ModelTag
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

type mockApplication struct {
	application.Application
	testing.Stub
}

func (a *mockApplication) SetCharm(cfg state.SetCharmConfig) error {
	a.MethodCall(a, "SetCharm", cfg)
	return a.NextErr()
}

type mockCharm struct {
	application.Charm
	testing.Stub
	config *charm.Config
}

func (c *mockCharm) Config() *charm.Config {
	c.MethodCall(c, "Config")
	c.PopNoErr()
	return c.config
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

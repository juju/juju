// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type upgradeEnvironConfigSuite struct {
	coretesting.BaseSuite
	stub     testing.Stub
	cfg      *config.Config
	reader   upgrades.EnvironConfigReader
	updater  upgrades.EnvironConfigUpdater
	registry *mockProviderRegistry
}

var _ = gc.Suite(&upgradeEnvironConfigSuite{})

func (s *upgradeEnvironConfigSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = testing.Stub{}
	s.cfg = coretesting.EnvironConfig(c)
	s.registry = &mockProviderRegistry{
		providers: make(map[string]environs.EnvironProvider),
	}

	s.reader = environConfigFunc(func() (*config.Config, error) {
		s.stub.AddCall("EnvironConfig")
		return s.cfg, s.stub.NextErr()
	})

	s.updater = updateEnvironConfigFunc(func(
		update map[string]interface{}, remove []string, validate state.ValidateConfigFunc,
	) error {
		s.stub.AddCall("UpdateEnvironConfig", update, remove, validate)
		return s.stub.NextErr()
	})
}

func (s *upgradeEnvironConfigSuite) TestUpgradeEnvironConfigEnvironConfigError(c *gc.C) {
	s.stub.SetErrors(errors.New("cannot read environ config"))
	err := upgrades.UpgradeEnvironConfig(s.reader, s.updater, s.registry)
	c.Assert(err, gc.ErrorMatches, "reading environment config: cannot read environ config")
	s.stub.CheckCallNames(c, "EnvironConfig")
}

func (s *upgradeEnvironConfigSuite) TestUpgradeEnvironConfigProviderNotRegistered(c *gc.C) {
	s.registry.SetErrors(errors.New(`no registered provider for "someprovider"`))
	err := upgrades.UpgradeEnvironConfig(s.reader, s.updater, s.registry)
	c.Assert(err, gc.ErrorMatches, `getting provider: no registered provider for "someprovider"`)
	s.stub.CheckCallNames(c, "EnvironConfig")
}

func (s *upgradeEnvironConfigSuite) TestUpgradeEnvironConfigProviderNotConfigUpgrader(c *gc.C) {
	s.registry.providers["someprovider"] = &mockEnvironProvider{}
	err := upgrades.UpgradeEnvironConfig(s.reader, s.updater, s.registry)
	c.Assert(err, jc.ErrorIsNil)
	s.registry.CheckCalls(c, []testing.StubCall{{
		FuncName: "Provider", Args: []interface{}{"someprovider"},
	}})
	s.stub.CheckCallNames(c, "EnvironConfig")
}

func (s *upgradeEnvironConfigSuite) TestUpgradeEnvironConfigProviderConfigUpgrader(c *gc.C) {
	var err error
	s.cfg, err = s.cfg.Apply(map[string]interface{}{"test-key": "test-value"})
	c.Assert(err, jc.ErrorIsNil)

	s.registry.providers["someprovider"] = &mockEnvironConfigUpgrader{
		upgradeConfig: func(cfg *config.Config) (*config.Config, error) {
			return cfg.Remove([]string{"test-key"})
		},
	}
	err = upgrades.UpgradeEnvironConfig(s.reader, s.updater, s.registry)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "EnvironConfig", "UpdateEnvironConfig")
	updateCall := s.stub.Calls()[1]
	expectedAttrs := s.cfg.AllAttrs()
	delete(expectedAttrs, "test-key")
	c.Assert(updateCall.Args, gc.HasLen, 3)
	c.Assert(updateCall.Args[0], jc.DeepEquals, expectedAttrs)
	c.Assert(updateCall.Args[1], jc.SameContents, []string{"test-key"})
	c.Assert(updateCall.Args[2], gc.IsNil)
}

func (s *upgradeEnvironConfigSuite) TestUpgradeEnvironConfigUpgradeConfigError(c *gc.C) {
	s.registry.providers["someprovider"] = &mockEnvironConfigUpgrader{
		upgradeConfig: func(cfg *config.Config) (*config.Config, error) {
			return nil, errors.New("cannot upgrade config")
		},
	}
	err := upgrades.UpgradeEnvironConfig(s.reader, s.updater, s.registry)
	c.Assert(err, gc.ErrorMatches, "upgrading config: cannot upgrade config")
	s.stub.CheckCallNames(c, "EnvironConfig")
}

func (s *upgradeEnvironConfigSuite) TestUpgradeEnvironConfigUpdateConfigError(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("cannot update environ config"))
	s.registry.providers["someprovider"] = &mockEnvironConfigUpgrader{
		upgradeConfig: func(cfg *config.Config) (*config.Config, error) {
			return cfg, nil
		},
	}
	err := upgrades.UpgradeEnvironConfig(s.reader, s.updater, s.registry)
	c.Assert(err, gc.ErrorMatches, "updating config in state: cannot update environ config")

	s.stub.CheckCallNames(c, "EnvironConfig", "UpdateEnvironConfig")
	updateCall := s.stub.Calls()[1]
	c.Assert(updateCall.Args, gc.HasLen, 3)
	c.Assert(updateCall.Args[0], jc.DeepEquals, s.cfg.AllAttrs())
	c.Assert(updateCall.Args[1], gc.IsNil)
	c.Assert(updateCall.Args[2], gc.IsNil)
}

type environConfigFunc func() (*config.Config, error)

func (f environConfigFunc) EnvironConfig() (*config.Config, error) {
	return f()
}

type updateEnvironConfigFunc func(map[string]interface{}, []string, state.ValidateConfigFunc) error

func (f updateEnvironConfigFunc) UpdateEnvironConfig(
	update map[string]interface{}, remove []string, validate state.ValidateConfigFunc,
) error {
	return f(update, remove, validate)
}

type mockProviderRegistry struct {
	environs.ProviderRegistry
	testing.Stub
	providers map[string]environs.EnvironProvider
}

func (r *mockProviderRegistry) Provider(name string) (environs.EnvironProvider, error) {
	r.MethodCall(r, "Provider", name)
	return r.providers[name], r.NextErr()
}

type mockEnvironProvider struct {
	testing.Stub
	environs.EnvironProvider
}

type mockEnvironConfigUpgrader struct {
	mockEnvironProvider
	upgradeConfig func(*config.Config) (*config.Config, error)
}

func (u *mockEnvironConfigUpgrader) UpgradeConfig(cfg *config.Config) (*config.Config, error) {
	u.MethodCall(u, "UpgradeConfig", cfg)
	return u.upgradeConfig(cfg)
}

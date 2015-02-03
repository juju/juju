// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"io"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/manual"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	coretesting.FakeJujuHomeSuite
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.PatchValue(manual.InitUbuntuUser, func(host, user, keys string, stdin io.Reader, stdout io.Writer) error {
		return nil
	})
}

func (s *providerSuite) TestPrepareForBootstrap(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = true
	delete(minimal, "storage-auth-key")
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	env, err := manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), testConfig)
	c.Assert(err, jc.ErrorIsNil)
	cfg := env.Config()
	key, _ := cfg.UnknownAttrs()["storage-auth-key"].(string)
	c.Assert(key, jc.Satisfies, utils.IsValidUUIDString)
}

func (s *providerSuite) TestPrepareUseSSHStorage(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = false
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	_, err = manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), testConfig)
	c.Assert(err, gc.ErrorMatches, "use-sshstorage must not be specified")

	s.PatchValue(manual.NewSSHStorage, func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return nil, fmt.Errorf("newSSHStorage failed")
	})
	minimal["use-sshstorage"] = true
	testConfig, err = config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	_, err = manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), testConfig)
	c.Assert(err, gc.ErrorMatches, "initialising SSH storage failed: newSSHStorage failed")
}

func (s *providerSuite) TestPrepareSetsUseSSHStorage(c *gc.C) {
	attrs := manual.MinimalConfigValues()
	delete(attrs, "use-sshstorage")
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), testConfig)
	c.Assert(err, jc.ErrorIsNil)
	cfg := env.Config()
	value := cfg.AllAttrs()["use-sshstorage"]
	c.Assert(value, jc.IsTrue)
}

func (s *providerSuite) TestOpenDoesntSetUseSSHStorage(c *gc.C) {
	attrs := manual.MinimalConfigValues()
	delete(attrs, "use-sshstorage")
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := manual.ProviderInstance.Open(testConfig)
	c.Assert(err, jc.ErrorIsNil)
	cfg := env.Config()
	_, ok := cfg.AllAttrs()["use-sshstorage"]
	c.Assert(ok, jc.IsFalse)
	ok = manual.EnvironUseSSHStorage(env)
	c.Assert(ok, jc.IsFalse)
}

func (s *providerSuite) TestNullAlias(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(p, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	p, err = environs.Provider("null")
	c.Assert(p, gc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestDisablesUpdatesByDefault(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)

	attrs := manual.MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testConfig.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(testConfig.EnableOSUpgrade(), jc.IsTrue)

	validCfg, err := p.Validate(testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Unless specified, update should default to true,
	// upgrade to false.
	c.Check(validCfg.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(validCfg.EnableOSUpgrade(), jc.IsFalse)
}

func (s *providerSuite) TestDefaultsCanBeOverriden(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)

	attrs := manual.MinimalConfigValues()
	attrs["enable-os-refresh-update"] = true
	attrs["enable-os-upgrade"] = true

	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	validCfg, err := p.Validate(testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Our preferences should not have been overwritten.
	c.Check(validCfg.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(validCfg.EnableOSUpgrade(), jc.IsTrue)
}

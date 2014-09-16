// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"fmt"
	"io"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/storage"
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

func (s *providerSuite) TestPrepare(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = true
	delete(minimal, "storage-auth-key")
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	env, err := manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
	c.Assert(err, gc.IsNil)
	cfg := env.Config()
	key, _ := cfg.UnknownAttrs()["storage-auth-key"].(string)
	c.Assert(key, jc.Satisfies, utils.IsValidUUIDString)
}

func (s *providerSuite) TestPrepareUseSSHStorage(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = false
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	_, err = manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
	c.Assert(err, gc.ErrorMatches, "use-sshstorage must not be specified")

	s.PatchValue(manual.NewSSHStorage, func(sshHost, storageDir, storageTmpdir string) (storage.Storage, error) {
		return nil, fmt.Errorf("newSSHStorage failed")
	})
	minimal["use-sshstorage"] = true
	testConfig, err = config.New(config.UseDefaults, minimal)
	c.Assert(err, gc.IsNil)
	_, err = manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
	c.Assert(err, gc.ErrorMatches, "initialising SSH storage failed: newSSHStorage failed")
}

func (s *providerSuite) TestPrepareSetsUseSSHStorage(c *gc.C) {
	attrs := manual.MinimalConfigValues()
	delete(attrs, "use-sshstorage")
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)

	env, err := manual.ProviderInstance.Prepare(coretesting.Context(c), testConfig)
	c.Assert(err, gc.IsNil)
	cfg := env.Config()
	value := cfg.AllAttrs()["use-sshstorage"]
	c.Assert(value, gc.Equals, true)
}

func (s *providerSuite) TestOpenDoesntSetUseSSHStorage(c *gc.C) {
	attrs := manual.MinimalConfigValues()
	delete(attrs, "use-sshstorage")
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)

	env, err := manual.ProviderInstance.Open(testConfig)
	c.Assert(err, gc.IsNil)
	cfg := env.Config()
	_, ok := cfg.AllAttrs()["use-sshstorage"]
	c.Assert(ok, jc.IsFalse)
	ok = manual.EnvironUseSSHStorage(env)
	c.Assert(ok, jc.IsFalse)
}

func (s *providerSuite) TestNullAlias(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(p, gc.NotNil)
	c.Assert(err, gc.IsNil)
	p, err = environs.Provider("null")
	c.Assert(p, gc.NotNil)
	c.Assert(err, gc.IsNil)
}

func (s *providerSuite) TestDisablesUpdatesByDefault(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, gc.IsNil)

	attrs := manual.MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	c.Check(testConfig.EnableOSRefreshUpdate(), gc.Equals, true)
	c.Check(testConfig.EnableOSUpgrade(), gc.Equals, true)

	validCfg, err := p.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)

	// Unless specified, update should default to true,
	// upgrade to false.
	c.Check(validCfg.EnableOSRefreshUpdate(), gc.Equals, true)
	c.Check(validCfg.EnableOSUpgrade(), gc.Equals, false)
}

func (s *providerSuite) TestDefaultsCanBeOverriden(c *gc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, gc.IsNil)

	attrs := manual.MinimalConfigValues()
	attrs["enable-os-refresh-update"] = true
	attrs["enable-os-upgrade"] = true

	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, gc.IsNil)
	validCfg, err := p.Validate(testConfig, nil)
	c.Assert(err, gc.IsNil)

	// Our preferences should not have been overwritten.
	c.Check(validCfg.EnableOSRefreshUpdate(), gc.Equals, true)
	c.Check(validCfg.EnableOSUpgrade(), gc.Equals, true)
}

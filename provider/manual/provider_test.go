// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/manual"
	coretesting "github.com/juju/juju/testing"
)

type providerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.Stub
}

var _ = gc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.PatchValue(manual.InitUbuntuUser, func(host, user, keys string, stdin io.Reader, stdout io.Writer) error {
		s.AddCall("InitUbuntuUser", host, user, keys, stdin, stdout)
		return s.NextErr()
	})
}

func (s *providerSuite) TestPrepareForBootstrapCloudEndpointAndRegion(c *gc.C) {
	ctx, err := s.testPrepareForBootstrap(c, "endpoint", "region")
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "InitUbuntuUser", "endpoint", "", "public auth key\n", ctx.GetStdin(), ctx.GetStdout())
}

func (s *providerSuite) TestPrepareForBootstrapCloudRegionOnly(c *gc.C) {
	ctx, err := s.testPrepareForBootstrap(c, "", "region")
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "InitUbuntuUser", "region", "", "public auth key\n", ctx.GetStdin(), ctx.GetStdout())
}

func (s *providerSuite) TestPrepareForBootstrapNoCloudEndpointOrRegion(c *gc.C) {
	_, err := s.testPrepareForBootstrap(c, "", "")
	c.Assert(err, gc.ErrorMatches,
		`missing address of host to bootstrap: please specify "juju bootstrap manual/<host>"`)
}

func (s *providerSuite) testPrepareForBootstrap(c *gc.C, endpoint, region string) (environs.BootstrapContext, error) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = true
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	ctx := envtesting.BootstrapContext(c)
	_, err = manual.ProviderInstance.PrepareForBootstrap(ctx, environs.PrepareForBootstrapParams{
		Config:        testConfig,
		CloudEndpoint: endpoint,
		CloudRegion:   region,
	})
	return ctx, err
}

func (s *providerSuite) TestPrepareUseSSHStorage(c *gc.C) {
	minimal := manual.MinimalConfigValues()
	minimal["use-sshstorage"] = false
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	_, err = manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), environs.PrepareForBootstrapParams{
		Config:        testConfig,
		CloudEndpoint: "hostname",
	})
	c.Assert(err, gc.ErrorMatches, "use-sshstorage must not be specified")

	minimal["use-sshstorage"] = true
	testConfig, err = config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	_, err = manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), environs.PrepareForBootstrapParams{
		Config:        testConfig,
		CloudEndpoint: "hostname",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestPrepareSetsUseSSHStorage(c *gc.C) {
	attrs := manual.MinimalConfigValues()
	delete(attrs, "use-sshstorage")
	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)

	env, err := manual.ProviderInstance.PrepareForBootstrap(envtesting.BootstrapContext(c), environs.PrepareForBootstrapParams{
		Config:        testConfig,
		CloudEndpoint: "hostname",
	})
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

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/manual/sshprovisioner"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/manual"
	coretesting "github.com/juju/juju/internal/testing"
)

type providerSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	testing.Stub
}

var _ = tc.Suite(&providerSuite{})

func (s *providerSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.Stub.ResetCalls()
	s.PatchValue(&sshprovisioner.CheckProvisioned, func(host string, login string) (bool, error) {
		s.AddCall("CheckProvisioned", host, login)
		return false, s.NextErr()
	})
	s.PatchValue(&sshprovisioner.DetectBaseAndHardwareCharacteristics, func(host string, login string) (hc instance.HardwareCharacteristics, base corebase.Base,
		err error) {
		s.AddCall("DetectBaseAndHardwareCharacteristics", host, login)
		arch := "fake"
		hc.Arch = &arch
		return hc, base, s.NextErr()
	})
	s.PatchValue(manual.InitUbuntuUser, func(host, user, keys string, privateKey string, stdin io.Reader, stdout io.Writer) error {
		s.AddCall("InitUbuntuUser", host, user, keys, privateKey, stdin, stdout)
		return s.NextErr()
	})
}

// TestPrepareForBootstrap verifies that Prepare For bootstrap is a noop for
// manual provider
func (s *providerSuite) TestPrepareForBootstrap(c *tc.C) {
	_, err := s.testPrepareForBootstrap(c)
	c.Assert(err, jc.ErrorIsNil)
	s.CheckNoCalls(c)
}

// TestBootstrapNoCloudEndpoint ensures that error messages are correctly
// returned when no cloud endpoint is specified during bootstrap.
func (s *providerSuite) TestBootstrapNoCloudEndpoint(c *tc.C) {
	_, err := s.testBootstrap(c, testBootstrapArgs{})
	c.Assert(err, tc.ErrorMatches,
		`validating cloud spec: missing address of host to bootstrap: please specify "juju bootstrap manual/\[user@\]<host>"`)
}

// TestBootstrap executes the bootstrap process for a manual provider,
// verifying key provisioning behaviors and call logic.
func (s *providerSuite) TestBootstrap(c *tc.C) {
	ctx, err := s.testBootstrap(c, testBootstrapArgs{
		endpoint: "hostname",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "CheckProvisioned", "hostname", "")
	s.CheckCall(c, 1, "InitUbuntuUser", "hostname", "", "", "", ctx.GetStdin(), ctx.GetStdout())
	s.CheckCall(c, 2, "DetectBaseAndHardwareCharacteristics", "hostname", "")
}

// TestBootstrapUserHost tests the bootstrap process for a manual provider with
// a "user@host" endpoint configuration.
func (s *providerSuite) TestBootstrapUserHost(c *tc.C) {
	ctx, err := s.testBootstrap(c, testBootstrapArgs{
		endpoint: "user@hostwithuser",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "CheckProvisioned", "hostwithuser", "user")
	s.CheckCall(c, 1, "InitUbuntuUser", "hostwithuser", "user", "", "", ctx.GetStdin(), ctx.GetStdout())
	s.CheckCall(c, 2, "DetectBaseAndHardwareCharacteristics", "hostwithuser", "user")
}

// TestBootstrapUserHostAuthorizedKeys tests bootstrapping with authorized SSH
// keys for a user on a specified host.
func (s *providerSuite) TestBootstrapUserHostAuthorizedKeys(c *tc.C) {
	ctx, err := s.testBootstrap(c, testBootstrapArgs{
		endpoint: "userwithauth@host",
		params: environs.BootstrapParams{
			AuthorizedKeys: []string{"key1", "key2"},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "CheckProvisioned", "host", "userwithauth")
	s.CheckCall(c, 1, "InitUbuntuUser", "host", "userwithauth", "key1\nkey2", "", ctx.GetStdin(), ctx.GetStdout())
	s.CheckCall(c, 2, "DetectBaseAndHardwareCharacteristics", "host", "userwithauth")
}

func (s *providerSuite) testPrepareForBootstrap(c *tc.C) (environs.BootstrapContext, error) {
	minimal := manual.MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	cloudSpec := environscloudspec.CloudSpec{
		Endpoint: "endpoint",
	}
	err = manual.ProviderInstance.ValidateCloud(context.Background(), cloudSpec)
	if err != nil {
		return nil, err
	}
	env, err := manual.ProviderInstance.Open(context.Background(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: testConfig,
	}, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, err
	}
	ctx := envtesting.BootstrapContext(context.Background(), c)
	return ctx, env.PrepareForBootstrap(ctx, "controller-1")
}

type testBootstrapArgs struct {
	endpoint string
	params   environs.BootstrapParams
}

func (s *providerSuite) testBootstrap(c *tc.C, args testBootstrapArgs) (environs.BootstrapContext, error) {
	minimal := manual.MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	cloudSpec := environscloudspec.CloudSpec{
		Endpoint: args.endpoint,
		Region:   "region",
	}
	err = manual.ProviderInstance.ValidateCloud(context.Background(), cloudSpec)
	if err != nil {
		return nil, err
	}
	env, err := manual.ProviderInstance.Open(context.Background(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: testConfig,
	}, environs.NoopCredentialInvalidator())
	if err != nil {
		return nil, err
	}
	ctx := envtesting.BootstrapContext(context.Background(), c)
	_, err = env.Bootstrap(ctx, args.params)
	return ctx, err
}

func (s *providerSuite) TestNullAlias(c *tc.C) {
	p, err := environs.Provider("manual")
	c.Assert(p, tc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
	p, err = environs.Provider("null")
	c.Assert(p, tc.NotNil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestDisablesUpdatesByDefault(c *tc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)

	attrs := manual.MinimalConfigValues()
	testConfig, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(testConfig.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(testConfig.EnableOSUpgrade(), jc.IsTrue)

	validCfg, err := p.Validate(context.Background(), testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Unless specified, update should default to true,
	// upgrade to false.
	c.Check(validCfg.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(validCfg.EnableOSUpgrade(), jc.IsFalse)
}

func (s *providerSuite) TestDefaultsCanBeOverriden(c *tc.C) {
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)

	attrs := manual.MinimalConfigValues()
	attrs["enable-os-refresh-update"] = true
	attrs["enable-os-upgrade"] = true

	testConfig, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	validCfg, err := p.Validate(context.Background(), testConfig, nil)
	c.Assert(err, jc.ErrorIsNil)

	// Our preferences should not have been overwritten.
	c.Check(validCfg.EnableOSRefreshUpdate(), jc.IsTrue)
	c.Check(validCfg.EnableOSUpgrade(), jc.IsTrue)
}

func (s *providerSuite) TestSchema(c *tc.C) {
	vals := map[string]interface{}{"endpoint": "http://foo.com/bar"}

	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
	err = p.CloudSchema().Validate(vals)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestPingEndpointWithUser(c *tc.C) {
	endpoint := "user@IP"
	called := false
	s.PatchValue(manual.Echo, func(s string) error {
		c.Assert(s, tc.Equals, endpoint)
		called = true
		return nil
	})
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Ping(context.Background(), endpoint), jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *providerSuite) TestPingIP(c *tc.C) {
	endpoint := "P"
	called := 0
	s.PatchValue(manual.Echo, func(s string) error {
		c.Assert(called < 2, jc.IsTrue)
		if called == 0 {
			c.Assert(s, tc.Equals, endpoint)
		} else {
			c.Assert(s, tc.Equals, fmt.Sprintf("ubuntu@%v", endpoint))
		}
		called++
		return nil
	})
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Ping(context.Background(), endpoint), jc.ErrorIsNil)
	// Expect the call to be made twice.
	c.Assert(called, tc.Equals, 1)
}

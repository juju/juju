// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual_test

import (
	"context"
	"fmt"
	"io"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/provider/manual"
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
	s.PatchValue(manual.InitUbuntuUser, func(host, user, keys string, privateKey string, stdin io.Reader, stdout io.Writer) error {
		s.AddCall("InitUbuntuUser", host, user, keys, privateKey, stdin, stdout)
		return s.NextErr()
	})
}

func (s *providerSuite) TestPrepareForBootstrapCloudEndpointAndRegion(c *gc.C) {
	ctx, err := s.testPrepareForBootstrap(c, "endpoint", "region")
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "InitUbuntuUser", "endpoint", "", "", "", ctx.GetStdin(), ctx.GetStdout())
}

func (s *providerSuite) TestPrepareForBootstrapUserHost(c *gc.C) {
	ctx, err := s.testPrepareForBootstrap(c, "user@host", "")
	c.Assert(err, jc.ErrorIsNil)
	s.CheckCall(c, 0, "InitUbuntuUser", "host", "user", "", "", ctx.GetStdin(), ctx.GetStdout())
}

func (s *providerSuite) TestPrepareForBootstrapNoCloudEndpoint(c *gc.C) {
	_, err := s.testPrepareForBootstrap(c, "", "region")
	c.Assert(err, gc.ErrorMatches,
		`missing address of host to bootstrap: please specify "juju bootstrap manual/\[user@\]<host>"`)
}

func (s *providerSuite) testPrepareForBootstrap(c *gc.C, endpoint, region string) (environs.BootstrapContext, error) {
	minimal := manual.MinimalConfigValues()
	testConfig, err := config.New(config.UseDefaults, minimal)
	c.Assert(err, jc.ErrorIsNil)
	cloudSpec := environscloudspec.CloudSpec{
		Endpoint: endpoint,
		Region:   region,
	}
	testConfig, err = manual.ProviderInstance.PrepareConfig(environs.PrepareConfigParams{
		Config: testConfig,
		Cloud:  cloudSpec,
	})
	if err != nil {
		return nil, err
	}
	env, err := manual.ProviderInstance.Open(context.Background(), environs.OpenParams{
		Cloud:  cloudSpec,
		Config: testConfig,
	})
	if err != nil {
		return nil, err
	}
	ctx := envtesting.BootstrapContext(context.Background(), c)
	return ctx, env.PrepareForBootstrap(ctx, "controller-1")
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
	testConfig, err := config.New(config.NoDefaults, attrs)
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

func (s *providerSuite) TestSchema(c *gc.C) {
	vals := map[string]interface{}{"endpoint": "http://foo.com/bar"}

	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
	err = p.CloudSchema().Validate(vals)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *providerSuite) TestPingEndpointWithUser(c *gc.C) {
	endpoint := "user@IP"
	called := false
	s.PatchValue(manual.Echo, func(s string) error {
		c.Assert(s, gc.Equals, endpoint)
		called = true
		return nil
	})
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Ping(envcontext.WithoutCredentialInvalidator(context.Background()), endpoint), jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *providerSuite) TestPingIP(c *gc.C) {
	endpoint := "P"
	called := 0
	s.PatchValue(manual.Echo, func(s string) error {
		c.Assert(called < 2, jc.IsTrue)
		if called == 0 {
			c.Assert(s, gc.Equals, endpoint)
		} else {
			c.Assert(s, gc.Equals, fmt.Sprintf("ubuntu@%v", endpoint))
		}
		called++
		return nil
	})
	p, err := environs.Provider("manual")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(p.Ping(envcontext.WithoutCredentialInvalidator(context.Background()), endpoint), jc.ErrorIsNil)
	// Expect the call to be made twice.
	c.Assert(called, gc.Equals, 1)
}

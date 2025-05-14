// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"fmt"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/testing"
)

type environSuite struct {
	gce.BaseSuite
}

var _ = tc.Suite(&environSuite{})

func (s *environSuite) TestName(c *tc.C) {
	name := s.Env.Name()

	c.Check(name, tc.Equals, "google")
}

func (s *environSuite) TestProvider(c *tc.C) {
	provider := s.Env.Provider()

	c.Check(provider, tc.Equals, gce.Provider)
}

func (s *environSuite) TestRegion(c *tc.C) {
	cloudSpec, err := s.Env.Region()
	c.Assert(err, tc.ErrorIsNil)

	c.Check(cloudSpec.Region, tc.Equals, "us-east1")
	c.Check(cloudSpec.Endpoint, tc.Equals, "https://www.googleapis.com")
}

func (s *environSuite) TestSetConfig(c *tc.C) {
	err := s.Env.SetConfig(c.Context(), s.Config)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(gce.ExposeEnvConfig(s.Env), tc.DeepEquals, s.EnvConfig)
	c.Check(gce.ExposeEnvConnection(s.Env), tc.Equals, s.FakeConn)
}

func (s *environSuite) TestSetConfigFake(c *tc.C) {
	err := s.Env.SetConfig(c.Context(), s.Config)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 0)
}

func (s *environSuite) TestConfig(c *tc.C) {
	cfg := s.Env.Config()

	c.Check(cfg, tc.DeepEquals, s.Config)
}

func (s *environSuite) TestBootstrap(c *tc.C) {
	s.FakeCommon.Arch = "amd64"
	s.FakeCommon.Base = corebase.MakeDefaultBase("ubuntu", "22.04")
	finalizer := func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
		return nil
	}
	s.FakeCommon.BSFinalizer = finalizer

	ctx := envtesting.BootstrapTestContext(c)
	params := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}
	result, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(result.Arch, tc.Equals, "amd64")
	c.Check(result.Base.DisplayString(), tc.Equals, "ubuntu@22.04")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(result.CloudBootstrapFinalizer, tc.NotNil)
}

func (s *environSuite) TestBootstrapInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	params := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}
	_, err := s.Env.Bootstrap(envtesting.BootstrapTestContext(c), params)
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environSuite) TestBootstrapOpensAPIPort(c *tc.C) {
	config := testing.FakeControllerConfig()
	s.checkAPIPorts(c, config, []int{config.APIPort()})
}

func (s *environSuite) TestBootstrapOpensAPIPortsWithAutocert(c *tc.C) {
	config := testing.FakeControllerConfig()
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"
	s.checkAPIPorts(c, config, []int{443, 80})
}

func (s *environSuite) checkAPIPorts(c *tc.C, config controller.Config, expectedPorts []int) {
	finalizer := func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
		return nil
	}
	s.FakeCommon.BSFinalizer = finalizer

	ctx := envtesting.BootstrapTestContext(c)
	params := environs.BootstrapParams{
		ControllerConfig:        config,
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}
	_, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, tc.ErrorIsNil)

	called, calls := s.FakeConn.WasCalled("OpenPorts")
	c.Check(called, tc.Equals, true)
	// NOTE(achilleasa): the bootstrap code will merge the port ranges
	// for the API and port 80 when using autocert in a single OpenPorts
	// call
	c.Check(calls, tc.HasLen, 1)

	var expRules firewall.IngressRules
	for _, port := range expectedPorts {
		expRules = append(
			expRules,
			firewall.NewIngressRule(network.MustParsePortRange(fmt.Sprintf("%d/tcp", port))),
		)
	}

	call := calls[0]
	c.Check(call.FirewallName, tc.Equals, gce.GlobalFirewallName(s.Env))
	c.Check(call.Rules, tc.DeepEquals, expRules)
}

func (s *environSuite) TestBootstrapCommon(c *tc.C) {
	ctx := envtesting.BootstrapTestContext(c)
	params := environs.BootstrapParams{
		ControllerConfig:        testing.FakeControllerConfig(),
		SupportedBootstrapBases: testing.FakeSupportedJujuBases,
	}
	_, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, tc.ErrorIsNil)

	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "Bootstrap",
		Args: gce.FakeCallArgs{
			"ctx":    ctx,
			"switch": s.Env,
			"params": params,
		},
	}})
}

func (s *environSuite) TestDestroyInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	err := s.Env.Destroy(c.Context())
	c.Check(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environSuite) TestDestroy(c *tc.C) {
	err := s.Env.Destroy(c.Context())

	c.Check(err, tc.ErrorIsNil)
}

func (s *environSuite) TestDestroyAPI(c *tc.C) {
	err := s.Env.Destroy(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, tc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, tc.Equals, "RemoveFirewall")
	fwname := common.EnvFullName(s.Env.Config().UUID())
	c.Check(s.FakeConn.Calls[0].FirewallName, tc.Equals, fwname)
	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "Destroy",
		Args: gce.FakeCallArgs{
			"switch": s.Env,
		},
	}})
}

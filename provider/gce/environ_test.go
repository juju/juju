// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/testing"
)

type environSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestName(c *gc.C) {
	name := s.Env.Name()

	c.Check(name, gc.Equals, "google")
}

func (s *environSuite) TestProvider(c *gc.C) {
	provider := s.Env.Provider()

	c.Check(provider, gc.Equals, gce.Provider)
}

func (s *environSuite) TestRegion(c *gc.C) {
	cloudSpec, err := s.Env.Region()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(cloudSpec.Region, gc.Equals, "us-east1")
	c.Check(cloudSpec.Endpoint, gc.Equals, "https://www.googleapis.com")
}

func (s *environSuite) TestSetConfig(c *gc.C) {
	err := s.Env.SetConfig(s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(gce.ExposeEnvConfig(s.Env), jc.DeepEquals, s.EnvConfig)
	c.Check(gce.ExposeEnvConnection(s.Env), gc.Equals, s.FakeConn)
}

func (s *environSuite) TestSetConfigFake(c *gc.C) {
	err := s.Env.SetConfig(s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 0)
}

func (s *environSuite) TestConfig(c *gc.C) {
	cfg := s.Env.Config()

	c.Check(cfg, jc.DeepEquals, s.Config)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.FakeCommon.Arch = "amd64"
	s.FakeCommon.Series = "trusty"
	finalizer := func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
		return nil
	}
	s.FakeCommon.BSFinalizer = finalizer

	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	}
	result, err := s.Env.Bootstrap(ctx, s.CallCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Arch, gc.Equals, "amd64")
	c.Check(result.Series, gc.Equals, "trusty")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(result.CloudBootstrapFinalizer, gc.NotNil)
}

func (s *environSuite) TestBootstrapInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	params := environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	}
	_, err := s.Env.Bootstrap(envtesting.BootstrapContext(c), s.CallCtx, params)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environSuite) TestBootstrapOpensAPIPort(c *gc.C) {
	config := testing.FakeControllerConfig()
	s.checkAPIPorts(c, config, []int{config.APIPort()})
}

func (s *environSuite) TestBootstrapOpensAPIPortsWithAutocert(c *gc.C) {
	config := testing.FakeControllerConfig()
	config["api-port"] = 443
	config["autocert-dns-name"] = "example.com"
	s.checkAPIPorts(c, config, []int{443, 80})
}

func (s *environSuite) checkAPIPorts(c *gc.C, config controller.Config, expectedPorts []int) {
	finalizer := func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
		return nil
	}
	s.FakeCommon.BSFinalizer = finalizer

	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{
		ControllerConfig: config,
	}
	_, err := s.Env.Bootstrap(ctx, s.CallCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	called, calls := s.FakeConn.WasCalled("OpenPorts")
	c.Check(called, gc.Equals, true)
	c.Check(calls, gc.HasLen, len(expectedPorts))
	for i, call := range calls {
		port := expectedPorts[i]
		c.Check(call.FirewallName, gc.Equals, gce.GlobalFirewallName(s.Env))
		c.Check(call.Rules, jc.DeepEquals, []network.IngressRule{
			network.MustNewIngressRule("tcp", port, port),
		})
	}
}

func (s *environSuite) TestBootstrapCommon(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{
		ControllerConfig: testing.FakeControllerConfig(),
	}
	_, err := s.Env.Bootstrap(ctx, s.CallCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "Bootstrap",
		Args: gce.FakeCallArgs{
			"ctx":    ctx,
			"switch": s.Env,
			"params": params,
		},
	}})
}

func (s *environSuite) TestCreateInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.Create(s.CallCtx, environs.CreateParams{})
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environSuite) TestDestroyInvalidCredentialError(c *gc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	err := s.Env.Destroy(s.CallCtx)
	c.Check(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environSuite) TestDestroy(c *gc.C) {
	err := s.Env.Destroy(s.CallCtx)

	c.Check(err, jc.ErrorIsNil)
}

func (s *environSuite) TestDestroyAPI(c *gc.C) {
	err := s.Env.Destroy(s.CallCtx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Ports")
	fwname := common.EnvFullName(s.Env.Config().UUID())
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "Destroy",
		Args: gce.FakeCallArgs{
			"switch": s.Env,
		},
	}})
}

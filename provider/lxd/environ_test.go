// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/gce"
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

	c.Check(cloudSpec.Region, gc.Equals, "home")
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

func (s *environSuite) TestSetConfigMissing(c *gc.C) {
	gce.UnsetEnvConfig(s.Env)

	err := s.Env.SetConfig(s.Config)

	c.Check(err, gc.ErrorMatches, "cannot set config on uninitialized env")
}

func (s *environSuite) TestConfig(c *gc.C) {
	cfg := s.Env.Config()

	c.Check(cfg, jc.DeepEquals, s.Config)
}

func (s *environSuite) TestBootstrap(c *gc.C) {
	s.FakeCommon.Arch = "amd64"
	s.FakeCommon.Series = "trusty"
	finalizer := func(environs.BootstrapContext, *instancecfg.InstanceConfig) error {
		return nil
	}
	s.FakeCommon.BSFinalizer = finalizer

	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{}
	arch, series, bsFinalizer, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(arch, gc.Equals, "amd64")
	c.Check(series, gc.Equals, "trusty")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(bsFinalizer, gc.NotNil)
}

func (s *environSuite) TestBootstrapCommon(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{}
	_, _, _, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, jc.ErrorIsNil)

	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "Bootstrap",
		Args: gce.FakeCallArgs{
			"ctx":    ctx,
			"env":    s.Env,
			"params": params,
		},
	}})
}

func (s *environSuite) TestDestroy(c *gc.C) {
	err := s.Env.Destroy()

	c.Check(err, jc.ErrorIsNil)
}

func (s *environSuite) TestDestroyAPI(c *gc.C) {
	err := s.Env.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(s.FakeConn.Calls, gc.HasLen, 1)
	c.Check(s.FakeConn.Calls[0].FuncName, gc.Equals, "Ports")
	fwname := s.Prefix[:len(s.Prefix)-1]
	c.Check(s.FakeConn.Calls[0].FirewallName, gc.Equals, fwname)
	s.FakeCommon.CheckCalls(c, []gce.FakeCall{{
		FuncName: "Destroy",
		Args: gce.FakeCallArgs{
			"env": s.Env,
		},
	}})
}

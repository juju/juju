// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd_test

import (
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/environs"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/lxd"
)

type environSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestName(c *gc.C) {
	name := s.Env.Name()

	c.Check(name, gc.Equals, "lxd")
}

func (s *environSuite) TestProvider(c *gc.C) {
	provider := s.Env.Provider()

	c.Check(provider, gc.Equals, lxd.Provider)
}

func (s *environSuite) TestSetConfigOkay(c *gc.C) {
	err := s.Env.SetConfig(s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(lxd.ExposeEnvConfig(s.Env), jc.DeepEquals, s.EnvConfig)
	// Ensure the client did not change.
	c.Check(lxd.ExposeEnvClient(s.Env), gc.Equals, s.Client)
}

func (s *environSuite) TestSetConfigNoAPI(c *gc.C) {
	err := s.Env.SetConfig(s.Config)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "asNonLocal")
}

func (s *environSuite) TestSetConfigMissing(c *gc.C) {
	lxd.UnsetEnvConfig(s.Env)

	err := s.Env.SetConfig(s.Config)

	c.Check(err, gc.ErrorMatches, "cannot set config on uninitialized env")
}

func (s *environSuite) TestConfig(c *gc.C) {
	cfg := s.Env.Config()

	c.Check(cfg, jc.DeepEquals, s.Config)
}

func (s *environSuite) TestBootstrapOkay(c *gc.C) {
	s.Common.BootstrapResult = &environs.BootstrapResult{
		Arch:   "amd64",
		Series: "trusty",
		Finalize: func(environs.BootstrapContext, *instancecfg.InstanceConfig) error {
			return nil
		},
	}

	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{}
	result, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Arch, gc.Equals, "amd64")
	c.Check(result.Series, gc.Equals, "trusty")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(result.Finalize, gc.NotNil)
}

func (s *environSuite) TestBootstrapAPI(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{}
	_, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "Bootstrap",
		Args: []interface{}{
			ctx,
			params,
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

	fwname := s.Prefix[:len(s.Prefix)-1]
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "Ports",
		Args: []interface{}{
			fwname,
		},
	}, {
		FuncName: "Destroy",
		Args:     nil,
	}})
}

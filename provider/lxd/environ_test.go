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
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/provider/lxd"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools/lxdclient"
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
}

func (s *environSuite) TestConfig(c *gc.C) {
	cfg := s.Env.Config()

	c.Check(cfg, jc.DeepEquals, s.Config)
}

func (s *environSuite) TestBootstrapOkay(c *gc.C) {
	s.Common.BootstrapResult = &environs.BootstrapResult{
		Arch:   "amd64",
		Series: "trusty",
		Finalize: func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
			return nil
		},
	}

	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
	}
	result, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Arch, gc.Equals, "amd64")
	c.Check(result.Series, gc.Equals, "trusty")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(result.Finalize, gc.NotNil)
}

func (s *environSuite) TestBootstrapAPI(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
	}
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

	fwname := common.EnvFullName(s.Env.Config().UUID())
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

func (s *environSuite) TestDestroyHostedModels(c *gc.C) {
	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	// machine0 is in the controller model.
	machine0 := s.NewRawInstance(c, "juju-whatever-machine-0")
	machine0.InstanceSummary.Metadata["juju-model-uuid"] = s.Config.UUID()
	machine0.InstanceSummary.Metadata["juju-controller-uuid"] = s.Config.UUID()
	// machine1 is not in the controller model, but managed
	// by the same controller.
	machine1 := s.NewRawInstance(c, "juju-whatever-machine-1")
	machine1.InstanceSummary.Metadata["juju-model-uuid"] = "not-" + s.Config.UUID()
	machine1.InstanceSummary.Metadata["juju-controller-uuid"] = s.Config.UUID()
	// machine2 is not managed by the same controller.
	machine2 := s.NewRawInstance(c, "juju-whatever-machine-2")
	machine2.InstanceSummary.Metadata["juju-model-uuid"] = "not-" + s.Config.UUID()
	machine2.InstanceSummary.Metadata["juju-controller-uuid"] = "not-" + s.Config.UUID()
	s.Client.Insts = append(s.Client.Insts, *machine0, *machine1, *machine2)

	err := s.Env.DestroyController(s.Config.UUID())
	c.Assert(err, jc.ErrorIsNil)

	prefix := s.Prefix()
	fwname := common.EnvFullName(s.Env.Config().UUID())
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"Ports", []interface{}{fwname}},
		{"Destroy", nil},
		{"Instances", []interface{}{prefix, lxdclient.AliveStatuses}},
		{"RemoveInstances", []interface{}{prefix, []string{machine1.Name}}},
	})
}

func (s *environSuite) TestPrepareForBootstrap(c *gc.C) {
	err := s.Env.PrepareForBootstrap(envtesting.BootstrapContext(c))
	c.Assert(err, jc.ErrorIsNil)
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"SetConfig", []interface{}{"core.https_address", "[::]"}},
	})
}

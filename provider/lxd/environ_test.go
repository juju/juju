// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/juju/cmd/cmdtesting"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/lxd"
	coretesting "github.com/juju/juju/testing"
)

type environSuite struct {
	lxd.BaseSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = context.NewCloudCallContext()
}

func (s *environSuite) TestName(c *gc.C) {
	c.Check(s.Env.Name(), gc.Equals, "lxd")
}

func (s *environSuite) TestProvider(c *gc.C) {
	c.Assert(s.Env.Provider(), gc.Equals, s.Provider)
}

func (s *environSuite) TestSetConfigOkay(c *gc.C) {
	err := s.Env.SetConfig(s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(lxd.ExposeEnvConfig(s.Env), jc.DeepEquals, s.EnvConfig)
	// Ensure the client did not change.
	c.Check(lxd.ExposeEnvServer(s.Env), gc.Equals, s.Client)
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

	ctx := cmdtesting.Context(c)
	params := environs.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
	}
	result, err := s.Env.Bootstrap(modelcmd.BootstrapContext(ctx), s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Arch, gc.Equals, "amd64")
	c.Check(result.Series, gc.Equals, "trusty")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(result.Finalize, gc.NotNil)

	out := cmdtesting.Stderr(ctx)
	c.Assert(out, gc.Equals, "To configure your system to better support LXD containers, please see: https://github.com/lxc/lxd/blob/master/doc/production-setup.md\n")
}

func (s *environSuite) TestBootstrapAPI(c *gc.C) {
	ctx := envtesting.BootstrapContext(c)
	params := environs.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
	}
	_, err := s.Env.Bootstrap(ctx, s.callCtx, params)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{{
		FuncName: "Bootstrap",
		Args: []interface{}{
			ctx,
			s.callCtx,
			params,
		},
	}})
}

func (s *environSuite) TestDestroy(c *gc.C) {
	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "not-ours",
			StorageVolumePut: api.StorageVolumePut{
				Config: map[string]string{
					"user.juju-model-uuid": "other",
				},
			},
		}, {
			Name: "ours",
			StorageVolumePut: api.StorageVolumePut{
				Config: map[string]string{
					"user.juju-model-uuid": s.Config.UUID(),
				},
			},
		}},
	}

	err := s.Env.Destroy(s.callCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"Destroy", []interface{}{s.callCtx}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"VolumeDelete", []interface{}{"juju", "ours"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
	})
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "not-ours",
			StorageVolumePut: api.StorageVolumePut{
				Config: map[string]string{
					"user.juju-controller-uuid": "other",
				},
			},
		}, {
			Name: "ours",
			StorageVolumePut: api.StorageVolumePut{
				Config: map[string]string{
					"user.juju-controller-uuid": s.Config.UUID(),
				},
			},
		}},
	}

	// machine0 is in the controller model.
	machine0 := s.NewContainer(c, "juju-controller-machine-0")
	machine0.Config["user.juju-model-uuid"] = s.Config.UUID()
	machine0.Config["user.juju-controller-uuid"] = s.Config.UUID()

	// machine1 is not in the controller model, but managed
	// by the same controller.
	machine1 := s.NewContainer(c, "juju-hosted-machine-1")
	machine1.Config["user.juju-model-uuid"] = "not-" + s.Config.UUID()
	machine1.Config["user.juju-controller-uuid"] = s.Config.UUID()

	// machine2 is not managed by the same controller.
	machine2 := s.NewContainer(c, "juju-controller-machine-2")
	machine2.Config["user.juju-model-uuid"] = "not-" + s.Config.UUID()
	machine2.Config["user.juju-controller-uuid"] = "not-" + s.Config.UUID()

	s.Client.Containers = append(s.Client.Containers, *machine0, *machine1, *machine2)

	err := s.Env.DestroyController(s.callCtx, s.Config.UUID())
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"Destroy", []interface{}{s.callCtx}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
		{"AliveContainers", []interface{}{"juju-"}},
		{"RemoveContainers", []interface{}{[]string{machine1.Name}}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"VolumeDelete", []interface{}{"juju", "ours"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
	})
}

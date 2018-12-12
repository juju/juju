// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/lxd"
	coretesting "github.com/juju/juju/testing"
)

var errTestUnAuth = errors.New("not authorized")

type environSuite struct {
	lxd.BaseSuite

	callCtx           context.ProviderCallContext
	invalidCredential bool
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.callCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidCredential = true
			return nil
		},
	}
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.invalidCredential = false
	s.BaseSuite.TearDownTest(c)
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
		CloudBootstrapFinalizer: func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
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
	c.Check(result.CloudBootstrapFinalizer, gc.NotNil)

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
		{"DeleteStoragePoolVolume", []interface{}{"juju", "custom", "ours"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
	})
}

func (s *environSuite) TestDestroyInvalidCredentials(c *gc.C) {
	c.Assert(s.invalidCredential, jc.IsFalse)
	s.Client.Stub.SetErrors(errTestUnAuth)
	err := s.Env.Destroy(s.callCtx)
	c.Assert(err, gc.ErrorMatches, "not authorized")
	c.Assert(s.invalidCredential, jc.IsTrue)
}

func (s *environSuite) TestDestroyInvalidCredentialsDestroyingFileSystems(c *gc.C) {
	c.Assert(s.invalidCredential, jc.IsFalse)
	// DeleteStoragePoolVolume will error w/ un-auth.
	s.Client.Stub.SetErrors(nil, nil, nil, errTestUnAuth)

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "ours",
			StorageVolumePut: api.StorageVolumePut{
				Config: map[string]string{
					"user.juju-model-uuid": s.Config.UUID(),
				},
			},
		}},
	}
	err := s.Env.Destroy(s.callCtx)
	c.Assert(err, gc.ErrorMatches, ".* not authorized")
	c.Assert(s.invalidCredential, jc.IsTrue)
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"Destroy", []interface{}{s.callCtx}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"DeleteStoragePoolVolume", []interface{}{"juju", "custom", "ours"}},
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
		{"DeleteStoragePoolVolume", []interface{}{"juju", "custom", "ours"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
	})
}

func (s *environSuite) TestDestroyControllerInvalidCredentialsHostedModels(c *gc.C) {
	c.Assert(s.invalidCredential, jc.IsFalse)
	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
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

	s.Client.Containers = append(s.Client.Containers, *machine0)

	// RemoveContainers will error not-auth.
	s.Client.Stub.SetErrors(nil, nil, nil, nil, nil, errTestUnAuth)

	err := s.Env.DestroyController(s.callCtx, s.Config.UUID())
	c.Assert(err, gc.ErrorMatches, "not authorized")
	c.Assert(s.invalidCredential, jc.IsTrue)
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"Destroy", []interface{}{s.callCtx}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
		{"AliveContainers", []interface{}{"juju-"}},
		{"RemoveContainers", []interface{}{[]string{}}},
	})
	s.Stub.CheckCallNames(c,
		"Destroy",
		"StorageSupported",
		"GetStoragePools",
		"GetStoragePoolVolumes",
		"GetStoragePoolVolumes",
		"AliveContainers",
		"RemoveContainers")
}

func (s *environSuite) TestDestroyControllerInvalidCredentialsDestroyFilesystem(c *gc.C) {
	c.Assert(s.invalidCredential, jc.IsFalse)
	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
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

	s.Client.Containers = append(s.Client.Containers, *machine0)

	// RemoveContainers will error not-auth.
	s.Client.Stub.SetErrors(nil, nil, nil, nil, nil, nil, nil, nil, errTestUnAuth)

	err := s.Env.DestroyController(s.callCtx, s.Config.UUID())
	c.Assert(err, gc.ErrorMatches, ".*not authorized")
	c.Assert(s.invalidCredential, jc.IsTrue)
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"Destroy", []interface{}{s.callCtx}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"GetStoragePoolVolumes", []interface{}{"juju-zfs"}},
		{"AliveContainers", []interface{}{"juju-"}},
		{"RemoveContainers", []interface{}{[]string{}}},
		{"StorageSupported", nil},
		{"GetStoragePools", nil},
		{"GetStoragePoolVolumes", []interface{}{"juju"}},
		{"DeleteStoragePoolVolume", []interface{}{"juju", "custom", "ours"}},
	})
}

func (s *environSuite) TestAvailabilityZonesInvalidCredentials(c *gc.C) {
	c.Assert(s.invalidCredential, jc.IsFalse)
	// GetClusterMembers will return un-auth error
	s.Client.Stub.SetErrors(errTestUnAuth)
	_, err := s.Env.AvailabilityZones(s.callCtx)
	c.Assert(err, gc.ErrorMatches, ".*not authorized")
	c.Assert(s.invalidCredential, jc.IsTrue)
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"IsClustered", nil},
		{"GetClusterMembers", nil},
	})
}

func (s *environSuite) TestInstanceAvailabilityZoneNamesInvalidCredentials(c *gc.C) {
	c.Assert(s.invalidCredential, jc.IsFalse)
	// AliveContainers will return un-auth error
	s.Client.Stub.SetErrors(errTestUnAuth)

	// the call to Instances takes care of updating invalid credential details
	_, err := s.Env.InstanceAvailabilityZoneNames(s.callCtx, []instance.Id{"not-valid"})
	c.Assert(err, gc.ErrorMatches, ".*not authorized")
	c.Assert(s.invalidCredential, jc.IsTrue)
	s.Stub.CheckCalls(c, []gitjujutesting.StubCall{
		{"AliveContainers", []interface{}{s.Prefix()}},
	})
}

type environProfileSuite struct {
	lxd.EnvironSuite

	callCtx context.ProviderCallContext
}

var _ = gc.Suite(&environProfileSuite{})

func (s *environProfileSuite) TestMaybeWriteLXDProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HasProfile("testname").Return(true, nil),
		exp.HasProfile("testname").Return(false, nil),
		exp.CreateProfile(api.ProfilesPost{
			Name: "testname",
			ProfilePut: api.ProfilePut{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "test profile",
			},
		}).Return(nil),
	)

	env := s.NewEnviron(c, svr, nil)
	lxdEnv, ok := env.(environs.LXDProfiler)
	c.Assert(ok, jc.IsTrue)
	err := lxdEnv.MaybeWriteLXDProfile("testname", nil)
	c.Assert(err, jc.ErrorIsNil)
	err = lxdEnv.MaybeWriteLXDProfile("testname", &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Description: "test profile",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProfileSuite) TestLXDProfileNames(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	svr := lxd.NewMockServer(ctrl)
	exp := svr.EXPECT()

	exp.GetContainerProfiles("testname").Return([]string{
		lxdprofile.Name("foo", "bar", 1),
	}, nil)

	env := s.NewEnviron(c, svr, nil)
	lxdEnv, ok := env.(environs.LXDProfiler)
	c.Assert(ok, jc.IsTrue)
	result, err := lxdEnv.LXDProfileNames("testname")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []string{
		lxdprofile.Name("foo", "bar", 1),
	})
}

func (s *environProfileSuite) TestReplaceOrAddInstanceProfile(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	instId := "testme"
	old := "old-profile"
	new := "new-profile"

	svr := lxd.NewMockServer(ctrl)
	exp := svr.EXPECT()
	gomock.InOrder(
		exp.HasProfile(new).Return(false, nil),
		exp.CreateProfile(api.ProfilesPost{
			Name: new,
			ProfilePut: api.ProfilePut{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "test profile",
			},
		}).Return(nil),
		exp.ReplaceOrAddContainerProfile(instId, old, new).Return(nil),
		exp.DeleteProfile(old),
		exp.GetContainerProfiles(instId).Return([]string{"default", "juju-default", new}, nil),
	)

	env := s.NewEnviron(c, svr, nil)
	lxdEnv, ok := env.(environs.LXDProfiler)
	c.Assert(ok, jc.IsTrue)
	put := &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Description: "test profile",
	}
	obtained, err := lxdEnv.ReplaceOrAddInstanceProfile(instId, old, new, put)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, []string{"default", "juju-default", new})
}

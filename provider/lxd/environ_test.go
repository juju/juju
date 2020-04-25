// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v7"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/lxc/lxd/shared/api"
	gc "gopkg.in/check.v1"

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
		ControllerConfig:         coretesting.FakeControllerConfig(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
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
		ControllerConfig:         coretesting.FakeControllerConfig(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
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

type environCloudProfileSuite struct {
	lxd.EnvironSuite

	callCtx context.ProviderCallContext

	svr          *lxd.MockServer
	cloudSpecEnv environs.CloudSpecSetter
}

var _ = gc.Suite(&environCloudProfileSuite{})

func (s *environCloudProfileSuite) TestSetCloudSpecCreateProfile(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectHasProfileFalse("juju-controller")
	s.expectCreateProfile("juju-controller", nil)

	err := s.cloudSpecEnv.SetCloudSpec(lxdCloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environCloudProfileSuite) TestSetCloudSpecCreateProfileErrorSucceeds(c *gc.C) {
	defer s.setup(c).Finish()
	s.expectForProfileCreateRace("juju-controller")
	s.expectCreateProfile("juju-controller", errors.New("The profile already exists"))

	err := s.cloudSpecEnv.SetCloudSpec(lxdCloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environCloudProfileSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.svr = lxd.NewMockServer(ctrl)

	svrFactory := lxd.NewMockServerFactory(ctrl)
	svrFactory.EXPECT().RemoteServer(lxdCloudSpec()).Return(s.svr, nil)

	env, ok := s.NewEnvironWithServerFactory(c, svrFactory, nil).(environs.CloudSpecSetter)
	c.Assert(ok, jc.IsTrue)
	s.cloudSpecEnv = env

	return ctrl
}

func (s *environCloudProfileSuite) expectForProfileCreateRace(name string) {
	exp := s.svr.EXPECT()
	gomock.InOrder(
		exp.HasProfile(name).Return(false, nil),
		exp.HasProfile(name).Return(true, nil),
	)
}

func (s *environCloudProfileSuite) expectHasProfileFalse(name string) {
	s.svr.EXPECT().HasProfile(name).Return(false, nil)
}

func (s *environCloudProfileSuite) expectCreateProfile(name string, err error) {
	s.svr.EXPECT().CreateProfileWithConfig(name,
		map[string]string{
			"boot.autostart":   "true",
			"security.nesting": "true",
		}).Return(err)
}

type environProfileSuite struct {
	lxd.EnvironSuite

	callCtx context.ProviderCallContext

	svr    *lxd.MockServer
	lxdEnv environs.LXDProfiler
}

var _ = gc.Suite(&environProfileSuite{})

func (s *environProfileSuite) TestMaybeWriteLXDProfileYes(c *gc.C) {
	defer s.setup(c).Finish()

	profile := "testname"
	s.expectMaybeWriteLXDProfile(false, profile)

	err := s.lxdEnv.MaybeWriteLXDProfile(profile, &charm.LXDProfile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Description: "test profile",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProfileSuite) TestMaybeWriteLXDProfileNo(c *gc.C) {
	defer s.setup(c).Finish()

	profile := "testname"
	s.expectMaybeWriteLXDProfile(true, profile)

	err := s.lxdEnv.MaybeWriteLXDProfile(profile, nil)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProfileSuite) TestLXDProfileNames(c *gc.C) {
	defer s.setup(c).Finish()

	exp := s.svr.EXPECT()
	exp.GetContainerProfiles("testname").Return([]string{
		lxdprofile.Name("foo", "bar", 1),
	}, nil)

	result, err := s.lxdEnv.LXDProfileNames("testname")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []string{
		lxdprofile.Name("foo", "bar", 1),
	})
}

func (s *environProfileSuite) TestAssignLXDProfiles(c *gc.C) {
	defer s.setup(c).Finish()

	instId := "testme"
	oldP := "old-profile"
	newP := "new-profile"
	expectedProfiles := []string{"default", "juju-default", newP}
	s.expectAssignLXDProfiles(instId, oldP, newP, []string{}, expectedProfiles, nil)

	obtained, err := s.lxdEnv.AssignLXDProfiles(instId, expectedProfiles, []lxdprofile.ProfilePost{
		{
			Name:    oldP,
			Profile: nil,
		}, {
			Name: newP,
			Profile: &lxdprofile.Profile{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "test profile",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, expectedProfiles)
}

func (s *environProfileSuite) TestAssignLXDProfilesErrorReturnsCurrent(c *gc.C) {
	defer s.setup(c).Finish()

	instId := "testme"
	oldP := "old-profile"
	newP := "new-profile"
	expectedProfiles := []string{"default", "juju-default", oldP}
	newProfiles := []string{"default", "juju-default", newP}
	expectedErr := "fail UpdateContainerProfiles"
	s.expectAssignLXDProfiles(instId, oldP, newP, expectedProfiles, newProfiles, errors.New(expectedErr))

	obtained, err := s.lxdEnv.AssignLXDProfiles(instId, newProfiles, []lxdprofile.ProfilePost{
		{
			Name:    oldP,
			Profile: nil,
		}, {
			Name: newP,
			Profile: &lxdprofile.Profile{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "test profile",
			},
		},
	})
	c.Assert(err, gc.ErrorMatches, expectedErr)
	c.Assert(obtained, gc.DeepEquals, []string{"default", "juju-default", oldP})
}

func (s *environProfileSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.svr = lxd.NewMockServer(ctrl)
	lxdEnv, ok := s.NewEnviron(c, s.svr, nil).(environs.LXDProfiler)
	c.Assert(ok, jc.IsTrue)
	s.lxdEnv = lxdEnv

	return ctrl
}

func (s *environProfileSuite) expectMaybeWriteLXDProfile(hasProfile bool, name string) {
	exp := s.svr.EXPECT()
	exp.HasProfile(name).Return(hasProfile, nil)
	if !hasProfile {
		exp.CreateProfile(api.ProfilesPost{
			Name: name,
			ProfilePut: api.ProfilePut{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "test profile",
			},
		}).Return(nil)
	}
}

func (s *environProfileSuite) expectAssignLXDProfiles(instId, old, new string, oldProfiles, newProfiles []string, updateErr error) {
	s.expectMaybeWriteLXDProfile(false, new)
	exp := s.svr.EXPECT()
	exp.UpdateContainerProfiles(instId, newProfiles).Return(updateErr)
	if updateErr != nil {
		exp.GetContainerProfiles(instId).Return(oldProfiles, nil)
		return
	}
	if old != "" {
		exp.DeleteProfile(old)
	}
	exp.GetContainerProfiles(instId).Return(newProfiles, nil)
}

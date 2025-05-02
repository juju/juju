// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"context"

	"github.com/canonical/lxd/shared/api"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	environscmd "github.com/juju/juju/environs/cmd"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/provider/lxd"
	coretesting "github.com/juju/juju/internal/testing"
)

var errTestUnAuth = errors.New("not authorized")

type environSuite struct {
	lxd.BaseSuite
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) TestName(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	c.Check(s.Env.Name(), gc.Equals, "lxd")
}

func (s *environSuite) TestProvider(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	c.Assert(s.Env.Provider(), gc.Equals, s.Provider)
}

func (s *environSuite) TestSetConfigOkay(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	err := s.Env.SetConfig(context.Background(), s.Config)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(lxd.ExposeEnvConfig(s.Env), jc.DeepEquals, s.EnvConfig)
	// Ensure the client did not change.
	c.Check(lxd.ExposeEnvServer(s.Env), gc.Equals, s.Client)
}

func (s *environSuite) TestSetConfigNoAPI(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	err := s.Env.SetConfig(context.Background(), s.Config)

	c.Assert(err, jc.ErrorIsNil)
}

func (s *environSuite) TestConfig(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	cfg := s.Env.Config()

	c.Check(cfg, jc.DeepEquals, s.Config)
}

func (s *environSuite) TestBootstrapOkay(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Common.BootstrapResult = &environs.BootstrapResult{
		Arch: "amd64",
		Base: corebase.MakeDefaultBase("ubuntu", "22.04"),
		CloudBootstrapFinalizer: func(environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts) error {
			return nil
		},
	}

	ctx := cmdtesting.Context(c)
	params := environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	}
	result, err := s.Env.Bootstrap(environscmd.BootstrapContext(context.Background(), ctx), params)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(result.Arch, gc.Equals, "amd64")
	c.Check(result.Base.DisplayString(), gc.Equals, "ubuntu@22.04")
	// We don't check bsFinalizer because functions cannot be compared.
	c.Check(result.CloudBootstrapFinalizer, gc.NotNil)

	out := cmdtesting.Stderr(ctx)
	c.Assert(out, gc.Matches, "To configure your system to better support LXD containers, please see: .*\n")
}

func (s *environSuite) TestBootstrapAPI(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	ctx := envtesting.BootstrapContext(context.Background(), c)
	params := environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	}
	_, err := s.Env.Bootstrap(ctx, params)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []jujutesting.StubCall{{
		FuncName: "Bootstrap",
		Args: []interface{}{
			ctx,
			params,
		},
	}})
}

func (s *environSuite) TestDestroy(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "not-ours",
			Config: map[string]string{
				"user.juju-model-uuid": "other",
			},
		}, {
			Name: "ours",
			Config: map[string]string{
				"user.juju-model-uuid": s.Config.UUID(),
			},
		}},
	}

	callCtx := context.Background()
	err := s.Env.Destroy(callCtx)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "Destroy", Args: []interface{}{callCtx}},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "DeleteStoragePoolVolume", Args: []interface{}{"juju", "custom", "ours"}},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju-zfs"}},
	})
}

func (s *environSuite) TestDestroyInvalidCredentials(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.Client.Stub.SetErrors(errTestUnAuth)
	err := s.Env.Destroy(context.Background())
	c.Assert(err, gc.ErrorMatches, "not authorized")
}

func (s *environSuite) TestDestroyInvalidCredentialsDestroyingFileSystems(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	// DeleteStoragePoolVolume will error w/ un-auth.
	s.Client.Stub.SetErrors(nil, nil, nil, errTestUnAuth)

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "ours",
			Config: map[string]string{
				"user.juju-model-uuid": s.Config.UUID(),
			},
		}},
	}
	err := s.Env.Destroy(context.Background())
	c.Assert(err, gc.ErrorMatches, ".* not authorized")
	// Nil the call context as if fails DeepEquals.
	calls := s.Stub.Calls()
	c.Assert(calls, gc.Not(gc.HasLen), 0)
	calls[0].Args = nil
	c.Assert(calls, jc.DeepEquals, []jujutesting.StubCall{
		{FuncName: "Destroy", Args: nil},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "DeleteStoragePoolVolume", Args: []interface{}{"juju", "custom", "ours"}},
	})
}

func (s *environSuite) TestDestroyController(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "not-ours",
			Config: map[string]string{
				"user.juju-controller-uuid": "other",
			},
		}, {
			Name: "ours",
			Config: map[string]string{
				"user.juju-controller-uuid": s.Config.UUID(),
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

	callCtx := context.Background()
	err := s.Env.DestroyController(callCtx, s.Config.UUID())
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "Destroy", Args: []interface{}{callCtx}},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju-zfs"}},
		{FuncName: "AliveContainers", Args: []interface{}{"juju-"}},
		{FuncName: "RemoveContainers", Args: []interface{}{[]string{machine1.Name}}},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "DeleteStoragePoolVolume", Args: []interface{}{"juju", "custom", "ours"}},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju-zfs"}},
	})
}

func (s *environSuite) TestDestroyControllerInvalidCredentialsHostedModels(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "ours",
			Config: map[string]string{
				"user.juju-controller-uuid": s.Config.UUID(),
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

	err := s.Env.DestroyController(context.Background(), s.Config.UUID())
	c.Assert(err, gc.ErrorMatches, "not authorized")

	// Nil the call context as if fails DeepEquals.
	calls := s.Stub.Calls()
	c.Assert(calls, gc.Not(gc.HasLen), 0)
	calls[0].Args = nil
	c.Assert(calls, jc.DeepEquals, []jujutesting.StubCall{
		{FuncName: "Destroy", Args: nil},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju-zfs"}},
		{FuncName: "AliveContainers", Args: []interface{}{"juju-"}},
		{FuncName: "RemoveContainers", Args: []interface{}{[]string{}}},
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
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	s.UpdateConfig(c, map[string]interface{}{
		"controller-uuid": s.Config.UUID(),
	})
	s.Stub.ResetCalls()

	s.Client.Volumes = map[string][]api.StorageVolume{
		"juju": {{
			Name: "ours",
			Config: map[string]string{
				"user.juju-controller-uuid": s.Config.UUID(),
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

	err := s.Env.DestroyController(context.Background(), s.Config.UUID())
	c.Assert(err, gc.ErrorMatches, ".*not authorized")

	// Nil the call context as if fails DeepEquals.
	calls := s.Stub.Calls()
	c.Assert(calls, gc.Not(gc.HasLen), 0)
	calls[0].Args = nil
	c.Assert(calls, jc.DeepEquals, []jujutesting.StubCall{
		{FuncName: "Destroy", Args: nil},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju-zfs"}},
		{FuncName: "AliveContainers", Args: []interface{}{"juju-"}},
		{FuncName: "RemoveContainers", Args: []interface{}{[]string{}}},
		{FuncName: "StorageSupported", Args: nil},
		{FuncName: "GetStoragePools", Args: nil},
		{FuncName: "GetStoragePoolVolumes", Args: []interface{}{"juju"}},
		{FuncName: "DeleteStoragePoolVolume", Args: []interface{}{"juju", "custom", "ours"}},
	})
}

func (s *environSuite) TestAvailabilityZonesInvalidCredentials(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	// GetClusterMembers will return un-auth error
	s.Client.Stub.SetErrors(errTestUnAuth)
	_, err := s.Env.AvailabilityZones(context.Background())
	c.Assert(err, gc.ErrorMatches, ".*not authorized")

	s.Stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "IsClustered", Args: nil},
		{FuncName: "GetClusterMembers", Args: nil},
	})
}

func (s *environSuite) TestInstanceAvailabilityZoneNamesInvalidCredentials(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	s.Invalidator.EXPECT().InvalidateCredentials(gomock.Any(), gomock.Any()).Return(nil)

	// AliveContainers will return un-auth error
	s.Client.Stub.SetErrors(errTestUnAuth)

	// the call to Instances takes care of updating invalid credential details
	_, err := s.Env.InstanceAvailabilityZoneNames(context.Background(), []instance.Id{"not-valid"})
	c.Assert(err, gc.ErrorMatches, ".*not authorized")

	s.Stub.CheckCalls(c, []jujutesting.StubCall{
		{FuncName: "AliveContainers", Args: []interface{}{s.Prefix()}},
	})
}

type environCloudProfileSuite struct {
	lxd.EnvironSuite

	svr          *lxd.MockServer
	cloudSpecEnv environs.CloudSpecSetter
}

var _ = gc.Suite(&environCloudProfileSuite{})

func (s *environCloudProfileSuite) TestSetCloudSpecCreateProfile(c *gc.C) {
	defer s.setup(c, nil).Finish()
	s.expectHasProfileFalse("juju-controller")
	s.expectCreateProfile("juju-controller", nil)

	err := s.cloudSpecEnv.SetCloudSpec(context.Background(), lxdCloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environCloudProfileSuite) TestSetCloudSpecCreateProfileErrorSucceeds(c *gc.C) {
	defer s.setup(c, nil).Finish()
	s.expectForProfileCreateRace("juju-controller")
	s.expectCreateProfile("juju-controller", errors.New("The profile already exists"))

	err := s.cloudSpecEnv.SetCloudSpec(context.Background(), lxdCloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environCloudProfileSuite) TestSetCloudSpecUsesConfiguredProject(c *gc.C) {
	defer s.setup(c, map[string]interface{}{"project": "my-project"}).Finish()
	s.expectHasProfileFalse("juju-controller")
	s.expectCreateProfile("juju-controller", nil)

	err := s.cloudSpecEnv.SetCloudSpec(context.Background(), lxdCloudSpec())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environCloudProfileSuite) setup(c *gc.C, cfgEdit map[string]interface{}) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.svr = lxd.NewMockServer(ctrl)

	project, _ := cfgEdit["project"].(string)
	cloudSpec := lxd.CloudSpec{
		CloudSpec: lxdCloudSpec(),
		Project:   project,
	}

	svrFactory := lxd.NewMockServerFactory(ctrl)
	svrFactory.EXPECT().RemoteServer(cloudSpec).Return(s.svr, nil)

	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	env, ok := s.NewEnvironWithServerFactory(c, svrFactory, cfgEdit, invalidator).(environs.CloudSpecSetter)
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

	svr    *lxd.MockServer
	lxdEnv environs.LXDProfiler
}

var _ = gc.Suite(&environProfileSuite{})

func (s *environProfileSuite) TestMaybeWriteLXDProfileYes(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{}).Finish()

	profile := "testname"
	s.expectMaybeWriteLXDProfile(false, profile)

	err := s.lxdEnv.MaybeWriteLXDProfile(profile, lxdprofile.Profile{
		Config: map[string]string{
			"security.nesting": "true",
		},
		Description: "test profile",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProfileSuite) TestMaybeWriteLXDProfileNo(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{}).Finish()

	profile := "testname"
	s.expectMaybeWriteLXDProfile(true, profile)

	err := s.lxdEnv.MaybeWriteLXDProfile(profile, lxdprofile.Profile{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *environProfileSuite) TestLXDProfileNames(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{}).Finish()

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
	defer s.setup(c, environscloudspec.CloudSpec{}).Finish()

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
	defer s.setup(c, environscloudspec.CloudSpec{}).Finish()

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

func (s *environProfileSuite) TestDetectCorrectHardwareEndpointIPOnly(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "1.1.1.1",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	c.Assert(err, gc.IsNil)
	// 1.1.1.1 is not a local IP address, so we don't set ARCH in hc
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareEndpointIPPort(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "1.1.1.1:8888",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	c.Assert(err, gc.IsNil)
	// 1.1.1.1 is not a local IP address, so we don't set ARCH in hc
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareEndpointSchemeIPPort(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "http://1.1.1.1:8888",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	c.Assert(err, gc.IsNil)
	// 1.1.1.1 is not a local IP address, so we don't set ARCH in hc
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareEndpointHostOnly(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "localhost",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	c.Assert(err, gc.IsNil)
	// 1.1.1.1 is not a local IP address, so we don't set ARCH in hc
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareEndpointHostPort(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "localhost:8888",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	c.Assert(err, gc.IsNil)
	// localhost is not considered as a local IP address, so we don't set ARCH in hc
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareEndpointSchemeHostPort(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "http://localhost:8888",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	c.Assert(err, gc.IsNil)
	// localhost is not considered as a local IP address, so we don't set ARCH in hc
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareWrongEndpoint(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "1.1:8888",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	// the endpoint is wrongly formatted but we don't return an error, that
	// would mean we are stopping the bootstrap
	c.Assert(err, gc.IsNil)
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) TestDetectCorrectHardwareEmptyEndpoint(c *gc.C) {
	defer s.setup(c, environscloudspec.CloudSpec{
		Endpoint: "",
	}).Finish()

	detector, supported := s.lxdEnv.(environs.HardwareCharacteristicsDetector)
	c.Assert(supported, jc.IsTrue)

	hc, err := detector.DetectHardware()
	// the endpoint is wrongly formatted but we don't return an error, that
	// would mean we are stopping the bootstrap
	c.Assert(err, gc.IsNil)
	c.Assert(hc, gc.IsNil)
}

func (s *environProfileSuite) setup(c *gc.C, cloudSpec environscloudspec.CloudSpec) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.svr = lxd.NewMockServer(ctrl)
	invalidator := lxd.NewMockCredentialInvalidator(ctrl)

	lxdEnv, ok := s.NewEnviron(c, s.svr, nil, cloudSpec, invalidator).(environs.LXDProfiler)
	c.Assert(ok, jc.IsTrue)
	s.lxdEnv = lxdEnv

	return ctrl
}

func (s *environProfileSuite) expectMaybeWriteLXDProfile(hasProfile bool, name string) {
	exp := s.svr.EXPECT()
	exp.HasProfile(name).Return(hasProfile, nil)
	if !hasProfile {
		post := api.ProfilesPost{
			Name: name,
			ProfilePut: api.ProfilePut{
				Config: map[string]string{
					"security.nesting": "true",
				},
				Description: "test profile",
			},
		}
		exp.CreateProfile(post).Return(nil)
		expProfile := api.Profile{
			Name:        post.Name,
			Description: post.Description,
			Config:      post.Config,
			Devices:     post.Devices,
		}
		exp.GetProfile(name).Return(&expProfile, "etag", nil)
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

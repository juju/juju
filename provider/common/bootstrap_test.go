// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"crypto"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/os/v2/series"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3/ssh"
	"github.com/juju/version/v2"
	cryptossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	envcontext "github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/provider/common"
	corestorage "github.com/juju/juju/storage"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

type BootstrapSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture

	callCtx envcontext.ProviderCallContext
}

var _ = gc.Suite(&BootstrapSuite{})

type cleaner interface {
	AddCleanup(func(*gc.C))
}

func (s *BootstrapSuite) SetUpTest(c *gc.C) {
	coretesting.SkipUnlessControllerOS(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string, opts *ssh.Options) error {
		return fmt.Errorf("mock connection failure to %s", host)
	})

	s.callCtx = envcontext.NewEmptyCloudCallContext()
}

func (s *BootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func newStorage(suite cleaner, c *gc.C) storage.Storage {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*gc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor, "released", "released")
	return stor
}

func minimalConfig(c *gc.C) *config.Config {
	return minimalConfigWithBase(c, jujuversion.DefaultSupportedLTSBase())
}

func minimalConfigWithBase(c *gc.C, base corebase.Base) *config.Config {
	series, _ := corebase.GetSeriesFromBase(base)
	attrs := map[string]interface{}{
		"name":               "whatever",
		"type":               "anything, really",
		"uuid":               coretesting.ModelTag.Id(),
		"controller-uuid":    coretesting.ControllerTag.Id(),
		"ca-cert":            coretesting.CACert,
		"ca-private-key":     coretesting.CAKey,
		"authorized-keys":    coretesting.FakeAuthKeys,
		"default-series":     series,
		"default-base":       base.String(),
		"cloudinit-userdata": validCloudInitUserData,
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, jc.ErrorIsNil)
	return cfg
}

func configGetter(c *gc.C) configFunc {
	cfg := minimalConfig(c)
	return func() *config.Config { return cfg }
}

func (s *BootstrapSuite) TestCannotStartInstance(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	checkPlacement := "directive"
	checkCons := constraints.MustParse("mem=8G")
	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}

	startInstance := func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		c.Assert(args.Placement, gc.DeepEquals, checkPlacement)
		c.Assert(args.Constraints, gc.DeepEquals, checkCons)

		// The machine config should set its upgrade behavior based on
		// the environment config.
		expectedMcfg, err := instancecfg.NewBootstrapInstanceConfig(
			coretesting.FakeControllerConfig(),
			args.Constraints,
			args.Constraints,
			args.InstanceConfig.Base,
			"",
			nil,
		)
		c.Assert(err, jc.ErrorIsNil)

		expectedMcfg.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
		expectedMcfg.EnableOSUpgrade = env.Config().EnableOSUpgrade()
		expectedMcfg.Tags = map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
			"juju-is-controller":   "true",
		}
		expectedMcfg.NetBondReconfigureDelay = env.Config().NetBondReconfigureDelay()
		args.InstanceConfig.Bootstrap.InitialSSHHostKeys = nil
		c.Assert(args.InstanceConfig, jc.DeepEquals, expectedMcfg)
		return nil, nil, nil, errors.Errorf("meh, not started")
	}

	env.startInstance = startInstance

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		BootstrapConstraints:     checkCons,
		ModelConstraints:         checkCons,
		Placement:                checkPlacement,
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
}

func (s *BootstrapSuite) TestBootstrapInstanceCancelled(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}

	startInstance := func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		return nil, nil, nil, errors.Errorf("some kind of error")
	}
	env.startInstance = startInstance

	stdCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := modelcmd.BootstrapContext(stdCtx, cmdtesting.Context(c))
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches, `starting controller \(cancelled\): some kind of error`)
}

func (s *BootstrapSuite) TestBootstrapSeries(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTODOContext(c)
	bootstrapSeries := jujuversion.DefaultSupportedLTS()
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		BootstrapSeries:          bootstrapSeries,
		AvailableTools:           availableTools,
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), gc.Equals, jujuversion.DefaultSupportedLTSBase().String())
}

func (s *BootstrapSuite) TestBootstrapInvalidSeries(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTODOContext(c)
	bootstrapSeries := "spock"
	availableTools := fakeAvailableTools()
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		BootstrapSeries:          bootstrapSeries,
		AvailableTools:           availableTools,
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches, `series "spock" not valid`)
}

func (s *BootstrapSuite) TestBootstrapFallbackSeries(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTODOContext(c)
	bootstrapSeries := ""
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		BootstrapSeries:          bootstrapSeries,
		AvailableTools:           availableTools,
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), gc.Equals, jujuversion.DefaultSupportedLTSBase().String())
}

func (s *BootstrapSuite) TestBootstrapSeriesWithForce(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTODOContext(c)
	bootstrapSeries := "xenial"
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		BootstrapSeries:          bootstrapSeries,
		AvailableTools:           availableTools,
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		Force:                    true,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(result.Arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), gc.Equals, corebase.MakeDefaultBase("ubuntu", "16.04").String())
}

func (s *BootstrapSuite) TestStartInstanceDerivedZone(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
			return []string{"derived-zone"}, nil
		},
	}

	env.startInstance = func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		c.Assert(args.AvailabilityZone, gc.Equals, "derived-zone")
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches,
		`cannot start bootstrap instance in availability zone "derived-zone": bloop`,
	)
}

func (s *BootstrapSuite) TestStartInstanceAttemptAllZones(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", false}
			z2 := &mockAvailabilityZone{"z2", true}
			return network.AvailabilityZones{z0, z1, z2}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches,
		`cannot start bootstrap instance in any availability zone \(z0, z2\)`,
	)
	c.Assert(callZones, jc.SameContents, []string{"z0", "z2"})
}

func (s *BootstrapSuite) TestStartInstanceAttemptZoneConstrained(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", true}
			z2 := &mockAvailabilityZone{"z2", true}
			z3 := &mockAvailabilityZone{"z3", true}
			return network.AvailabilityZones{z0, z1, z2, z3}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		BootstrapConstraints: constraints.Value{
			Zones: &[]string{"z0", "z2"},
		},
	})
	c.Assert(err, gc.ErrorMatches,
		`cannot start bootstrap instance in any availability zone \(z0, z2\)`,
	)
	c.Assert(callZones, jc.SameContents, []string{"z0", "z2"})
}

func (s *BootstrapSuite) TestStartInstanceNoMatchingConstraintZones(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", true}
			z2 := &mockAvailabilityZone{"z2", true}
			z3 := &mockAvailabilityZone{"z3", true}
			return network.AvailabilityZones{z0, z1, z2, z3}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		BootstrapConstraints: constraints.Value{
			Zones: &[]string{"z4", "z5"},
		},
	})
	c.Assert(err, gc.ErrorMatches,
		`no available zones \(\["z0" "z1" "z2" "z3"\]\) matching bootstrap zone constraints \(\["z4" "z5"\]\)`,
	)
	c.Assert(callZones, gc.IsNil)
}

func (s *BootstrapSuite) TestStartInstanceStopOnZoneIndependentError(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", true}
			return network.AvailabilityZones{z0, z1}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, environs.ZoneIndependentError(errors.New("bloop"))
	}

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches, `cannot start bootstrap instance: bloop`)
	c.Assert(callZones, jc.SameContents, []string{"z0"})
}

func (s *BootstrapSuite) TestStartInstanceNoUsableZones(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(envcontext.ProviderCallContext, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx envcontext.ProviderCallContext) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", false}
			return network.AvailabilityZones{z0}, nil
		},
	}

	ctx := envtesting.BootstrapTODOContext(c)
	_, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, gc.ErrorMatches, `cannot start bootstrap instance: no usable availability zones`)
}

func (s *BootstrapSuite) TestStartInstanceRootDisk(c *gc.C) {
	startInstance := func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		c.Assert(args.RootDisk, jc.DeepEquals, &corestorage.VolumeParams{
			Provider: "dummy",
			Attributes: map[string]interface{}{
				"type": "dummy",
				"foo":  "bar",
			},
		})
		hw := instance.MustParseHardware("arch=ppc64el")
		return &mockInstance{}, &hw, nil, nil
	}
	env := &mockEnviron{
		startInstance: startInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTODOContext(c)
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           availableTools,
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
		BootstrapConstraints:     constraints.MustParse("root-disk-source=spool"),
		StoragePools: map[string]corestorage.Attrs{
			"spool": {
				"type": "dummy",
				"foo":  "bar",
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "ppc64el")
}

func (s *BootstrapSuite) TestSuccess(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	var innerInstanceConfig *instancecfg.InstanceConfig
	inst := &mockInstance{
		id:        checkInstanceId,
		addresses: network.NewMachineAddresses([]string{"testing.invalid"}).AsProviderAddresses(),
	}
	startInstance := func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		icfg := args.InstanceConfig
		innerInstanceConfig = icfg
		c.Assert(icfg.Bootstrap.InitialSSHHostKeys, gc.HasLen, 3)
		for _, key := range icfg.Bootstrap.InitialSSHHostKeys {
			privKey, err := cryptossh.ParseRawPrivateKey([]byte(key.Private))
			c.Assert(err, jc.ErrorIsNil)
			_, fits := privKey.(interface {
				Public() crypto.PublicKey
				Equal(crypto.PrivateKey) bool
			})
			c.Assert(fits, jc.IsTrue)
			pubKey, _, _, _, err := cryptossh.ParseAuthorizedKey([]byte(key.Public))
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(pubKey.Type(), gc.Equals, key.PublicKeyAlgorithm)
		}
		return inst, &checkHardware, nil, nil
	}
	var mocksConfig = minimalConfig(c)
	var getConfigCalled int
	getConfig := func() *config.Config {
		getConfigCalled++
		return mocksConfig
	}
	setConfig := func(c *config.Config) error {
		mocksConfig = c
		return nil
	}

	var instancesMu sync.Mutex
	env := &mockEnviron{
		storage:       stor,
		startInstance: startInstance,
		config:        getConfig,
		setConfig:     setConfig,
		instances: func(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
			instancesMu.Lock()
			defer instancesMu.Unlock()
			return []instances.Instance{inst}, nil
		},
	}
	inner := cmdtesting.Context(c)
	ctx := modelcmd.BootstrapContext(context.Background(), inner)
	result, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		AvailableTools:           fakeAvailableTools(),
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Arch, gc.Equals, "ppc64el") // based on hardware characteristics
	c.Assert(result.Base, gc.Equals, config.PreferredBase(mocksConfig))
	c.Assert(result.CloudBootstrapFinalizer, gc.NotNil)

	// Check that we make the SSH connection with desired options.
	var knownHosts string
	var hostKeyAlgos string
	re := regexp.MustCompile(
		"ssh '-o' 'StrictHostKeyChecking yes' " +
			"'-o' 'PasswordAuthentication no' " +
			"'-o' 'ServerAliveInterval 30' " +
			"'-o' 'UserKnownHostsFile (.*)' " +
			"'-o' 'HostKeyAlgorithms (.*)' " +
			"'ubuntu@testing.invalid' '/bin/bash'")
	testing.PatchExecutableAsEchoArgs(c, s, "ssh")
	testing.PatchExecutableAsEchoArgs(c, s, "scp")
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string, opts *ssh.Options) error {
		// Stop WaitSSH from continuing.
		client, err := ssh.NewOpenSSHClient()
		if err != nil {
			return err
		}
		cmd := client.Command("ubuntu@"+host, []string{"/bin/bash"}, opts)
		if err := cmd.Run(); err != nil {
			return err
		}
		sshArgs := testing.ReadEchoArgs(c, "ssh")
		submatch := re.FindStringSubmatch(sshArgs)
		if c.Check(submatch, gc.NotNil, gc.Commentf("%s", sshArgs)) {
			knownHostsFile := submatch[1]
			knownHostsFile = strings.Replace(knownHostsFile, `\"`, ``, -1)
			knownHostsBytes, err := os.ReadFile(knownHostsFile)
			if err != nil {
				return err
			}
			knownHosts = string(knownHostsBytes)
			hostKeyAlgos = submatch[2]
		}
		return nil
	})
	err = result.CloudBootstrapFinalizer(ctx, innerInstanceConfig, environs.BootstrapDialOpts{
		Timeout: coretesting.LongWait,
	})
	c.Assert(err, gc.ErrorMatches, "invalid machine configuration: .*") // icfg hasn't been finalized
	c.Assert(innerInstanceConfig.Bootstrap.InitialSSHHostKeys, gc.HasLen, 3)
	computedKnownHosts := ""
	computedHostKeyAlgos := []string{}
	for _, key := range innerInstanceConfig.Bootstrap.InitialSSHHostKeys {
		computedKnownHosts += "testing.invalid " + key.Public
		computedHostKeyAlgos = append(computedHostKeyAlgos, key.PublicKeyAlgorithm)
	}
	c.Assert(
		knownHosts,
		gc.Equals,
		computedKnownHosts,
	)
	c.Assert(strings.Split(hostKeyAlgos, ","), jc.SameContents, computedHostKeyAlgos)
}

func (s *BootstrapSuite) TestBootstrapFinalizeCloudInitUserData(c *gc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	s.PatchValue(&series.HostSeries, func() (string, error) { return "xenial", nil })
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	var innerInstanceConfig *instancecfg.InstanceConfig
	inst := &mockInstance{
		id:        "i-success",
		addresses: network.NewMachineAddresses([]string{"testing.invalid"}).AsProviderAddresses(),
	}
	startInstance := func(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		icfg := args.InstanceConfig
		innerInstanceConfig = icfg
		return inst, &checkHardware, nil, nil
	}

	var instancesMu sync.Mutex
	env := &mockEnviron{
		startInstance: startInstance,
		config:        configGetter(c),
		instances: func(ctx envcontext.ProviderCallContext, ids []instance.Id) ([]instances.Instance, error) {
			instancesMu.Lock()
			defer instancesMu.Unlock()
			return []instances.Instance{inst}, nil
		},
	}
	ctx := envtesting.BootstrapTODOContext(c)
	bootstrapSeries := jujuversion.DefaultSupportedLTS()
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, s.callCtx, environs.BootstrapParams{
		ControllerConfig:         coretesting.FakeControllerConfig(),
		BootstrapSeries:          bootstrapSeries,
		AvailableTools:           availableTools,
		SupportedBootstrapSeries: coretesting.FakeSupportedJujuSeries,
	})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result.CloudBootstrapFinalizer, gc.NotNil)
	err = result.CloudBootstrapFinalizer(ctx, innerInstanceConfig, environs.BootstrapDialOpts{
		Timeout: coretesting.ShortWait,
	})
	c.Assert(err, gc.ErrorMatches, "waited for 50ms without being able to connect.*")
	c.Assert(innerInstanceConfig.CloudInitUserData, gc.DeepEquals, map[string]interface{}{
		"packages":        []interface{}{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []interface{}{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []interface{}{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
		"package_upgrade": false})
}

var validCloudInitUserData = `
packages:
  - 'python-keystoneclient'
  - 'python-glanceclient'
preruncmd:
  - mkdir /tmp/preruncmd
  - mkdir /tmp/preruncmd2
postruncmd:
  - mkdir /tmp/postruncmd
  - mkdir /tmp/postruncmd2
package_upgrade: false
`[1:]

type neverRefreshes struct {
}

func (neverRefreshes) Refresh(ctx envcontext.ProviderCallContext) error {
	return nil
}

func (neverRefreshes) Status(ctx envcontext.ProviderCallContext) instance.Status {
	return instance.Status{}
}

type neverAddresses struct {
	neverRefreshes
}

func (neverAddresses) Addresses(ctx envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	return nil, nil
}

type failsProvisioning struct {
	neverAddresses
	message string
}

func (f failsProvisioning) Status(_ envcontext.ProviderCallContext) instance.Status {
	return instance.Status{
		Status:  status.ProvisioningError,
		Message: f.message,
	}
}

var testSSHTimeout = environs.BootstrapDialOpts{
	Timeout:        coretesting.ShortWait,
	RetryDelay:     1 * time.Millisecond,
	AddressesDelay: 1 * time.Millisecond,
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForAddresses(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		context.Background(), ctx.Stderr, ssh.DefaultClient, "/bin/true", neverAddresses{}, s.callCtx, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, `waited for `+testSSHTimeout.Timeout.String()+` without getting any addresses`)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *gc.C) {
	cmdCtx := cmdtesting.Context(c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := common.WaitSSH(
		ctx, cmdCtx.Stderr, ssh.DefaultClient, "/bin/true", neverAddresses{}, s.callCtx, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, "cancelled")
	c.Check(cmdtesting.Stderr(cmdCtx), gc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHNoticesProvisioningFailures(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		context.Background(), ctx.Stderr, ssh.DefaultClient, "/bin/true", failsProvisioning{}, s.callCtx, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, `instance provisioning failed`)
	_, err = common.WaitSSH(
		context.Background(), ctx.Stderr, ssh.DefaultClient, "/bin/true", failsProvisioning{message: "blargh"}, s.callCtx, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, `instance provisioning failed \(blargh\)`)
}

type brokenAddresses struct {
	neverRefreshes
}

func (brokenAddresses) Addresses(ctx envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	return nil, errors.Errorf("Addresses will never work")
}

func (s *BootstrapSuite) TestWaitSSHStopsOnBadError(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		context.Background(), ctx.Stderr, ssh.DefaultClient, "/bin/true", brokenAddresses{}, s.callCtx, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, "getting addresses: Addresses will never work")
	c.Check(cmdtesting.Stderr(ctx), gc.Equals, "Waiting for address\n")
}

type neverOpensPort struct {
	neverRefreshes
	addr string
}

func (n *neverOpensPort) Addresses(ctx envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	return network.NewMachineAddresses([]string{n.addr}).AsProviderAddresses(), nil
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForDial(c *gc.C) {
	ctx := cmdtesting.Context(c)
	// 0.x.y.z addresses are always invalid
	_, err := common.WaitSSH(
		context.Background(), ctx.Stderr, ssh.DefaultClient, "/bin/true", &neverOpensPort{addr: "0.1.2.3"}, s.callCtx, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.3`)
	c.Check(cmdtesting.Stderr(ctx), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type cancelOnDial struct {
	neverRefreshes
	name     string
	cancel   context.CancelFunc
	returned bool
}

func (c *cancelOnDial) Addresses(ctx envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	// kill the tomb the second time Addresses is called
	if !c.returned {
		c.returned = true
	} else {
		if c.cancel != nil {
			c.cancel()
			c.cancel = nil
		}
	}
	return network.NewMachineAddresses([]string{c.name}).AsProviderAddresses(), nil
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForDial(c *gc.C) {
	cmdCtx := cmdtesting.Context(c)
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	ctx, cancel := context.WithCancel(context.Background())
	_, err := common.WaitSSH(
		ctx, cmdCtx.Stderr, ssh.DefaultClient, "", &cancelOnDial{name: "0.1.2.3", cancel: cancel}, s.callCtx, timeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, gc.ErrorMatches, "cancelled")
	// Exact timing is imprecise but it should have tried a few times before being killed
	c.Check(cmdtesting.Stderr(cmdCtx), gc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type addressesChange struct {
	addrs [][]string
}

func (ac *addressesChange) Refresh(ctx envcontext.ProviderCallContext) error {
	if len(ac.addrs) > 1 {
		ac.addrs = ac.addrs[1:]
	}
	return nil
}

func (ac *addressesChange) Status(ctx envcontext.ProviderCallContext) instance.Status {
	return instance.Status{}
}

func (ac *addressesChange) Addresses(ctx envcontext.ProviderCallContext) (network.ProviderAddresses, error) {
	return network.NewMachineAddresses(ac.addrs[0]).AsProviderAddresses(), nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *gc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(context.Background(), ctx.Stderr, ssh.DefaultClient, "", &addressesChange{addrs: [][]string{
		nil,
		nil,
		{"0.1.2.3"},
		{"0.1.2.3"},
		nil,
		{"0.1.2.4"},
	}}, s.callCtx, testSSHTimeout, common.DefaultHostSSHOptions)
	// Not necessarily the last one in the list, due to scheduling.
	c.Check(err, gc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.[34]`)
	stderr := cmdtesting.Stderr(ctx)
	c.Check(stderr, gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.3:22\n)+(.|\n)*")
	c.Check(stderr, gc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.4:22\n)+(.|\n)*")
}

type FormatHardwareSuite struct{}

var _ = gc.Suite(&FormatHardwareSuite{})

func (s *FormatHardwareSuite) check(c *gc.C, hw *instance.HardwareCharacteristics, expected string) {
	c.Check(common.FormatHardware(hw), gc.Equals, expected)
}

func (s *FormatHardwareSuite) TestNil(c *gc.C) {
	s.check(c, nil, "")
}

func (s *FormatHardwareSuite) TestFieldsNil(c *gc.C) {
	s.check(c, &instance.HardwareCharacteristics{}, "")
}

func (s *FormatHardwareSuite) TestArch(c *gc.C) {
	arch := ""
	s.check(c, &instance.HardwareCharacteristics{Arch: &arch}, "")
	arch = "amd64"
	s.check(c, &instance.HardwareCharacteristics{Arch: &arch}, "arch=amd64")
}

func (s *FormatHardwareSuite) TestCores(c *gc.C) {
	var cores uint64
	s.check(c, &instance.HardwareCharacteristics{CpuCores: &cores}, "")
	cores = 24
	s.check(c, &instance.HardwareCharacteristics{CpuCores: &cores}, "cores=24")
}

func (s *FormatHardwareSuite) TestMem(c *gc.C) {
	var mem uint64
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "")
	mem = 800
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=800M")
	mem = 1024
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=1G")
	mem = 2712
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=2.6G")
}

func (s *FormatHardwareSuite) TestVirtType(c *gc.C) {
	var virtType string
	s.check(c, &instance.HardwareCharacteristics{VirtType: &virtType}, "")
	virtType = string(instance.DefaultInstanceType)
	s.check(c, &instance.HardwareCharacteristics{VirtType: &virtType}, "")
	virtType = "virtual-machine"
	s.check(c, &instance.HardwareCharacteristics{VirtType: &virtType}, "virt-type=virtual-machine")
}

func (s *FormatHardwareSuite) TestAll(c *gc.C) {
	var (
		arch            = "ppc64"
		cores    uint64 = 2
		mem      uint64 = 123
		virtType        = "virtual-machine"
	)
	hw := &instance.HardwareCharacteristics{
		Arch:     &arch,
		CpuCores: &cores,
		Mem:      &mem,
		VirtType: &virtType,
	}
	s.check(c, hw, "arch=ppc64 mem=123M cores=2 virt-type=virtual-machine")
}

func fakeAvailableTools() tools.List {
	return tools.List{
		&tools.Tools{
			Version: version.Binary{
				Number:  jujuversion.Current,
				Arch:    arch.HostArch(),
				Release: "ubuntu",
			},
		},
	}
}

func fakeStartInstance(ctx envcontext.ProviderCallContext, args environs.StartInstanceParams) (
	instances.Instance,
	*instance.HardwareCharacteristics,
	network.InterfaceInfos,
	error,
) {
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")
	return &mockInstance{id: checkInstanceId}, &checkHardware, nil, nil
}

func fakeMinimalConfig(c *gc.C) func() *config.Config {
	var mocksConfig = minimalConfig(c)
	return func() *config.Config {
		return mocksConfig
	}
}

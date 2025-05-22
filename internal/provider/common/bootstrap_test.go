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
	stdtesting "testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	cryptossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/provider/common"
	corestorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
)

type BootstrapSuite struct {
	coretesting.FakeJujuXDGDataHomeSuite
	envtesting.ToolsFixture
}

func TestBootstrapSuite(t *stdtesting.T) {
	tc.Run(t, &BootstrapSuite{})
}

type cleaner interface {
	AddCleanup(func(*tc.C))
}

func (s *BootstrapSuite) SetUpTest(c *tc.C) {
	coretesting.SkipUnlessControllerOS(c)
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, host, checkHostScript string, opts *ssh.Options) error {
		return fmt.Errorf("mock connection failure to %s", host)
	})
}

func (s *BootstrapSuite) TearDownTest(c *tc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.FakeJujuXDGDataHomeSuite.TearDownTest(c)
}

func newStorage(suite cleaner, c *tc.C) storage.Storage {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	suite.AddCleanup(func(*tc.C) { closer.Close() })
	envtesting.UploadFakeTools(c, stor, "released")
	return stor
}

func minimalConfig(c *tc.C) *config.Config {
	return minimalConfigWithBase(c, jujuversion.DefaultSupportedLTSBase())
}

func minimalConfigWithBase(c *tc.C, base corebase.Base) *config.Config {
	attrs := map[string]interface{}{
		"name":               "whatever",
		"type":               "anything, really",
		"uuid":               coretesting.ModelTag.Id(),
		"controller-uuid":    coretesting.ControllerTag.Id(),
		"ca-cert":            coretesting.CACert,
		"ca-private-key":     coretesting.CAKey,
		"authorized-keys":    coretesting.FakeAuthKeys,
		"default-base":       base.String(),
		"cloudinit-userdata": validCloudInitUserData,
	}
	cfg, err := config.New(config.UseDefaults, attrs)
	c.Assert(err, tc.ErrorIsNil)
	return cfg
}

func configGetter(c *tc.C) configFunc {
	cfg := minimalConfig(c)
	return func() *config.Config { return cfg }
}

func (s *BootstrapSuite) TestCannotStartInstance(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	checkPlacement := "directive"
	checkCons := constraints.MustParse("mem=8G")
	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}
	rvalErr := errors.Errorf("meh, not started")

	startInstance := func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		c.Assert(args.Placement, tc.DeepEquals, checkPlacement)
		c.Assert(args.Constraints, tc.DeepEquals, checkCons)

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
		c.Assert(err, tc.ErrorIsNil)

		expectedMcfg.EnableOSRefreshUpdate = env.Config().EnableOSRefreshUpdate()
		expectedMcfg.EnableOSUpgrade = env.Config().EnableOSUpgrade()
		expectedMcfg.Tags = map[string]string{
			"juju-model-uuid":      coretesting.ModelTag.Id(),
			"juju-controller-uuid": coretesting.ControllerTag.Id(),
			"juju-is-controller":   "true",
		}
		expectedMcfg.NetBondReconfigureDelay = env.Config().NetBondReconfigureDelay()
		args.InstanceConfig.Bootstrap.InitialSSHHostKeys = nil
		c.Assert(args.InstanceConfig, tc.DeepEquals, expectedMcfg)
		return nil, nil, nil, rvalErr
	}

	env.startInstance = startInstance

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapConstraints:    checkCons,
		ModelConstraints:        checkCons,
		Placement:               checkPlacement,
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
	// We do this check to make sure that errors propagated from start instance
	// are then passed on through Bootstrap().
	c.Assert(err, tc.ErrorIs, rvalErr)
}

func (s *BootstrapSuite) TestBootstrapInstanceCancelled(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}

	startInstance := func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		return nil, nil, nil, errors.Errorf("some kind of error")
	}
	env.startInstance = startInstance

	stdCtx, cancel := context.WithCancel(c.Context())
	cancel()
	ctx := environscmd.BootstrapContext(stdCtx, cmdtesting.Context(c))
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches, `starting controller \(cancelled\): some kind of error`)
}

func (s *BootstrapSuite) TestBootstrapBase(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTestContext(c)

	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapBase:           jujuversion.DefaultSupportedLTSBase(),
		AvailableTools:          availableTools,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Arch, tc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), tc.Equals, jujuversion.DefaultSupportedLTSBase().String())
}

func (s *BootstrapSuite) TestBootstrapFallbackBase(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTestContext(c)

	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapBase:           corebase.Base{},
		AvailableTools:          availableTools,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Arch, tc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), tc.Equals, jujuversion.DefaultSupportedLTSBase().String())
}

func (s *BootstrapSuite) TestBootstrapSeriesWithForce(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTestContext(c)
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapBase:           corebase.MustParseBaseFromString("ubuntu@16.04"),
		AvailableTools:          availableTools,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		Force:                   true,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Arch, tc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), tc.Equals, corebase.MakeDefaultBase("ubuntu", "16.04").String())
}

func (s *BootstrapSuite) TestStartInstanceDerivedZone(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(context.Context, environs.StartInstanceParams) ([]string, error) {
			return []string{"derived-zone"}, nil
		},
	}

	env.startInstance = func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		c.Assert(args.AvailabilityZone, tc.Equals, "derived-zone")
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches,
		`cannot start bootstrap instance in availability zone "derived-zone": bloop`,
	)
}

func (s *BootstrapSuite) TestStartInstanceAttemptAllZones(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(context.Context, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx context.Context) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", false}
			z2 := &mockAvailabilityZone{"z2", true}
			return network.AvailabilityZones{z0, z1, z2}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches,
		`(?ms)cannot start bootstrap instance in any availability zone \(z0, z2\).*`,
	)
	c.Assert(callZones, tc.SameContents, []string{"z0", "z2"})
}

func (s *BootstrapSuite) TestStartInstanceAttemptZoneConstrained(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(context.Context, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx context.Context) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", true}
			z2 := &mockAvailabilityZone{"z2", true}
			z3 := &mockAvailabilityZone{"z3", true}
			return network.AvailabilityZones{z0, z1, z2, z3}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		BootstrapConstraints: constraints.Value{
			Zones: &[]string{"z0", "z2"},
		},
	})
	c.Assert(err, tc.ErrorMatches,
		`(?ms)cannot start bootstrap instance in any availability zone \(z0, z2\).*`,
	)
	c.Assert(callZones, tc.SameContents, []string{"z0", "z2"})
}

func (s *BootstrapSuite) TestStartInstanceNoMatchingConstraintZones(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(context.Context, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx context.Context) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", true}
			z2 := &mockAvailabilityZone{"z2", true}
			z3 := &mockAvailabilityZone{"z3", true}
			return network.AvailabilityZones{z0, z1, z2, z3}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, errors.New("bloop")
	}

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		BootstrapConstraints: constraints.Value{
			Zones: &[]string{"z4", "z5"},
		},
	})
	c.Assert(err, tc.ErrorMatches,
		`no available zones \(\["z0" "z1" "z2" "z3"\]\) matching bootstrap zone constraints \(\["z4" "z5"\]\)`,
	)
	c.Assert(callZones, tc.IsNil)
}

func (s *BootstrapSuite) TestStartInstanceStopOnZoneIndependentError(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(context.Context, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx context.Context) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", true}
			z1 := &mockAvailabilityZone{"z1", true}
			return network.AvailabilityZones{z0, z1}, nil
		},
	}

	var callZones []string
	env.startInstance = func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		callZones = append(callZones, args.AvailabilityZone)
		return nil, nil, nil, environs.ZoneIndependentError(errors.New("bloop"))
	}

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches, `cannot start bootstrap instance: bloop`)
	c.Assert(callZones, tc.SameContents, []string{"z0"})
}

func (s *BootstrapSuite) TestStartInstanceNoUsableZones(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		mockEnviron: mockEnviron{
			storage: newStorage(s, c),
			config:  configGetter(c),
		},
		deriveAvailabilityZones: func(context.Context, environs.StartInstanceParams) ([]string, error) {
			return nil, nil
		},
		availabilityZones: func(ctx context.Context) (network.AvailabilityZones, error) {
			z0 := &mockAvailabilityZone{"z0", false}
			return network.AvailabilityZones{z0}, nil
		},
	}

	ctx := envtesting.BootstrapTestContext(c)
	_, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorMatches, `cannot start bootstrap instance: no usable availability zones`)
}

func (s *BootstrapSuite) TestStartInstanceRootDisk(c *tc.C) {
	startInstance := func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		c.Assert(args.RootDisk, tc.DeepEquals, &corestorage.VolumeParams{
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
	ctx := envtesting.BootstrapTestContext(c)
	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          availableTools,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		BootstrapConstraints:    constraints.MustParse("root-disk-source=spool"),
		StoragePools: map[string]corestorage.Attrs{
			"spool": {
				"type": "dummy",
				"foo":  "bar",
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Arch, tc.Equals, "ppc64el")
}

func (s *BootstrapSuite) TestSuccess(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	stor := newStorage(s, c)
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	var innerInstanceConfig *instancecfg.InstanceConfig
	inst := &mockInstance{
		id:        checkInstanceId,
		addresses: network.NewMachineAddresses([]string{"testing.invalid"}).AsProviderAddresses(),
	}
	startInstance := func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		icfg := args.InstanceConfig
		innerInstanceConfig = icfg
		c.Assert(icfg.Bootstrap.InitialSSHHostKeys, tc.HasLen, 3)
		for _, key := range icfg.Bootstrap.InitialSSHHostKeys {
			privKey, err := cryptossh.ParseRawPrivateKey([]byte(key.Private))
			c.Assert(err, tc.ErrorIsNil)
			_, fits := privKey.(interface {
				Public() crypto.PublicKey
				Equal(crypto.PrivateKey) bool
			})
			c.Assert(fits, tc.IsTrue)
			pubKey, _, _, _, err := cryptossh.ParseAuthorizedKey([]byte(key.Public))
			c.Assert(err, tc.ErrorIsNil)
			c.Assert(pubKey.Type(), tc.Equals, key.PublicKeyAlgorithm)
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
		instances: func(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
			instancesMu.Lock()
			defer instancesMu.Unlock()
			return []instances.Instance{inst}, nil
		},
	}
	inner := cmdtesting.Context(c)
	ctx := environscmd.BootstrapContext(c.Context(), inner)
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Arch, tc.Equals, "ppc64el") // based on hardware characteristics
	c.Assert(result.Base, tc.Equals, config.PreferredBase(mocksConfig))
	c.Assert(result.CloudBootstrapFinalizer, tc.NotNil)

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
	testhelpers.PatchExecutableAsEchoArgs(c, s, "ssh")
	testhelpers.PatchExecutableAsEchoArgs(c, s, "scp")
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
		sshArgs := testhelpers.ReadEchoArgs(c, "ssh")
		submatch := re.FindStringSubmatch(sshArgs)
		if c.Check(submatch, tc.NotNil, tc.Commentf("%s", sshArgs)) {
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
	c.Assert(err, tc.ErrorMatches, "invalid machine configuration: .*") // icfg hasn't been finalized
	c.Assert(innerInstanceConfig.Bootstrap.InitialSSHHostKeys, tc.HasLen, 3)
	computedKnownHosts := ""
	computedHostKeyAlgos := []string{}
	for _, key := range innerInstanceConfig.Bootstrap.InitialSSHHostKeys {
		computedKnownHosts += "testing.invalid " + key.Public
		computedHostKeyAlgos = append(computedHostKeyAlgos, key.PublicKeyAlgorithm)
	}
	c.Assert(
		knownHosts,
		tc.Equals,
		computedKnownHosts,
	)
	c.Assert(strings.Split(hostKeyAlgos, ","), tc.SameContents, computedHostKeyAlgos)
}

func (s *BootstrapSuite) TestBootstrapFinalizeCloudInitUserData(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")

	var innerInstanceConfig *instancecfg.InstanceConfig
	inst := &mockInstance{
		id:        "i-success",
		addresses: network.NewMachineAddresses([]string{"testing.invalid"}).AsProviderAddresses(),
	}
	startInstance := func(ctx context.Context, args environs.StartInstanceParams) (
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
		instances: func(ctx context.Context, ids []instance.Id) ([]instances.Instance, error) {
			instancesMu.Lock()
			defer instancesMu.Unlock()
			return []instances.Instance{inst}, nil
		},
	}
	ctx := envtesting.BootstrapTestContext(c)

	availableTools := fakeAvailableTools()
	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapBase:           jujuversion.DefaultSupportedLTSBase(),
		AvailableTools:          availableTools,
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result.CloudBootstrapFinalizer, tc.NotNil)
	err = result.CloudBootstrapFinalizer(ctx, innerInstanceConfig, environs.BootstrapDialOpts{
		Timeout: coretesting.ShortWait,
	})
	c.Assert(err, tc.ErrorMatches, "waited for 50ms without being able to connect.*")
	c.Assert(innerInstanceConfig.CloudInitUserData, tc.DeepEquals, map[string]interface{}{
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

func (neverRefreshes) Refresh(ctx context.Context) error {
	return nil
}

func (neverRefreshes) Status(ctx context.Context) instance.Status {
	return instance.Status{}
}

type neverAddresses struct {
	neverRefreshes
}

func (neverAddresses) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	return nil, nil
}

type failsProvisioning struct {
	neverAddresses
	message string
}

func (f failsProvisioning) Status(_ context.Context) instance.Status {
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

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForAddresses(c *tc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, `waited for `+testSSHTimeout.Timeout.String()+` without getting any addresses`)
	c.Check(cmdtesting.Stderr(ctx), tc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForAddresses(c *tc.C) {
	cmdCtx := cmdtesting.Context(c)
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	_, err := common.WaitSSH(
		ctx, cmdCtx.Stderr, ssh.DefaultClient, "/bin/true", neverAddresses{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, "cancelled")
	c.Check(cmdtesting.Stderr(cmdCtx), tc.Matches, "Waiting for address\n")
}

func (s *BootstrapSuite) TestWaitSSHNoticesProvisioningFailures(c *tc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "/bin/true", failsProvisioning{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, `instance provisioning failed`)
	_, err = common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "/bin/true", failsProvisioning{message: "blargh"}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, `instance provisioning failed \(blargh\)`)
}

type brokenAddresses struct {
	neverRefreshes
}

func (brokenAddresses) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	return nil, errors.Errorf("Addresses will never work")
}

func (s *BootstrapSuite) TestWaitSSHStopsOnBadError(c *tc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "/bin/true", brokenAddresses{}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, "getting addresses: Addresses will never work")
	c.Check(cmdtesting.Stderr(ctx), tc.Equals, "Waiting for address\n")
}

type neverOpensPort struct {
	neverRefreshes
	addr string
}

func (n *neverOpensPort) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	return network.NewMachineAddresses([]string{n.addr}).AsProviderAddresses(), nil
}

func (s *BootstrapSuite) TestWaitSSHTimesOutWaitingForDial(c *tc.C) {
	ctx := cmdtesting.Context(c)
	// 0.x.y.z addresses are always invalid
	_, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "/bin/true", &neverOpensPort{addr: "0.1.2.3"}, testSSHTimeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.3`)
	c.Check(cmdtesting.Stderr(ctx), tc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type cancelOnDial struct {
	neverRefreshes
	name     string
	cancel   context.CancelFunc
	returned bool
}

func (c *cancelOnDial) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
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

func (s *BootstrapSuite) TestWaitSSHKilledWaitingForDial(c *tc.C) {
	cmdCtx := cmdtesting.Context(c)
	timeout := testSSHTimeout
	timeout.Timeout = 1 * time.Minute
	ctx, cancel := context.WithCancel(c.Context())
	_, err := common.WaitSSH(
		ctx, cmdCtx.Stderr, ssh.DefaultClient, "", &cancelOnDial{name: "0.1.2.3", cancel: cancel}, timeout,
		common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, "cancelled")
	// Exact timing is imprecise but it should have tried a few times before being killed
	c.Check(cmdtesting.Stderr(cmdCtx), tc.Matches,
		"Waiting for address\n"+
			"(Attempting to connect to 0.1.2.3:22\n)+")
}

type addressesChange struct {
	addrs [][]string
}

func (ac *addressesChange) Refresh(ctx context.Context) error {
	if len(ac.addrs) > 1 {
		ac.addrs = ac.addrs[1:]
	}
	return nil
}

func (ac *addressesChange) Status(ctx context.Context) instance.Status {
	return instance.Status{}
}

func (ac *addressesChange) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	return network.NewMachineAddresses(ac.addrs[0]).AsProviderAddresses(), nil
}

func (s *BootstrapSuite) TestWaitSSHRefreshAddresses(c *tc.C) {
	ctx := cmdtesting.Context(c)
	_, err := common.WaitSSH(c.Context(), ctx.Stderr, ssh.DefaultClient, "", &addressesChange{addrs: [][]string{
		nil,
		nil,
		{"0.1.2.3"},
		{"0.1.2.3"},
		nil,
		{"0.1.2.4"},
	}}, testSSHTimeout, common.DefaultHostSSHOptions)
	// Not necessarily the last one in the list, due to scheduling.
	c.Check(err, tc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+` without being able to connect: mock connection failure to 0.1.2.[34]`)
	stderr := cmdtesting.Stderr(ctx)
	c.Check(stderr, tc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.3:22\n)+(.|\n)*")
	c.Check(stderr, tc.Matches,
		"Waiting for address\n"+
			"(.|\n)*(Attempting to connect to 0.1.2.4:22\n)+(.|\n)*")
}

type FormatHardwareSuite struct{}

func TestFormatHardwareSuite(t *stdtesting.T) {
	tc.Run(t, &FormatHardwareSuite{})
}

func (s *FormatHardwareSuite) check(c *tc.C, hw *instance.HardwareCharacteristics, expected string) {
	c.Check(common.FormatHardware(hw), tc.Equals, expected)
}

func (s *FormatHardwareSuite) TestNil(c *tc.C) {
	s.check(c, nil, "")
}

func (s *FormatHardwareSuite) TestFieldsNil(c *tc.C) {
	s.check(c, &instance.HardwareCharacteristics{}, "")
}

func (s *FormatHardwareSuite) TestArch(c *tc.C) {
	arch := ""
	s.check(c, &instance.HardwareCharacteristics{Arch: &arch}, "")
	arch = "amd64"
	s.check(c, &instance.HardwareCharacteristics{Arch: &arch}, "arch=amd64")
}

func (s *FormatHardwareSuite) TestCores(c *tc.C) {
	var cores uint64
	s.check(c, &instance.HardwareCharacteristics{CpuCores: &cores}, "")
	cores = 24
	s.check(c, &instance.HardwareCharacteristics{CpuCores: &cores}, "cores=24")
}

func (s *FormatHardwareSuite) TestMem(c *tc.C) {
	var mem uint64
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "")
	mem = 800
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=800M")
	mem = 1024
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=1G")
	mem = 2712
	s.check(c, &instance.HardwareCharacteristics{Mem: &mem}, "mem=2.6G")
}

func (s *FormatHardwareSuite) TestVirtType(c *tc.C) {
	var virtType string
	s.check(c, &instance.HardwareCharacteristics{VirtType: &virtType}, "")
	virtType = string(instance.DefaultInstanceType)
	s.check(c, &instance.HardwareCharacteristics{VirtType: &virtType}, "")
	virtType = "virtual-machine"
	s.check(c, &instance.HardwareCharacteristics{VirtType: &virtType}, "virt-type=virtual-machine")
}

func (s *FormatHardwareSuite) TestAll(c *tc.C) {
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
			Version: semversion.Binary{
				Number:  jujuversion.Current,
				Arch:    arch.HostArch(),
				Release: "ubuntu",
			},
		},
	}
}

func fakeStartInstance(ctx context.Context, args environs.StartInstanceParams) (
	instances.Instance,
	*instance.HardwareCharacteristics,
	network.InterfaceInfos,
	error,
) {
	checkInstanceId := "i-success"
	checkHardware := instance.MustParseHardware("arch=ppc64el mem=2T")
	return &mockInstance{id: checkInstanceId}, &checkHardware, nil, nil
}

func fakeMinimalConfig(c *tc.C) func() *config.Config {
	var mocksConfig = minimalConfig(c)
	return func() *config.Config {
		return mocksConfig
	}
}

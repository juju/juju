// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"crypto"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/ssh"
	cryptossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/status"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
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

func TestBootstrapSuite(t *testing.T) {
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
	attrs := map[string]any{
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

// TestBootstrapInstanceUsesExplicitMachineId checks that a non-empty
// BootstrapParams.MachineId overrides the default machine "0" identity on the
// InstanceConfig passed to StartInstance, so providers create per-machine
// resources (hostnames, security groups, tags) for the archived machine ID.
func (s *BootstrapSuite) TestBootstrapInstanceUsesExplicitMachineId(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}

	stopErr := errors.New("stop after capture")
	var gotCfg *instancecfg.InstanceConfig
	env.startInstance = func(_ context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		gotCfg = args.InstanceConfig
		return nil, nil, nil, stopErr
	}

	_, err := common.Bootstrap(envtesting.BootstrapTestContext(c), env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
		MachineID:               "7",
	})
	c.Assert(err, tc.ErrorIs, stopErr)
	c.Assert(gotCfg, tc.NotNil)
	c.Check(gotCfg.MachineId, tc.Equals, "7")
	c.Check(gotCfg.MachineAgentServiceName, tc.Equals, "jujuagentd-machine-7")
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

func (s *BootstrapSuite) TestBootstrapInstanceID(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        fakeMinimalConfig(c),
	}
	ctx := envtesting.BootstrapTestContext(c)

	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapBase:           jujuversion.DefaultSupportedLTSBase(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Arch, tc.Equals, "ppc64el")
	// The instance ID is reported from the started instance itself.
	c.Check(result.InstanceID, tc.Equals, instance.Id("i-success"))
}

func (s *BootstrapSuite) TestBootstrapPostAllocationError(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	// The instance is allocated, then a subsequent step fails after
	// StartInstance has returned.
	openPortsErr := errors.New("no firewall for you")
	env := &mockEnviron{
		startInstance:     fakeStartInstance,
		config:            fakeMinimalConfig(c),
		openModelPortsErr: openPortsErr,
	}
	ctx := envtesting.BootstrapTestContext(c)

	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		BootstrapBase:           jujuversion.DefaultSupportedLTSBase(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	// The result is nil on error; the allocated instance is only
	// identified through the error.
	c.Check(result, tc.IsNil)
	c.Assert(err, tc.ErrorMatches,
		`bootstrap instance "i-success": cannot open SSH: no firewall for you`)
	// The original error classification is preserved.
	c.Assert(err, tc.ErrorIs, openPortsErr)
}

func (s *BootstrapSuite) TestBootstrapErrorBeforeAllocationHasNoInstanceID(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
	}
	startInstanceErr := errors.New("meh, not started")
	env.startInstance = func(ctx context.Context, args environs.StartInstanceParams) (
		instances.Instance,
		*instance.HardwareCharacteristics,
		network.InterfaceInfos,
		error,
	) {
		return nil, nil, nil, startInstanceErr
	}
	ctx := envtesting.BootstrapTestContext(c)

	result, err := common.Bootstrap(ctx, env, environs.BootstrapParams{
		ControllerConfig:        coretesting.FakeControllerConfig(),
		AvailableTools:          fakeAvailableTools(),
		SupportedBootstrapBases: coretesting.FakeSupportedJujuBases,
	})
	c.Check(result, tc.IsNil)
	// No instance was allocated, so the error must not carry an
	// instance ID.
	c.Assert(err, tc.ErrorMatches, "cannot start bootstrap instance: meh, not started")
	c.Assert(err, tc.ErrorIs, startInstanceErr)
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

func (s *BootstrapSuite) TestBootstrapWithSubstrateNotSupportsIPV6CIDR(c *tc.C) {
	s.testBootstrapWithSubstrateWithIPV6Support(c, false)
}

func (s *BootstrapSuite) TestBootstrapWithSubstrateSupportsIPV6CIDR(c *tc.C) {
	s.testBootstrapWithSubstrateWithIPV6Support(c, true)
}

func (s *BootstrapSuite) testBootstrapWithSubstrateWithIPV6Support(c *tc.C, supportsIPv6 bool) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)

	env := &mockEnviron{
		startInstance: fakeStartInstance,
		config:        configGetter(c),
		supportsIPv6:  supportsIPv6,
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

	var rules firewall.IngressRules
	if supportsIPv6 {
		rules = firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), "0.0.0.0/0", "::/0"),
		}
	} else {
		rules = firewall.IngressRules{
			firewall.NewIngressRule(network.MustParsePortRange("22/tcp"), "0.0.0.0/0"),
		}
	}
	c.Assert(env.modelRules.EqualTo(rules), tc.IsTrue)

	c.Check(result.Arch, tc.Equals, "ppc64el") // based on hardware characteristics
	c.Check(result.Base.String(), tc.Equals, jujuversion.DefaultSupportedLTSBase().String())
}

func (s *BootstrapSuite) TestStartInstanceDerivedZone(c *tc.C) {
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	env := &mockZonedEnviron{
		storage: newStorage(s, c),
		config:  configGetter(c),
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
		storage: newStorage(s, c),
		config:  configGetter(c),
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
		storage: newStorage(s, c),
		config:  configGetter(c),
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
		storage: newStorage(s, c),
		config:  configGetter(c),
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
		storage: newStorage(s, c),
		config:  configGetter(c),
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
		storage: newStorage(s, c),
		config:  configGetter(c),
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
			Attributes: map[string]any{
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
	c.Assert(result.InstanceID, tc.Equals, instance.Id(checkInstanceId))
	c.Assert(result.CloudBootstrapFinalizer, tc.NotNil)

	// Check that we make the SSH connection with desired options.
	var knownHosts string
	var hostKeyAlgos string
	identityFiles := []string{
		filepath.Join(c.MkDir(), "identity-one"),
		filepath.Join(c.MkDir(), "identity-two"),
	}
	for _, identityFile := range identityFiles {
		c.Assert(os.WriteFile(identityFile, []byte("key"), 0600), tc.ErrorIsNil)
	}
	re := regexp.MustCompile(
		"ssh '-o' 'StrictHostKeyChecking yes' " +
			"'-o' 'PasswordAuthentication no' " +
			"'-o' 'ServerAliveInterval 30' " +
			"'-o' 'UserKnownHostsFile (.*)' " +
			"'-o' 'HostKeyAlgorithms ([^']*)' " +
			"'-i' '" + regexp.QuoteMeta(identityFiles[0]) + "' " +
			"'-i' '" + regexp.QuoteMeta(identityFiles[1]) + "' " +
			// When an identity file is added, the utils/ssh library adds the
			// default home as well. To avoid messing with the XDG_DATA_HOME,
			// we just match an additional -i option.
			"'-i' '[^']+' " +
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
		IdentityFiles: identityFiles,
		Timeout:       coretesting.LongWait,
	})
	c.Assert(err, tc.ErrorMatches, "invalid machine configuration: .*") // icfg hasn't been finalized
	c.Assert(innerInstanceConfig.Bootstrap.InitialSSHHostKeys, tc.HasLen, 3)
	var computedKnownHosts strings.Builder
	computedHostKeyAlgos := []string{}
	for _, key := range innerInstanceConfig.Bootstrap.InitialSSHHostKeys {
		computedKnownHosts.WriteString("testing.invalid " + key.Public)
		computedHostKeyAlgos = append(computedHostKeyAlgos, key.PublicKeyAlgorithm)
	}
	c.Assert(
		knownHosts,
		tc.Equals,
		computedKnownHosts.String(),
	)
	c.Assert(strings.Split(hostKeyAlgos, ","), tc.SameContents, computedHostKeyAlgos)
}

// TestConfigureMachineSkipsSetupWhenCancelled verifies that a cancelled
// bootstrap does not attempt to stage or configure the machine.
func (s *BootstrapSuite) TestConfigureMachineSkipsSetupWhenCancelled(c *tc.C) {
	ctx, cancel := context.WithCancel(c.Context())
	cancel()
	err := common.ConfigureMachine(envtesting.BootstrapContext(ctx, c), nil, "10.0.0.1",
		&instancecfg.InstanceConfig{}, nil)
	c.Check(err, tc.ErrorIs, bootstrap.Cancelled())
}

// TestConfigureMachineTrapCleansUpOnRemoteFailure checks that a setup
// script that ran and failed is cleaned up by its own EXIT trap: the
// remote failure is preserved and no fallback cleanup reconnects.
func (s *BootstrapSuite) TestConfigureMachineTrapCleansUpOnRemoteFailure(c *tc.C) {
	root, logPath, err := s.runConfigureWithFakeSSH(c, "remote-fail")
	c.Check(err, tc.ErrorMatches, "subprocess encountered error code 1")
	c.Check(readSSHLog(c, logPath), tc.DeepEquals,
		[]string{"mkdir", "stream", "setup", "trap-cleanup", "setup-end"})
	c.Check(stagedFiles(c, root), tc.HasLen, 0)
}

// TestConfigureMachineNoReconnectAfterSuccessfulSetup checks that a
// completed setup leaves cleanup to the remote trap: no fallback
// connection is made.
func (s *BootstrapSuite) TestConfigureMachineNoReconnectAfterSuccessfulSetup(c *tc.C) {
	root, logPath, err := s.runConfigureWithFakeSSH(c, "")
	c.Check(err, tc.ErrorIsNil)
	c.Check(readSSHLog(c, logPath), tc.DeepEquals,
		[]string{"mkdir", "stream", "setup", "trap-cleanup", "setup-end"})
	c.Check(stagedFiles(c, root), tc.HasLen, 0)
}

// TestConfigureMachineFallsBackToCleanupWhenSetupNeverRuns checks that a
// connection failure after files were staged leaves them to the deferred
// fallback cleanup, which runs over a fresh connection.
func (s *BootstrapSuite) TestConfigureMachineFallsBackToCleanupWhenSetupNeverRuns(c *tc.C) {
	root, logPath, err := s.runConfigureWithFakeSSH(c, "conn-fail")
	c.Check(err, tc.ErrorMatches, "subprocess encountered error code 255")
	c.Check(readSSHLog(c, logPath), tc.DeepEquals,
		[]string{"mkdir", "stream", "cleanup"})
	c.Check(stagedFiles(c, root), tc.HasLen, 0)
}

// sudo can return the same status as the setup script before it starts bash.
func (s *BootstrapSuite) TestConfigureMachineFallsBackWhenSudoFails(c *tc.C) {
	root, logPath, err := s.runConfigureWithFakeSSH(c, "sudo-fail")
	c.Check(err, tc.ErrorMatches, "subprocess encountered error code 1")
	c.Check(readSSHLog(c, logPath), tc.DeepEquals, []string{"mkdir", "stream", "cleanup"})
	c.Check(stagedFiles(c, root), tc.HasLen, 0)
}

func (s *BootstrapSuite) TestConfigureMachineFallsBackWithoutAcknowledgement(c *tc.C) {
	root, logPath, err := s.runConfigureWithFakeSSH(c, "lost-ack")
	c.Check(err, tc.ErrorIsNil)
	c.Check(readSSHLog(c, logPath), tc.DeepEquals,
		[]string{"mkdir", "stream", "setup", "trap-cleanup", "setup-end", "cleanup"})
	c.Check(stagedFiles(c, root), tc.HasLen, 0)
}

// TestConfigureMachineCleanupFailurePreservesSetupError checks that a
// failing fallback cleanup is reported alongside the failure that made
// it necessary, never replacing it.
func (s *BootstrapSuite) TestConfigureMachineCleanupFailurePreservesSetupError(c *tc.C) {
	root, logPath, err := s.runConfigureWithFakeSSH(c, "conn-fail-rm")
	c.Assert(err, tc.NotNil)
	c.Check(strings.Contains(err.Error(), "subprocess encountered error code 255"), tc.IsTrue)
	c.Check(strings.Contains(err.Error(), "cleaning up staged files"), tc.IsTrue)
	c.Check(readSSHLog(c, logPath), tc.DeepEquals,
		[]string{"mkdir", "stream", "cleanup"})
	// The failed cleanup leaves the staging directory in place; the
	// failure is reported rather than hidden.
	c.Check(stagedFiles(c, root), tc.HasLen, 1)
}

// TestConfigureMachineCancelledDuringDispatch checks that a context
// cancelled while files are being transported is reported as the
// cancellation sentinel with the transport context: the configuration
// script never runs and the staged remainder is cleaned up.
func (s *BootstrapSuite) TestConfigureMachineCancelledDuringDispatch(c *tc.C) {
	fixture := s.newConfigureFixture(c, "")
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	client, err := ssh.NewOpenSSHClient()
	c.Assert(err, tc.ErrorIsNil)

	err = fixture.run(envtesting.BootstrapContext(ctx, c),
		&cancellingClient{Client: client, cancelOn: "umask", cancel: cancel})

	c.Check(err, tc.ErrorIs, bootstrap.Cancelled())
	c.Check(strings.Contains(err.Error(), "transporting files to machine"), tc.IsTrue)
	// The configure script never ran: the trailing ssh call is the
	// fallback cleanup of the partially staged files.
	c.Check(readSSHLog(c, fixture.logPath), tc.DeepEquals,
		[]string{"mkdir", "cleanup"})
}

// TestConfigureMachineCancelledBeforeConfigure checks that a context
// cancelled once the files are staged stops before the configuration
// script: the cancellation sentinel is returned and the staged files
// are cleaned up without running setup.
func (s *BootstrapSuite) TestConfigureMachineCancelledBeforeConfigure(c *tc.C) {
	fixture := s.newConfigureFixture(c, "")
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()
	s.PatchValue(common.FilesDispatched, func(context.Context) { cancel() })
	client, err := ssh.NewOpenSSHClient()
	c.Assert(err, tc.ErrorIsNil)

	err = fixture.run(envtesting.BootstrapContext(ctx, c), client)

	c.Check(err, tc.ErrorIs, bootstrap.Cancelled())
	c.Check(readSSHLog(c, fixture.logPath), tc.DeepEquals,
		[]string{"mkdir", "stream", "cleanup"})
}

// cancellingClient cancels the run's context synchronously when the
// named remote command is issued, driving a cancellation mid-dispatch
// without sleeps or timers.
type cancellingClient struct {
	ssh.Client
	cancelOn string
	cancel   context.CancelFunc
}

func (c *cancellingClient) Command(host string, command []string, options *ssh.Options) *ssh.Cmd {
	if len(command) > 0 && command[0] == c.cancelOn {
		c.cancel()
	}
	return c.Client.Command(host, command, options)
}

// runConfigureWithFakeSSH renders a replacement machine configuration
// whose recovery archive is staged over SSH, then runs ConfigureMachine
// against a fake ssh executable. It returns the fake machine's root,
// the invocation log path and the ConfigureMachine result.
func (s *BootstrapSuite) runConfigureWithFakeSSH(
	c *tc.C, mode string,
) (string, string, error) {
	fixture := s.newConfigureFixture(c, mode)
	client, err := ssh.NewOpenSSHClient()
	c.Assert(err, tc.ErrorIsNil)
	return fixture.root, fixture.logPath,
		fixture.run(envtesting.BootstrapTestContext(c), client)
}

// configureFixture is a provisioned machine replacement configuration
// staging its recovery archive over SSH against a fake machine.
type configureFixture struct {
	icfg    *instancecfg.InstanceConfig
	root    string
	logPath string
}

// run executes ConfigureMachine over the given context and client.
func (f *configureFixture) run(ctx environs.BootstrapContext, client ssh.Client) error {
	return common.ConfigureMachine(ctx, client, "10.0.0.1", f.icfg, nil)
}

// newConfigureFixture provisions the machine replacement configuration
// and patches the fake ssh executable; see fakeSetupSSHScript for the
// modes and the invocation log.
func (s *BootstrapSuite) newConfigureFixture(c *tc.C, mode string) configureFixture {
	remoteRoot := c.MkDir()
	logPath := filepath.Join(c.MkDir(), "ssh.log")
	testhelpers.PatchExecutable(c, s, "ssh", fakeSetupSSHScript(remoteRoot, logPath, mode))

	localArchive := filepath.Join(c.MkDir(), "backup.tar.gz")
	payload := []byte("recovery archive payload")
	c.Assert(os.WriteFile(localArchive, payload, 0o600), tc.ErrorIsNil)
	digest := sha256.Sum256(payload)

	modelUUID := tc.Must0(c, coremodel.NewUUID)
	modelConfig := coretesting.CustomModelConfig(c, coretesting.Attrs{
		"name": "controller", "uuid": modelUUID.String(), "type": "dummy",
		"agent-version": jujuversion.Current.String(),
	})
	icfg, err := instancecfg.NewBootstrapInstanceConfig(
		coretesting.FakeControllerConfig(), constraints.Value{}, constraints.Value{},
		corebase.MustParseBaseFromString("ubuntu@24.04"), "",
		map[string]string{"test": "true"},
	)
	c.Assert(err, tc.ErrorIsNil)
	icfg.DataDir = filepath.Join(remoteRoot, "var", "lib", "juju")
	// Setup replaces the initial host keys, so a fallback cleanup must
	// not expect to reconnect with them.
	icfg.Bootstrap.InitialSSHHostKeys = instancecfg.SSHHostKeys{{
		PublicKeyAlgorithm: "ssh-ed25519", Public: "ssh-ed25519 AAAA",
		Private: "private",
	}}
	icfg.APIInfo = &api.Info{
		ModelTag: names.NewModelTag(modelUUID.String()),
		CACert:   coretesting.CACert,
		Password: "secret",
	}
	icfg.Bootstrap.ControllerModelConfig = modelConfig
	icfg.Bootstrap.ControllerConfig = coretesting.FakeControllerConfig()
	icfg.Bootstrap.ControllerCloud = cloud.Cloud{Name: "manual", Type: "manual"}
	icfg.Bootstrap.ControllerAgentInfo = controller.ControllerAgentInfo{
		Cert: coretesting.ServerCert, PrivateKey: coretesting.ServerKey,
		CAPrivateKey: coretesting.CAKey, APIPort: 17070,
	}
	icfg.Bootstrap.AgentVersion = coretesting.FakeVersionNumber
	icfg.Bootstrap.BootstrapMachineInstanceId = "i-test"
	c.Assert(icfg.SetTools(tools.List{{
		Version: semversion.Binary{
			Number: coretesting.FakeVersionNumber, Release: "ubuntu", Arch: arch.AMD64,
		},
		URL:    "https://example.invalid/agent.tgz",
		SHA256: strings.Repeat("0", 64),
	}}), tc.ErrorIsNil)
	icfg.Bootstrap.RecoveryMode = true
	icfg.Bootstrap.RecoveryArchivePath = localArchive
	icfg.Bootstrap.RecoveryArchiveSHA256 = "sha256-" + hex.EncodeToString(digest[:])

	return configureFixture{icfg: icfg, root: remoteRoot, logPath: logPath}
}

// fakeSetupSSHScript runs the staging commands and the real setup wrapper
// against a local root. It substitutes a harmless script for machine setup
// and records local and fallback cleanup separately.
func fakeSetupSSHScript(root, log, mode string) string {
	return fmt.Sprintf(`#!/bin/bash --norc
set -euo pipefail
args=()
found=0
for arg in "$@"; do
	if [[ $found -eq 0 ]]; then
		if [[ "$arg" == ubuntu@* ]]; then
			found=1
		fi
	else
		args+=("$arg")
	fi
done
[[ $found == 1 ]]
command="${args[*]}"
command="${command//\/tmp\//%[1]s/}"
if [[ "${args[0]}" == sudo ]]; then
	if [[ %[2]q == conn-fail* ]]; then
		echo 'ssh: connect to host 10.0.0.1 port 22: Connection refused' >&2
		exit 255
	fi
	if [[ %[2]q == sudo-fail ]]; then
		exit 1
	fi
	echo setup >> %[3]q
	trap 'echo setup-end >> %[3]q' EXIT
	# The wrapper uses cat to spool the configuration script.
	cat() {
		command cat >/dev/null
		if [[ %[2]q == remote-fail ]]; then
			echo 'exit 1'
		fi
	}
	rm() {
		if [[ $1 == -rf && $3 == /tmp/juju-* ]]; then
			echo trap-cleanup >> %[3]q
			command rm -rf -- "%[1]s/${3##*/}"
		else
			command rm "$@"
		fi
	}
	export -f cat rm
	if [[ %[2]q == lost-ack ]]; then
		exec 1>/dev/null
	fi
	"${args[@]:1}"
	exit
fi
case "${args[0]}" in
	mkdir) echo mkdir >> %[3]q ;;
	umask) echo stream >> %[3]q ;;
	rm)
		echo cleanup >> %[3]q
		if [[ %[2]q == conn-fail-rm ]]; then
			echo 'cannot remove staging' >&2
			exit 1
		fi
		;;
	*) echo "unexpected command: $command" >&2; exit 1 ;;
esac
eval "$command"
`, root, mode, log)
}

// readSSHLog returns the tags logged by the fake ssh executable, in
// the order the remote end saw the invocations.
func readSSHLog(c *tc.C, path string) []string {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	c.Assert(err, tc.ErrorIsNil)
	log := strings.TrimSpace(string(data))
	if log == "" {
		return nil
	}
	return strings.Split(log, "\n")
}

// stagedFiles returns the names left in the fake machine's staging
// root.
func stagedFiles(c *tc.C, root string) []string {
	entries, err := os.ReadDir(root)
	c.Assert(err, tc.ErrorIsNil)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
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
	c.Assert(innerInstanceConfig.CloudInitUserData, tc.DeepEquals, map[string]any{
		"packages":        []any{"python-keystoneclient", "python-glanceclient"},
		"preruncmd":       []any{"mkdir /tmp/preruncmd", "mkdir /tmp/preruncmd2"},
		"postruncmd":      []any{"mkdir /tmp/postruncmd", "mkdir /tmp/postruncmd2"},
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

// refreshFailsThenAddress is an InstanceRefresher whose Refresh fails
// the first fails times with the given error, after which the refresher
// reports the given address.
type refreshFailsThenAddress struct {
	// fails is the number of Refresh calls that fail before Refresh
	// starts succeeding.
	fails int
	// err is the error returned by the failing Refresh calls.
	err error
	// calls counts the Refresh calls made.
	calls int
	addr  string
}

func (r *refreshFailsThenAddress) Refresh(ctx context.Context) error {
	r.calls++
	if r.calls <= r.fails {
		return r.err
	}
	return nil
}

func (*refreshFailsThenAddress) Status(ctx context.Context) instance.Status {
	return instance.Status{}
}

func (r *refreshFailsThenAddress) Addresses(ctx context.Context) (network.ProviderAddresses, error) {
	if r.calls <= r.fails {
		return nil, nil
	}
	return network.NewMachineAddresses([]string{r.addr}).AsProviderAddresses(), nil
}

func (s *BootstrapSuite) TestWaitSSHRetriesInstanceNotVisible(c *tc.C) {
	ctx := cmdtesting.Context(c)
	// The instance is not visible to the provider for the first two
	// refreshes, as can happen due to eventual consistency shortly
	// after an instance is started. WaitSSH must keep retrying rather
	// than fail.
	refresher := &refreshFailsThenAddress{
		fails: 2,
		err:   environs.ErrNoInstances,
		addr:  "0.1.2.3",
	}
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, _, _ string, _ *ssh.Options) error {
		return nil
	})
	addr, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "", refresher,
		testSSHTimeout, common.DefaultHostSSHOptions,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.Equals, "0.1.2.3")
	c.Check(refresher.calls > 2, tc.IsTrue)
}

func (s *BootstrapSuite) TestWaitSSHRetriesPartialInstances(c *tc.C) {
	ctx := cmdtesting.Context(c)
	// An instance refresh may report that only some of the requested
	// instances were found (ErrPartialInstances). The instance may
	// still become visible, so WaitSSH must retry rather than fail.
	refresher := &refreshFailsThenAddress{
		fails: 2,
		err:   environs.ErrPartialInstances,
		addr:  "0.1.2.3",
	}
	s.PatchValue(common.ConnectSSH, func(_ ssh.Client, _, _ string, _ *ssh.Options) error {
		return nil
	})
	addr, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "", refresher,
		testSSHTimeout, common.DefaultHostSSHOptions,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addr, tc.Equals, "0.1.2.3")
	c.Check(refresher.calls > 2, tc.IsTrue)
}

func (s *BootstrapSuite) TestWaitSSHTimesOutRetryingInstanceNotVisible(c *tc.C) {
	ctx := cmdtesting.Context(c)
	// The instance never becomes visible: the timeout error should
	// surface the last refresh error so the cause is visible.
	refresher := &refreshFailsThenAddress{
		fails: 1000000,
		err:   environs.ErrNoInstances,
	}
	_, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "", refresher,
		testSSHTimeout, common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches,
		`waited for `+testSSHTimeout.Timeout.String()+
			` without getting any addresses, last refresh error: instances not found`)
	c.Check(refresher.calls > 1, tc.IsTrue)
}

func (s *BootstrapSuite) TestWaitSSHStopsOnPermanentRefreshError(c *tc.C) {
	ctx := cmdtesting.Context(c)
	// Errors other than the instance not being visible (yet), such as
	// invalid credentials, are not retried.
	refresher := &refreshFailsThenAddress{
		fails: 1000000,
		err:   errors.Errorf("some credential problem"),
	}
	_, err := common.WaitSSH(
		c.Context(), ctx.Stderr, ssh.DefaultClient, "", refresher,
		testSSHTimeout, common.DefaultHostSSHOptions,
	)
	c.Check(err, tc.ErrorMatches, "refreshing addresses: some credential problem")
	c.Check(refresher.calls, tc.Equals, 1)
}

type FormatHardwareSuite struct{}

func TestFormatHardwareSuite(t *testing.T) {
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

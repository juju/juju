// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/cmd/cmdtesting"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/deployment/charm"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	environscmd "github.com/juju/juju/environs/cmd"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	_ "github.com/juju/juju/internal/provider/dummy"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/testcharms"
)

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

var (
	bionicBootstrapBase = corebase.MustParseBaseFromString("ubuntu@18.04")
	focalBootstrapBase  = corebase.MustParseBaseFromString("ubuntu@20.04")
	jammyBootstrapBase  = corebase.MustParseBaseFromString("ubuntu@22.04")
	// Ensure that we add the default supported series so that tests that
	// use the default supported lts internally will always work in the
	// future.
	supportedJujuBases = append(coretesting.FakeSupportedJujuBases,
		corebase.MustParseBaseFromString("ubuntu@18.04"),
	)
)

type bootstrapSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture
}

func TestBootstrapSuite(t *testing.T) {
	tc.Run(t, &bootstrapSuite{})
}

func (s *bootstrapSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, tc.ErrorIsNil)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	envtesting.UploadFakeTools(c, stor, "released")
}

func (s *bootstrapSuite) TearDownTest(c *tc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *tc.C) {
	env := newEnviron("bar", noKeysDefined, nil)
	s.setDummyStorage(c, env)

	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, tc.ErrorMatches, "validating bootstrap parameters: admin-secret is empty")

	controllerCfg := coretesting.FakeControllerConfig()
	delete(controllerCfg, "ca-cert")
	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig: controllerCfg,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, tc.ErrorMatches, "validating bootstrap parameters: controller configuration has no ca-cert")

	controllerCfg = coretesting.FakeControllerConfig()
	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig: controllerCfg,
			AdminSecret:      "admin-secret",
		})
	c.Assert(err, tc.ErrorMatches, "validating bootstrap parameters: empty ca-private-key")

	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        controllerCfg,
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: supportedJujuBases,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapCredentialMismatch(c *tc.C) {
	env := newEnviron("bar", noKeysDefined, nil)
	s.setDummyStorage(c, env)

	cred := cloud.NewCredential(cloud.InstanceRoleAuthType, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			CAPrivateKey:            coretesting.CAKey,
			AdminSecret:             "admin-secret",
			SupportedBootstrapBases: supportedJujuBases,
			BootstrapConstraints:    constraints.MustParse("instance-role=foo"),
			CloudCredential:         &cred,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorMatches, "instance role constraint with instance role credential not supported")

	cred = cloud.NewCredential(cloud.ManagedIdentityAuthType, nil)
	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			CAPrivateKey:            coretesting.CAKey,
			AdminSecret:             "admin-secret",
			SupportedBootstrapBases: supportedJujuBases,
			BootstrapConstraints:    constraints.MustParse("instance-role=foo"),
			CloudCredential:         &cred,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorMatches, "instance role constraint with managed identity credential not supported")

}

func (s *bootstrapSuite) TestBootstrapTestingOptions(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:           coretesting.FakeControllerConfig(),
			AdminSecret:                "admin-secret",
			CAPrivateKey:               coretesting.CAKey,
			SSHServerHostKey:           coretesting.SSHServerHostKey,
			SupportedBootstrapBases:    supportedJujuBases,
			ExtraAgentValuesForTesting: map[string]string{"foo": "bar"},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.instanceConfig.AgentEnvironment, tc.DeepEquals, map[string]string{"foo": "bar"})
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: supportedJujuBases,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	env.args.AvailableTools = nil
	env.args.SupportedBootstrapBases = nil
	c.Assert(env.args, tc.DeepEquals, environs.BootstrapParams{
		ControllerConfig:     coretesting.FakeControllerConfig(),
		BootstrapBase:        corebase.MustParseBaseFromString("ubuntu@24.04"),
		BootstrapConstraints: constraints.MustParse("mem=3.5G"),
	})
}

// TestBootstrapControllerModelAuthorizedKeys is asserting that the authorized
// keys for the controller model are being populated as authorized keys for the
// controller machine during bootstrap.
func (s *bootstrapSuite) TestBootstrapControllerModelAuthorizedKeys(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:                   "admin-secret",
			ControllerConfig:              coretesting.FakeControllerConfig(),
			CAPrivateKey:                  coretesting.CAKey,
			ControllerModelAuthorizedKeys: []string{"key1"},
			BootstrapSSHAuthorizedKeys:    []string{"key1"},
			SupportedBootstrapBases:       supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	env.args.AvailableTools = nil
	env.args.SupportedBootstrapBases = nil
	c.Assert(env.args, tc.DeepEquals, environs.BootstrapParams{
		ControllerConfig:     coretesting.FakeControllerConfig(),
		AuthorizedKeys:       []string{"key1"},
		BootstrapBase:        corebase.MustParseBaseFromString("ubuntu@24.04"),
		BootstrapConstraints: constraints.MustParse("mem=3.5G"),
	})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	bootstrapCons := constraints.MustParse("cores=3 mem=7G")
	modelCons := constraints.MustParse("cores=2 mem=4G")
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			BootstrapConstraints:    bootstrapCons,
			ModelConstraints:        modelCons,
			SupportedBootstrapBases: supportedJujuBases,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.args.BootstrapConstraints, tc.DeepEquals, bootstrapCons)
	c.Assert(env.args.ModelConstraints, tc.DeepEquals, modelCons)
}

func (s *bootstrapSuite) TestBootstrapWithStoragePools(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: supportedJujuBases,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			StoragePools: map[string]internalstorage.Attrs{
				"spool": {
					"type": "loop",
					"foo":  "bar",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.args.StoragePools, tc.DeepEquals, map[string]internalstorage.Attrs{
		"spool": {
			"type": "loop",
			"foo":  "bar",
		},
	})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedBootstrapBase(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]any{
		"default-base": "ubuntu@20.04",
	})
	c.Assert(err, tc.ErrorIsNil)
	env.cfg = cfg

	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(env.bootstrapCount, tc.Equals, 1)
	c.Check(env.args.BootstrapBase, tc.Equals, jammyBootstrapBase)
	c.Check(env.args.AvailableTools.AllReleases(), tc.SameContents, []string{"ubuntu"})
}

func (s *bootstrapSuite) TestBootstrapFallbackBootstrapBase(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]any{
		"default-base": jujuversion.DefaultSupportedLTSBase().String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	env.cfg = cfg

	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SupportedBootstrapBases: supportedJujuBases,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(env.bootstrapCount, tc.Equals, 1)
	c.Check(env.args.AvailableTools.AllReleases(), tc.SameContents, []string{"ubuntu"})
}

func (s *bootstrapSuite) TestBootstrapForcedBootstrapBase(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]any{
		"default-base": "ubuntu@22.04",
	})
	c.Assert(err, tc.ErrorIsNil)
	env.cfg = cfg

	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			BootstrapBase:           focalBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
			Force:                   true,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(env.bootstrapCount, tc.Equals, 1)
	c.Check(env.args.BootstrapBase, tc.Equals, focalBootstrapBase)
	c.Check(env.args.AvailableTools.AllReleases(), tc.SameContents, []string{"ubuntu"})
}

func (s *bootstrapSuite) TestBootstrapWithInvalidBootstrapBase(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]any{
		"default-base": "ubuntu@22.04",
	})
	c.Assert(err, tc.ErrorIsNil)
	env.cfg = cfg

	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			BootstrapBase:           corebase.MustParseBaseFromString("spock@1"),
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorMatches, `non-ubuntu bootstrap base "spock@1/stable" not valid`)
}

func (s *bootstrapSuite) TestBootstrapSpecifiedPlacement(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	placement := "directive"
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			Placement:               placement,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.args.Placement, tc.DeepEquals, placement)
}

func (s *bootstrapSuite) TestFinalizePodBootstrapConfig(c *tc.C) {
	s.assertFinalizePodBootstrapConfig(c, "", "", nil)
}

func (s *bootstrapSuite) TestFinalizePodBootstrapConfigExternalService(c *tc.C) {
	s.assertFinalizePodBootstrapConfig(c, "external", "externalName", []string{"10.0.0.1"})
}

func (s *bootstrapSuite) assertFinalizePodBootstrapConfig(c *tc.C, serviceType, externalName string, externalIps []string) {
	podConfig, err := podcfg.NewBootstrapControllerPodConfig(
		coretesting.FakeControllerConfig(),
		"test",
		"ubuntu",
		constraints.Value{},
	)
	c.Assert(err, tc.ErrorIsNil)

	modelCfg, err := config.New(config.UseDefaults, coretesting.FakeConfig().Merge(coretesting.Attrs{
		"agent-version": "6.6.6",
	}))
	c.Assert(err, tc.ErrorIsNil)
	params := bootstrap.BootstrapParams{
		CAPrivateKey:               coretesting.CAKey,
		SSHServerHostKey:           coretesting.SSHServerHostKey,
		ControllerServiceType:      serviceType,
		ControllerExternalName:     externalName,
		ControllerExternalIPs:      externalIps,
		ExtraAgentValuesForTesting: map[string]string{"foo": "bar"},
	}
	err = bootstrap.FinalizePodBootstrapConfig(envtesting.BootstrapTestContext(c), podConfig, params, modelCfg)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(podConfig.Bootstrap.ControllerModelConfig, tc.DeepEquals, modelCfg)
	c.Assert(podConfig.Bootstrap.ControllerServiceType, tc.Equals, serviceType)
	c.Assert(podConfig.Bootstrap.ControllerExternalName, tc.Equals, externalName)
	c.Assert(podConfig.Bootstrap.ControllerExternalIPs, tc.DeepEquals, externalIps)
	c.Assert(podConfig.AgentEnvironment, tc.DeepEquals, map[string]string{"foo": "bar"})
	c.Check(podConfig.Bootstrap.StateInitializationParams.RecoveryMode, tc.IsFalse)
}

// validRecoveryParams describes an archived amd64 controller without any
// fresh-bootstrap registration details or agent-building options.
func validRecoveryParams(archivedVersion semversion.Number) bootstrap.RecoveryParams {
	return bootstrap.RecoveryParams{
		AllocationParams: bootstrap.AllocationParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			BootstrapBase:    jammyBootstrapBase,
		},
		Arch:                       arch.AMD64,
		AgentVersion:               archivedVersion,
		MachineID:                  "7",
		CAPrivateKey:               coretesting.CAKey,
		AdminSecret:                "fake-moon-landing",
		AvailableTools:             recoveryTools(archivedVersion, arch.AMD64),
		BootstrapSSHAuthorizedKeys: []string{"ssh-ed25519 AAAA"},
	}
}

// recoveryTools returns the archived agent tools for the given platform.
func recoveryTools(version semversion.Number, arch string) tools.List {
	return tools.List{{
		Version: semversion.Binary{Number: version, Release: "ubuntu", Arch: arch},
		URL:     "file:///var/lib/juju/tools/juju" + version.String() + "-ubuntu-" + arch + ".tgz",
		SHA256:  "sha256",
		Size:    42,
	}}
}

// failingDataSourceFactory fails the test when used, proving that no
// simplestreams data source is fetched.
type failingDataSourceFactory struct {
	c *tc.C
}

func (f failingDataSourceFactory) NewDataSource(simplestreams.Config) simplestreams.DataSource {
	f.c.Fatalf("packaged tool discovery ran during recovery")
	return nil
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAAS(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return arch.AMD64
	})
	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	env.result = &environs.BootstrapResult{
		InstanceID: "test-instance",
		Arch:       arch.AMD64,
		Base:       jammyBootstrapBase,
		CloudBootstrapFinalizer: func(
			_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts,
		) error {
			env.finalizerCount++
			env.instanceConfig = icfg
			return nil
		},
	}

	archivedVersion := coretesting.FakeVersionNumber

	// Packaged tool discovery must not run during recovery.
	ss := simplestreams.NewSimpleStreams(failingDataSourceFactory{c})
	ctx := envtesting.BootstrapContext(
		context.WithValue(c.Context(), bootstrap.SimplestreamsFetcherContextKey, ss), c)

	// The archived architecture can differ from the client's architecture.
	params := validRecoveryParams(archivedVersion)
	params.Arch = arch.ARM64
	params.AvailableTools = recoveryTools(archivedVersion, arch.ARM64)
	env.result.Arch = arch.ARM64
	id, err := bootstrap.Recover(ctx, env, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, instance.Id("test-instance"))

	// The provider receives the archived tools for image selection and the
	// bootstrap SSH keys before the agent starts.
	c.Check(env.bootstrapCount, tc.Equals, 1)
	c.Check(env.args.AvailableTools, tc.DeepEquals, recoveryTools(archivedVersion, arch.ARM64))
	c.Check(env.args.AuthorizedKeys, tc.DeepEquals, params.BootstrapSSHAuthorizedKeys)
	c.Check(env.args.MachineID, tc.Equals, "7")
	c.Check(env.args.BootstrapBase, tc.Equals, jammyBootstrapBase)
	c.Assert(env.args.BootstrapConstraints.Arch, tc.NotNil)
	c.Check(*env.args.BootstrapConstraints.Arch, tc.Equals, arch.ARM64)
	agentVersion, ok := env.cfg.AgentVersion()
	c.Assert(ok, tc.IsTrue)
	c.Check(agentVersion, tc.Equals, archivedVersion)

	// The provider finaliser installs and starts the archived agent.
	c.Check(env.finalizerCount, tc.Equals, 1)
	c.Assert(env.instanceConfig, tc.NotNil)
	c.Check(env.instanceConfig.MachineId, tc.Equals, "7")
	c.Check(env.instanceConfig.MachineAgentServiceName, tc.Equals, "jujuagentd-machine-7")
	c.Assert(env.instanceConfig.APIInfo, tc.NotNil)
	c.Check(env.instanceConfig.APIInfo.CACert, tc.Equals, coretesting.CACert)
	c.Check(env.instanceConfig.APIInfo.ModelTag.Id(), tc.Equals, env.cfg.UUID())
	c.Check(env.instanceConfig.Bootstrap.StateInitializationParams.AgentVersion,
		tc.Equals, archivedVersion)
	c.Check(env.instanceConfig.Bootstrap.StateInitializationParams.RecoveryMode, tc.IsTrue)
}

// TestBootstrapRecoveryIAASDiscoversPackagedTools checks that when
// AvailableTools is omitted, recovery queries packaged tools for the exact
// archived version and architecture without invoking BuildAgentTarball.
func (s *bootstrapSuite) TestBootstrapRecoveryIAASDiscoversPackagedTools(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	archivedVersion := coretesting.FakeVersionNumber
	packaged := recoveryTools(archivedVersion, arch.ARM64)
	var gotFilter tools.Filter
	s.PatchValue(bootstrap.FindTools, func(
		_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron,
		_, _ int, _ []string, filter tools.Filter,
	) (tools.List, error) {
		gotFilter = filter
		return packaged, nil
	})

	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	env.result = &environs.BootstrapResult{
		InstanceID: "test-instance",
		Arch:       arch.ARM64,
		Base:       jammyBootstrapBase,
		CloudBootstrapFinalizer: func(
			_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts,
		) error {
			env.finalizerCount++
			env.instanceConfig = icfg
			return nil
		},
	}

	params := validRecoveryParams(archivedVersion)
	params.Arch = arch.ARM64
	params.AvailableTools = nil
	params.BuildAgentTarball = func(bool, string, func(semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
		c.Fatalf("BuildAgentTarball must not run when packaged tools are found")
		return nil, nil
	}

	id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, instance.Id("test-instance"))
	c.Check(gotFilter.Number, tc.Equals, archivedVersion)
	c.Check(gotFilter.Arch, tc.Equals, arch.ARM64)
	c.Check(gotFilter.OSType, tc.Equals, "ubuntu")
	c.Check(env.args.AvailableTools, tc.DeepEquals, packaged)
	c.Check(env.finalizerCount, tc.Equals, 1)
}

// TestBootstrapRecoveryIAASFallsBackToLocalAgentBinary checks that when no
// packaged tools match the archived version, recovery validates the host
// platform and packages the local jujuagentd binary with build=false.
func (s *bootstrapSuite) TestBootstrapRecoveryIAASFallsBackToLocalAgentBinary(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	archivedVersion := coretesting.FakeVersionNumber
	s.PatchValue(bootstrap.FindTools, func(
		context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron,
		int, int, []string, tools.Filter,
	) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})

	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	env.result = &environs.BootstrapResult{
		InstanceID: "test-instance",
		Arch:       arch.AMD64,
		Base:       jammyBootstrapBase,
		CloudBootstrapFinalizer: func(
			_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts,
		) error {
			env.finalizerCount++
			env.instanceConfig = icfg
			return nil
		},
	}

	var gotBuild bool
	var gotVersion semversion.Number
	builtDir := c.MkDir()
	params := validRecoveryParams(archivedVersion)
	params.AvailableTools = nil
	params.BuildAgentTarball = func(
		build bool, _ string, getForceVersion func(semversion.Number) semversion.Number,
	) (*sync.BuiltAgent, error) {
		gotBuild = build
		gotVersion = getForceVersion(semversion.Zero)
		return &sync.BuiltAgent{
			Dir:         builtDir,
			StorageName: "juju.tgz",
			Sha256Hash:  "deadbeef",
			Size:        128,
			Version: semversion.Binary{
				Number:  archivedVersion,
				Release: "ubuntu",
				Arch:    arch.AMD64,
			},
		}, nil
	}

	id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, instance.Id("test-instance"))
	c.Check(gotBuild, tc.IsFalse)
	c.Check(gotVersion, tc.Equals, archivedVersion)
	c.Assert(env.args.AvailableTools, tc.HasLen, 1)
	c.Check(env.args.AvailableTools[0].URL, tc.Equals, "file://"+filepath.Join(builtDir, "juju.tgz"))
	c.Check(env.finalizerCount, tc.Equals, 1)
}

// TestBootstrapRecoveryIAASRejectsCrossArchLocalFallback checks that when no
// packaged tools exist and the host architecture differs from the archived
// controller, recovery rejects the local fallback before invoking
// BuildAgentTarball.
func (s *bootstrapSuite) TestBootstrapRecoveryIAASRejectsCrossArchLocalFallback(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	archivedVersion := coretesting.FakeVersionNumber
	s.PatchValue(bootstrap.FindTools, func(
		context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron,
		int, int, []string, tools.Filter,
	) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})

	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	params := validRecoveryParams(archivedVersion)
	params.Arch = arch.ARM64
	params.AvailableTools = nil
	params.BuildAgentTarball = func(bool, string, func(semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
		c.Fatalf("BuildAgentTarball must not run when host arch differs from archived arch")
		return nil, nil
	}

	id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, params)
	c.Check(id, tc.Equals, instance.Id(""))
	c.Check(err, tc.ErrorMatches, `cannot use agent built for "arm64" using a machine running on "amd64"`)
	c.Check(env.bootstrapCount, tc.Equals, 0)
}

// TestBootstrapRecoveryIAASForwardsProvisioningInputs checks that every
// archived allocation input reaches the provider: the constraints, the
// controller config, the storage pools, the placement and the bootstrap image.
// Losing any of them allocates a controller that does not match the source.
func (s *bootstrapSuite) TestBootstrapRecoveryIAASForwardsProvisioningInputs(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	archivedVersion := coretesting.FakeVersionNumber
	env := &recoveryBootstrapEnvironWithRegion{
		recoveryBootstrapEnviron: &recoveryBootstrapEnviron{
			bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
			result: &environs.BootstrapResult{
				InstanceID: "test-instance",
				Arch:       arch.ARM64,
				Base:       jammyBootstrapBase,
				CloudBootstrapFinalizer: func(
					environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts,
				) error {
					return nil
				},
			},
		},
		region: simplestreams.CloudSpec{Region: "north", Endpoint: "https://cloud.example"},
	}

	storagePools := map[string]internalstorage.Attrs{
		"pool-a": {
			corestorage.BootstrapStoragePoolNameKey: "pool-a",
			corestorage.BootstrapStoragePoolTypeKey: "lxd",
			"source":                                "/dev/sda1",
		},
	}
	params := validRecoveryParams(archivedVersion)
	params.Arch = arch.ARM64
	params.AvailableTools = recoveryTools(archivedVersion, arch.ARM64)
	params.Cloud = cloud.Cloud{Name: "lxd", Type: "lxd"}
	params.CloudRegion = "north"
	params.ControllerName = "source-ctrl"
	params.ModelConstraints = constraints.MustParse("cores=4 mem=2048")
	params.BootstrapConstraints = constraints.MustParse(
		"virt-type=virtual-machine mem=4096 zones=zone-a root-disk-source=pool-a")
	params.StoragePools = storagePools
	params.Placement = "zone=zone-a"
	params.BootstrapImage = "images:bootstrap-image"
	// The archived controller space config implies a space constraint, as it
	// does on fresh bootstrap.
	params.ControllerConfig = coretesting.FakeControllerConfig()
	params.ControllerConfig[controller.JujuManagementSpace] = "space-a"

	id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, params)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(id, tc.Equals, instance.Id("test-instance"))

	args := env.args
	c.Check(args.CloudName, tc.Equals, "lxd")
	c.Check(args.CloudRegion, tc.Equals, "north")
	c.Check(args.ControllerConfig[controller.JujuManagementSpace], tc.Equals, "space-a")
	c.Check(args.StoragePools, tc.DeepEquals, storagePools)
	c.Check(args.Placement, tc.Equals, "zone=zone-a")
	c.Check(args.BootstrapBase, tc.Equals, jammyBootstrapBase)
	c.Check(args.SupportedBootstrapBases, tc.DeepEquals, []corebase.Base{jammyBootstrapBase})
	c.Check(args.AuthorizedKeys, tc.DeepEquals, params.BootstrapSSHAuthorizedKeys)
	c.Check(args.MachineID, tc.Equals, "7")

	// The model constraints are passed through untouched.
	c.Assert(args.ModelConstraints.CpuCores, tc.NotNil)
	c.Check(*args.ModelConstraints.CpuCores, tc.Equals, uint64(4))
	c.Assert(args.ModelConstraints.Mem, tc.NotNil)
	c.Check(*args.ModelConstraints.Mem, tc.Equals, uint64(2048))

	// The bootstrap constraints keep every archived attribute, with the
	// archived architecture pinned over them.
	c.Assert(args.BootstrapConstraints.Arch, tc.NotNil)
	c.Check(*args.BootstrapConstraints.Arch, tc.Equals, arch.ARM64)
	c.Assert(args.BootstrapConstraints.VirtType, tc.NotNil)
	c.Check(*args.BootstrapConstraints.VirtType, tc.Equals, "virtual-machine")
	c.Assert(args.BootstrapConstraints.Mem, tc.NotNil)
	c.Check(*args.BootstrapConstraints.Mem, tc.Equals, uint64(4096))
	c.Assert(args.BootstrapConstraints.RootDiskSource, tc.NotNil)
	c.Check(*args.BootstrapConstraints.RootDiskSource, tc.Equals, "pool-a")
	c.Assert(args.BootstrapConstraints.Zones, tc.NotNil)
	c.Check(*args.BootstrapConstraints.Zones, tc.DeepEquals, []string{"zone-a"})
	c.Assert(args.BootstrapConstraints.Spaces, tc.NotNil)
	c.Check(*args.BootstrapConstraints.Spaces, tc.DeepEquals, []string{"space-a"})

	ids := make([]string, 0, len(args.ImageMetadata))
	for _, meta := range args.ImageMetadata {
		ids = append(ids, meta.Id)
	}
	c.Check(ids, tc.DeepEquals, []string{"images:bootstrap-image"})
}

// The archived architecture is authoritative: an archived constraint naming a
// different one is rejected rather than silently provisioning the wrong
// platform.
func (s *bootstrapSuite) TestBootstrapRecoveryIAASRejectsConstraintArchConflict(c *tc.C) {
	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	params := validRecoveryParams(coretesting.FakeVersionNumber)
	params.Arch = arch.AMD64
	params.BootstrapConstraints = constraints.MustParse("arch=arm64")

	id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, params)
	c.Check(id, tc.Equals, instance.Id(""))
	c.Check(err, tc.ErrorMatches,
		`validating recovery parameters: recovery controller architecture "amd64" conflicts with constraint architecture "arm64" not valid`)
	c.Check(env.bootstrapCount, tc.Equals, 0)
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAASResultChecks(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return arch.AMD64
	})
	archivedVersion := coretesting.FakeVersionNumber

	tests := []struct {
		name   string
		result *environs.BootstrapResult
		wantID instance.Id
		err    string
	}{{
		name: "no replacement instance reported",
		result: &environs.BootstrapResult{
			Arch: arch.AMD64,
			Base: jammyBootstrapBase,
		},
		err: "recovery provisioning returned no replacement instance",
	}, {
		name:   "nil bootstrap result",
		result: nil,
		err:    "provider returned no replacement instance details",
	}, {
		name: "no machine controller finaliser",
		result: &environs.BootstrapResult{
			InstanceID: "test-instance",
			Arch:       arch.AMD64,
			Base:       jammyBootstrapBase,
		},
		wantID: "test-instance",
		err:    `replacement instance "test-instance" may require manual removal: provider returned no machine controller finaliser`,
	}, {
		name: "wrong base",
		result: &environs.BootstrapResult{
			InstanceID: "test-instance",
			Arch:       arch.AMD64,
			Base:       focalBootstrapBase,
			CloudBootstrapFinalizer: func(
				environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts,
			) error {
				return nil
			},
		},
		wantID: "test-instance",
		err:    `replacement instance "test-instance" may require manual removal: replacement base differs from archived controller`,
	}, {
		name: "wrong arch",
		result: &environs.BootstrapResult{
			InstanceID: "test-instance",
			Arch:       arch.ARM64,
			Base:       jammyBootstrapBase,
			CloudBootstrapFinalizer: func(
				environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts,
			) error {
				return nil
			},
		},
		wantID: "test-instance",
		err:    `replacement instance "test-instance" may require manual removal: replacement architecture differs from archived agent`,
	}}

	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		env := &recoveryBootstrapEnviron{
			bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
			result:           test.result,
		}
		id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, validRecoveryParams(archivedVersion))
		c.Check(id, tc.Equals, test.wantID)
		c.Check(err, tc.ErrorMatches, test.err)
		c.Check(env.finalizerCount, tc.Equals, 0)
	}
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAASAllocationError(c *tc.C) {
	archivedVersion := coretesting.FakeVersionNumber
	// allocationFailure stands in for a provider error. Providers
	// already name the instance in the error when they allocated one
	// before failing, so recovery must return provider errors
	// unchanged instead of wrapping them a second time.
	allocationFailure := errors.New("allocation failed")

	tests := []struct {
		name string
		err  error
		msg  string
	}{{
		name: "instance allocated before the failure",
		// The provider boundary names the instance it allocated.
		err: fmt.Errorf(`allocating bootstrap instance %q: %w`,
			instance.Id("test-instance"), allocationFailure),
		msg: `allocating bootstrap instance "test-instance": allocation failed`,
	}, {
		name: "no instance allocated",
		err:  allocationFailure,
		msg:  "allocation failed",
	}}
	for i, test := range tests {
		c.Logf("test %d: %s", i, test.name)
		env := &recoveryBootstrapEnviron{
			bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
			err:              test.err,
		}
		id, err := bootstrap.Recover(
			envtesting.BootstrapTestContext(c), env,
			validRecoveryParams(archivedVersion))
		// The provider error arrives unchanged, and no ID is
		// returned: naming any allocated instance is the provider's
		// responsibility.
		c.Check(id, tc.Equals, instance.Id(""))
		c.Check(err, tc.ErrorMatches, test.msg)
		// The original error classification is preserved.
		c.Check(err, tc.ErrorIs, allocationFailure)
		c.Check(env.bootstrapCount, tc.Equals, 1)
	}
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAASCancelled(c *tc.C) {
	archivedVersion := coretesting.FakeVersionNumber
	ctx, cancel := context.WithCancel(c.Context())
	defer cancel()

	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
		// Cancel while the provider is returning the allocated
		// instance, so the cancellation is observed after a valid
		// allocation and finalisation. The hook belongs to the fake
		// alone; production code has no such callback.
		afterAllocate: cancel,
	}
	env.result = &environs.BootstrapResult{
		InstanceID: "test-instance",
		Arch:       arch.AMD64,
		Base:       jammyBootstrapBase,
		CloudBootstrapFinalizer: func(
			environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts,
		) error {
			env.finalizerCount++
			return nil
		},
	}
	id, err := bootstrap.Recover(
		envtesting.BootstrapContext(ctx, c), env, validRecoveryParams(archivedVersion))
	// Cleanup gets the allocated ID without parsing the error message.
	c.Check(id, tc.Equals, instance.Id("test-instance"))
	c.Check(err, tc.ErrorIs, bootstrap.Cancelled())
	c.Check(err, tc.ErrorMatches,
		`replacement instance "test-instance" may require manual removal: cancelled`)
	c.Check(env.finalizerCount, tc.Equals, 1)
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAASFinaliserError(c *tc.C) {
	finaliseErr := errors.New("finalise failed")
	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	env.result = &environs.BootstrapResult{
		InstanceID: "test-instance",
		Arch:       arch.AMD64,
		Base:       jammyBootstrapBase,
		CloudBootstrapFinalizer: func(
			environs.BootstrapContext, *instancecfg.InstanceConfig, environs.BootstrapDialOpts,
		) error {
			env.finalizerCount++
			return finaliseErr
		},
	}
	id, err := bootstrap.Recover(
		envtesting.BootstrapTestContext(c), env,
		validRecoveryParams(coretesting.FakeVersionNumber))
	c.Check(id, tc.Equals, instance.Id("test-instance"))
	c.Check(err, tc.ErrorIs, finaliseErr)
	c.Check(err, tc.ErrorMatches,
		`replacement instance "test-instance" may require manual removal: finalise failed`)
	c.Check(env.finalizerCount, tc.Equals, 1)
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAASPreAllocationFailure(c *tc.C) {
	env := &recoveryBootstrapEnviron{
		bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
	}
	ctx := envtesting.BootstrapContext(context.WithValue(
		c.Context(), bootstrap.SimplestreamsFetcherContextKey, "invalid fetcher"), c)
	id, err := bootstrap.Recover(ctx, env, validRecoveryParams(coretesting.FakeVersionNumber))
	c.Check(id, tc.Equals, instance.Id(""))
	c.Check(err, tc.ErrorMatches, "expected a valid simple streams type")
	c.Check(env.bootstrapCount, tc.Equals, 0)
}

func (s *bootstrapSuite) TestBootstrapRecoveryIAASInvalidParams(c *tc.C) {
	tests := []struct {
		name   string
		mutate func(*bootstrap.RecoveryParams)
		err    string
	}{{
		name:   "no agent version",
		mutate: func(p *bootstrap.RecoveryParams) { p.AgentVersion = semversion.Number{} },
		err:    ".*recovery without an agent version not valid",
	}, {
		name:   "no base",
		mutate: func(p *bootstrap.RecoveryParams) { p.BootstrapBase = corebase.Base{} },
		err:    ".*recovery without a controller base not valid",
	}, {
		name:   "no arch",
		mutate: func(p *bootstrap.RecoveryParams) { p.Arch = "" },
		err:    `.*recovery controller architecture "" not valid`,
	}, {
		name:   "unsupported arch",
		mutate: func(p *bootstrap.RecoveryParams) { p.Arch = "invalid" },
		err:    `.*recovery controller architecture "invalid" not valid`,
	}, {
		name:   "no machine ID",
		mutate: func(p *bootstrap.RecoveryParams) { p.MachineID = "" },
		err:    `.*recovery machine ID "" not valid`,
	}, {
		name:   "invalid machine ID",
		mutate: func(p *bootstrap.RecoveryParams) { p.MachineID = "not-a-machine" },
		err:    `.*recovery machine ID "not-a-machine" not valid`,
	}, {
		name:   "no CA private key",
		mutate: func(p *bootstrap.RecoveryParams) { p.CAPrivateKey = "" },
		err:    ".*recovery without a CA private key not valid",
	}, {
		name:   "no admin secret",
		mutate: func(p *bootstrap.RecoveryParams) { p.AdminSecret = "" },
		err:    ".*recovery without an admin secret not valid",
	}, {
		name: "no tools or builder",
		mutate: func(p *bootstrap.RecoveryParams) {
			p.AvailableTools = nil
			p.BuildAgentTarball = nil
		},
		err: ".*recovery without agent tools or builder not valid",
	}, {
		name: "nil tool entry",
		mutate: func(p *bootstrap.RecoveryParams) {
			p.AvailableTools = tools.List{nil}
		},
		err: ".*recovery with nil agent tool entry not valid",
	}, {
		name: "mismatched tool version",
		mutate: func(p *bootstrap.RecoveryParams) {
			p.AvailableTools = recoveryTools(semversion.MustParse("9.9.9"), arch.AMD64)
		},
		err: `.*recovery tool version .* not matching agent version .* not valid`,
	}, {
		name:   "no controller UUID",
		mutate: func(p *bootstrap.RecoveryParams) { delete(p.ControllerConfig, "controller-uuid") },
		err:    ".*controller configuration has no controller UUID",
	}, {
		name:   "no CA certificate",
		mutate: func(p *bootstrap.RecoveryParams) { delete(p.ControllerConfig, "ca-cert") },
		err:    ".*controller configuration has no ca-cert",
	}, {
		name:   "Kubernetes allocation",
		mutate: func(p *bootstrap.RecoveryParams) { p.Cloud.Type = cloud.CloudTypeKubernetes },
		err:    ".*recovery of a Kubernetes controller not supported",
	}}
	for _, test := range tests {
		c.Logf("test: %s", test.name)
		env := &recoveryBootstrapEnviron{
			bootstrapEnviron: newEnviron("foo", useDefaultKeys, nil),
		}
		params := validRecoveryParams(coretesting.FakeVersionNumber)
		test.mutate(&params)
		id, err := bootstrap.Recover(envtesting.BootstrapTestContext(c), env, params)
		c.Check(id, tc.Equals, instance.Id(""))
		c.Check(err, tc.ErrorMatches, test.err)
		c.Check(env.bootstrapCount, tc.Equals, 0)
	}
}

func (s *bootstrapSuite) TestBootstrapImage(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "nether",
			Endpoint: "hearnoretheir",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	bootstrapCons := constraints.MustParse("arch=amd64")
	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			BootstrapImage:          "img-id",
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
			BootstrapConstraints:    bootstrapCons,
			MetadataDir:             metadataDir,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.args.ImageMetadata, tc.HasLen, 1)
	c.Assert(env.args.ImageMetadata[0], tc.DeepEquals, &imagemetadata.ImageMetadata{
		Id:         "img-id",
		Arch:       "amd64",
		Version:    "22.04",
		RegionName: "nether",
		Endpoint:   "hearnoretheir",
		Stream:     "released",
	})
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata, tc.HasLen, 2)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[0], tc.DeepEquals, metadata[0])
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[1], tc.DeepEquals, env.args.ImageMetadata[0])
	expectedCons := bootstrapCons
	expectedCons.Mem = new(uint64(3584))
	c.Assert(env.instanceConfig.Bootstrap.BootstrapMachineConstraints, tc.DeepEquals, expectedCons)
	c.Assert(env.instanceConfig.Bootstrap.ControllerModelEnvironVersion, tc.Equals, 123)
}

func (s *bootstrapSuite) TestBootstrapAddsArchFromImageToExistingProviderSupportedArches(c *tc.C) {
	data := s.setupImageMetadata(c)
	env := s.setupProviderWithSomeSupportedArches(c)
	// Even though test provider does not explicitly support architecture used by this test,
	// the fact that we have an image for it, adds this architecture to those supported by provider.
	// Bootstrap should succeed with no failures as constraints validator used internally
	// would have both provider supported architectures and architectures retrieved from images metadata.
	bootstrapCons := constraints.MustParse(fmt.Sprintf("arch=%v", data.architecture))
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			BootstrapImage:          "img-id",
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
			BootstrapConstraints:    bootstrapCons,
			MetadataDir:             data.metadataDir,
		})
	c.Assert(err, tc.ErrorIsNil)
	expectedCons := bootstrapCons
	expectedCons.Mem = new(uint64(3584))
	s.assertBootstrapImageMetadata(c, env.bootstrapEnviron, data, expectedCons)
}

type testImageMetadata struct {
	architecture string
	metadataDir  string
	metadata     []*imagemetadata.ImageMetadata
}

// setupImageMetadata returns architecture for which metadata was setup
func (s *bootstrapSuite) setupImageMetadata(c *tc.C) testImageMetadata {
	testArch := arch.S390X
	s.PatchValue(&arch.HostArch, func() string { return testArch })

	metadataDir, metadata := createImageMetadataForArch(c, testArch)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	return testImageMetadata{testArch, metadataDir, metadata}
}

func (s *bootstrapSuite) assertBootstrapImageMetadata(c *tc.C, env *bootstrapEnviron, testData testImageMetadata, bootstrapCons constraints.Value) {
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.args.ImageMetadata, tc.HasLen, 1)
	c.Assert(env.args.ImageMetadata[0], tc.DeepEquals, &imagemetadata.ImageMetadata{
		Id:         "img-id",
		Arch:       testData.architecture,
		Version:    "22.04",
		RegionName: "nether",
		Endpoint:   "hearnoretheir",
		Stream:     "released",
	})
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata, tc.HasLen, 2)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[0], tc.DeepEquals, testData.metadata[0])
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[1], tc.DeepEquals, env.args.ImageMetadata[0])
	c.Assert(env.instanceConfig.Bootstrap.BootstrapMachineConstraints, tc.DeepEquals, bootstrapCons)

}

func (s *bootstrapSuite) setupProviderWithSomeSupportedArches(c *tc.C) bootstrapEnvironWithRegion {
	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "nether",
			Endpoint: "hearnoretheir",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	// test provider constraints only has amd64 and arm64 as supported architectures
	consBefore, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	desiredArch := constraints.MustParse("arch=s390x")
	unsupported, err := consBefore.Validate(desiredArch)
	c.Assert(err.Error(), tc.Contains, `invalid constraint value: arch=s390x`)
	c.Assert(unsupported, tc.HasLen, 0)

	return env
}

func (s *bootstrapSuite) TestBootstrapAddsArchFromImageToProviderWithNoSupportedArches(c *tc.C) {
	data := s.setupImageMetadata(c)
	env := s.setupProviderWithNoSupportedArches(c)
	// Even though test provider does not explicitly support architecture used by this test,
	// the fact that we have an image for it, adds this architecture to those supported by provider.
	// Bootstrap should succeed with no failures as constraints validator used internally
	// would have both provider supported architectures and architectures retrieved from images metadata.
	bootstrapCons := constraints.MustParse(fmt.Sprintf("arch=%v", data.architecture))
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			BootstrapImage:          "img-id",
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
			BootstrapConstraints:    bootstrapCons,
			MetadataDir:             data.metadataDir,
		})
	c.Assert(err, tc.ErrorIsNil)
	expectedCons := bootstrapCons
	expectedCons.Mem = new(uint64(3584))
	s.assertBootstrapImageMetadata(c, env.bootstrapEnviron, data, expectedCons)
}

func (s *bootstrapSuite) setupProviderWithNoSupportedArches(c *tc.C) bootstrapEnvironNoExplicitArchitectures {
	env := bootstrapEnvironNoExplicitArchitectures{
		&bootstrapEnvironWithRegion{
			newEnviron("foo", useDefaultKeys, nil),
			simplestreams.CloudSpec{
				Region:   "nether",
				Endpoint: "hearnoretheir",
			},
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	consBefore, err := env.ConstraintsValidator(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	// test provider constraints only has amd64 and arm64 as supported architectures
	desiredArch := constraints.MustParse("arch=s390x")
	unsupported, err := consBefore.Validate(desiredArch)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unsupported, tc.HasLen, 0)

	return env
}

// TestBootstrapImageMetadataFromAllSources tests that we are looking for
// image metadata in all data sources available to environment.
// Abandoning look up too soon led to misleading bootstrap failures:
// Juju reported no images available for a particular configuration,
// despite image metadata in other data sources compatible with the same configuration as well.
// Related to bug#1560625.
func (s *bootstrapSuite) TestBootstrapImageMetadataFromAllSources(c *tc.C) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer server.Close()

	s.PatchValue(&imagemetadata.DefaultUbuntuBaseURL, server.URL)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	// Ensure that we can find at least one image metadata
	// early on in the image metadata lookup.
	// We should continue looking despite it.
	metadataDir, _ := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "region",
			Endpoint: "endpoint",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	ctx, ss := bootstrapContext(c)
	bootstrapCons := constraints.MustParse("arch=amd64")
	err = bootstrap.Bootstrap(ctx, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			BootstrapConstraints:    bootstrapCons,
			MetadataDir:             metadataDir,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	datasources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	for _, source := range datasources {
		_ = source
		//	// make sure we looked in each and all...
		//	c.Assert(c.GetTestLog(), tc.Contains, fmt.Sprintf("image metadata in %s", source.Description()))
	}
}

func (s *bootstrapSuite) TestBootstrapLocalTools(c *tc.C) {
	// Client host is CentOS system, wanting to bootstrap a trusty
	// controller. This is fine.

	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.CentOS })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, string, func(localBinaryVersion semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(env.bootstrapCount, tc.Equals, 1)
	c.Check(env.args.BootstrapBase, tc.Equals, jammyBootstrapBase)
	c.Check(env.args.AvailableTools.AllReleases(), tc.SameContents, []string{"ubuntu"})
}

func (s *bootstrapSuite) TestBootstrapLocalToolsMismatchingOS(c *tc.C) {
	// Client host is a Windows system, wanting to bootstrap a jammy
	// controller with local tools. This can't work.

	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.Windows })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, string, func(localBinaryVersion semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorMatches, `cannot use agent built for "ubuntu@22.04/stable" using a machine running "Windows"`)
}

func (s *bootstrapSuite) TestBootstrapLocalToolsDifferentLinuxes(c *tc.C) {
	// Client host is some unspecified Linux system, wanting to
	// bootstrap a trusty controller with local tools. This should be
	// OK.

	s.PatchValue(&jujuos.HostOS, func() ostype.OSType { return ostype.GenericLinux })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, string, func(localBinaryVersion semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(env.bootstrapCount, tc.Equals, 1)
	c.Check(env.args.BootstrapBase, tc.Equals, jammyBootstrapBase)
	c.Check(env.args.AvailableTools.AllReleases(), tc.SameContents, []string{"ubuntu"})
}

func (s *bootstrapSuite) TestBootstrapBuildAgentOfficial(c *tc.C) {
	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		c.Fatal("should not call FindTools if BuildAgent is specified")
		return nil, errors.NotFoundf("tools")
	})

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			BuildAgent:       true,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(build bool, _ string,
				getForceVersion func(semversion.Number) semversion.Number,
			) (*sync.BuiltAgent, error) {
				ver := getForceVersion(semversion.Zero)
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, tc.IsTrue)
				c.Assert(ver.String(), tc.Equals, "2.99.0.1")
				localVer := ver
				return &sync.BuiltAgent{
					Dir:      c.MkDir(),
					Official: true,
					Version: semversion.Binary{
						// If we found an official build we suppress the build number.
						Number:  localVer.ToPatch(),
						Release: "ubuntu",
						Arch:    "arm64",
					},
				}, nil
			},
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	// Check that the model config has the correct version set.
	cfg := env.instanceConfig.Bootstrap.ControllerModelConfig
	agentVersion, valid := cfg.AgentVersion()
	c.Check(valid, tc.IsTrue)
	c.Check(agentVersion.String(), tc.Equals, "2.99.0")
}

func (s *bootstrapSuite) TestBootstrapBuildAgentDevel(c *tc.C) {
	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		c.Fatal("should not call FindTools if BuildAgent is specified")
		return nil, errors.NotFoundf("tools")
	})
	s.PatchValue(&jujuversion.Grade, "devel")

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			BuildAgent:       true,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(build bool, _ string,
				getForceVersion func(semversion.Number) semversion.Number,
			) (*sync.BuiltAgent, error) {
				ver := getForceVersion(semversion.Zero)
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, tc.IsTrue)
				c.Assert(ver.String(), tc.Equals, "2.99.0.1")
				localVer := ver
				return &sync.BuiltAgent{
					Dir:      c.MkDir(),
					Official: true,
					Version: semversion.Binary{
						// If we found an official build we suppress the build number.
						Number:  localVer.ToPatch(),
						Release: "ubuntu",
						Arch:    "arm64",
					},
				}, nil
			},
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	// Check that the model config has the correct version set.
	cfg := env.instanceConfig.Bootstrap.ControllerModelConfig
	agentVersion, valid := cfg.AgentVersion()
	c.Check(valid, tc.IsTrue)
	c.Check(agentVersion.String(), tc.Equals, "2.99.0.1")
}

func (s *bootstrapSuite) assertBootstrapPackagedToolsAvailable(c *tc.C, clientArch string) {
	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return clientArch })
	toolsArch := clientArch
	findToolsOk := false
	s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher, _ environs.BootstrapEnviron, _ int, _ int, _ []string, filter tools.Filter) (tools.List, error) {
		c.Assert(filter.Arch, tc.Equals, toolsArch)
		c.Assert(filter.OSType, tc.Equals, "ubuntu")
		findToolsOk = true
		vers := semversion.Binary{
			Number:  jujuversion.Current,
			Release: "ubuntu",
			Arch:    toolsArch,
		}
		return tools.List{{
			Version: vers,
		}}, nil
	})

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			ControllerConfig:        coretesting.FakeControllerConfig(),
			BootstrapBase:           jammyBootstrapBase,
			SupportedBootstrapBases: supportedJujuBases,
			BuildAgentTarball: func(bool, string, func(localBinaryVersion semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
				c.Fatal("should not call BuildAgentTarball if there are packaged tools")
				return nil, nil
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(findToolsOk, tc.IsTrue)
}

func (s *bootstrapSuite) TestBootstrapPackagedTools(c *tc.C) {
	for _, a := range arch.AllSupportedArches {
		s.assertBootstrapPackagedToolsAvailable(c, a)
	}
}

func (s *bootstrapSuite) TestBootstrapNoToolsNonReleaseStream(c *tc.C) {
	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]any{
		"agent-stream": "proposed"})
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, string, func(localBinaryVersion semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapBases: supportedJujuBases,
		})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapNoToolsDevelopmentConfig(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]any{
		"development": true})
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			BuildAgentTarball: func(bool, string, func(localBinaryVersion semversion.Number) semversion.Number) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapBases: supportedJujuBases,
		})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, tc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapToolsVersion(c *tc.C) {
	availableVersions := []semversion.Binary{
		semversion.MustParseBinary("1.18.0-ubuntu-arm64"),
		semversion.MustParseBinary("1.18.1-ubuntu-arm64"),
		semversion.MustParseBinary("1.18.1.1-ubuntu-arm64"),
		semversion.MustParseBinary("1.18.1.2-ubuntu-arm64"),
		semversion.MustParseBinary("1.18.1.3-ubuntu-arm64"),
	}
	availableTools := make(tools.List, len(availableVersions))
	for i, v := range availableVersions {
		availableTools[i] = &tools.Tools{Version: v}
	}

	type test struct {
		currentVersion semversion.Number
		expectedTools  semversion.Number
	}
	tests := []test{{
		currentVersion: semversion.MustParse("1.18.0"),
		expectedTools:  semversion.MustParse("1.18.0"),
	}, {
		currentVersion: semversion.MustParse("1.18.1.4"),
		expectedTools:  semversion.MustParse("1.18.1.3"),
	}, {
		// build number is ignored unless major/minor don't
		// match the latest.
		currentVersion: semversion.MustParse("1.18.1.2"),
		expectedTools:  semversion.MustParse("1.18.1.3"),
	}, {
		// If the current patch level exceeds whatever's in
		// the tools source (e.g. when bootstrapping from trunk)
		// then the latest available tools will be chosen.
		currentVersion: semversion.MustParse("1.18.2"),
		expectedTools:  semversion.MustParse("1.18.1.3"),
	}}

	env := newEnviron("foo", useDefaultKeys, nil)
	for i, t := range tests {
		c.Logf("test %d: %+v", i, t)
		cfg, err := env.Config().Remove([]string{"agent-version"})
		c.Assert(err, tc.ErrorIsNil)
		err = env.SetConfig(c.Context(), cfg)
		c.Assert(err, tc.ErrorIsNil)
		s.PatchValue(&jujuversion.Current, t.currentVersion)
		tools, err := bootstrap.GetBootstrapToolsVersion(c.Context(), availableTools)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(tools, tc.Not(tc.HasLen), 0)
		toolsVersion, _ := tools.Newest()
		c.Assert(toolsVersion, tc.Equals, t.expectedTools)
	}
}

func (s *bootstrapSuite) TestBootstrapControllerCharmLocal(c *tc.C) {
	path := testcharms.RepoForSeries("quantal").CharmDir("juju-controller").Path
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(environscmd.BootstrapContext(c.Context(), ctx), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			SupportedBootstrapBases: supportedJujuBases,
			ControllerCharmPath:     path,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCharm, tc.Equals, path)
}

func (s *bootstrapSuite) TestBootstrapControllerCharmChannel(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	ch := charm.Channel{Track: "3.0", Risk: "beta"}
	err := bootstrap.Bootstrap(environscmd.BootstrapContext(c.Context(), ctx), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			SupportedBootstrapBases: supportedJujuBases,
			ControllerCharmChannel:  ch,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCharmChannel, tc.Equals, ch)
}

// createImageMetadata creates some image metadata in a local directory.
func createImageMetadata(c *tc.C) (dir string, _ []*imagemetadata.ImageMetadata) {
	return createImageMetadataForArch(c, "amd64")
}

// createImageMetadataForArch creates some image metadata in a local directory for
// specified arch.
func createImageMetadataForArch(c *tc.C, arch string) (dir string, _ []*imagemetadata.ImageMetadata) {
	// Generate some image metadata.
	im := []*imagemetadata.ImageMetadata{{
		Id:         "1234",
		Arch:       arch,
		Version:    "22.04",
		RegionName: "region",
		Endpoint:   "endpoint",
	}}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	sourceDir := c.MkDir()
	sourceStor, err := filestorage.NewFileStorageWriter(sourceDir)
	c.Assert(err, tc.ErrorIsNil)
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	base := corebase.MustParseBaseFromString("ubuntu@22.04")
	err = imagemetadata.MergeAndWriteMetadata(c.Context(), ss, base, im, cloudSpec, sourceStor)
	c.Assert(err, tc.ErrorIsNil)
	return sourceDir, im
}

// TestBootstrapMetadata tests:
// `juju bootstrap --metadata-source <dir>` where <dir>/images
// and <dir>/tools exist
func (s *bootstrapSuite) TestBootstrapMetadata(c *tc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	ctx, ss := bootstrapContext(c)
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(ctx, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			MetadataDir:             metadataDir,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(envtools.DefaultBaseURL, tc.Equals, metadataDir)

	datasources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(datasources, tc.HasLen, 2)
	c.Assert(datasources[0].Description(), tc.Equals, "bootstrap metadata")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a user provided key.
	// for this test, user provided key is empty.
	// Bugs #1542127, #1542131
	c.Assert(datasources[0].PublicSigningKey(), tc.Equals, "")
	c.Assert(env.instanceConfig, tc.NotNil)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata, tc.HasLen, 1)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[0], tc.DeepEquals, metadata[0])
}

func (s *bootstrapSuite) TestBootstrapMetadataDirNonexistend(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	nonExistentFileName := "/tmp/TestBootstrapMetadataDirNonexistend"
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			MetadataDir:             nonExistentFileName,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.NotNil)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("simplestreams metadata source: %s not found", nonExistentFileName))
}

// TestBootstrapMetadataImagesNoTools tests 2 cases:
// juju bootstrap --metadata-source <dir>
// juju bootstrap --metadata-source <dir>/images
// where <dir>/tools doesn't exist
func (s *bootstrapSuite) TestBootstrapMetadataImagesNoTools(c *tc.C) {

	metadataDir, _ := createImageMetadata(c)
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)

	startingDefaultBaseURL := envtools.DefaultBaseURL
	for i, suffix := range []string{"", "images"} {
		environs.UnregisterImageDataSourceFunc("bootstrap metadata")

		ctx, ss := bootstrapContext(c)
		err := bootstrap.Bootstrap(ctx, env,
			bootstrap.BootstrapParams{
				ControllerConfig:        coretesting.FakeControllerConfig(),
				AdminSecret:             "admin-secret",
				CAPrivateKey:            coretesting.CAKey,
				SSHServerHostKey:        coretesting.SSHServerHostKey,
				MetadataDir:             filepath.Join(metadataDir, suffix),
				SupportedBootstrapBases: supportedJujuBases,
			})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(env.bootstrapCount, tc.Equals, i+1)
		c.Assert(envtools.DefaultBaseURL, tc.Equals, startingDefaultBaseURL)

		datasources, err := environs.ImageMetadataSources(env, ss)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(datasources, tc.HasLen, 2)
		c.Assert(datasources[0].Description(), tc.Equals, "bootstrap metadata")
	}
}

// TestBootstrapMetadataToolsNoImages tests 2 cases:
// juju bootstrap --metadata-source <dir>
// juju bootstrap --metadata-source <dir>/tools
// where <dir>/images doesn't exist
func (s *bootstrapSuite) TestBootstrapMetadataToolsNoImages(c *tc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	metadataDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	for i, suffix := range []string{"", "tools"} {
		ctx, ss := bootstrapContext(c)
		err = bootstrap.Bootstrap(ctx, env,
			bootstrap.BootstrapParams{
				ControllerConfig:        coretesting.FakeControllerConfig(),
				AdminSecret:             "admin-secret",
				CAPrivateKey:            coretesting.CAKey,
				SSHServerHostKey:        coretesting.SSHServerHostKey,
				MetadataDir:             filepath.Join(metadataDir, suffix),
				SupportedBootstrapBases: supportedJujuBases,
			})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(env.bootstrapCount, tc.Equals, i+1)
		c.Assert(envtools.DefaultBaseURL, tc.Equals, metadataDir)

		datasources, err := environs.ImageMetadataSources(env, ss)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(datasources, tc.HasLen, 1)
		c.Assert(datasources[0].Description(), tc.Not(tc.Equals), "bootstrap metadata")
	}
}

func (s *bootstrapSuite) TestBootstrapCloudCredential(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	credential := cloud.NewCredential(cloud.EmptyAuthType, map[string]string{"what": "ever"})
	args := bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      "admin-secret",
		CAPrivateKey:     coretesting.CAKey,
		SSHServerHostKey: coretesting.SSHServerHostKey,
		Cloud: cloud.Cloud{
			Name:      "cloud-name",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions:   []cloud.Region{{Name: "region-name"}},
		},
		CloudRegion:             "region-name",
		CloudCredentialName:     "credential-name",
		CloudCredential:         &credential,
		SupportedBootstrapBases: supportedJujuBases,
	}
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env, args)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.instanceConfig, tc.NotNil)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloud, tc.DeepEquals, args.Cloud)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloudRegion, tc.DeepEquals, args.CloudRegion)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloudCredential, tc.DeepEquals, args.CloudCredential)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloudCredentialName, tc.DeepEquals, args.CloudCredentialName)
}

func (s *bootstrapSuite) TestPublicKeyEnvVar(c *tc.C) {
	path := filepath.Join(c.MkDir(), "key")
	os.WriteFile(path, []byte("publickey"), 0644)
	s.PatchEnvironment("JUJU_STREAMS_PUBLICKEY_FILE", path)

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.instanceConfig.PublicImageSigningKey, tc.Equals, "publickey")
}

func (s *bootstrapSuite) TestFinishBootstrapConfig(c *tc.C) {
	path := filepath.Join(c.MkDir(), "key")
	os.WriteFile(path, []byte("publickey"), 0644)
	s.PatchEnvironment("JUJU_STREAMS_PUBLICKEY_FILE", path)

	password := "lisboan-pork"

	dummyCloud := cloud.Cloud{
		Name: "dummy",
		RegionConfig: cloud.RegionConfig{
			"a-region": cloud.Attrs{
				"a-key": "a-value",
			},
			"b-region": cloud.Attrs{
				"b-key": "b-value",
			},
		},
	}

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig:          coretesting.FakeControllerConfig(),
			ControllerInheritedConfig: map[string]any{"ftp-proxy": "http://proxy"},
			Cloud:                     dummyCloud,
			AdminSecret:               password,
			CAPrivateKey:              coretesting.CAKey,
			SSHServerHostKey:          coretesting.SSHServerHostKey,
			SupportedBootstrapBases:   supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	icfg := env.instanceConfig

	c.Check(icfg.APIInfo, tc.DeepEquals, &api.Info{
		Password: password,
		CACert:   coretesting.CACert,
		ModelTag: coretesting.ModelTag,
	})
	c.Check(icfg.Bootstrap.ControllerInheritedConfig, tc.DeepEquals, map[string]any{"ftp-proxy": "http://proxy"})
	c.Check(icfg.Bootstrap.RegionInheritedConfig, tc.DeepEquals, cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"a-key": "a-value",
		},
		"b-region": cloud.Attrs{
			"b-key": "b-value",
		},
	})
	controllerCfg := icfg.ControllerConfig
	c.Check(controllerCfg["ca-private-key"], tc.IsNil)
	c.Check(icfg.Bootstrap.ControllerAgentInfo.APIPort, tc.Equals, controllerCfg.APIPort())
	c.Check(icfg.Bootstrap.ControllerAgentInfo.CAPrivateKey, tc.Equals, coretesting.CAKey)
}

func (s *bootstrapSuite) TestBootstrapMetadataImagesMissing(c *tc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	noImagesDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(noImagesDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	ctx, ss := bootstrapContext(c)

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(ctx, env,
		bootstrap.BootstrapParams{
			ControllerConfig:        coretesting.FakeControllerConfig(),
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			MetadataDir:             noImagesDir,
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)

	datasources, err := environs.ImageMetadataSources(env, ss)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(datasources, tc.HasLen, 1)
	c.Assert(datasources[0].Description(), tc.Equals, "default ubuntu cloud images")
}

func (s *bootstrapSuite) setupBootstrapSpecificVersion(c *tc.C, clientMajor, clientMinor int, toolsVersion *semversion.Number) (error, int, semversion.Number) {
	currentVersion := jujuversion.Current
	currentVersion.Major = clientMajor
	currentVersion.Minor = clientMinor
	currentVersion.Tag = ""
	s.PatchValue(&jujuversion.Current, currentVersion)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		return storage.NewStorageSimpleStreamsDataSource("test datasource", env.storage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	toolsBinaries := []semversion.Binary{
		semversion.MustParseBinary("10.11.12-ubuntu-amd64"),
		semversion.MustParseBinary("10.11.13-ubuntu-amd64"),
		semversion.MustParseBinary("10.11-beta1-ubuntu-amd64"),
	}
	stream := "released"
	if toolsVersion != nil && toolsVersion.Tag != "" {
		stream = "devel"
		currentVersion.Tag = toolsVersion.Tag
	}
	_, err := envtesting.UploadFakeToolsVersions(c, env.storage, stream, toolsBinaries...)
	c.Assert(err, tc.ErrorIsNil)

	env.checkToolsFunc = func(t tools.List) {
		mockInstanceCfg := &instancecfg.InstanceConfig{}
		// All providers call SetTools on instance config during StartInstance
		// (which is called by Bootstrap). Checking here that the call will pass.
		err := mockInstanceCfg.SetTools(t)
		c.Assert(err, tc.ErrorIsNil)
	}

	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			AgentVersion:     toolsVersion,
			BuildAgentTarball: func(
				build bool, _ string,
				getForceVersion func(semversion.Number) semversion.Number,
			) (*sync.BuiltAgent, error) {
				ver := getForceVersion(semversion.Zero)
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, tc.IsFalse)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapBases: supportedJujuBases,
		})
	vers, _ := env.cfg.AgentVersion()
	return err, env.bootstrapCount, vers
}

func (s *bootstrapSuite) TestBootstrapSpecificVersion(c *tc.C) {
	toolsVersion := semversion.MustParse("10.11.12")
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, &toolsVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bootstrapCount, tc.Equals, 1)
	c.Assert(vers, tc.DeepEquals, semversion.Number{
		Major: 10,
		Minor: 11,
		Patch: 12,
	})
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionWithTag(c *tc.C) {
	toolsVersion := semversion.MustParse("10.11-beta1")
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, &toolsVersion)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bootstrapCount, tc.Equals, 1)
	c.Assert(vers, tc.DeepEquals, semversion.Number{
		Major: 10,
		Minor: 11,
		Patch: 1,
		Tag:   "beta",
	})
}

func (s *bootstrapSuite) TestBootstrapNoSpecificVersion(c *tc.C) {
	// bootstrap with no specific version will use latest major.minor tools version.
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(bootstrapCount, tc.Equals, 1)
	c.Assert(vers, tc.DeepEquals, semversion.Number{
		Major: 10,
		Minor: 11,
		Patch: 13,
	})
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMinorMismatch(c *tc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := semversion.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 10, 1, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), tc.Matches, ".* no agent binaries are available .*")
	c.Assert(bootstrapCount, tc.Equals, 0)
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMajorMismatch(c *tc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := semversion.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 1, 11, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), tc.Matches, ".* no agent binaries are available .*")
	c.Assert(bootstrapCount, tc.Equals, 0)
}

func (s *bootstrapSuite) TestAvailableToolsInvalidArch(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return arch.S390X
	})
	s.PatchValue(bootstrap.FindTools, func(context.Context, envtools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		c.Fatal("find packaged tools should not be called")
		return nil, errors.New("unexpected")
	})

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			BuildAgent:       true,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			SSHServerHostKey: coretesting.SSHServerHostKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(
				build bool, _ string,
				getForceVersion func(semversion.Number) semversion.Number,
			) (*sync.BuiltAgent, error) {
				ver := getForceVersion(semversion.Zero)
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, tc.IsTrue)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			SupportedBootstrapBases: supportedJujuBases,
		})
	c.Assert(err, tc.ErrorMatches, `model "foo" of type dummy does not support instances running on "s390x"`)
}

// Detected model constraints must survive allocation preparation and reach
// state initialisation. Local LXD asks for this when no arch was specified.
func (s *bootstrapSuite) TestDetectedModelArchitecture(c *tc.C) {
	for _, explicitArch := range []bool{false, true} {
		c.Logf("explicit model architecture: %v", explicitArch)
		env := &modelConstraintsEnviron{
			newBootstrapEnvironWithHardwareDetection("foo", jammyBootstrapBase, arch.ARM64, useDefaultKeys, nil),
		}
		modelCons := constraints.Value{}
		wantArch := arch.ARM64
		if explicitArch {
			modelCons = constraints.MustParse("arch=amd64")
			wantArch = arch.AMD64
		}
		s.PatchValue(bootstrap.FindTools, func(_ context.Context, _ envtools.SimplestreamsFetcher,
			_ environs.BootstrapEnviron, _, _ int, _ []string, filter tools.Filter,
		) (tools.List, error) {
			c.Check(filter.Arch, tc.Equals, wantArch)
			return tools.List{{Version: semversion.Binary{
				Number: coretesting.FakeVersionNumber, Release: "ubuntu", Arch: wantArch,
			}, URL: "file:///unused-agent.tgz"}}, nil
		})
		err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env, bootstrap.BootstrapParams{
			AdminSecret:             "admin-secret",
			CAPrivateKey:            coretesting.CAKey,
			ControllerConfig:        coretesting.FakeControllerConfig(),
			SupportedBootstrapBases: supportedJujuBases,
			ModelConstraints:        modelCons,
		})
		c.Assert(err, tc.ErrorIsNil)
		want := constraints.Value{Arch: &wantArch}
		c.Check(env.args.ModelConstraints, tc.DeepEquals, want)
		c.Check(env.args.BootstrapBase, tc.Equals, jammyBootstrapBase)
		c.Assert(env.instanceConfig, tc.NotNil)
		c.Check(env.instanceConfig.Bootstrap.ModelConstraints, tc.DeepEquals, want)
		c.Check(env.instanceConfig.Base, tc.Equals, jammyBootstrapBase)
		if !explicitArch {
			c.Check(modelCons.Arch, tc.IsNil)
		}
	}
}

func (s *bootstrapSuite) TestTargetSeriesOverride(c *tc.C) {
	env := newBootstrapEnvironWithHardwareDetection("foo", corebase.MustParseBaseFromString("ubuntu@17.10"), "amd64", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:             "fake-moon-landing",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			ControllerConfig:        coretesting.FakeControllerConfig(),
			SupportedBootstrapBases: supportedJujuBases,
		})

	c.Assert(err, tc.ErrorMatches, ".*ubuntu@17.10/stable not supported.*", tc.Commentf("expected bootstrap series to be overridden using the value returned by the environment"))
}

func (s *bootstrapSuite) TestTargetArchOverride(c *tc.C) {
	env := newBootstrapEnvironWithHardwareDetection("foo", corebase.MustParseBaseFromString("ubuntu@18.04"), "riscv", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:             "fake-moon-landing",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			ControllerConfig:        coretesting.FakeControllerConfig(),
			SupportedBootstrapBases: supportedJujuBases,
			BuildAgentTarball: func(
				build bool, _ string,
				getForceVersion func(semversion.Number) semversion.Number,
			) (*sync.BuiltAgent, error) {
				ver := getForceVersion(semversion.Zero)
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, tc.IsTrue)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
		})

	c.Assert(err, tc.ErrorMatches, "(?s)invalid constraint value: arch=riscv.*", tc.Commentf("expected bootstrap arch to be overridden using the value returned by the environment"))
}

func (s *bootstrapSuite) TestTargetSeriesAndArchOverridePriority(c *tc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return arch.AMD64
	})
	metadataDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, metadataDir)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, tc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released")

	env := newBootstrapEnvironWithHardwareDetection("foo", corebase.MustParseBaseFromString("ubuntu@17.04"), "riscv", useDefaultKeys, nil)
	err = bootstrap.Bootstrap(envtesting.BootstrapTestContext(c), env,
		bootstrap.BootstrapParams{
			AdminSecret:             "fake-moon-landing",
			CAPrivateKey:            coretesting.CAKey,
			SSHServerHostKey:        coretesting.SSHServerHostKey,
			ControllerConfig:        coretesting.FakeControllerConfig(),
			SupportedBootstrapBases: supportedJujuBases,
			BuildAgentTarball: func(
				build bool, _ string,
				getForceVersion func(semversion.Number) semversion.Number,
			) (*sync.BuiltAgent, error) {
				ver := getForceVersion(semversion.Zero)
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, tc.IsTrue)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			// Operator provided constraints must always supersede
			// any values reported by the environment.
			BootstrapBase:        bionicBootstrapBase,
			BootstrapConstraints: constraints.MustParse("arch=amd64"),
			MetadataDir:          metadataDir,
		})

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapEnviron.instanceConfig.ToolsList().String(), tc.Matches, ".*-ubuntu-amd64", tc.Commentf("expected bootstrap constraints to supersede the values detected by the environment"))
}

type bootstrapEnviron struct {
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount            int
	finalizerCount            int
	constraintsValidatorCount int
	args                      environs.BootstrapParams
	instanceConfig            *instancecfg.InstanceConfig
	storage                   storage.Storage

	// Providers are expected to receive a list of available
	// agent binaries (aka tools). This list needs to be valid.
	// For example, as discovered in lp#1745951, all items in that list
	// must be of the same version.
	checkToolsFunc func(tools.List)
}

func newEnviron(name string, defaultKeys bool, extraAttrs map[string]any) *bootstrapEnviron {
	m := coretesting.FakeConfig().Merge(extraAttrs)
	if !defaultKeys {
		m = m.Delete(
			"ca-cert",
			"ca-private-key",
			"admin-secret",
		)
	}
	m["name"] = name // overwrite name provided by dummy.SampleConfig
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		cfg:            cfg,
		checkToolsFunc: func(t tools.List) {},
	}
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func (s *bootstrapSuite) setDummyStorage(c *tc.C, env *bootstrapEnviron) {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	env.storage = stor
	s.AddCleanup(func(c *tc.C) { closer.Close() })
}

func (e *bootstrapEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	e.bootstrapCount++
	e.args = args

	e.checkToolsFunc(args.AvailableTools)

	finalizer := func(_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) error {
		e.finalizerCount++
		e.instanceConfig = icfg
		return nil
	}
	base := args.BootstrapBase
	if base.Empty() {
		base = jujuversion.DefaultSupportedLTSBase()
	}
	arch, _ := args.AvailableTools.OneArch()
	return &environs.BootstrapResult{
		Arch:                    arch,
		Base:                    base,
		CloudBootstrapFinalizer: finalizer,
	}, nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(ctx context.Context, cfg *config.Config) error {
	e.cfg = cfg
	return nil
}

func (e *bootstrapEnviron) Storage() storage.Storage {
	return e.storage
}

func (e *bootstrapEnviron) ConstraintsValidator(ctx context.Context) (constraints.Validator, error) {
	e.constraintsValidatorCount++
	v := constraints.NewValidator()
	v.RegisterVocabulary(constraints.Arch, []string{arch.AMD64, arch.ARM64})
	return v, nil
}

func (e *bootstrapEnviron) Provider() environs.EnvironProvider {
	return bootstrapEnvironProvider{}
}

type bootstrapEnvironProvider struct {
	environs.EnvironProvider
}

func (p bootstrapEnvironProvider) Version() int {
	return 123
}

// recoveryBootstrapEnviron extends bootstrapEnviron for IAAS recovery
// tests. Bootstrap returns the configured error, if any, or the
// configured result, whose InstanceID records the allocated replacement
// instance. afterAllocate, when set, runs after the instance is
// recorded as allocated and before Bootstrap returns; it is a testing
// hook for interrupting the provider call deterministically, and has no
// production counterpart.
type recoveryBootstrapEnviron struct {
	*bootstrapEnviron

	result        *environs.BootstrapResult
	err           error
	afterAllocate func()
}

func (e *recoveryBootstrapEnviron) Bootstrap(
	ctx environs.BootstrapContext,
	args environs.BootstrapParams,
) (*environs.BootstrapResult, error) {
	e.bootstrapCount++
	e.args = args
	if e.err != nil {
		// Providers report no result alongside an error; the error
		// itself names any instance allocated before the failure.
		return nil, e.err
	}
	if e.afterAllocate != nil && e.result != nil && e.result.InstanceID != "" {
		e.afterAllocate()
	}
	return e.result, nil
}

// recoveryBootstrapEnvironWithRegion is a recoveryBootstrapEnviron that reports
// a region, so bootstrap image selection runs the simplestreams path.
type recoveryBootstrapEnvironWithRegion struct {
	*recoveryBootstrapEnviron

	region simplestreams.CloudSpec
}

func (e recoveryBootstrapEnvironWithRegion) Region() (simplestreams.CloudSpec, error) {
	return e.region, nil
}

type bootstrapEnvironWithRegion struct {
	*bootstrapEnviron
	region simplestreams.CloudSpec
}

func (e bootstrapEnvironWithRegion) Region() (simplestreams.CloudSpec, error) {
	return e.region, nil
}

type bootstrapEnvironNoExplicitArchitectures struct {
	*bootstrapEnvironWithRegion
}

func (e bootstrapEnvironNoExplicitArchitectures) ConstraintsValidator(context.Context) (constraints.Validator, error) {
	e.constraintsValidatorCount++
	v := constraints.NewValidator()
	return v, nil
}

// A bootstrapEnviron that implements environs.HardwareCharacteristicsDetector.
type bootstrapEnvironWithHardwareDetection struct {
	*bootstrapEnviron

	detectedBase corebase.Base
	detectedHW   *instance.HardwareCharacteristics
}

func newBootstrapEnvironWithHardwareDetection(name string, detectedBase corebase.Base, detectedArch string, defaultKeys bool, extraAttrs map[string]any) *bootstrapEnvironWithHardwareDetection {
	var hw = new(instance.HardwareCharacteristics)
	if detectedArch != "" {
		hw.Arch = &detectedArch
	}

	return &bootstrapEnvironWithHardwareDetection{
		bootstrapEnviron: newEnviron(name, defaultKeys, extraAttrs),
		detectedBase:     detectedBase,
		detectedHW:       hw,
	}
}

func (e bootstrapEnvironWithHardwareDetection) DetectBase() (corebase.Base, error) {
	return e.detectedBase, nil
}

func (e bootstrapEnvironWithHardwareDetection) DetectHardware() (*instance.HardwareCharacteristics, error) {
	return e.detectedHW, nil
}

func (e bootstrapEnvironWithHardwareDetection) UpdateModelConstraints() bool {
	return false
}

// modelConstraintsEnviron implements the hardware detection policy of LXD.
type modelConstraintsEnviron struct {
	*bootstrapEnvironWithHardwareDetection
}

func (e modelConstraintsEnviron) UpdateModelConstraints() bool {
	return true
}

func bootstrapContext(c *tc.C) (environs.BootstrapContext, *simplestreams.Simplestreams) {
	ss := simplestreams.NewSimpleStreams(sstesting.TestDataSourceFactory())
	ctx := context.WithValue(c.Context(), bootstrap.SimplestreamsFetcherContextKey, ss)
	return envtesting.BootstrapContext(ctx, c), ss
}

type BootstrapContextSuite struct {
	testhelpers.IsolationSuite
}

func TestBootstrapContextSuite(t *testing.T) {
	tc.Run(t, &BootstrapContextSuite{})
}

func (s *BootstrapContextSuite) TestContextDone(c *tc.C) {
	testCases := []struct {
		name string
		ctx  context.Context
		done bool
	}{{
		name: "todo context",
		ctx:  c.Context(),
		done: false,
	}, {
		name: "background context",
		ctx:  c.Context(),
		done: false,
	}, {
		name: "cancel context",
		ctx: func() context.Context {
			ctx, cancel := context.WithCancel(c.Context())
			cancel()

			return ctx
		}(),
		done: true,
	}, {
		name: "timeout context",
		ctx: func() context.Context {
			ctx, cancel := context.WithTimeout(c.Context(), time.Nanosecond)
			time.Sleep(time.Millisecond)
			// Cancel is called here to not leak the context. In reality it
			// will still be trapped as "context deadline exceeded"
			cancel()
			return ctx
		}(),
		done: true,
	}}
	for _, t := range testCases {
		c.Logf("test %q", t.name)
		done := bootstrap.IsContextDone(t.ctx)
		c.Assert(done, tc.DeepEquals, t.done)
	}
}

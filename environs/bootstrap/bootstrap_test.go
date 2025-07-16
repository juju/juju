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
	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	jujuos "github.com/juju/juju/core/os"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
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
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/cloudconfig/podcfg"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	_ "github.com/juju/juju/internal/provider/dummy"
	corestorage "github.com/juju/juju/internal/storage"
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
			SupportedBootstrapBases:       supportedJujuBases,
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	env.args.AvailableTools = nil
	env.args.SupportedBootstrapBases = nil
	c.Assert(env.args, tc.DeepEquals, environs.BootstrapParams{
		ControllerConfig:     coretesting.FakeControllerConfig(),
		AuthorizedKeys:       []string{"key1"},
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
			StoragePools: map[string]corestorage.Attrs{
				"spool": {
					"type": "loop",
					"foo":  "bar",
				},
			},
		})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(env.bootstrapCount, tc.Equals, 1)
	c.Assert(env.args.StoragePools, tc.DeepEquals, map[string]corestorage.Attrs{
		"spool": {
			"type": "loop",
			"foo":  "bar",
		},
	})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedBootstrapBase(c *tc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]interface{}{
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
	cfg, err := env.Config().Apply(map[string]interface{}{
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
	cfg, err := env.Config().Apply(map[string]interface{}{
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
	cfg, err := env.Config().Apply(map[string]interface{}{
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
}

func intPtr(i uint64) *uint64 {
	return &i
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
	expectedCons.Mem = intPtr(3584)
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
	expectedCons.Mem = intPtr(3584)
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
	expectedCons.Mem = intPtr(3584)
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

func (s *bootstrapSuite) TestBootstrapBuildAgent(c *tc.C) {
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
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
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
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
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
			ControllerInheritedConfig: map[string]interface{}{"ftp-proxy": "http://proxy"},
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
	c.Check(icfg.Bootstrap.ControllerInheritedConfig, tc.DeepEquals, map[string]interface{}{"ftp-proxy": "http://proxy"})
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

func newEnviron(name string, defaultKeys bool, extraAttrs map[string]interface{}) *bootstrapEnviron {
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

func newBootstrapEnvironWithHardwareDetection(name string, detectedBase corebase.Base, detectedArch string, defaultKeys bool, extraAttrs map[string]interface{}) *bootstrapEnvironWithHardwareDetection {
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

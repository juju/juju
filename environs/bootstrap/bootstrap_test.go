// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jujuos "github.com/juju/os"
	"github.com/juju/os/series"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/arch"
	utilscert "github.com/juju/utils/cert"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/cert"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/gui"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/environs/storage"
	"github.com/juju/juju/environs/sync"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	jujuversion "github.com/juju/juju/version"
)

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture

	callContext context.ProviderCallContext
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&jujuversion.Current, coretesting.FakeVersionNumber)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	// Patch the function used to retrieve GUI archive info from simplestreams.
	s.PatchValue(bootstrap.GUIFetchMetadata, func(string, ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		return nil, nil
	})
	s.callContext = context.NewCloudCallContext()
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *gc.C) {
	env := newEnviron("bar", noKeysDefined, nil)
	s.setDummyStorage(c, env)

	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext,
		bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, gc.ErrorMatches, "validating bootstrap parameters: admin-secret is empty")

	controllerCfg := coretesting.FakeControllerConfig()
	delete(controllerCfg, "ca-cert")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: controllerCfg,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, gc.ErrorMatches, "validating bootstrap parameters: controller configuration has no ca-cert")

	controllerCfg = coretesting.FakeControllerConfig()
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: controllerCfg,
			AdminSecret:      "admin-secret",
		})
	c.Assert(err, gc.ErrorMatches, "validating bootstrap parameters: empty ca-private-key")

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: controllerCfg,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	env.args.AvailableTools = nil
	c.Assert(env.args, gc.DeepEquals, environs.BootstrapParams{
		ControllerConfig:     coretesting.FakeControllerConfig(),
		BootstrapConstraints: constraints.MustParse("mem=3.5G"),
	})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	bootstrapCons := constraints.MustParse("cores=3 mem=7G")
	modelCons := constraints.MustParse("cores=2 mem=4G")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: bootstrapCons,
			ModelConstraints:     modelCons,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.BootstrapConstraints, gc.DeepEquals, bootstrapCons)
	c.Assert(env.args.ModelConstraints, gc.DeepEquals, modelCons)
}

func (s *bootstrapSuite) TestBootstrapSpecifiedBootstrapSeries(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cfg, err := env.Config().Apply(map[string]interface{}{
		"default-series": "wily",
	})
	c.Assert(err, jc.ErrorIsNil)
	env.cfg = cfg

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			BootstrapSeries:  "trusty",
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(env.bootstrapCount, gc.Equals, 1)
	c.Check(env.args.BootstrapSeries, gc.Equals, "trusty")
	c.Check(env.args.AvailableTools.AllSeries(), jc.SameContents, []string{"trusty"})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedPlacement(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	placement := "directive"
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			Placement:        placement})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.Placement, gc.DeepEquals, placement)
}

func intPtr(i uint64) *uint64 {
	return &i
}

func (s *bootstrapSuite) TestBootstrapImage(c *gc.C) {
	s.PatchValue(&series.MustHostSeries, func() string { return "precise" })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "nether",
			Endpoint: "hearnoretheir",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	bootstrapCons := constraints.MustParse("arch=amd64")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			BootstrapImage:       "img-id",
			BootstrapSeries:      "precise",
			BootstrapConstraints: bootstrapCons,
			MetadataDir:          metadataDir,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.ImageMetadata, gc.HasLen, 1)
	c.Assert(env.args.ImageMetadata[0], jc.DeepEquals, &imagemetadata.ImageMetadata{
		Id:         "img-id",
		Arch:       "amd64",
		Version:    "12.04",
		RegionName: "nether",
		Endpoint:   "hearnoretheir",
		Stream:     "released",
	})
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata, gc.HasLen, 2)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[0], jc.DeepEquals, metadata[0])
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[1], jc.DeepEquals, env.args.ImageMetadata[0])
	expectedCons := bootstrapCons
	expectedCons.Mem = intPtr(3584)
	c.Assert(env.instanceConfig.Bootstrap.BootstrapMachineConstraints, jc.DeepEquals, expectedCons)
	c.Assert(env.instanceConfig.Bootstrap.ControllerModelEnvironVersion, gc.Equals, 123)
}

func (s *bootstrapSuite) TestBootstrapAddsArchFromImageToExistingProviderSupportedArches(c *gc.C) {
	data := s.setupImageMetadata(c)
	env := s.setupProviderWithSomeSupportedArches(c)
	// Even though test provider does not explicitly support architecture used by this test,
	// the fact that we have an image for it, adds this architecture to those supported by provider.
	// Bootstrap should succeed with no failures as constraints validator used internally
	// would have both provider supported architectures and architectures retrieved from images metadata.
	bootstrapCons := constraints.MustParse(fmt.Sprintf("arch=%v", data.architecture))
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			BootstrapImage:       "img-id",
			BootstrapSeries:      "precise",
			BootstrapConstraints: bootstrapCons,
			MetadataDir:          data.metadataDir,
		})
	c.Assert(err, jc.ErrorIsNil)
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
func (s *bootstrapSuite) setupImageMetadata(c *gc.C) testImageMetadata {
	testArch := arch.S390X
	s.PatchValue(&series.MustHostSeries, func() string { return "precise" })
	s.PatchValue(&arch.HostArch, func() string { return testArch })

	metadataDir, metadata := createImageMetadataForArch(c, testArch)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	return testImageMetadata{testArch, metadataDir, metadata}
}

func (s *bootstrapSuite) assertBootstrapImageMetadata(c *gc.C, env *bootstrapEnviron, testData testImageMetadata, bootstrapCons constraints.Value) {
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.ImageMetadata, gc.HasLen, 1)
	c.Assert(env.args.ImageMetadata[0], jc.DeepEquals, &imagemetadata.ImageMetadata{
		Id:         "img-id",
		Arch:       testData.architecture,
		Version:    "12.04",
		RegionName: "nether",
		Endpoint:   "hearnoretheir",
		Stream:     "released",
	})
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata, gc.HasLen, 2)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[0], jc.DeepEquals, testData.metadata[0])
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[1], jc.DeepEquals, env.args.ImageMetadata[0])
	c.Assert(env.instanceConfig.Bootstrap.BootstrapMachineConstraints, jc.DeepEquals, bootstrapCons)

}
func (s *bootstrapSuite) setupProviderWithSomeSupportedArches(c *gc.C) bootstrapEnvironWithRegion {
	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "nether",
			Endpoint: "hearnoretheir",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	// test provider constraints only has amd64 and arm64 as supported architectures
	consBefore, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	desiredArch := constraints.MustParse("arch=i386")
	unsupported, err := consBefore.Validate(desiredArch)
	c.Assert(err.Error(), jc.Contains, `invalid constraint value: arch=i386`)
	c.Assert(unsupported, gc.HasLen, 0)

	return env
}

func (s *bootstrapSuite) TestBootstrapAddsArchFromImageToProviderWithNoSupportedArches(c *gc.C) {
	data := s.setupImageMetadata(c)
	env := s.setupProviderWithNoSupportedArches(c)
	// Even though test provider does not explicitly support architecture used by this test,
	// the fact that we have an image for it, adds this architecture to those supported by provider.
	// Bootstrap should succeed with no failures as constraints validator used internally
	// would have both provider supported architectures and architectures retrieved from images metadata.
	bootstrapCons := constraints.MustParse(fmt.Sprintf("arch=%v", data.architecture))
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			BootstrapImage:       "img-id",
			BootstrapSeries:      "precise",
			BootstrapConstraints: bootstrapCons,
			MetadataDir:          data.metadataDir,
		})
	c.Assert(err, jc.ErrorIsNil)
	expectedCons := bootstrapCons
	expectedCons.Mem = intPtr(3584)
	s.assertBootstrapImageMetadata(c, env.bootstrapEnviron, data, expectedCons)
}

func (s *bootstrapSuite) setupProviderWithNoSupportedArches(c *gc.C) bootstrapEnvironNoExplicitArchitectures {
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

	consBefore, err := env.ConstraintsValidator(context.NewCloudCallContext())
	c.Assert(err, jc.ErrorIsNil)
	// test provider constraints only has amd64 and arm64 as supported architectures
	desiredArch := constraints.MustParse("arch=i386")
	unsupported, err := consBefore.Validate(desiredArch)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unsupported, gc.HasLen, 0)

	return env
}

// TestBootstrapImageMetadataFromAllSources tests that we are looking for
// image metadata in all data sources available to environment.
// Abandoning look up too soon led to misleading bootstrap failures:
// Juju reported no images available for a particular configuration,
// despite image metadata in other data sources compatible with the same configuration as well.
// Related to bug#1560625.
func (s *bootstrapSuite) TestBootstrapImageMetadataFromAllSources(c *gc.C) {
	s.PatchValue(&series.MustHostSeries, func() string { return "raring" })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	// Ensure that we can find at least one image metadata
	// early on in the image metadata lookup.
	// We should continue looking despite it.
	metadataDir, _ := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := bootstrapEnvironWithRegion{
		newEnviron("foo", useDefaultKeys, nil),
		simplestreams.CloudSpec{
			Region:   "region",
			Endpoint: "endpoint",
		},
	}
	s.setDummyStorage(c, env.bootstrapEnviron)

	bootstrapCons := constraints.MustParse("arch=amd64")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			BootstrapConstraints: bootstrapCons,
			MetadataDir:          metadataDir,
		})
	c.Assert(err, jc.ErrorIsNil)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	for _, source := range datasources {
		// make sure we looked in each and all...
		c.Assert(c.GetTestLog(), jc.Contains, fmt.Sprintf("image metadata in %s", source.Description()))
	}
}

func (s *bootstrapSuite) TestBootstrapLocalTools(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	// Client host is CentOS, wanting to bootstrap a CentOS
	// controller. This is fine.

	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.CentOS })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, *version.Number, string) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			BootstrapSeries: "centos7",
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env.bootstrapCount, gc.Equals, 1)
	c.Check(env.args.BootstrapSeries, gc.Equals, "centos7")
	c.Check(env.args.AvailableTools.AllSeries(), jc.SameContents, []string{"centos7"})
}

func (s *bootstrapSuite) TestBootstrapLocalToolsMismatchingOS(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	// Client host is a Windows system, wanting to bootstrap a trusty
	// controller with local tools. This can't work.

	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.Windows })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, *version.Number, string) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			BootstrapSeries: "trusty",
		})
	c.Assert(err, gc.ErrorMatches, `cannot use agent built for "trusty" using a machine running "Windows"`)
}

func (s *bootstrapSuite) TestBootstrapLocalToolsDifferentLinuxes(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	// Client host is some unspecified Linux system, wanting to
	// bootstrap a trusty controller with local tools. This should be
	// OK.

	s.PatchValue(&jujuos.HostOS, func() jujuos.OSType { return jujuos.GenericLinux })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, *version.Number, string) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
			BootstrapSeries: "trusty",
		})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(env.bootstrapCount, gc.Equals, 1)
	c.Check(env.args.BootstrapSeries, gc.Equals, "trusty")
	c.Check(env.args.AvailableTools.AllSeries(), jc.SameContents, []string{"trusty"})
}

func (s *bootstrapSuite) TestBootstrapBuildAgent(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		c.Fatal("should not call FindTools if BuildAgent is specified")
		return nil, errors.NotFoundf("tools")
	})

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			BuildAgent:       true,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(build bool, ver *version.Number, _ string) (*sync.BuiltAgent, error) {
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, jc.IsTrue)
				c.Assert(ver.String(), gc.Equals, "1.99.0.1")
				localVer := *ver
				// If we found an official build we suppress the build number.
				localVer.Build = 0
				return &sync.BuiltAgent{
					Dir:      c.MkDir(),
					Official: true,
					Version: version.Binary{
						Number: localVer,
						Series: "quental",
						Arch:   "arm64",
					},
				}, nil
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	// Check that the model config has the correct version set.
	cfg := env.instanceConfig.Bootstrap.ControllerModelConfig
	agentVersion, valid := cfg.AgentVersion()
	c.Check(valid, jc.IsTrue)
	c.Check(agentVersion.String(), gc.Equals, "1.99.0")
}

func (s *bootstrapSuite) assertBootstrapPackagedToolsAvailable(c *gc.C, clientArch string) {
	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return clientArch })
	toolsArch := clientArch
	if toolsArch == "i386" {
		toolsArch = "amd64"
	}
	findToolsOk := false
	s.PatchValue(bootstrap.FindTools, func(_ environs.BootstrapEnviron, _ int, _ int, _ []string, filter tools.Filter) (tools.List, error) {
		c.Assert(filter.Arch, gc.Equals, toolsArch)
		c.Assert(filter.Series, gc.Equals, "quantal")
		findToolsOk = true
		vers := version.Binary{
			Number: jujuversion.Current,
			Series: "quantal",
			Arch:   toolsArch,
		}
		return tools.List{{
			Version: vers,
		}}, nil
	})

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BootstrapSeries:  "quantal",
			BuildAgentTarball: func(bool, *version.Number, string) (*sync.BuiltAgent, error) {
				c.Fatal("should not call BuildAgentTarball if there are packaged tools")
				return nil, nil
			},
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(findToolsOk, jc.IsTrue)
}

func (s *bootstrapSuite) TestBootstrapPackagedTools(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}
	for _, a := range arch.AllSupportedArches {
		s.assertBootstrapPackagedToolsAvailable(c, a)
	}
}

func (s *bootstrapSuite) TestBootstrapNoToolsNonReleaseStream(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	// Patch out HostArch and FindTools to allow the test to pass on other architectures,
	// such as s390.
	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-stream": "proposed"})
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(bool, *version.Number, string) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
		})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapNoToolsDevelopmentConfig(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"development": true})
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			BuildAgentTarball: func(bool, *version.Number, string) (*sync.BuiltAgent, error) {
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
		})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapToolsVersion(c *gc.C) {
	availableVersions := []version.Binary{
		version.MustParseBinary("1.18.0-trusty-arm64"),
		version.MustParseBinary("1.18.1-trusty-arm64"),
		version.MustParseBinary("1.18.1.1-trusty-arm64"),
		version.MustParseBinary("1.18.1.2-trusty-arm64"),
		version.MustParseBinary("1.18.1.3-trusty-arm64"),
	}
	availableTools := make(tools.List, len(availableVersions))
	for i, v := range availableVersions {
		availableTools[i] = &tools.Tools{Version: v}
	}

	type test struct {
		currentVersion version.Number
		expectedTools  version.Number
	}
	tests := []test{{
		currentVersion: version.MustParse("1.18.0"),
		expectedTools:  version.MustParse("1.18.0"),
	}, {
		currentVersion: version.MustParse("1.18.1.4"),
		expectedTools:  version.MustParse("1.18.1.3"),
	}, {
		// build number is ignored unless major/minor don't
		// match the latest.
		currentVersion: version.MustParse("1.18.1.2"),
		expectedTools:  version.MustParse("1.18.1.3"),
	}, {
		// If the current patch level exceeds whatever's in
		// the tools source (e.g. when bootstrapping from trunk)
		// then the latest available tools will be chosen.
		currentVersion: version.MustParse("1.18.2"),
		expectedTools:  version.MustParse("1.18.1.3"),
	}}

	env := newEnviron("foo", useDefaultKeys, nil)
	for i, t := range tests {
		c.Logf("test %d: %+v", i, t)
		cfg, err := env.Config().Remove([]string{"agent-version"})
		c.Assert(err, jc.ErrorIsNil)
		err = env.SetConfig(cfg)
		c.Assert(err, jc.ErrorIsNil)
		s.PatchValue(&jujuversion.Current, t.currentVersion)
		tools, err := bootstrap.GetBootstrapToolsVersion(availableTools)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(tools, gc.Not(gc.HasLen), 0)
		toolsVersion, _ := tools.Newest()
		c.Assert(toolsVersion, gc.Equals, t.expectedTools)
	}
}

func (s *bootstrapSuite) TestBootstrapGUISuccessRemote(c *gc.C) {
	s.PatchValue(bootstrap.GUIFetchMetadata, func(stream string, sources ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		c.Assert(stream, gc.Equals, gui.ReleasedStream)
		c.Assert(sources[0].Description(), gc.Equals, "gui simplestreams")
		c.Assert(sources[0].RequireSigned(), jc.IsTrue)
		return []*gui.Metadata{{
			Version:  version.MustParse("2.0.42"),
			FullPath: "https://1.2.3.4/juju-gui-2.0.42.tar.bz2",
			SHA256:   "hash-2.0.42",
			Size:     42,
		}, {
			Version:  version.MustParse("2.0.47"),
			FullPath: "https://1.2.3.4/juju-gui-2.0.47.tar.bz2",
			SHA256:   "hash-2.0.47",
			Size:     47,
		}}, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			GUIDataSourceBaseURL: "https://1.2.3.4/gui/sources",
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "Fetching Juju GUI 2.0.42\n")

	// The most recent GUI release info has been stored.
	c.Assert(env.instanceConfig.Bootstrap.GUI.URL, gc.Equals, "https://1.2.3.4/juju-gui-2.0.42.tar.bz2")
	c.Assert(env.instanceConfig.Bootstrap.GUI.Version.String(), gc.Equals, "2.0.42")
	c.Assert(env.instanceConfig.Bootstrap.GUI.Size, gc.Equals, int64(42))
	c.Assert(env.instanceConfig.Bootstrap.GUI.SHA256, gc.Equals, "hash-2.0.42")
}

func (s *bootstrapSuite) TestBootstrapGUISuccessLocal(c *gc.C) {
	path := makeGUIArchive(c, "jujugui-2.2.0")
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "Fetching Juju GUI 2.2.0 from local archive\n")

	// Check GUI URL and version.
	c.Assert(env.instanceConfig.Bootstrap.GUI.URL, gc.Equals, "file://"+path)
	c.Assert(env.instanceConfig.Bootstrap.GUI.Version.String(), gc.Equals, "2.2.0")

	// Check GUI size.
	f, err := os.Open(path)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	info, err := f.Stat()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instanceConfig.Bootstrap.GUI.Size, gc.Equals, info.Size())

	// Check GUI hash.
	h := sha256.New()
	_, err = io.Copy(h, f)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instanceConfig.Bootstrap.GUI.SHA256, gc.Equals, fmt.Sprintf("%x", h.Sum(nil)))
}

func (s *bootstrapSuite) TestBootstrapGUISuccessNoGUI(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "Juju GUI installation has been disabled\n")
	c.Assert(env.instanceConfig.Bootstrap.GUI, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapGUINoStreams(c *gc.C) {
	s.PatchValue(bootstrap.GUIFetchMetadata, func(string, ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		return nil, nil
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			GUIDataSourceBaseURL: "https://1.2.3.4/gui/sources",
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "No available Juju GUI archives found\n")
	c.Assert(env.instanceConfig.Bootstrap.GUI, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapGUIStreamsFailure(c *gc.C) {
	s.PatchValue(bootstrap.GUIFetchMetadata, func(string, ...simplestreams.DataSource) ([]*gui.Metadata, error) {
		return nil, errors.New("bad wolf")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:     coretesting.FakeControllerConfig(),
			AdminSecret:          "admin-secret",
			CAPrivateKey:         coretesting.CAKey,
			GUIDataSourceBaseURL: "https://1.2.3.4/gui/sources",
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "Unable to fetch Juju GUI info: bad wolf\n")
	c.Assert(env.instanceConfig.Bootstrap.GUI, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapGUIErrorNotFound(c *gc.C) {
	s.PatchEnvironment("JUJU_GUI", "/no/such/file")
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, `Cannot use Juju GUI at "/no/such/file": cannot open Juju GUI archive:`)
}

func (s *bootstrapSuite) TestBootstrapGUIErrorInvalidArchive(c *gc.C) {
	path := filepath.Join(c.MkDir(), "gui.bz2")
	err := ioutil.WriteFile(path, []byte("invalid"), 0666)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err = bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, fmt.Sprintf("Cannot use Juju GUI at %q: cannot read Juju GUI archive", path))
}

func (s *bootstrapSuite) TestBootstrapGUIErrorInvalidVersion(c *gc.C) {
	path := makeGUIArchive(c, "jujugui-invalid")
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, fmt.Sprintf(`Cannot use Juju GUI at %q: cannot parse version "invalid"`, path))
}

func (s *bootstrapSuite) TestBootstrapGUIErrorUnexpectedArchive(c *gc.C) {
	path := makeGUIArchive(c, "not-a-gui")
	s.PatchEnvironment("JUJU_GUI", path)
	env := newEnviron("foo", useDefaultKeys, nil)
	ctx := cmdtesting.Context(c)
	err := bootstrap.Bootstrap(modelcmd.BootstrapContext(ctx), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, fmt.Sprintf("Cannot use Juju GUI at %q: cannot find Juju GUI version", path))
}

func makeGUIArchive(c *gc.C, dir string) string {
	if runtime.GOOS == "windows" {
		c.Skip("tar command not available")
	}
	target := filepath.Join(c.MkDir(), "gui.tar.bz2")
	source := c.MkDir()
	err := os.Mkdir(filepath.Join(source, dir), 0777)
	c.Assert(err, jc.ErrorIsNil)
	err = exec.Command("tar", "cjf", target, "-C", source, dir).Run()
	c.Assert(err, jc.ErrorIsNil)
	return target
}

// createImageMetadata creates some image metadata in a local directory.
func createImageMetadata(c *gc.C) (dir string, _ []*imagemetadata.ImageMetadata) {
	return createImageMetadataForArch(c, "amd64")
}

// createImageMetadataForArch creates some image metadata in a local directory for
// specified arch.
func createImageMetadataForArch(c *gc.C, arch string) (dir string, _ []*imagemetadata.ImageMetadata) {
	// Generate some image metadata.
	im := []*imagemetadata.ImageMetadata{{
		Id:         "1234",
		Arch:       arch,
		Version:    "13.04",
		RegionName: "region",
		Endpoint:   "endpoint",
	}}
	cloudSpec := &simplestreams.CloudSpec{
		Region:   "region",
		Endpoint: "endpoint",
	}
	sourceDir := c.MkDir()
	sourceStor, err := filestorage.NewFileStorageWriter(sourceDir)
	c.Assert(err, jc.ErrorIsNil)
	err = imagemetadata.MergeAndWriteMetadata("raring", im, cloudSpec, sourceStor)
	c.Assert(err, jc.ErrorIsNil)
	return sourceDir, im
}

// TestBootstrapMetadata tests:
// `juju bootstrap --metadata-source <dir>` where <dir>/images
// and <dir>/tools exist
func (s *bootstrapSuite) TestBootstrapMetadata(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			MetadataDir:      metadataDir,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(envtools.DefaultBaseURL, gc.Equals, metadataDir)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(datasources, gc.HasLen, 3)
	c.Assert(datasources[0].Description(), gc.Equals, "bootstrap metadata")
	// This data source does not require to contain signed data.
	// However, it may still contain it.
	// Since we will always try to read signed data first,
	// we want to be able to try to read this signed data
	// with a user provided key.
	// for this test, user provided key is empty.
	// Bugs #1542127, #1542131
	c.Assert(datasources[0].PublicSigningKey(), gc.Equals, "")
	c.Assert(env.instanceConfig, gc.NotNil)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata, gc.HasLen, 1)
	c.Assert(env.instanceConfig.Bootstrap.CustomImageMetadata[0], gc.DeepEquals, metadata[0])
}

func (s *bootstrapSuite) TestBootstrapMetadataDirNonexistend(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	nonExistentFileName := "/tmp/TestBootstrapMetadataDirNonexistend"
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			MetadataDir:      nonExistentFileName,
		})
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("simplestreams metadata source: %s not found", nonExistentFileName))
}

// TestBootstrapMetadataImagesNoTools tests 2 cases:
// juju bootstrap --metadata-source <dir>
// juju bootstrap --metadata-source <dir>/images
// where <dir>/tools doesn't exist
func (s *bootstrapSuite) TestBootstrapMetadataImagesNoTools(c *gc.C) {

	metadataDir, _ := createImageMetadata(c)
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)

	startingDefaultBaseURL := envtools.DefaultBaseURL
	for i, suffix := range []string{"", "images"} {
		environs.UnregisterImageDataSourceFunc("bootstrap metadata")
		err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
			s.callContext, bootstrap.BootstrapParams{
				ControllerConfig: coretesting.FakeControllerConfig(),
				AdminSecret:      "admin-secret",
				CAPrivateKey:     coretesting.CAKey,
				MetadataDir:      filepath.Join(metadataDir, suffix),
			})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(env.bootstrapCount, gc.Equals, i+1)
		c.Assert(envtools.DefaultBaseURL, gc.Equals, startingDefaultBaseURL)

		datasources, err := environs.ImageMetadataSources(env)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(datasources, gc.HasLen, 3)
		c.Assert(datasources[0].Description(), gc.Equals, "bootstrap metadata")
	}
}

// TestBootstrapMetadataToolsNoImages tests 2 cases:
// juju bootstrap --metadata-source <dir>
// juju bootstrap --metadata-source <dir>/tools
// where <dir>/images doesn't exist
func (s *bootstrapSuite) TestBootstrapMetadataToolsNoImages(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	metadataDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	for i, suffix := range []string{"", "tools"} {
		err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
			s.callContext, bootstrap.BootstrapParams{
				ControllerConfig: coretesting.FakeControllerConfig(),
				AdminSecret:      "admin-secret",
				CAPrivateKey:     coretesting.CAKey,
				MetadataDir:      filepath.Join(metadataDir, suffix),
			})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(env.bootstrapCount, gc.Equals, i+1)
		c.Assert(envtools.DefaultBaseURL, gc.Equals, metadataDir)
		datasources, err := environs.ImageMetadataSources(env)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(datasources, gc.HasLen, 2)
		c.Assert(datasources[0].Description(), gc.Not(gc.Equals), "bootstrap metadata")
	}
}

func (s *bootstrapSuite) TestBootstrapCloudCredential(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	credential := cloud.NewCredential(cloud.EmptyAuthType, map[string]string{"what": "ever"})
	args := bootstrap.BootstrapParams{
		ControllerConfig: coretesting.FakeControllerConfig(),
		AdminSecret:      "admin-secret",
		CAPrivateKey:     coretesting.CAKey,
		Cloud: cloud.Cloud{
			Name:      "cloud-name",
			Type:      "dummy",
			AuthTypes: []cloud.AuthType{cloud.EmptyAuthType},
			Regions:   []cloud.Region{{Name: "region-name"}},
		},
		CloudRegion:         "region-name",
		CloudCredentialName: "credential-name",
		CloudCredential:     &credential,
	}
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, s.callContext, args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.instanceConfig, gc.NotNil)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloud, jc.DeepEquals, args.Cloud)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloudRegion, jc.DeepEquals, args.CloudRegion)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloudCredential, jc.DeepEquals, args.CloudCredential)
	c.Assert(env.instanceConfig.Bootstrap.ControllerCloudCredentialName, jc.DeepEquals, args.CloudCredentialName)
}

func (s *bootstrapSuite) TestPublicKeyEnvVar(c *gc.C) {
	path := filepath.Join(c.MkDir(), "key")
	ioutil.WriteFile(path, []byte("publickey"), 0644)
	s.PatchEnvironment("JUJU_STREAMS_PUBLICKEY_FILE", path)

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.instanceConfig.Controller.PublicImageSigningKey, gc.Equals, "publickey")
}

func (s *bootstrapSuite) TestFinishBootstrapConfig(c *gc.C) {
	path := filepath.Join(c.MkDir(), "key")
	ioutil.WriteFile(path, []byte("publickey"), 0644)
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
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig:          coretesting.FakeControllerConfig(),
			ControllerInheritedConfig: map[string]interface{}{"ftp-proxy": "http://proxy"},
			Cloud:                     dummyCloud,
			AdminSecret:               password,
			CAPrivateKey:              coretesting.CAKey,
		})
	c.Assert(err, jc.ErrorIsNil)
	icfg := env.instanceConfig

	c.Check(icfg.APIInfo, jc.DeepEquals, &api.Info{
		Password: password,
		CACert:   coretesting.CACert,
		ModelTag: coretesting.ModelTag,
	})
	c.Check(icfg.Controller.MongoInfo, jc.DeepEquals, &mongo.MongoInfo{
		Password: password, Info: mongo.Info{CACert: coretesting.CACert},
	})
	c.Check(icfg.Bootstrap.ControllerInheritedConfig, gc.DeepEquals, map[string]interface{}{"ftp-proxy": "http://proxy"})
	c.Check(icfg.Bootstrap.RegionInheritedConfig, jc.DeepEquals, cloud.RegionConfig{
		"a-region": cloud.Attrs{
			"a-key": "a-value",
		},
		"b-region": cloud.Attrs{
			"b-key": "b-value",
		},
	})
	controllerCfg := icfg.Controller.Config
	c.Check(controllerCfg["ca-private-key"], gc.IsNil)
	c.Check(icfg.Bootstrap.StateServingInfo.StatePort, gc.Equals, controllerCfg.StatePort())
	c.Check(icfg.Bootstrap.StateServingInfo.APIPort, gc.Equals, controllerCfg.APIPort())
	c.Check(icfg.Bootstrap.StateServingInfo.CAPrivateKey, gc.Equals, coretesting.CAKey)

	srvCertPEM := icfg.Bootstrap.StateServingInfo.Cert
	srvKeyPEM := icfg.Bootstrap.StateServingInfo.PrivateKey
	_, _, err = utilscert.ParseCertAndKey(srvCertPEM, srvKeyPEM)
	c.Check(err, jc.ErrorIsNil)

	// TODO(perrito666) 2016-05-02 lp:1558657
	err = cert.Verify(srvCertPEM, coretesting.CACert, time.Now())
	c.Assert(err, jc.ErrorIsNil)
	err = cert.Verify(srvCertPEM, coretesting.CACert, time.Now().AddDate(9, 0, 0))
	c.Assert(err, jc.ErrorIsNil)
	err = cert.Verify(srvCertPEM, coretesting.CACert, time.Now().AddDate(10, 0, 1))
	c.Assert(err, gc.NotNil)
}

func (s *bootstrapSuite) TestBootstrapMetadataImagesMissing(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	noImagesDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(noImagesDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			MetadataDir:      noImagesDir,
		})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(datasources, gc.HasLen, 2)
	c.Assert(datasources[0].Description(), gc.Equals, "default cloud images")
	c.Assert(datasources[1].Description(), gc.Equals, "default ubuntu cloud images")
}

func (s *bootstrapSuite) setupBootstrapSpecificVersion(c *gc.C, clientMajor, clientMinor int, toolsVersion *version.Number) (error, int, version.Number) {
	currentVersion := jujuversion.Current
	currentVersion.Major = clientMajor
	currentVersion.Minor = clientMinor
	currentVersion.Tag = ""
	s.PatchValue(&jujuversion.Current, currentVersion)
	s.PatchValue(&series.MustHostSeries, func() string { return "trusty" })
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		return storage.NewStorageSimpleStreamsDataSource("test datasource", env.storage, "tools", simplestreams.CUSTOM_CLOUD_DATA, false), nil
	})
	defer envtools.UnregisterToolsDataSourceFunc("local storage")

	toolsBinaries := []version.Binary{
		version.MustParseBinary("10.11.12-trusty-amd64"),
		version.MustParseBinary("10.11.13-trusty-amd64"),
		version.MustParseBinary("10.11-beta1-trusty-amd64"),
	}
	stream := "released"
	if toolsVersion != nil && toolsVersion.Tag != "" {
		stream = "devel"
		currentVersion.Tag = toolsVersion.Tag
	}
	_, err := envtesting.UploadFakeToolsVersions(env.storage, stream, stream, toolsBinaries...)
	c.Assert(err, jc.ErrorIsNil)

	env.checkToolsFunc = func(t tools.List) {
		mockInstanceCfg := &instancecfg.InstanceConfig{}
		// All providers call SetTools on instance config during StartInstance
		// (which is called by Bootstrap). Checking here that the call will pass.
		err := mockInstanceCfg.SetTools(t)
		c.Assert(err, jc.ErrorIsNil)
	}

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			ControllerConfig: coretesting.FakeControllerConfig(),
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			AgentVersion:     toolsVersion,
			BuildAgentTarball: func(build bool, ver *version.Number, _ string) (*sync.BuiltAgent, error) {
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, jc.IsFalse)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
		})
	vers, _ := env.cfg.AgentVersion()
	return err, env.bootstrapCount, vers
}

func (s *bootstrapSuite) TestBootstrapSpecificVersion(c *gc.C) {
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, &toolsVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCount, gc.Equals, 1)
	c.Assert(vers, gc.DeepEquals, version.Number{
		Major: 10,
		Minor: 11,
		Patch: 12,
	})
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionWithTag(c *gc.C) {
	toolsVersion := version.MustParse("10.11-beta1")
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, &toolsVersion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCount, gc.Equals, 1)
	c.Assert(vers, gc.DeepEquals, version.Number{
		Major: 10,
		Minor: 11,
		Patch: 1,
		Tag:   "beta",
	})
}

func (s *bootstrapSuite) TestBootstrapNoSpecificVersion(c *gc.C) {
	// bootstrap with no specific version will use latest major.minor tools version.
	err, bootstrapCount, vers := s.setupBootstrapSpecificVersion(c, 10, 11, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bootstrapCount, gc.Equals, 1)
	c.Assert(vers, gc.DeepEquals, version.Number{
		Major: 10,
		Minor: 11,
		Patch: 13,
	})
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMinorMismatch(c *gc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 10, 1, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), gc.Matches, ".* no agent binaries are available .*")
	c.Assert(bootstrapCount, gc.Equals, 0)
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMajorMismatch(c *gc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 1, 11, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), gc.Matches, ".* no agent binaries are available .*")
	c.Assert(bootstrapCount, gc.Equals, 0)
}

func (s *bootstrapSuite) TestAvailableToolsInvalidArch(c *gc.C) {
	s.PatchValue(&arch.HostArch, func() string {
		return arch.S390X
	})
	s.PatchValue(bootstrap.FindTools, func(environs.BootstrapEnviron, int, int, []string, tools.Filter) (tools.List, error) {
		c.Fatal("find packaged tools should not be called")
		return nil, errors.New("unexpected")
	})

	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env,
		s.callContext, bootstrap.BootstrapParams{
			BuildAgent:       true,
			AdminSecret:      "admin-secret",
			CAPrivateKey:     coretesting.CAKey,
			ControllerConfig: coretesting.FakeControllerConfig(),
			BuildAgentTarball: func(build bool, ver *version.Number, _ string) (*sync.BuiltAgent, error) {
				c.Logf("BuildAgentTarball version %s", ver)
				c.Assert(build, jc.IsTrue)
				return &sync.BuiltAgent{Dir: c.MkDir()}, nil
			},
		})
	c.Assert(err, gc.ErrorMatches, `model "foo" of type dummy does not support instances running on "s390x"`)
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
	m := dummy.SampleConfig().Merge(extraAttrs)
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
func (s *bootstrapSuite) setDummyStorage(c *gc.C, env *bootstrapEnviron) {
	closer, stor, _ := envtesting.CreateLocalTestStorage(c)
	env.storage = stor
	s.AddCleanup(func(c *gc.C) { closer.Close() })
}

func (e *bootstrapEnviron) Bootstrap(ctx environs.BootstrapContext, callCtx context.ProviderCallContext, args environs.BootstrapParams) (*environs.BootstrapResult, error) {
	e.bootstrapCount++
	e.args = args

	e.checkToolsFunc(args.AvailableTools)

	finalizer := func(_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig, _ environs.BootstrapDialOpts) error {
		e.finalizerCount++
		e.instanceConfig = icfg
		return nil
	}
	series := series.MustHostSeries()
	if args.BootstrapSeries != "" {
		series = args.BootstrapSeries
	}
	return &environs.BootstrapResult{
		Arch:                    args.AvailableTools.Arches()[0],
		Series:                  series,
		CloudBootstrapFinalizer: finalizer,
	}, nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}

func (e *bootstrapEnviron) Storage() storage.Storage {
	return e.storage
}

func (e *bootstrapEnviron) ConstraintsValidator(ctx context.ProviderCallContext) (constraints.Validator, error) {
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

func (e bootstrapEnvironNoExplicitArchitectures) ConstraintsValidator(context.ProviderCallContext) (constraints.Validator, error) {
	e.constraintsValidatorCount++
	v := constraints.NewValidator()
	return v, nil
}

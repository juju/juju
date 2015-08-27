// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"fmt"
	"runtime"
	"strings"
	stdtesting "testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtesting "github.com/juju/juju/environs/testing"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/provider/dummy"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	coretesting.BaseSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)

	storageDir := c.MkDir()
	s.PatchValue(&envtools.DefaultBaseURL, storageDir)
	stor, err := filestorage.NewFileStorageWriter(storageDir)
	c.Assert(err, jc.ErrorIsNil)
	s.PatchValue(&version.Current.Number, coretesting.FakeVersionNumber)
	envtesting.UploadFakeTools(c, stor, "released", "released")
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.BaseSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *gc.C) {
	env := newEnviron("bar", noKeysDefined, nil)
	s.setDummyStorage(c, env)
	fixEnv := func(key string, value interface{}) {
		cfg, err := env.Config().Apply(map[string]interface{}{
			key: value,
		})
		c.Assert(err, jc.ErrorIsNil)
		env.cfg = cfg
	}

	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no admin-secret")

	fixEnv("admin-secret", "whatever")
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-cert")

	fixEnv("ca-cert", coretesting.CACert)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-private-key")

	fixEnv("ca-private-key", coretesting.CAKey)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	env.args.AvailableTools = nil
	c.Assert(env.args, gc.DeepEquals, environs.BootstrapParams{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{Constraints: cons})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.Constraints, gc.DeepEquals, cons)
}

func (s *bootstrapSuite) TestBootstrapSpecifiedPlacement(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	placement := "directive"
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{Placement: placement})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.Placement, gc.DeepEquals, placement)
}

func (s *bootstrapSuite) TestBootstrapNoToolsNonReleaseStream(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.Environ, int, int, string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"agent-stream": "proposed"})
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestBootstrapNoToolsDevelopmentConfig(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("issue 1403084: Currently does not work because of jujud problems")
	}

	s.PatchValue(&arch.HostArch, func() string { return arch.ARM64 })
	s.PatchValue(bootstrap.FindTools, func(environs.Environ, int, int, string, tools.Filter) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, map[string]interface{}{
		"development": true})
	err := bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, jc.ErrorIsNil)
}

func (s *bootstrapSuite) TestSetBootstrapTools(c *gc.C) {
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
		currentVersion       version.Number
		expectedTools        version.Number
		expectedAgentVersion version.Number
	}
	tests := []test{{
		currentVersion:       version.MustParse("1.18.0"),
		expectedTools:        version.MustParse("1.18.0"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}, {
		currentVersion:       version.MustParse("1.18.1.4"),
		expectedTools:        version.MustParse("1.18.1.3"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}, {
		// build number is ignored unless major/minor don't
		// match the latest.
		currentVersion:       version.MustParse("1.18.1.2"),
		expectedTools:        version.MustParse("1.18.1.3"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}, {
		// If the current patch level exceeds whatever's in
		// the tools source (e.g. when bootstrapping from trunk)
		// then the latest available tools will be chosen.
		currentVersion:       version.MustParse("1.18.2"),
		expectedTools:        version.MustParse("1.18.1.3"),
		expectedAgentVersion: version.MustParse("1.18.1.3"),
	}}

	env := newEnviron("foo", useDefaultKeys, nil)
	for i, t := range tests {
		c.Logf("test %d: %+v", i, t)
		cfg, err := env.Config().Remove([]string{"agent-version"})
		c.Assert(err, jc.ErrorIsNil)
		err = env.SetConfig(cfg)
		c.Assert(err, jc.ErrorIsNil)
		s.PatchValue(&version.Current.Number, t.currentVersion)
		bootstrapTools, err := bootstrap.SetBootstrapTools(env, availableTools)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(bootstrapTools.Version.Number, gc.Equals, t.expectedTools)
		agentVersion, _ := env.Config().AgentVersion()
		c.Assert(agentVersion, gc.Equals, t.expectedAgentVersion)
	}
}

// createImageMetadata creates some image metadata in a local directory.
func createImageMetadata(c *gc.C) (dir string, _ []*imagemetadata.ImageMetadata) {
	// Generate some image metadata.
	im := []*imagemetadata.ImageMetadata{{
		Id:         "1234",
		Arch:       "amd64",
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

func (s *bootstrapSuite) TestBootstrapMetadata(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	metadataDir, metadata := createImageMetadata(c)
	stor, err := filestorage.NewFileStorageWriter(metadataDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		MetadataDir: metadataDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(envtools.DefaultBaseURL, gc.Equals, metadataDir)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(datasources, gc.HasLen, 2)
	c.Assert(datasources[0].Description(), gc.Equals, "bootstrap metadata")
	c.Assert(env.instanceConfig, gc.NotNil)
	c.Assert(env.instanceConfig.CustomImageMetadata, gc.HasLen, 1)
	c.Assert(env.instanceConfig.CustomImageMetadata[0], gc.DeepEquals, metadata[0])
}

func (s *bootstrapSuite) TestBootstrapMetadataImagesMissing(c *gc.C) {
	environs.UnregisterImageDataSourceFunc("bootstrap metadata")

	noImagesDir := c.MkDir()
	stor, err := filestorage.NewFileStorageWriter(noImagesDir)
	c.Assert(err, jc.ErrorIsNil)
	envtesting.UploadFakeTools(c, stor, "released", "released")

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		MetadataDir: noImagesDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)

	datasources, err := environs.ImageMetadataSources(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(datasources, gc.HasLen, 1)
	c.Assert(datasources[0].Description(), gc.Equals, "default cloud images")
}

func (s *bootstrapSuite) setupBootstrapSpecificVersion(
	c *gc.C, clientMajor, clientMinor int, toolsVersion *version.Number,
) (error, int, version.Number) {
	currentVersion := version.Current
	currentVersion.Major = clientMajor
	currentVersion.Minor = clientMinor
	currentVersion.Series = "trusty"
	currentVersion.Arch = "amd64"
	currentVersion.Tag = ""
	s.PatchValue(&version.Current, currentVersion)
	s.PatchValue(&arch.HostArch, func() string { return arch.AMD64 })

	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	envtools.RegisterToolsDataSourceFunc("local storage", func(environs.Environ) (simplestreams.DataSource, error) {
		return storage.NewStorageSimpleStreamsDataSource("test datasource", env.storage, "tools"), nil
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

	err = bootstrap.Bootstrap(envtesting.BootstrapContext(c), env, bootstrap.BootstrapParams{
		AgentVersion: toolsVersion,
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
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), gc.Matches, ".* no tools are available .*")
	c.Assert(bootstrapCount, gc.Equals, 0)
}

func (s *bootstrapSuite) TestBootstrapSpecificVersionClientMajorMismatch(c *gc.C) {
	// bootstrap using a specified version only works if the patch number is different.
	// The bootstrap client major and minor versions need to match the tools asked for.
	toolsVersion := version.MustParse("10.11.12")
	err, bootstrapCount, _ := s.setupBootstrapSpecificVersion(c, 1, 11, &toolsVersion)
	c.Assert(strings.Replace(err.Error(), "\n", "", -1), gc.Matches, ".* no tools are available .*")
	c.Assert(bootstrapCount, gc.Equals, 0)
}

type bootstrapEnviron struct {
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount              int
	finalizerCount              int
	supportedArchitecturesCount int
	args                        environs.BootstrapParams
	instanceConfig              *instancecfg.InstanceConfig
	storage                     storage.Storage
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
		cfg: cfg,
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

func (e *bootstrapEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (string, string, environs.BootstrapFinalizer, error) {
	e.bootstrapCount++
	e.args = args
	finalizer := func(_ environs.BootstrapContext, icfg *instancecfg.InstanceConfig) error {
		e.finalizerCount++
		e.instanceConfig = icfg
		return nil
	}
	return arch.HostArch(), version.Current.Series, finalizer, nil
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

func (e *bootstrapEnviron) SupportedArchitectures() ([]string, error) {
	e.supportedArchitecturesCount++
	return []string{arch.AMD64, arch.ARM64}, nil
}

func (e *bootstrapEnviron) ConstraintsValidator() (constraints.Validator, error) {
	return constraints.NewValidator(), nil
}

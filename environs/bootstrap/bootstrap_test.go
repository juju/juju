// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
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
	s.PatchValue(bootstrap.EnvironsVerifyStorage, func(storage.Storage) error { return nil })
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
		c.Assert(err, gc.IsNil)
		env.cfg = cfg
	}

	err := bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no admin-secret")

	fixEnv("admin-secret", "whatever")
	err = bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-cert")

	fixEnv("ca-cert", coretesting.CACert)
	err = bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-private-key")

	fixEnv("ca-private-key", coretesting.CAKey)
	uploadTools(c, env)
	err = bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.IsNil)
}

func uploadTools(c *gc.C, env environs.Environ) {
	usefulVersion := version.Current
	usefulVersion.Series = config.PreferredSeries(env.Config())
	envtesting.AssertUploadFakeToolsVersions(c, env.Storage(), usefulVersion)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	err := bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{})
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	env.args.AvailableTools = nil
	c.Assert(env.args, gc.DeepEquals, environs.BootstrapParams{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	cons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{Constraints: cons})
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.Constraints, gc.DeepEquals, cons)
}

func (s *bootstrapSuite) TestBootstrapSpecifiedPlacement(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys, nil)
	s.setDummyStorage(c, env)
	placement := "directive"
	err := bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{Placement: placement})
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.args.Placement, gc.DeepEquals, placement)
}

func (s *bootstrapSuite) TestBootstrapNoTools(c *gc.C) {
	s.PatchValue(&version.Current.Arch, "arm64")
	s.PatchValue(&arch.HostArch, func() string {
		return "arm64"
	})
	s.PatchValue(bootstrap.FindTools, func(environs.ConfigGetter, int, int, tools.Filter, bool) (tools.List, error) {
		return nil, errors.NotFoundf("tools")
	})
	env := newEnviron("foo", useDefaultKeys, nil)
	err := bootstrap.Bootstrap(coretesting.Context(c), env, bootstrap.BootstrapParams{})
	// bootstrap.Bootstrap leaves it to the provider to
	// locate bootstrap tools.
	c.Assert(err, gc.IsNil)
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
		c.Assert(err, gc.IsNil)
		err = env.SetConfig(cfg)
		c.Assert(err, gc.IsNil)
		s.PatchValue(&version.Current.Number, t.currentVersion)
		bootstrapTools, err := bootstrap.SetBootstrapTools(env, availableTools)
		c.Assert(err, gc.IsNil)
		c.Assert(bootstrapTools.Version.Number, gc.Equals, t.expectedTools)
		agentVersion, _ := env.Config().AgentVersion()
		c.Assert(agentVersion, gc.Equals, t.expectedAgentVersion)
	}
}

type bootstrapEnviron struct {
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount              int
	finalizerCount              int
	supportedArchitecturesCount int
	args                        environs.BootstrapParams
	machineConfig               *cloudinit.MachineConfig
	storage                     storage.Storage
}

var _ envtools.SupportsCustomSources = (*bootstrapEnviron)(nil)

// GetToolsSources returns a list of sources which are used to search for simplestreams tools metadata.
func (e *bootstrapEnviron) GetToolsSources() ([]simplestreams.DataSource, error) {
	// Add the simplestreams source off the control bucket.
	return []simplestreams.DataSource{
		storage.NewStorageSimpleStreamsDataSource("cloud storage", e.Storage(), storage.BaseToolsPath)}, nil
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
	envtesting.UploadFakeTools(c, env.storage)
	s.AddCleanup(func(c *gc.C) { closer.Close() })
}

func (e *bootstrapEnviron) Bootstrap(ctx environs.BootstrapContext, args environs.BootstrapParams) (arch, series string, _ environs.BootstrapFinalizer, _ error) {
	e.bootstrapCount++
	e.args = args
	finalizer := func(_ environs.BootstrapContext, mcfg *cloudinit.MachineConfig) error {
		e.finalizerCount++
		e.machineConfig = mcfg
		return nil
	}
	return version.Current.Arch, version.Current.Series, finalizer, nil
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
	return []string{"amd64", "arm64"}, nil
}

func (e *bootstrapEnviron) SupportNetworks() bool {
	return true
}

// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"fmt"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/environs/localstorage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/version"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/tools"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	home *coretesting.FakeHome
	coretesting.LoggingSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
	s.home = coretesting.MakeFakeHomeNoEnvironments(c, "foo")
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
	s.ToolsFixture.TearDownTest(c)
	s.LoggingSuite.TearDownTest(c)
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *gc.C) {
	env := newEnviron("bar", noKeysDefined)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	fixEnv := func(key string, value interface{}) {
		cfg, err := env.Config().Apply(map[string]interface{}{
			key: value,
		})
		c.Assert(err, gc.IsNil)
		env.cfg = cfg
	}

	err := bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no admin-secret")

	fixEnv("admin-secret", "whatever")
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-cert")

	fixEnv("ca-cert", coretesting.CACert)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-private-key")

	fixEnv("ca-private-key", coretesting.CAKey)
	uploadTools(c, env)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func uploadTools(c *gc.C, env environs.Environ) {
	usefulVersion := version.Current
	usefulVersion.Series = env.Config().DefaultSeries()
	envtesting.UploadFakeToolsVersion(c, env.Storage(), usefulVersion)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	uploadTools(c, env)
	err := bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.constraints, gc.DeepEquals, constraints.Value{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	uploadTools(c, env)
	cons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := bootstrap.Bootstrap(env, cons)
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.constraints, gc.DeepEquals, cons)
}

var bootstrapSetAgentVersionTests = []envtesting.BootstrapToolsTest{
	{
		Info:          "released cli with dev setting picks newest matching 1",
		Available:     envtesting.V100Xall,
		CliVersion:    envtesting.V100q32,
		DefaultSeries: "precise",
		Development:   true,
		Expect:        []version.Binary{envtesting.V1001p64},
	}, {
		Info:          "released cli with dev setting picks newest matching 2",
		Available:     envtesting.V1all,
		CliVersion:    envtesting.V120q64,
		DefaultSeries: "precise",
		Development:   true,
		Arch:          "i386",
		Expect:        []version.Binary{envtesting.V120p32},
	}, {
		Info:          "dev cli picks newest matching 1",
		Available:     envtesting.V110Xall,
		CliVersion:    envtesting.V110q32,
		DefaultSeries: "precise",
		Expect:        []version.Binary{envtesting.V1101p64},
	}, {
		Info:          "dev cli picks newest matching 2",
		Available:     envtesting.V1all,
		CliVersion:    envtesting.V120q64,
		DefaultSeries: "precise",
		Arch:          "i386",
		Expect:        []version.Binary{envtesting.V120p32},
	}}

func (s *bootstrapSuite) TestBootstrapTools(c *gc.C) {
	allTests := append(envtesting.BootstrapToolsTests, bootstrapSetAgentVersionTests...)
	for i, test := range allTests {
		c.Logf("\ntest %d: %s", i, test.Info)
		dummy.Reset()
		attrs := dummy.SampleConfig.Merge(coretesting.Attrs{
			"state-server": false,
			"development":     test.Development,
			"default-series":  test.DefaultSeries,
		})
		if test.AgentVersion != version.Zero {
			attrs["agent-version"] = test.AgentVersion.String()
		}
		env, err := environs.NewFromAttrs(attrs)
		c.Assert(err, gc.IsNil)
		env, err = environs.Prepare(env.Config())
		c.Assert(err, gc.IsNil)
		envtesting.RemoveAllTools(c, env)

		version.Current = test.CliVersion
		for _, vers := range test.Available {
			envtesting.UploadFakeToolsVersion(c, env.Storage(), vers)
		}

		cons := constraints.Value{}
		if test.Arch != "" {
			cons = constraints.MustParse("arch=" + test.Arch)
		}
		err = bootstrap.Bootstrap(env, cons)
		if test.Err != nil {
			c.Check(err, gc.ErrorMatches, ".*"+test.Err.Error())
			continue
		} else {
			c.Assert(err, gc.IsNil)
		}
		unique := map[version.Number]bool{}
		for _, expected := range test.Expect {
			unique[expected.Number] = true
		}
		for expectAgentVersion := range unique {
			agentVersion, ok := env.Config().AgentVersion()
			c.Check(ok, gc.Equals, true)
			c.Check(agentVersion, gc.Equals, expectAgentVersion)
		}
	}
}

func (s *bootstrapSuite) TestBootstrapNeedsTools(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	envtesting.RemoveFakeTools(c, env.Storage())
	err := bootstrap.Bootstrap(env, constraints.Value{})
	c.Check(err, gc.ErrorMatches, "cannot find bootstrap tools: no tools available")
}

type bootstrapEnviron struct {
	name             string
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount int
	constraints    constraints.Value
	storage        environs.Storage
}

func newEnviron(name string, defaultKeys bool) *bootstrapEnviron {
	m := dummy.SampleConfig
	if !defaultKeys {
		m = m.Delete(
			"ca-cert",
			"ca-private-key",
			"admin-secret",
		)
	}
	cfg, err := config.New(config.NoDefaults, m)
	if err != nil {
		panic(fmt.Errorf("cannot create config from %#v: %v", m, err))
	}
	return &bootstrapEnviron{
		name: name,
		cfg:  cfg,
	}
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
// Returns a cleanup function that must be called when done with the storage.
func setDummyStorage(c *gc.C, env *bootstrapEnviron) func() {
	listener, err := localstorage.Serve("127.0.0.1:0", c.MkDir())
	c.Assert(err, gc.IsNil)
	env.storage = localstorage.Client(listener.Addr().String())
	envtesting.UploadFakeTools(c, env.storage)
	return func() { listener.Close() }
}

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(cons constraints.Value, possibleTools tools.List, machineID string) error {
	e.bootstrapCount++
	e.constraints = cons
	return nil
}

func (e *bootstrapEnviron) Config() *config.Config {
	return e.cfg
}

func (e *bootstrapEnviron) SetConfig(cfg *config.Config) error {
	e.cfg = cfg
	return nil
}

func (e *bootstrapEnviron) Storage() environs.Storage {
	return e.storage
}

func (e *bootstrapEnviron) PublicStorage() environs.StorageReader {
	return environs.EmptyStorage
}

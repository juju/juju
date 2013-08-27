// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap_test

import (
	"fmt"
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/bootstrap"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/localstorage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/provider/dummy"
	coretesting "launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	home *coretesting.FakeHome
	coretesting.LoggingSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = coretesting.MakeFakeHomeNoEnvironments(c, "foo")
}

func (s *bootstrapSuite) TearDownTest(c *gc.C) {
	s.home.Restore()
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

var (
	v100p64 = version.MustParseBinary("1.0.0-precise-amd64")
	v100p32 = version.MustParseBinary("1.0.0-precise-i386")
	v100p   = []version.Binary{v100p64, v100p32}

	v100q64 = version.MustParseBinary("1.0.0-quantal-amd64")
	v100q32 = version.MustParseBinary("1.0.0-quantal-i386")
	v100q   = []version.Binary{v100q64, v100q32}
	v100all = append(v100p, v100q...)

	v1001    = version.MustParse("1.0.0.1")
	v1001p64 = version.MustParseBinary("1.0.0.1-precise-amd64")
	v100Xall = append(v100all, v1001p64)

	v110p64 = version.MustParseBinary("1.1.0-precise-amd64")
	v110p32 = version.MustParseBinary("1.1.0-precise-i386")
	v110p   = []version.Binary{v110p64, v110p32}

	v1101p64 = version.MustParseBinary("1.1.0.1-precise-amd64")
	v110Xall = append(v110all, v1101p64)

	v110q64 = version.MustParseBinary("1.1.0-quantal-amd64")
	v110q32 = version.MustParseBinary("1.1.0-quantal-i386")
	v110q   = []version.Binary{v110q64, v110q32}
	v110all = append(v110p, v110q...)

	v120p64 = version.MustParseBinary("1.2.0-precise-amd64")
	v120p32 = version.MustParseBinary("1.2.0-precise-i386")
	v120p   = []version.Binary{v120p64, v120p32}

	v120q64 = version.MustParseBinary("1.2.0-quantal-amd64")
	v120q32 = version.MustParseBinary("1.2.0-quantal-i386")
	v120q   = []version.Binary{v120q64, v120q32}
	v120all = append(v120p, v120q...)
	v1all   = append(v100Xall, append(v110all, v120all...)...)
)

var bootstrapTests = []struct {
	info          string
	available     []version.Binary
	cliVersion    version.Binary
	defaultSeries string
	agentVersion  version.Number
	development   bool
	arch          string
	expect        []version.Binary
	err           error
}{{
	info:          "released cli with dev setting picks newest matching 1",
	available:     v100Xall,
	cliVersion:    v100q32,
	defaultSeries: "precise",
	development:   true,
	expect:        []version.Binary{v1001p64},
}, {
	info:          "released cli with dev setting picks newest matching 2",
	available:     v1all,
	cliVersion:    v120q64,
	defaultSeries: "precise",
	development:   true,
	arch:          "i386",
	expect:        []version.Binary{v120p32},
}, {
	info:          "released cli with dev setting respects agent-version",
	available:     v1all,
	cliVersion:    v100q32,
	agentVersion:  v1001,
	defaultSeries: "precise",
	development:   true,
	expect:        []version.Binary{v1001p64},
}, {
	info:          "dev cli picks newest matching 1",
	available:     v110Xall,
	cliVersion:    v110q32,
	defaultSeries: "precise",
	expect:        []version.Binary{v1101p64},
}, {
	info:          "dev cli picks newest matching 2",
	available:     v1all,
	cliVersion:    v120q64,
	defaultSeries: "precise",
	arch:          "i386",
	expect:        []version.Binary{v120p32},
}, {
	info:          "dev cli respects agent-version",
	available:     v1all,
	cliVersion:    v100q32,
	agentVersion:  v1001,
	defaultSeries: "precise",
	expect:        []version.Binary{v1001p64},
}}

func (s *bootstrapSuite) TestBootstrapAgentVersion(c *gc.C) {
	for i, test := range bootstrapTests {
		c.Logf("\ntest %d: %s", i, test.info)
		dummy.Reset()
		attrs := map[string]interface{}{
			"name":            "test",
			"type":            "dummy",
			"state-server":    false,
			"admin-secret":    "a-secret",
			"authorized-keys": "i-am-a-key",
			"ca-cert":         coretesting.CACert,
			"ca-private-key":  coretesting.CAKey,
			"development":     test.development,
			"default-series":  test.defaultSeries,
		}
		if test.agentVersion != version.Zero {
			attrs["agent-version"] = test.agentVersion.String()
		}
		env, err := environs.NewFromAttrs(attrs)
		c.Assert(err, gc.IsNil)
		envtesting.RemoveAllTools(c, env)

		version.Current = test.cliVersion
		for _, vers := range test.available {
			envtesting.UploadFakeToolsVersion(c, env.Storage(), vers)
		}

		cons := constraints.Value{}
		if test.arch != "" {
			cons = constraints.MustParse("arch=" + test.arch)
		}
		err = bootstrap.Bootstrap(env, cons)
		if test.err != nil {
			c.Check(err, jc.Satisfies, errors.IsNotFoundError)
			continue
		} else {
			c.Assert(err, gc.IsNil)
		}
		unique := map[version.Number]bool{}
		for _, expected := range test.expect {
			unique[expected.Number] = true
		}
		for expectAgentVersion := range unique {
			agentVersion, ok := env.Config().AgentVersion()
			c.Check(ok, gc.Equals, true)
			c.Check(agentVersion, gc.Equals, expectAgentVersion)
		}
	}
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
	m := map[string]interface{}{
		"name":            name,
		"type":            "test",
		"ca-cert":         "",
		"ca-private-key":  "",
		"authorized-keys": "",
	}
	if defaultKeys {
		m["ca-cert"] = coretesting.CACert
		m["ca-private-key"] = coretesting.CAKey
		m["admin-secret"] = version.Current.Number.String()
		m["authorized-keys"] = "foo"
	}
	cfg, err := config.New(m)
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
	return nil
}

package environs_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
)

const (
	useDefaultKeys = true
	noKeysDefined  = false
)

type bootstrapSuite struct {
	home *testing.FakeHome
	testing.LoggingSuite
}

var _ = Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeNoEnvironments(c, "foo")
}

func (s *bootstrapSuite) TearDownTest(c *C) {
	s.home.Restore()
}

func (s *bootstrapSuite) TestBootstrapNeedsSettings(c *C) {
	env := newEnviron("bar", noKeysDefined)
	fixEnv := func(key string, value interface{}) {
		cfg, err := env.Config().Apply(map[string]interface{}{
			key: value,
		})
		c.Assert(err, IsNil)
		env.cfg = cfg
	}

	err := environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, ErrorMatches, "environment configuration missing admin-secret")

	fixEnv("admin-secret", "whatever")
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, ErrorMatches, "environment configuration missing CA certificate")

	fixEnv("ca-cert", testing.CACert)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, ErrorMatches, "environment configuration missing CA private key")

	fixEnv("ca-private-key", testing.CAKey)
	err = environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *C) {
	env := newEnviron("foo", useDefaultKeys)
	err := environs.Bootstrap(env, constraints.Value{})
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.constraints, DeepEquals, constraints.Value{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *C) {
	env := newEnviron("foo", useDefaultKeys)
	cons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := environs.Bootstrap(env, cons)
	c.Assert(err, IsNil)
	c.Assert(env.bootstrapCount, Equals, 1)
	c.Assert(env.constraints, DeepEquals, cons)
}

type bootstrapEnviron struct {
	name             string
	cfg              *config.Config
	environs.Environ // stub out all methods we don't care about.

	// The following fields are filled in when Bootstrap is called.
	bootstrapCount int
	constraints    constraints.Value
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
		m["ca-cert"] = testing.CACert
		m["ca-private-key"] = testing.CAKey
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

func (e *bootstrapEnviron) Name() string {
	return e.name
}

func (e *bootstrapEnviron) Bootstrap(cons constraints.Value) error {
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

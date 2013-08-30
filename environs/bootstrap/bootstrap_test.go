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
	"launchpad.net/juju-core/environs/localstorage"
	envtesting "launchpad.net/juju-core/environs/testing"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/tools"
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

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.home = testing.MakeFakeHomeNoEnvironments(c, "foo")
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

	fixEnv("ca-cert", testing.CACert)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "environment configuration has no ca-private-key")

	fixEnv("ca-private-key", testing.CAKey)
	err = bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
}

func (s *bootstrapSuite) TestBootstrapEmptyConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	err := bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.constraints, gc.DeepEquals, constraints.Value{})
}

func (s *bootstrapSuite) TestBootstrapSpecifiedConstraints(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	cons := constraints.MustParse("cpu-cores=2 mem=4G")
	err := bootstrap.Bootstrap(env, cons)
	c.Assert(err, gc.IsNil)
	c.Assert(env.bootstrapCount, gc.Equals, 1)
	c.Assert(env.constraints, gc.DeepEquals, cons)
}

func (s *bootstrapSuite) TestBootstrapNeedsTools(c *gc.C) {
	env := newEnviron("foo", useDefaultKeys)
	cleanup := setDummyStorage(c, env)
	defer cleanup()
	envtesting.RemoveFakeTools(c, env.Storage())
	err := bootstrap.Bootstrap(env, constraints.Value{})
	c.Assert(err, gc.ErrorMatches, "cannot find bootstrap tools: no tools available")
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

// setDummyStorage injects the local provider's fake storage implementation
// into the given enviornment, so that tests can manipulate storage as if it
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

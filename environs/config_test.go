// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/provider/dummy"
	"launchpad.net/juju-core/testing"
	jc "launchpad.net/juju-core/testing/checkers"
)

type suite struct{}

var _ = gc.Suite(suite{})

func (suite) TearDownTest(c *gc.C) {
	dummy.Reset()
}

var invalidConfigTests = []struct {
	env string
	err string
}{
	{"'", "YAML error:.*"},
	{`
default: unknown
environments:
    only:
        type: unknown
`, `default environment .* does not exist`,
	},
}

func (suite) TestInvalidConfig(c *gc.C) {
	for i, t := range invalidConfigTests {
		c.Logf("running test %v", i)
		_, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gc.ErrorMatches, t.err)
	}
}

var invalidEnvTests = []struct {
	env  string
	name string
	err  string
}{
	{`
environments:
    only:
        foo: bar
`, "", `environment "only" has no type`,
	}, {`
environments:
    only:
        foo: bar
`, "only", `environment "only" has no type`,
	}, {`
environments:
    only:
        foo: bar
        type: crazy
`, "only", `environment "only" has an unknown provider type "crazy"`,
	},
}

func (suite) TestInvalidEnv(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	for i, t := range invalidEnvTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gc.IsNil)
		cfg, err := es.Config(t.name)
		c.Check(err, gc.ErrorMatches, t.err)
		c.Check(cfg, gc.IsNil)
	}
}

func (suite) TestNoEnv(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c).Restore()
	es, err := environs.ReadEnvirons("")
	c.Assert(es, gc.IsNil)
	c.Assert(err, jc.Satisfies, environs.IsNoEnv)
}

var configTests = []struct {
	env   string
	check func(c *gc.C, envs *environs.Environs)
}{
	{`
environments:
    only:
        type: dummy
        state-server: false
`, func(c *gc.C, envs *environs.Environs) {
		cfg, err := envs.Config("")
		c.Assert(err, gc.IsNil)
		c.Assert(cfg.Name(), gc.Equals, "only")
	}}, {`
default:
    invalid
environments:
    valid:
        type: dummy
        state-server: false
    invalid:
        type: crazy
`, func(c *gc.C, envs *environs.Environs) {
		cfg, err := envs.Config("")
		c.Assert(err, gc.ErrorMatches, `environment "invalid" has an unknown provider type "crazy"`)
		c.Assert(cfg, gc.IsNil)
		cfg, err = envs.Config("valid")
		c.Assert(err, gc.IsNil)
		c.Assert(cfg.Name(), gc.Equals, "valid")
	}}, {`
environments:
    one:
        type: dummy
        state-server: false
    two:
        type: dummy
        state-server: false
`, func(c *gc.C, envs *environs.Environs) {
		cfg, err := envs.Config("")
		c.Assert(err, gc.ErrorMatches, `no default environment found`)
		c.Assert(cfg, gc.IsNil)
	}},
}

func (suite) TestConfig(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only", "valid", "one", "two").Restore()
	for i, t := range configTests {
		c.Logf("running test %v", i)
		envs, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, gc.IsNil)
		t.check(c, envs)
	}
}

func (suite) TestDefaultConfigFile(c *gc.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, gc.IsNil)
	path := testing.HomePath(".juju", "environments.yaml")
	c.Assert(path, gc.Equals, outfile)

	envs, err := environs.ReadEnvirons("")
	c.Assert(err, gc.IsNil)
	cfg, err := envs.Config("")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "only")
}

func (suite) TestConfigPerm(c *gc.C) {
	defer testing.MakeSampleHome(c).Restore()

	path := testing.HomePath(".juju")
	info, err := os.Lstat(path)
	c.Assert(err, gc.IsNil)
	oldPerm := info.Mode().Perm()
	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, gc.IsNil)

	info, err = os.Lstat(outfile)
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode().Perm(), gc.Equals, os.FileMode(0600))

	info, err = os.Lstat(filepath.Dir(outfile))
	c.Assert(err, gc.IsNil)
	c.Assert(info.Mode().Perm(), gc.Equals, oldPerm)

}

func (suite) TestNamedConfigFile(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	path := filepath.Join(c.MkDir(), "a-file")
	outfile, err := environs.WriteEnvirons(path, env)
	c.Assert(err, gc.IsNil)
	c.Assert(path, gc.Equals, outfile)

	envs, err := environs.ReadEnvirons(path)
	c.Assert(err, gc.IsNil)
	cfg, err := envs.Config("")
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.Name(), gc.Equals, "only")
}

func (suite) TestConfigRoundTrip(c *gc.C) {
	c.Skip("what is this really meant to be testing")
	cfg, err := config.New(config.NoDefaults, dummySampleConfig())
	c.Assert(err, gc.IsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, gc.IsNil)
	cfg, err = provider.Validate(cfg, nil)
	c.Assert(err, gc.IsNil)
	// This fails because the configuration isn't prepared.
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, env.Config().AllAttrs())
}

func inMap(attrs testing.Attrs, attr string) bool {
	_, ok := attrs[attr]
	return ok
}

func (suite) TestBootstrapConfig(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "bladaam").Restore()
	attrs := dummySampleConfig().Merge(testing.Attrs{
		"agent-version": "1.2.3",
	})
	c.Assert(inMap(attrs, "secret"), jc.IsTrue)
	c.Assert(inMap(attrs, "ca-private-key"), jc.IsTrue)
	c.Assert(inMap(attrs, "admin-secret"), jc.IsTrue)

	cfg, err := config.New(config.NoDefaults, attrs)
	c.Assert(err, gc.IsNil)
	c.Assert(err, gc.IsNil)

	cfg1, err := environs.BootstrapConfig(cfg)
	c.Assert(err, gc.IsNil)

	expect := cfg.AllAttrs()
	delete(expect, "secret")
	expect["admin-secret"] = ""
	expect["ca-private-key"] = ""
	c.Assert(cfg1.AllAttrs(), gc.DeepEquals, expect)
}

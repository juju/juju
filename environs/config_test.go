// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/dummy"
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
	}, {`
environments:
    only:
        type: dummy
`, "only", `.*state-server: expected bool, got nothing`,
	},
}

func (suite) TestInvalidEnv(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	for i, t := range invalidEnvTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gc.IsNil)
		e, err := es.Open(t.name)
		c.Check(err, gc.ErrorMatches, t.err)
		c.Check(e, gc.IsNil)
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
	check func(c *gc.C, es *environs.Environs)
}{
	{`
environments:
    only:
        type: dummy
        state-server: false
`, func(c *gc.C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, gc.IsNil)
		c.Assert(e.Name(), gc.Equals, "only")
	}}, {`
default:
    invalid
environments:
    valid:
        type: dummy
        state-server: false
    invalid:
        type: crazy
`, func(c *gc.C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, gc.ErrorMatches, `environment "invalid" has an unknown provider type "crazy"`)
		c.Assert(e, gc.IsNil)
		e, err = es.Open("valid")
		c.Assert(err, gc.IsNil)
		c.Assert(e.Name(), gc.Equals, "valid")
	}}, {`
environments:
    one:
        type: dummy
        state-server: false
    two:
        type: dummy
        state-server: false
`, func(c *gc.C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, gc.ErrorMatches, `no default environment found`)
		c.Assert(e, gc.IsNil)
	}},
}

func (suite) TestConfig(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only", "valid", "one", "two").Restore()
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, gc.IsNil)
		t.check(c, es)
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

	es, err := environs.ReadEnvirons("")
	c.Assert(err, gc.IsNil)
	e, err := es.Open("")
	c.Assert(err, gc.IsNil)
	c.Assert(e.Name(), gc.Equals, "only")
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

	es, err := environs.ReadEnvirons(path)
	c.Assert(err, gc.IsNil)
	e, err := es.Open("")
	c.Assert(err, gc.IsNil)
	c.Assert(e.Name(), gc.Equals, "only")
}

func (suite) TestConfigRoundTrip(c *gc.C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "bladaam",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, gc.IsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, gc.IsNil)
	cfg, err = provider.Validate(cfg, nil)
	c.Assert(err, gc.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gc.IsNil)
	c.Assert(cfg.AllAttrs(), gc.DeepEquals, env.Config().AllAttrs())
}

func (suite) TestBootstrapConfig(c *gc.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "bladaam").Restore()
	cfg, err := config.New(map[string]interface{}{
		"name":            "bladaam",
		"type":            "dummy",
		"state-server":    false,
		"admin-secret":    "highly",
		"secret":          "um",
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  testing.CAKey,
		"agent-version":   "1.2.3",
	})
	c.Assert(err, gc.IsNil)
	cfg1, err := environs.BootstrapConfig(cfg)
	c.Assert(err, gc.IsNil)

	expect := cfg.AllAttrs()
	delete(expect, "secret")
	expect["admin-secret"] = ""
	expect["ca-private-key"] = ""
	c.Assert(cfg1.AllAttrs(), gc.DeepEquals, expect)
}

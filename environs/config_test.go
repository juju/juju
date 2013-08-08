// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs_test

import (
	"os"
	"path/filepath"

	"launchpad.net/gocheck"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
)

type suite struct{}

var _ = gocheck.Suite(suite{})

func (suite) TearDownTest(c *gocheck.C) {
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

func (suite) TestInvalidConfig(c *gocheck.C) {
	for i, t := range invalidConfigTests {
		c.Logf("running test %v", i)
		_, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gocheck.ErrorMatches, t.err)
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

func (suite) TestInvalidEnv(c *gocheck.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only").Restore()
	for i, t := range invalidEnvTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, gocheck.IsNil)
		e, err := es.Open(t.name)
		c.Check(err, gocheck.ErrorMatches, t.err)
		c.Check(e, gocheck.IsNil)
	}
}

func (suite) TestNoEnv(c *gocheck.C) {
	defer testing.MakeFakeHomeNoEnvironments(c).Restore()
	es, err := environs.ReadEnvirons("")
	c.Assert(es, gocheck.IsNil)
	c.Assert(err, checkers.Satisfies, environs.IsNoEnv)
}

var configTests = []struct {
	env   string
	check func(c *gocheck.C, es *environs.Environs)
}{
	{`
environments:
    only:
        type: dummy
        state-server: false
`, func(c *gocheck.C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, gocheck.IsNil)
		c.Assert(e.Name(), gocheck.Equals, "only")
	}}, {`
default:
    invalid
environments:
    valid:
        type: dummy
        state-server: false
    invalid:
        type: crazy
`, func(c *gocheck.C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, gocheck.ErrorMatches, `environment "invalid" has an unknown provider type "crazy"`)
		c.Assert(e, gocheck.IsNil)
		e, err = es.Open("valid")
		c.Assert(err, gocheck.IsNil)
		c.Assert(e.Name(), gocheck.Equals, "valid")
	}}, {`
environments:
    one:
        type: dummy
        state-server: false
    two:
        type: dummy
        state-server: false
`, func(c *gocheck.C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, gocheck.ErrorMatches, `no default environment found`)
		c.Assert(e, gocheck.IsNil)
	}},
}

func (suite) TestConfig(c *gocheck.C) {
	defer testing.MakeFakeHomeNoEnvironments(c, "only", "valid", "one", "two").Restore()
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, gocheck.IsNil)
		t.check(c, es)
	}
}

func (suite) TestDefaultConfigFile(c *gocheck.C) {
	defer testing.MakeEmptyFakeHome(c).Restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, gocheck.IsNil)
	path := testing.HomePath(".juju", "environments.yaml")
	c.Assert(path, gocheck.Equals, outfile)

	es, err := environs.ReadEnvirons("")
	c.Assert(err, gocheck.IsNil)
	e, err := es.Open("")
	c.Assert(err, gocheck.IsNil)
	c.Assert(e.Name(), gocheck.Equals, "only")
}

func (suite) TestConfigPerm(c *gocheck.C) {
	defer testing.MakeSampleHome(c).Restore()

	path := testing.HomePath(".juju")
	info, err := os.Lstat(path)
	c.Assert(err, gocheck.IsNil)
	oldPerm := info.Mode().Perm()
	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	outfile, err := environs.WriteEnvirons("", env)
	c.Assert(err, gocheck.IsNil)

	info, err = os.Lstat(outfile)
	c.Assert(err, gocheck.IsNil)
	c.Assert(info.Mode().Perm(), gocheck.Equals, os.FileMode(0600))

	info, err = os.Lstat(filepath.Dir(outfile))
	c.Assert(err, gocheck.IsNil)
	c.Assert(info.Mode().Perm(), gocheck.Equals, oldPerm)

}

func (suite) TestNamedConfigFile(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	c.Assert(path, gocheck.Equals, outfile)

	es, err := environs.ReadEnvirons(path)
	c.Assert(err, gocheck.IsNil)
	e, err := es.Open("")
	c.Assert(err, gocheck.IsNil)
	c.Assert(e.Name(), gocheck.Equals, "only")
}

func (suite) TestConfigRoundTrip(c *gocheck.C) {
	cfg, err := config.New(map[string]interface{}{
		"name":            "bladaam",
		"type":            "dummy",
		"state-server":    false,
		"authorized-keys": "i-am-a-key",
		"ca-cert":         testing.CACert,
		"ca-private-key":  "",
	})
	c.Assert(err, gocheck.IsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, gocheck.IsNil)
	cfg, err = provider.Validate(cfg, nil)
	c.Assert(err, gocheck.IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, gocheck.IsNil)
	c.Assert(cfg.AllAttrs(), gocheck.DeepEquals, env.Config().AllAttrs())
}

func (suite) TestBootstrapConfig(c *gocheck.C) {
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
	c.Assert(err, gocheck.IsNil)
	cfg1, err := environs.BootstrapConfig(cfg)
	c.Assert(err, gocheck.IsNil)

	expect := cfg.AllAttrs()
	delete(expect, "secret")
	expect["admin-secret"] = ""
	expect["ca-private-key"] = ""
	c.Assert(cfg1.AllAttrs(), gocheck.DeepEquals, expect)
}

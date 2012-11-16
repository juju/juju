package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	_ "launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/version"
	"os"
	"path/filepath"
)

type suite struct{}

var _ = Suite(suite{})

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

func (suite) TestInvalidConfig(c *C) {
	for i, t := range invalidConfigTests {
		c.Logf("running test %v", i)
		_, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, ErrorMatches, t.err)
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
	}, {`
environments:
    only:
        type: dummy
        state-server: false
        unknown-value: causes-an-error
`, "only", `.*unknown-value: expected nothing, got "causes-an-error"`,
	},
}

func (suite) TestInvalidEnv(c *C) {
	defer makeFakeHome(c, "only").restore()
	for i, t := range invalidEnvTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Check(err, IsNil)
		e, err := es.Open(t.name)
		c.Check(err, ErrorMatches, t.err)
		c.Check(e, IsNil)
	}
}

var configTests = []struct {
	env   string
	check func(c *C, es *environs.Environs)
}{
	{`
environments:
    only:
        type: dummy
        state-server: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		c.Assert(e.Name(), Equals, "only")
	}}, {`
default:
    invalid
environments:
    valid:
        type: dummy
        state-server: false
    invalid:
        type: crazy
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, ErrorMatches, `environment "invalid" has an unknown provider type "crazy"`)
		c.Assert(e, IsNil)
		e, err = es.Open("valid")
		c.Assert(err, IsNil)
		c.Assert(e.Name(), Equals, "valid")
	}}, {`
environments:
    one:
        type: dummy
        state-server: false
    two:
        type: dummy
        state-server: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, ErrorMatches, `no default environment found`)
		c.Assert(e, IsNil)
	}},
}

func (suite) TestConfig(c *C) {
	defer makeFakeHome(c, "only", "valid", "one", "two").restore()
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, IsNil)
		t.check(c, es)
	}
}

func (suite) TestDefaultConfigFile(c *C) {
	defer makeFakeHome(c, "only").restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	err := ioutil.WriteFile(homePath(".juju", "environments.yaml"), []byte(env), 0666)
	c.Assert(err, IsNil)

	es, err := environs.ReadEnvirons("")
	c.Assert(err, IsNil)
	e, err := es.Open("")
	c.Assert(err, IsNil)
	c.Assert(e.Name(), Equals, "only")
}

func (suite) TestNamedConfigFile(c *C) {
	defer makeFakeHome(c, "only").restore()

	env := `
environments:
    only:
        type: dummy
        state-server: false
        authorized-keys: i-am-a-key
`
	path := filepath.Join(c.MkDir(), "a-file")
	err := ioutil.WriteFile(path, []byte(env), 0666)
	c.Assert(err, IsNil)

	es, err := environs.ReadEnvirons(path)
	c.Assert(err, IsNil)
	e, err := es.Open("")
	c.Assert(err, IsNil)
	c.Assert(e.Name(), Equals, "only")
}

func (suite) TestConfigRoundTrip(c *C) {
	cfg, err := config.New(map[string]interface{}{
		"name":             "bladaam",
		"type":             "dummy",
		"state-server":     false,
		"authorized-keys":  "i-am-a-key",
		"root-cert":        testing.RootCertPEM,
		"root-private-key": "",
	})
	c.Assert(err, IsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, IsNil)
	cfg, err = provider.Validate(cfg, nil)
	c.Assert(err, IsNil)
	env, err := environs.New(cfg)
	c.Assert(err, IsNil)
	c.Assert(cfg.AllAttrs(), DeepEquals, env.Config().AllAttrs())
}

func (suite) TestBootstrapConfig(c *C) {
	cfg, err := config.New(map[string]interface{}{
		"name":             "bladaam",
		"type":             "dummy",
		"state-server":     false,
		"admin-secret":     "highly",
		"secret":           "um",
		"authorized-keys":  "i-am-a-key",
		"root-cert":        testing.RootCertPEM,
		"root-private-key": testing.RootKeyPEM,
	})
	c.Assert(err, IsNil)
	provider, err := environs.Provider(cfg.Type())
	c.Assert(err, IsNil)

	tools := &state.Tools{
		URL:    "http://x",
		Binary: version.MustParseBinary("1.2.3-foo-bar"),
	}
	cfg1, err := environs.BootstrapConfig(provider, cfg, tools)
	c.Assert(err, IsNil)

	expect := cfg.AllAttrs()
	delete(expect, "secret")
	expect["admin-secret"] = ""
	expect["root-private-key"] = ""
	expect["agent-version"] = "1.2.3"
	c.Assert(cfg1.AllAttrs(), DeepEquals, expect)
}

type fakeHome string

func makeFakeHome(c *C, certNames ...string) fakeHome {
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", c.MkDir())

	err := os.Mkdir(homePath(".juju"), 0777)
	c.Assert(err, IsNil)
	for _, name := range certNames {
		err := ioutil.WriteFile(homePath(".juju", name+"-root-cert.pem"), testing.RootCertPEMBytes, 0666)
		c.Assert(err, IsNil)
	}

	err = os.Mkdir(homePath(".ssh"), 0777)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(homePath(".ssh", "id_rsa.pub"), []byte("auth key\n"), 0666)
	c.Assert(err, IsNil)

	return fakeHome(oldHome)
}

func homePath(names ...string) string {
	all := append([]string{os.Getenv("HOME")}, names...)
	return filepath.Join(all...)
}

func (h fakeHome) restore() {
	os.Setenv("HOME", string(h))
}

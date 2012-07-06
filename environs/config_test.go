package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	_ "launchpad.net/juju-core/environs/dummy"
	"os"
	"path/filepath"
)

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
`, "only", `.*zookeeper: expected bool, got nothing`,
	},
}

func (suite) TestInvalidEnv(c *C) {
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
        zookeeper: false
        unknown-setting: ignored
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
        zookeeper: false
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
        zookeeper: false
    two:
        type: dummy
        zookeeper: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, ErrorMatches, `no default environment found`)
		c.Assert(e, IsNil)
	}},
}

func (suite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		c.Assert(err, IsNil)
		t.check(c, es)
	}
}

func (suite) TestConfigFile(c *C) {
	d := c.MkDir()
	err := os.Mkdir(filepath.Join(d, ".juju"), 0777)
	c.Assert(err, IsNil)

	path := filepath.Join(d, ".juju", "environments.yaml")
	env := `
environments:
    only:
        type: dummy
        zookeeper: false
`
	err = ioutil.WriteFile(path, []byte(env), 0666)
	c.Assert(err, IsNil)

	// test reading from a named file
	es, err := environs.ReadEnvirons(path)
	c.Assert(err, IsNil)
	e, err := es.Open("")
	c.Assert(err, IsNil)
	c.Assert(e.Name(), Equals, "only")

	// test reading from the default environments.yaml file.
	h := os.Getenv("HOME")
	os.Setenv("HOME", d)

	es, err = environs.ReadEnvirons("")
	c.Assert(err, IsNil)
	e, err = es.Open("")
	c.Assert(err, IsNil)
	c.Assert(e.Name(), Equals, "only")

	// reset $HOME just in case something else relies on it.
	os.Setenv("HOME", h)
}

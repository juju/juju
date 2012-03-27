package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	_ "launchpad.net/juju/go/environs/dummy"
	"os"
	"path/filepath"
)

type configTest struct {
	env   string
	check func(c *C, es *environs.Environs)
}

var configTests = []struct {
	env   string
	check func(c *C, es *environs.Environs)
}{
	{`
environments:
    only:
        type: unknown
        other: anything
        zookeeper: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(e, IsNil)
		c.Assert(err, NotNil)
		c.Assert(err.Error(), Equals, `environment "only" has an unknown provider type: "unknown"`)
	},
	},
	// one known environment, no defaults, bad attribute -> parse error
	{`
environments:
    only:
        type: dummy
        badattr: anything
        zookeeper: false
`, nil,
	},
	// one known environment, no defaults -> parse ok, instantiate ok
	{`
environments:
    only:
        type: dummy
        zookeeper: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		checkEnvironName(c, e, "only")
	},
	},
	// several environments, no defaults -> parse ok, instantiate maybe error
	{`
environments:
    one:
        type: dummy
        zookeeper: false
    two:
        type: dummy
        zookeeper: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, NotNil)
		e, err = es.Open("one")
		c.Assert(err, IsNil)
		checkEnvironName(c, e, "one")
	},
	},
	// several environments, default -> parse ok, instantiate ok
	{`
default:
    two
environments:
   one:
        type: dummy
        zookeeper: false
   two:
        type: dummy
        zookeeper: false
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		checkEnvironName(c, e, "two")
	},
	},
}

// checkEnvironName checks that a new instance started
// by the given Environ has an id starting with name,
// which implies that it is the expected environment.
func checkEnvironName(c *C, e environs.Environ, name string) {
	i0, err := e.StartInstance(0, nil)
	c.Assert(err, IsNil)
	c.Assert(i0, NotNil)
	c.Assert(i0.Id(), Matches, name+".*")
}

func (suite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := environs.ReadEnvironsBytes([]byte(t.env))
		if es == nil {
			c.Logf("parse failed\n")
			if t.check != nil {
				c.Errorf("test %d failed: %v", i, err)
			}
		} else {
			if t.check == nil {
				c.Errorf("test %d parsed ok but should have failed", i)
				continue
			}
			c.Logf("checking...")
			t.check(c, es)
		}
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
	checkEnvironName(c, e, "only")

	// test reading from the default environments.yaml file.
	h := os.Getenv("HOME")
	os.Setenv("HOME", d)

	es, err = environs.ReadEnvirons("")
	c.Assert(err, IsNil)
	e, err = es.Open("")
	c.Assert(err, IsNil)
	checkEnvironName(c, e, "only")

	// reset $HOME just in case something else relies on it.
	os.Setenv("HOME", h)
}

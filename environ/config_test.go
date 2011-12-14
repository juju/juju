package environ_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environ"
	"os"
	"path/filepath"
)

type configTest struct {
	env   string
	check func(c *C, es *environ.Environs)
}

var configTests = []struct {
	env   string
	check func(c *C, es *environ.Environs)
}{
	{`
environments:
    only:
        type: unknown
        other: anything
`, func(c *C, es *environ.Environs) {
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
`, nil,
	},
	// one known environment, no defaults -> parse ok, instantiate ok
	{`
environments:
    only:
        type: dummy
        basename: foo
`, func(c *C, es *environ.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		checkDummyEnviron(c, e, "foo")
	},
	},
	// several environments, no defaults -> parse ok, instantiate maybe error
	{`
environments:
    one:
        type: dummy
        basename: foo
    two:
        type: dummy
        basename: bar
`, func(c *C, es *environ.Environs) {
		e, err := es.Open("")
		c.Assert(err, NotNil)
		e, err = es.Open("one")
		c.Assert(err, IsNil)
		checkDummyEnviron(c, e, "foo")
	},
	},
	// several environments, default -> parse ok, instantiate ok
	{`
default:
    two
environments:
    one:
        type: dummy
        basename: foo
    two:
        type: dummy
        basename: bar
`, func(c *C, es *environ.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		checkDummyEnviron(c, e, "bar")
	},
	},
}

func checkDummyEnviron(c *C, e environ.Environ, basename string) {
	i0, err := e.StartInstance(0)
	c.Assert(err, IsNil)
	c.Assert(i0, NotNil)
	c.Assert(i0.DNSName(), Equals, basename+"-0")

	is, err := e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(is), Equals, 1)
	c.Assert(is[0], Equals, i0)

	i1, err := e.StartInstance(1)
	c.Assert(err, IsNil)
	c.Assert(i1, NotNil)
	c.Assert(i1.DNSName(), Equals, basename+"-1")

	is, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(is), Equals, 2)
	if is[0] == i1 {
		is[0], is[1] = is[1], is[0]
	}
	c.Assert(is[0], Equals, i0)
	c.Assert(is[1], Equals, i1)

	err = e.StopInstances([]environ.Instance{i0})
	c.Assert(err, IsNil)

	is, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(is), Equals, 1)
	c.Assert(is[0], Equals, i1)

	err = e.Destroy()
	c.Assert(err, IsNil)
}

func (suite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := environ.ReadEnvironsBytes([]byte(t.env))
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
        basename: foo
`
	err = ioutil.WriteFile(path, []byte(env), 0666)
	c.Assert(err, IsNil)

	// test reading from a named file
	es, err := environ.ReadEnvirons(path)
	c.Assert(err, IsNil)
	e, err := es.Open("")
	c.Assert(err, IsNil)
	checkDummyEnviron(c, e, "foo")

	// test reading from the default environments.yaml file.
	h := os.Getenv("HOME")
	os.Setenv("HOME", d)

	es, err = environ.ReadEnvirons("")
	c.Assert(err, IsNil)
	e, err = es.Open("")
	c.Assert(err, IsNil)
	checkDummyEnviron(c, e, "foo")

	// reset $HOME just in case something else relies on it.
	os.Setenv("HOME", h)
}

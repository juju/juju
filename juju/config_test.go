package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"os"
	"path/filepath"
)

type configTest struct {
	env   string
	check func(c *C, es *juju.Environs)
}

var configTests = []struct {
	env   string
	check func(c *C, es *juju.Environs)
}{
	{`
environments:
    only:
        type: unknown
        other: anything
`, func(c *C, es *juju.Environs) {
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
`, func(c *C, es *juju.Environs) {
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
`, func(c *C, es *juju.Environs) {
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
`, func(c *C, es *juju.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		checkDummyEnviron(c, e, "bar")
	},
	},
}

func checkDummyEnviron(c *C, e juju.Environ, basename string) {
	c.Assert(e, NotNil)
	err := e.Bootstrap()
	c.Assert(err, IsNil)

	m0, err := e.StartMachine("0")
	c.Assert(err, IsNil)
	c.Assert(m0, NotNil)
	c.Assert(m0.DNSName(), Equals, basename+"-0")

	ms, err := e.Machines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 1)
	c.Assert(ms[0], Equals, m0)

	m1, err := e.StartMachine("1")
	c.Assert(err, IsNil)
	c.Assert(m1, NotNil)
	c.Assert(m1.DNSName(), Equals, basename+"-1")

	ms, err = e.Machines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 2)
	if ms[0] == m1 {
		ms[0], ms[1] = ms[1], ms[0]
	}
	c.Assert(ms[0], Equals, m0)
	c.Assert(ms[1], Equals, m1)

	err = e.StopMachines([]juju.Machine{m0})
	c.Assert(err, IsNil)

	ms, err = e.Machines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 1)
	c.Assert(ms[0], Equals, m1)

	err = e.Destroy()
	c.Assert(err, IsNil)
}

func (suite) TestConfig(c *C) {
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := juju.ReadEnvironsBytes([]byte(t.env))
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
	es, err := juju.ReadEnvirons(path)
	c.Assert(err, IsNil)
	e, err := es.Open("")
	c.Assert(err, IsNil)
	checkDummyEnviron(c, e, "foo")

	// test reading from the default environments.yaml file.
	h := os.Getenv("HOME")
	os.Setenv("HOME", d)

	es, err = juju.ReadEnvirons("")
	c.Assert(err, IsNil)
	e, err = es.Open("")
	c.Assert(err, IsNil)
	checkDummyEnviron(c, e, "foo")

	// reset $HOME just in case something else relies on it.
	os.Setenv("HOME", h)
}

package juju_test

import (
	"io/ioutil"
	C "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
	"os"
	"path/filepath"
)

type configTest struct {
	env   string
	check func(c *C.C, es *juju.Environs)
}

var configTests = []struct {
	env   string
	check func(c *C.C, es *juju.Environs)
}{
	{`
environments:
    only:
        type: unknown
        other: anything
`, func(c *C.C, es *juju.Environs) {
		e, err := es.New("")
		c.Assert(e, C.IsNil)
		c.Assert(err, C.NotNil)
		c.Assert(err.String(), C.Equals, `environment "only" has an unknown provider type: "unknown"`)
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
`, func(c *C.C, es *juju.Environs) {
		e, err := es.New("")
		c.Assert(err, C.IsNil)
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
`, func(c *C.C, es *juju.Environs) {
		e, err := es.New("")
		c.Assert(err, C.NotNil)
		e, err = es.New("one")
		c.Assert(err, C.IsNil)
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
`, func(c *C.C, es *juju.Environs) {
		conn, err := es.New("")
		c.Assert(err, C.IsNil)
		checkDummyEnviron(c, conn, "bar")
	},
	},
}

func checkDummyEnviron(c *C.C, conn *juju.Conn, basename string) {
	c.Assert(conn, C.NotNil)
	err := conn.Bootstrap()
	c.Assert(err, C.IsNil)
	e := conn.Environ()

	m0, err := e.StartMachine()
	c.Assert(err, C.IsNil)
	c.Assert(m0, C.NotNil)
	c.Assert(m0.DNSName(), C.Equals, basename+"-0")

	ms, err := e.Machines()
	c.Assert(err, C.IsNil)
	c.Assert(len(ms), C.Equals, 1)
	c.Assert(ms[0], C.Equals, m0)

	m1, err := e.StartMachine()
	c.Assert(err, C.IsNil)
	c.Assert(m1, C.NotNil)
	c.Assert(m1.DNSName(), C.Equals, basename+"-1")

	ms, err = e.Machines()
	c.Assert(err, C.IsNil)
	c.Assert(len(ms), C.Equals, 2)
	if ms[0] == m1 {
		ms[0], ms[1] = ms[1], ms[0]
	}
	c.Assert(ms[0], C.Equals, m0)
	c.Assert(ms[1], C.Equals, m1)

	err = e.StopMachines([]juju.Machine{m0})
	c.Assert(err, C.IsNil)

	ms, err = e.Machines()
	c.Assert(err, C.IsNil)
	c.Assert(len(ms), C.Equals, 1)
	c.Assert(ms[0], C.Equals, m1)

	err = e.Destroy()
	c.Assert(err, C.IsNil)
}

func (suite) TestConfig(c *C.C) {
	for i, t := range configTests {
		c.Logf("running test %v", i)
		es, err := juju.ParseEnvironments([]byte(t.env))
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

func (suite) TestConfigFile(c *C.C) {
	d := c.MkDir()
	err := os.Mkdir(filepath.Join(d, ".juju"), 0777)
	c.Assert(err, C.IsNil)

	path := filepath.Join(d, ".juju", "environments.yaml")
	env := `
environments:
    only:
        type: dummy
        basename: foo
`
	err = ioutil.WriteFile(path, []byte(env), 0666)
	c.Assert(err, C.IsNil)

	// test reading from a named file
	es, err := juju.ReadEnvironments(path)
	c.Assert(err, C.IsNil)
	e, err := es.New("")
	c.Assert(err, C.IsNil)
	checkDummyEnviron(c, e, "foo")

	// test reading from the default environments.yaml file.
	h := os.Getenv("HOME")
	os.Setenv("HOME", d)

	es, err = juju.ReadEnvironments("")
	c.Assert(err, C.IsNil)
	e, err = es.New("")
	c.Assert(err, C.IsNil)
	checkDummyEnviron(c, e, "foo")

	// reset $HOME just in case something else relies on it.
	os.Setenv("HOME", h)
}

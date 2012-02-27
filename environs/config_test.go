package environs_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
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
`, nil,
	},
	// one known environment, no defaults -> parse ok, instantiate ok
	{`
environments:
    only:
        type: dummy
        basename: foo
`, func(c *C, es *environs.Environs) {
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
`, func(c *C, es *environs.Environs) {
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
`, func(c *C, es *environs.Environs) {
		e, err := es.Open("")
		c.Assert(err, IsNil)
		checkDummyEnviron(c, e, "bar")
	},
	},
}

func checkDummyEnviron(c *C, e environs.Environ, basename string) {
	i0, err := e.StartInstance(0, nil)
	c.Assert(err, IsNil)
	c.Assert(i0, NotNil)
	c.Assert(i0.DNSName(), Equals, basename+"-0.foo")

	is, err := e.Instances([]string{i0.Id()})
	c.Assert(err, IsNil)
	c.Assert(is, HasLen, 1)
	c.Assert(is[0].Id(), Equals, i0.Id())

	i1, err := e.StartInstance(1, nil)
	c.Assert(err, IsNil)
	c.Assert(i1, NotNil)
	c.Assert(i1.DNSName(), Equals, basename+"-1.foo")

	is, err = e.Instances([]string{i0.Id(), i1.Id()})
	c.Assert(err, IsNil)
	c.Assert(is, HasLen, 2)
	c.Assert(is[0].Id(), Equals, i0.Id())
	c.Assert(is[1].Id(), Equals, i1.Id())

	err = e.StopInstances([]environs.Instance{i0})
	c.Assert(err, IsNil)

	is, err = e.Instances([]string{i0.Id(), i1.Id()})
	c.Assert(err, Equals, environs.ErrMissingInstance)
	c.Assert(is, HasLen, 2)
	c.Assert(is[0], IsNil)
	c.Assert(is[1].Id(), Equals, i1.Id())

	err = e.Destroy(nil)
	c.Assert(err, IsNil)
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
        basename: foo
`
	err = ioutil.WriteFile(path, []byte(env), 0666)
	c.Assert(err, IsNil)

	// test reading from a named file
	es, err := environs.ReadEnvirons(path)
	c.Assert(err, IsNil)
	e, err := es.Open("")
	c.Assert(err, IsNil)
	checkDummyEnviron(c, e, "foo")

	// test reading from the default environments.yaml file.
	h := os.Getenv("HOME")
	os.Setenv("HOME", d)

	es, err = environs.ReadEnvirons("")
	c.Assert(err, IsNil)
	e, err = es.Open("")
	c.Assert(err, IsNil)
	checkDummyEnviron(c, e, "foo")

	// reset $HOME just in case something else relies on it.
	os.Setenv("HOME", h)
}

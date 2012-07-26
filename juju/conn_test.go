package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state/testing"
	coretesting "launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	coretesting.ZkTestPackage(t)
}

type ConnSuite struct {
	coretesting.LoggingSuite
	testing.StateSuite
}

var _ = Suite(&ConnSuite{})

func (cs *ConnSuite) SetUpTest(c *C) {
	cs.LoggingSuite.SetUpTest(c)
	cs.StateSuite.SetUpTest(c)
}

func (cs *ConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	cs.StateSuite.TearDownTest(c)
	cs.LoggingSuite.TearDownTest(c)
}

func (*ConnSuite) TestNewConn(c *C) {
	home := c.MkDir()
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", home)
	conn, err := juju.NewConn("")
	c.Assert(conn, IsNil)
	c.Assert(err, ErrorMatches, ".*: no such file or directory")

	if err := os.Mkdir(filepath.Join(home, ".juju"), 0755); err != nil {
		c.Log("Could not create directory structure")
		c.Fail()
	}
	envs := filepath.Join(home, ".juju", "environments.yaml")
	err = ioutil.WriteFile(envs, []byte(`
default:
    erewhemos
environments:
    erewhemos:
        type: dummy
        zookeeper: true
        authorized-keys: i-am-a-key
`), 0644)
	if err != nil {
		c.Log("Could not create environments.yaml")
		c.Fail()
	}

	// Just run through a few operations on the dummy provider and verify that
	// they behave as expected.
	conn, err = juju.NewConn("")
	c.Assert(err, IsNil)
	defer conn.Close()
	st, err := conn.State()
	c.Assert(st, IsNil)
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")
	err = conn.Bootstrap(false)
	c.Assert(err, IsNil)
	st, err = conn.State()
	c.Check(err, IsNil)
	c.Check(st, NotNil)
	err = conn.Destroy()
	c.Assert(err, IsNil)

	// Close the conn (thereby closing its state) a couple of times to
	// verify that multiple closes are safe.
	c.Assert(conn.Close(), IsNil)
	c.Assert(conn.Close(), IsNil)
}

func (*ConnSuite) TestNewConnFromAttrs(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	conn, err := juju.NewConnFromAttrs(attrs)
	c.Assert(err, IsNil)
	defer conn.Close()
	st, err := conn.State()
	c.Assert(st, IsNil)
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")
}

func (cs *ConnSuite) TestConnStateSecretsSideEffect(c *C) {
	env, err := cs.State.EnvironConfig()
	c.Assert(err, IsNil)
	secret, ok := env.Get("secret")
	c.Assert(ok, Equals, false)
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"zookeeper":       true,
		"authorized-keys": "i-am-a-key",
	}
	conn, err := juju.NewConnFromAttrs(attrs)
	c.Assert(err, IsNil)
	defer conn.Close()
	err = conn.Bootstrap(false)
	c.Assert(err, IsNil)
	// fetch a state connection via the conn, which will 
	// update the secrets.
	_, err = conn.State()
	c.Assert(err, IsNil)
	err = env.Read()
	c.Assert(err, IsNil)
	secret, ok = env.Get("secret")
	c.Assert(ok, Equals, true)
	c.Assert(secret, Equals, "pork")
}

func (*ConnSuite) TestValidRegexps(c *C) {
	assertService := func(s string, expect bool) {
		c.Assert(juju.ValidService.MatchString(s), Equals, expect)
		c.Assert(juju.ValidUnit.MatchString(s+"/0"), Equals, expect)
		c.Assert(juju.ValidUnit.MatchString(s+"/99"), Equals, expect)
		c.Assert(juju.ValidUnit.MatchString(s+"/-1"), Equals, false)
		c.Assert(juju.ValidUnit.MatchString(s+"/blah"), Equals, false)
	}
	assertService("", false)
	assertService("33", false)
	assertService("wordpress", true)
	assertService("w0rd-pre55", true)
	assertService("foo2", true)
	assertService("foo-2", false)
	assertService("foo-2foo", true)
}

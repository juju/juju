package juju_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/dummy"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/trivial"
	coretesting "launchpad.net/juju-core/testing"
	"os"
	"path/filepath"
	stdtesting "testing"
)

func Test(t *stdtesting.T) {
	coretesting.MgoTestPackage(t)
}

type ConnSuite struct {
	coretesting.LoggingSuite
}

var _ = Suite(&ConnSuite{})

func (cs *ConnSuite) TearDownTest(c *C) {
	dummy.Reset()
	cs.LoggingSuite.TearDownTest(c)
}

func (*ConnSuite) TestNewConnFromName(c *C) {
	home := c.MkDir()
	defer os.Setenv("HOME", os.Getenv("HOME"))
	os.Setenv("HOME", home)
	conn, err := juju.NewConnFromName("")
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
        state-server: true
        authorized-keys: i-am-a-key
`), 0644)
	if err != nil {
		c.Log("Could not create environments.yaml")
		c.Fail()
	}

	// Just run through a few operations on the dummy provider and verify that
	// they behave as expected.
	conn, err = juju.NewConnFromName("")
	c.Assert(err, ErrorMatches, "dummy environment not bootstrapped")

	environ, err := environs.NewFromName("")
	c.Assert(err, IsNil)
	err = environ.Bootstrap(false)
	c.Assert(err, IsNil)

	conn, err = juju.NewConnFromName("")
	c.Assert(err, IsNil)
	defer conn.Close()
	c.Assert(conn.Environ, NotNil)
	c.Assert(conn.Environ.Name(), Equals, "erewhemos")
	c.Assert(conn.State, NotNil)

	// Close the conn (thereby closing its state) a couple of times to
	// verify that multiple closes will not panic. We ignore the error,
	// as the underlying State will return an error the second
	// time.
	conn.Close()
	conn.Close()
}

func (cs *ConnSuite) TestConnStateSecretsSideEffect(c *C) {
	attrs := map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "pork",
	}
	env, err := environs.NewFromAttrs(attrs)
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)
	info, err := env.StateInfo()
	c.Assert(err, IsNil)
	st, err := state.Open(info)
	c.Assert(err, IsNil)

	// Verify we have no secret in the environ config
	cfg, err := st.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], IsNil)

	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()
	// fetch a state connection via the conn, which will 
	// push the secrets.
	cfg, err = conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], Equals, "pork")
}

func (cs *ConnSuite) TestConnStateDoesNotUpdateExistingSecrets(c *C) {
	cs.TestConnStateSecretsSideEffect(c)
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "squirrel",
	})
	c.Assert(err, IsNil)
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	defer conn.Close()
	// check that the secret has not changed
	cfg, err := conn.State.EnvironConfig()
	c.Assert(err, IsNil)
	c.Assert(cfg.UnknownAttrs()["secret"], Equals, "pork")
}

func (cs *ConnSuite) TestConnWithPassword(c *C) {
	env, err := environs.NewFromAttrs(map[string]interface{}{
		"name":            "erewhemos",
		"type":            "dummy",
		"state-server":    true,
		"authorized-keys": "i-am-a-key",
		"secret":          "squirrel",
		"admin-secret": "nutkin",
	})
	c.Assert(err, IsNil)
	err = env.Bootstrap(false)
	c.Assert(err, IsNil)
	info, err := env.StateInfo()
	c.Assert(err, IsNil)
	st, err := state.Open(info)
	c.Assert(err, IsNil)
	defer st.Close()

	err = st.SetAdminPassword(trivial.PasswordHash("nutkin"))
	c.Assert(err, IsNil)
	defer func() {
		c.Check(st.SetAdminPassword(""), IsNil)
	}()

	// Check that we can connect with the original environment.
	conn, err := juju.NewConn(env)
	c.Assert(err, IsNil)
	conn.Close()

	// Check that the password has been changed to
	// the original admin password.
	info.Password = "nutkin"
	st1, err := state.Open(info)
	c.Assert(err, IsNil)
	st1.Close()

	// Finally check that we can still connect with the
	// original environment.
	conn, err = juju.NewConn(env)
	c.Assert(err, IsNil)
	conn.Close()
}
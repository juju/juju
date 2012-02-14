package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
	"launchpad.net/juju/go/state"
)


// InvalidStateInfo holds information about no state - it will always give
// an error when connected to.
var InvalidStateInfo = &state.Info{
	Addrs: []string{"0.1.2.3:1234"},
}
	
// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with Name that must exist within Environs.
// Env holds an instance of that environment that is opened before each
// test and Destroyed after each test.
type Tests struct {
	Environs *environs.Environs
	Name     string
	Env environs.Environ
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *C) environs.Environ {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil, Bug("opening environ %q", t.Name))
	c.Assert(e, NotNil)
	return e
}

func (t *Tests) SetUpSuite(*C) {
}

func (t *Tests) TearDownSuite(*C) {
}

func (t *Tests) SetUpTest(c *C) {
	t.Env = t.Open(c)
}

func (t *Tests) TearDownTest(*C) {
	t.Env.Destroy(nil)
	t.Env = nil
}

// LiveTests is a gocheck suite containing tests designed to run against a
// live server.  It opens the environment with Name once only for the whole
// suite, stores it in Env, and Destroys it after the suite has completed.
type LiveTests struct {
	Environs *environs.Environs
	Name     string
	Env      environs.Environ
}

func (t *LiveTests) SetUpSuite(c *C) {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil, Bug("opening environ %q", t.Name))
	c.Assert(e, NotNil)
	t.Env = e
}

func (t *LiveTests) TearDownSuite(c *C) {
	err := t.Env.Destroy(nil)
	c.Check(err, IsNil)
	t.Env = nil
}

func (t *LiveTests) SetUpTest(*C) {
}

func (t *LiveTests) TearDownTest(*C) {
}

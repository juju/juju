package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/environs"
	"launchpad.net/juju-core/juju/state"
	"time"
)

// InvalidStateInfo holds information about no state - it will always give
// an error when connected to.
var InvalidStateInfo = &state.Info{
	Addrs: []string{"0.1.2.3:1234"},
}

// Tests is a gocheck suite containing tests verifying juju functionality
// against the environment with Name that must exist within Environs. The
// tests are not designed to be run against a live server - the Environ
// is opened once for each test, and some potentially expensive operations
// may be executed.
type Tests struct {
	Environs *environs.Environs
	Name     string
	Env      environs.Environ
}

// Open opens an instance of the testing environment.
func (t *Tests) Open(c *C) environs.Environ {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil, Commentf("opening environ %q", t.Name))
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

// LiveTests contains tests that are designed to run against a live server
// (e.g. Amazon EC2).  The Environ is opened once only for all the tests
// in the suite, stored in Env, and Destroyed after the suite has completed.
type LiveTests struct {
	Environs *environs.Environs
	Name     string
	Env      environs.Environ
	// ConsistencyDelay gives the length of time to wait before
	// environ becomes logically consistent.
	ConsistencyDelay time.Duration

	// CanOpenState should be true if the testing environment allows
	// the state to be opened after bootstrapping.
	CanOpenState bool
	bootstrapped bool
}

func (t *LiveTests) SetUpSuite(c *C) {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil, Commentf("opening environ %q", t.Name))
	c.Assert(e, NotNil)
	t.Env = e
}

func (t *LiveTests) BootstrapOnce(c *C) {
	if t.bootstrapped {
		return
	}
	err := t.Env.Bootstrap(true)
	c.Assert(err, IsNil)
	t.bootstrapped = true
}

func (t *LiveTests) Destroy(c *C) {
	err := t.Env.Destroy(nil)
	c.Assert(err, IsNil)
	t.bootstrapped = false
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

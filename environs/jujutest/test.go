package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
)

// Tests is a gocheck suite containing tests verifying
// juju functionality against the environment with Name that
// must exist within Environs.
type Tests struct {
	Environs *environs.Environs
	Name     string
	Env      environs.Environ

	environs []environs.Environ
}

func (t *Tests) Open(c *C) environs.Environ {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil, Bug("opening environ %q", t.Name))
	c.Assert(e, NotNil)
	t.environs = append(t.environs, e)
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
	t.Env.Destroy()
	t.Env = nil
}

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
	t.Env = nil
}

func (t *LiveTests) SetUpTest(*C) {
}

func (t *LiveTests) TearDownTest(*C) {
}

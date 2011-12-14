package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
)

// Tests is a gocheck suite containing tests verifying
// juju functionality against the environment with Name that
// must exist within Environs.
type Tests struct {
	Environs *juju.Environs
	Name     string
	SetUp func(c *C)
	TearDown func(c *C)

	environs []juju.Environ
}

func (t *Tests) open(c *C) juju.Environ {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil, Bug("opening environ %q", t.Name))
	c.Assert(e, NotNil)
	t.environs = append(t.environs, e)
	return e
}

func (t *Tests) TearDownTest(c *C) {
	for _, e := range t.environs {
		err := e.Destroy()
		if err != nil {
			c.Errorf("error destroying environment after test: %v", err)
		}
	}
	t.environs = nil
}

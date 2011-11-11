package jujutest

import (
	"launchpad.net/juju/go/juju"
	. "launchpad.net/gocheck"
)

func (t *Tests) TestStartStop(c *C) {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)

	err = e.Bootstrap()
	c.Assert(err, IsNil)

	t.addEnviron(e)

	ms, err := e.Machines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 0)

	m, err := e.StartMachine("0")
	c.Assert(m.Id(), Equals, "0")

	ms, err = e.Machines()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 1)
	c.Assert(m.Id(), Equals, "0")

	err = e.StopMachines([]juju.Machine{m})
	c.Assert(err, IsNil)
}

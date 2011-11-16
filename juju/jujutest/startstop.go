package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
)

func (t *Tests) TestStartStop(c *C) {
	e, err := t.Environs.Open(t.Name)
	c.Assert(err, IsNil)
	c.Assert(e, NotNil)

	err = e.Bootstrap()
	c.Assert(err, IsNil)

	t.addEnviron(e)

	ms, err := e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 0)

	m, err := e.StartInstance("0")
	c.Assert(err, IsNil)
	c.Assert(m, NotNil)
	id0 := m.Id()

	ms, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(ms), Equals, 1)
	c.Assert(m.Id(), Equals, id0)

	err = e.StopInstances([]juju.Instance{m})
	c.Assert(err, IsNil)
}

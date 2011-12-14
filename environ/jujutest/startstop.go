package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/juju"
)

func (t *Tests) TestStartStop(c *C) {
	e := t.open(c)

	is, err := e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(is), Equals, 0)

	m, err := e.StartInstance(0)
	c.Assert(err, IsNil)
	c.Assert(m, NotNil)
	id0 := m.Id()

	is, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(is), Equals, 1)
	c.Assert(m.Id(), Equals, id0)

	err = e.StopInstances([]juju.Instance{m})
	c.Assert(err, IsNil)

	is, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(is), Equals, 0)
}

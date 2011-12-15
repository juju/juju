package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
)

func (t *Tests) TestStartStop(c *C) {
	e := t.open(c)

	insts, err := e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)

	inst, err := e.StartInstance(0)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 1)
	c.Assert(insts[0].Id(), Equals, id0)

	err = e.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	insts, err = e.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)
}

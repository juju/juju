package jujutest

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/environs"
)

// TestStartStop is similar to Tests.TestStartStop except
// that it does not assume a pristine environment.
func (t *LiveTests) TestStartStop(c *C) {
	names := make(map[string] environs.Instance)
	insts, err := t.env.Instances()
	c.Assert(err, IsNil)
	c.Assert(insts, NotNil)

	// check there are no duplicate instance ids
	for _, inst := range insts {
		id := inst.Id()
		c.Assert(names[id], IsNil)
		names[id] = inst
	}

	inst, err := t.env.StartInstance(0)
	c.Assert(err, IsNil)
	c.Assert(inst, NotNil)
	id0 := inst.Id()

	insts, err = t.env.Instances()
	c.Assert(err, IsNil)

	// check the new instance is found
	found := false
	for _, inst := range insts {
		if inst.Id() == id0 {
			c.Assert(found, Equals, false)
			found = true
		}
	}
	c.Check(found, Equals, true)

	err = t.env.StopInstances([]environs.Instance{inst})
	c.Assert(err, IsNil)

	insts, err = t.env.Instances()
	c.Assert(err, IsNil)
	c.Assert(len(insts), Equals, 0)

	// check the instance is no longer there.
	found = true
	for _, inst := range insts {
		c.Assert(inst.Id(), Not(Equals), id0)
	}
}

package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type ConstraintsSuite struct{}

var _ = Suite(&ConstraintsSuite{})

func (s *ConstraintsSuite) TestString(c *C) {
	cons := state.Constraints{}
	c.Assert(cons.String(), Equals, "")

	var cores int
	cons.Cores = &cores
	c.Assert(cons.String(), Equals, "cores=0")

	cores = 128
	c.Assert(cons.String(), Equals, "cores=128")

	var mem float64
	cons.Mem = &mem
	c.Assert(cons.String(), Equals, "cores=128 mem=0M")

	cons.Cores = nil
	mem = 9876543.21
	c.Assert(cons.String(), Equals, "mem=9876543.21M")
}

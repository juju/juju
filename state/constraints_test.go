package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type ConstraintsSuite struct{}

var _ = Suite(&ConstraintsSuite{})

func uint64p(i uint64) *uint64 {
	return &i
}

var constraintsStringTests = []struct {
	cons state.Constraints
	str  string
}{
	{}, // Nothing set, nothing stringed.
	{
		state.Constraints{CpuCores: uint64p(0)},
		"cpu-cores=",
	}, {
		state.Constraints{CpuCores: uint64p(128)},
		"cpu-cores=128",
	}, {
		state.Constraints{CpuPower: uint64p(0)},
		"cpu-power=",
	}, {
		state.Constraints{CpuPower: uint64p(250)},
		"cpu-power=250",
	}, {
		state.Constraints{Mem: uint64p(0)},
		"mem=",
	}, {
		state.Constraints{Mem: uint64p(98765)},
		"mem=98765M",
	}, {
		state.Constraints{
			CpuCores: uint64p(4096),
			CpuPower: uint64p(9001),
			Mem:      uint64p(18000000000),
		},
		"cpu-cores=4096 cpu-power=9001 mem=18000000000M",
	},
}

func (s *ConstraintsSuite) TestString(c *C) {
	for i, t := range constraintsStringTests {
		c.Logf("test %d", i)
		c.Assert(t.cons.String(), Equals, t.str)
	}
}

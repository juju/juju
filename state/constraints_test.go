package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type ConstraintsSuite struct{}

var _ = Suite(&ConstraintsSuite{})

var constraintsStringTests = []struct {
	cpuCores interface{}
	cpuPower interface{}
	mem      interface{}
	str      string
}{
	{}, // Nothing set, nothing stringed.
	{
		cpuCores: 0,
		str:      "cpu-cores=",
	}, {
		cpuCores: 128,
		str:      "cpu-cores=128",
	}, {
		cpuPower: 0.0,
		str:      "cpu-power=",
	}, {
		cpuPower: 123.45,
		str:      "cpu-power=123.45",
	}, {
		mem: 0.0,
		str: "mem=",
	}, {
		mem: 98765.4321,
		str: "mem=98765.4321M",
	}, {
		cpuCores: 4096,
		cpuPower: 9000.3,
		mem:      18000000000.0,
		str:      "cpu-cores=4096 cpu-power=9000.3 mem=18000000000M",
	},
}

func (s *ConstraintsSuite) TestString(c *C) {
	for i, t := range constraintsStringTests {
		c.Logf("test %d", i)
		cons := state.Constraints{}
		if t.cpuCores != nil {
			cpuCores := t.cpuCores.(int)
			cons.CpuCores = &cpuCores
		}
		if t.cpuPower != nil {
			cpuPower := t.cpuPower.(float64)
			cons.CpuPower = &cpuPower
		}
		if t.mem != nil {
			mem := t.mem.(float64)
			cons.Mem = &mem
		}
		c.Assert(cons.String(), Equals, t.str)
	}
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/instance"
)

type HardwareSuite struct{}

var _ = gc.Suite(&HardwareSuite{})

var parseHardwareTests = []struct {
	summary string
	args    []string
	err     string
}{
	// Simple errors.
	{
		summary: "nothing at all",
	}, {
		summary: "empty",
		args:    []string{"     "},
	}, {
		summary: "complete nonsense",
		args:    []string{"cheese"},
		err:     `malformed characteristic "cheese"`,
	}, {
		summary: "missing name",
		args:    []string{"=cheese"},
		err:     `malformed characteristic "=cheese"`,
	}, {
		summary: "unknown characteristic",
		args:    []string{"cheese=edam"},
		err:     `unknown characteristic "cheese"`,
	},

	// "arch" in detail.
	{
		summary: "set arch empty",
		args:    []string{"arch="},
	}, {
		summary: "set arch amd64",
		args:    []string{"arch=amd64"},
	}, {
		summary: "set arch i386",
		args:    []string{"arch=i386"},
	}, {
		summary: "set arch armhf",
		args:    []string{"arch=armhf"},
	}, {
		summary: "set nonsense arch 1",
		args:    []string{"arch=cheese"},
		err:     `bad "arch" characteristic: "cheese" not recognized`,
	}, {
		summary: "set nonsense arch 2",
		args:    []string{"arch=123.45"},
		err:     `bad "arch" characteristic: "123.45" not recognized`,
	}, {
		summary: "double set arch together",
		args:    []string{"arch=amd64 arch=amd64"},
		err:     `bad "arch" characteristic: already set`,
	}, {
		summary: "double set arch separately",
		args:    []string{"arch=armhf", "arch="},
		err:     `bad "arch" characteristic: already set`,
	},

	// "cpu-cores" in detail.
	{
		summary: "set cpu-cores empty",
		args:    []string{"cpu-cores="},
	}, {
		summary: "set cpu-cores zero",
		args:    []string{"cpu-cores=0"},
	}, {
		summary: "set cpu-cores",
		args:    []string{"cpu-cores=4"},
	}, {
		summary: "set nonsense cpu-cores 1",
		args:    []string{"cpu-cores=cheese"},
		err:     `bad "cpu-cores" characteristic: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-cores 2",
		args:    []string{"cpu-cores=-1"},
		err:     `bad "cpu-cores" characteristic: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-cores 3",
		args:    []string{"cpu-cores=123.45"},
		err:     `bad "cpu-cores" characteristic: must be a non-negative integer`,
	}, {
		summary: "double set cpu-cores together",
		args:    []string{"cpu-cores=128 cpu-cores=1"},
		err:     `bad "cpu-cores" characteristic: already set`,
	}, {
		summary: "double set cpu-cores separately",
		args:    []string{"cpu-cores=128", "cpu-cores=1"},
		err:     `bad "cpu-cores" characteristic: already set`,
	},

	// "cpu-power" in detail.
	{
		summary: "set cpu-power empty",
		args:    []string{"cpu-power="},
	}, {
		summary: "set cpu-power zero",
		args:    []string{"cpu-power=0"},
	}, {
		summary: "set cpu-power",
		args:    []string{"cpu-power=44"},
	}, {
		summary: "set nonsense cpu-power 1",
		args:    []string{"cpu-power=cheese"},
		err:     `bad "cpu-power" characteristic: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-power 2",
		args:    []string{"cpu-power=-1"},
		err:     `bad "cpu-power" characteristic: must be a non-negative integer`,
	}, {
		summary: "double set cpu-power together",
		args:    []string{"  cpu-power=300 cpu-power=1700 "},
		err:     `bad "cpu-power" characteristic: already set`,
	}, {
		summary: "double set cpu-power separately",
		args:    []string{"cpu-power=300  ", "  cpu-power=1700"},
		err:     `bad "cpu-power" characteristic: already set`,
	},

	// "mem" in detail.
	{
		summary: "set mem empty",
		args:    []string{"mem="},
	}, {
		summary: "set mem zero",
		args:    []string{"mem=0"},
	}, {
		summary: "set mem without suffix",
		args:    []string{"mem=512"},
	}, {
		summary: "set mem with M suffix",
		args:    []string{"mem=512M"},
	}, {
		summary: "set mem with G suffix",
		args:    []string{"mem=1.5G"},
	}, {
		summary: "set mem with T suffix",
		args:    []string{"mem=36.2T"},
	}, {
		summary: "set mem with P suffix",
		args:    []string{"mem=18.9P"},
	}, {
		summary: "set nonsense mem 1",
		args:    []string{"mem=cheese"},
		err:     `bad "mem" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 2",
		args:    []string{"mem=-1"},
		err:     `bad "mem" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 3",
		args:    []string{"mem=32Y"},
		err:     `bad "mem" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set mem together",
		args:    []string{"mem=1G  mem=2G"},
		err:     `bad "mem" characteristic: already set`,
	}, {
		summary: "double set mem separately",
		args:    []string{"mem=1G", "mem=2G"},
		err:     `bad "mem" characteristic: already set`,
	},

	// "root-disk" in detail.
	{
		summary: "set root-disk empty",
		args:    []string{"root-disk="},
	}, {
		summary: "set root-disk zero",
		args:    []string{"root-disk=0"},
	}, {
		summary: "set root-disk without suffix",
		args:    []string{"root-disk=512"},
	}, {
		summary: "set root-disk with M suffix",
		args:    []string{"root-disk=512M"},
	}, {
		summary: "set root-disk with G suffix",
		args:    []string{"root-disk=1.5G"},
	}, {
		summary: "set root-disk with T suffix",
		args:    []string{"root-disk=36.2T"},
	}, {
		summary: "set root-disk with P suffix",
		args:    []string{"root-disk=18.9P"},
	}, {
		summary: "set nonsense root-disk 1",
		args:    []string{"root-disk=cheese"},
		err:     `bad "root-disk" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense root-disk 2",
		args:    []string{"root-disk=-1"},
		err:     `bad "root-disk" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense root-disk 3",
		args:    []string{"root-disk=32Y"},
		err:     `bad "root-disk" characteristic: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set root-disk together",
		args:    []string{"root-disk=1G  root-disk=2G"},
		err:     `bad "root-disk" characteristic: already set`,
	}, {
		summary: "double set root-disk separately",
		args:    []string{"root-disk=1G", "root-disk=2G"},
		err:     `bad "root-disk" characteristic: already set`,
	},

	// Everything at once.
	{
		summary: "kitchen sink together",
		args:    []string{" root-disk=4G mem=2T  arch=i386  cpu-cores=4096 cpu-power=9001"},
	}, {
		summary: "kitchen sink separately",
		args:    []string{"root-disk=4G", "mem=2T", "cpu-cores=4096", "cpu-power=9001", "arch=armhf"},
	},
}

func (s *HardwareSuite) TestParseHardware(c *gc.C) {
	for i, t := range parseHardwareTests {
		c.Logf("test %d: %s", i, t.summary)
		hwc, err := instance.ParseHardware(t.args...)
		if t.err == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}
		cons1, err := instance.ParseHardware(hwc.String())
		c.Assert(err, gc.IsNil)
		c.Assert(cons1, gc.DeepEquals, hwc)
	}
}

type PortsSuite struct{}

var _ = gc.Suite(&PortsSuite{})

var sortPortsTests = []struct {
	have, want []instance.Port
}{
	{nil, []instance.Port{}},
	{[]instance.Port{{"b", 1}, {"a", 99}, {"a", 1}}, []instance.Port{{"a", 1}, {"a", 99}, {"b", 1}}},
}

func (*PortsSuite) TestSortPorts(c *gc.C) {
	for _, t := range sortPortsTests {
		p := make([]instance.Port, len(t.have))
		copy(p, t.have)
		instance.SortPorts(p)
		c.Check(p, gc.DeepEquals, t.want)
		instance.SortPorts(p)
		c.Check(p, gc.DeepEquals, t.want)
	}
}

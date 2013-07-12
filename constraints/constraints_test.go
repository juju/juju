// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	"encoding/json"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/instance"
	"testing"
)

func TestPackage(t *testing.T) {
	TestingT(t)
}

type ConstraintsSuite struct{}

var _ = Suite(&ConstraintsSuite{})

var parseConstraintsTests = []struct {
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
		err:     `malformed constraint "cheese"`,
	}, {
		summary: "missing name",
		args:    []string{"=cheese"},
		err:     `malformed constraint "=cheese"`,
	}, {
		summary: "unknown constraint",
		args:    []string{"cheese=edam"},
		err:     `unknown constraint "cheese"`,
	},

	// "container" in detail.
	{
		summary: "set container empty",
		args:    []string{"container="},
	}, {
		summary: "set container to none",
		args:    []string{"container=none"},
	}, {
		summary: "set container lxc",
		args:    []string{"container=lxc"},
	}, {
		summary: "set nonsense container",
		args:    []string{"container=foo"},
		err:     `bad "container" constraint: invalid container type "foo"`,
	}, {
		summary: "double set container together",
		args:    []string{"container=lxc container=lxc"},
		err:     `bad "container" constraint: already set`,
	}, {
		summary: "double set container separately",
		args:    []string{"container=lxc", "container="},
		err:     `bad "container" constraint: already set`,
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
		summary: "set arch arm",
		args:    []string{"arch=arm"},
	}, {
		summary: "set nonsense arch 1",
		args:    []string{"arch=cheese"},
		err:     `bad "arch" constraint: "cheese" not recognized`,
	}, {
		summary: "set nonsense arch 2",
		args:    []string{"arch=123.45"},
		err:     `bad "arch" constraint: "123.45" not recognized`,
	}, {
		summary: "double set arch together",
		args:    []string{"arch=amd64 arch=amd64"},
		err:     `bad "arch" constraint: already set`,
	}, {
		summary: "double set arch separately",
		args:    []string{"arch=arm", "arch="},
		err:     `bad "arch" constraint: already set`,
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
		err:     `bad "cpu-cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-cores 2",
		args:    []string{"cpu-cores=-1"},
		err:     `bad "cpu-cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-cores 3",
		args:    []string{"cpu-cores=123.45"},
		err:     `bad "cpu-cores" constraint: must be a non-negative integer`,
	}, {
		summary: "double set cpu-cores together",
		args:    []string{"cpu-cores=128 cpu-cores=1"},
		err:     `bad "cpu-cores" constraint: already set`,
	}, {
		summary: "double set cpu-cores separately",
		args:    []string{"cpu-cores=128", "cpu-cores=1"},
		err:     `bad "cpu-cores" constraint: already set`,
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
		err:     `bad "cpu-power" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cpu-power 2",
		args:    []string{"cpu-power=-1"},
		err:     `bad "cpu-power" constraint: must be a non-negative integer`,
	}, {
		summary: "double set cpu-power together",
		args:    []string{"  cpu-power=300 cpu-power=1700 "},
		err:     `bad "cpu-power" constraint: already set`,
	}, {
		summary: "double set cpu-power separately",
		args:    []string{"cpu-power=300  ", "  cpu-power=1700"},
		err:     `bad "cpu-power" constraint: already set`,
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
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 2",
		args:    []string{"mem=-1"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 3",
		args:    []string{"mem=32Y"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set mem together",
		args:    []string{"mem=1G  mem=2G"},
		err:     `bad "mem" constraint: already set`,
	}, {
		summary: "double set mem separately",
		args:    []string{"mem=1G", "mem=2G"},
		err:     `bad "mem" constraint: already set`,
	},

	// Everything at once.
	{
		summary: "kitchen sink together",
		args:    []string{" mem=2T  arch=i386  cpu-cores=4096 cpu-power=9001 container=lxc"},
	}, {
		summary: "kitchen sink separately",
		args:    []string{"mem=2T", "cpu-cores=4096", "cpu-power=9001", "arch=arm", "container=lxc"},
	},
}

func (s *ConstraintsSuite) TestParseConstraints(c *C) {
	for i, t := range parseConstraintsTests {
		c.Logf("test %d: %s", i, t.summary)
		cons0, err := constraints.Parse(t.args...)
		if t.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, t.err)
			continue
		}
		cons1, err := constraints.Parse(cons0.String())
		c.Assert(err, IsNil)
		c.Assert(cons1, DeepEquals, cons0)
	}
}

func uint64p(i uint64) *uint64 {
	return &i
}

func strp(s string) *string {
	return &s
}

func ctypep(ctype string) *instance.ContainerType {
	res := instance.ContainerType(ctype)
	return &res
}

var constraintsRoundtripTests = []constraints.Value{
	{},
	{Arch: strp("")},
	{Arch: strp("amd64")},
	{Container: ctypep("")},
	{Container: ctypep("lxc")},
	{CpuCores: uint64p(0)},
	{CpuCores: uint64p(128)},
	{CpuPower: uint64p(0)},
	{CpuPower: uint64p(250)},
	{Mem: uint64p(0)},
	{Mem: uint64p(98765)},
	{
		Arch:      strp("i386"),
		Container: ctypep("lxc"),
		CpuCores:  uint64p(4096),
		CpuPower:  uint64p(9001),
		Mem:       uint64p(18000000000),
	},
}

func (s *ConstraintsSuite) TestRoundtripGnuflagValue(c *C) {
	for i, t := range constraintsRoundtripTests {
		c.Logf("test %d", i)
		var cons constraints.Value
		val := constraints.ConstraintsValue{&cons}
		err := val.Set(t.String())
		c.Assert(err, IsNil)
		c.Assert(cons, DeepEquals, t)
	}
}

func (s *ConstraintsSuite) TestRoundtripString(c *C) {
	for i, t := range constraintsRoundtripTests {
		c.Logf("test %d", i)
		cons, err := constraints.Parse(t.String())
		c.Assert(err, IsNil)
		c.Assert(cons, DeepEquals, t)
	}
}

func (s *ConstraintsSuite) TestRoundtripJson(c *C) {
	for i, t := range constraintsRoundtripTests {
		c.Logf("test %d", i)
		data, err := json.Marshal(t)
		c.Assert(err, IsNil)
		var cons constraints.Value
		err = json.Unmarshal(data, &cons)
		c.Assert(err, IsNil)
		c.Assert(cons, DeepEquals, t)
	}
}

func (s *ConstraintsSuite) TestRoundtripYaml(c *C) {
	for i, t := range constraintsRoundtripTests {
		c.Logf("test %d", i)
		data, err := goyaml.Marshal(t)
		c.Assert(err, IsNil)
		c.Logf("%s", data)
		var cons constraints.Value
		err = goyaml.Unmarshal(data, &cons)
		c.Assert(err, IsNil)
		c.Assert(cons, DeepEquals, t)
	}
}

var withFallbacksTests = []struct {
	desc      string
	initial   string
	fallbacks string
	final     string
}{
	{
		desc: "empty all round",
	}, {
		desc:    "container with empty fallback",
		initial: "container=lxc",
		final:   "container=lxc",
	}, {
		desc:      "container from fallback",
		fallbacks: "container=lxc",
		final:     "container=lxc",
	}, {
		desc:    "arch with empty fallback",
		initial: "arch=amd64",
		final:   "arch=amd64",
	}, {
		desc:      "arch with ignored fallback",
		initial:   "arch=amd64",
		fallbacks: "arch=i386",
		final:     "arch=amd64",
	}, {
		desc:      "arch from fallback",
		fallbacks: "arch=i386",
		final:     "arch=i386",
	}, {
		desc:    "cpu-cores with empty fallback",
		initial: "cpu-cores=2",
		final:   "cpu-cores=2",
	}, {
		desc:      "cpu-cores with ignored fallback",
		initial:   "cpu-cores=4",
		fallbacks: "cpu-cores=8",
		final:     "cpu-cores=4",
	}, {
		desc:      "cpu-cores from fallback",
		fallbacks: "cpu-cores=8",
		final:     "cpu-cores=8",
	}, {
		desc:    "cpu-power with empty fallback",
		initial: "cpu-power=100",
		final:   "cpu-power=100",
	}, {
		desc:      "cpu-power with ignored fallback",
		initial:   "cpu-power=100",
		fallbacks: "cpu-power=200",
		final:     "cpu-power=100",
	}, {
		desc:      "cpu-power from fallback",
		fallbacks: "cpu-power=200",
		final:     "cpu-power=200",
	}, {
		desc:    "mem with empty fallback",
		initial: "mem=4G",
		final:   "mem=4G",
	}, {
		desc:      "mem with ignored fallback",
		initial:   "mem=4G",
		fallbacks: "mem=8G",
		final:     "mem=4G",
	}, {
		desc:      "mem from fallback",
		fallbacks: "mem=8G",
		final:     "mem=8G",
	}, {
		desc:      "non-overlapping mix",
		initial:   "mem=4G arch=amd64",
		fallbacks: "cpu-power=1000 cpu-cores=4",
		final:     "mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		desc:      "overlapping mix",
		initial:   "mem=4G arch=amd64",
		fallbacks: "cpu-power=1000 cpu-cores=4 mem=8G",
		final:     "mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	},
}

func (s *ConstraintsSuite) TestWithFallbacks(c *C) {
	for i, t := range withFallbacksTests {
		c.Logf("test %d", i)
		initial := constraints.MustParse(t.initial)
		fallbacks := constraints.MustParse(t.fallbacks)
		final := constraints.MustParse(t.final)
		c.Assert(initial.WithFallbacks(fallbacks), DeepEquals, final)
	}
}

var hasContainerTests = []struct {
	constraints  string
	hasContainer bool
}{
	{
		hasContainer: false,
	}, {
		constraints:  "container=lxc",
		hasContainer: true,
	}, {
		constraints:  "container=none",
		hasContainer: false,
	},
}

func (s *ConstraintsSuite) TestHasContainer(c *C) {
	for i, t := range hasContainerTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.constraints)
		c.Assert(cons.HasContainer(), Equals, t.hasContainer)
	}
}

package main

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
)

type ConstraintsValueSuite struct{}

var _ = Suite(&ConstraintsValueSuite{})

var constraintsValueTests = []struct {
	summary string
	args    []string
	err     string
}{
	// Simple errors.
	{
		summary: "nothing at all",
		args:    []string{""},
		err:     `malformed constraint ""`,
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

	// "cores" in detail.
	{
		summary: "set cores empty",
		args:    []string{"cores="},
	}, {
		summary: "set cores zero",
		args:    []string{"cores=0"},
	}, {
		summary: "set cores",
		args:    []string{"cores=4"},
	}, {
		summary: "set nonsense cores 1",
		args:    []string{"cores=cheese"},
		err:     `bad "cores" constraint: must be a non-negative integer`,
	}, {
		summary: "set nonsense cores 2",
		args:    []string{"cores=-1"},
		err:     `bad "cores" constraint: must be a non-negative integer`,
	}, {
		summary: "double set cores together",
		args:    []string{"cores=128 cores=1"},
		err:     `bad "cores" constraint: already set`,
	}, {
		summary: "double set cores separately",
		args:    []string{"cores=128", "cores=1"},
		err:     `bad "cores" constraint: already set`,
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
		args:    []string{"mem=512G"},
	}, {
		summary: "set mem with T suffix",
		args:    []string{"mem=512T"},
	}, {
		summary: "set mem with P suffix",
		args:    []string{"mem=512P"},
	}, {
		summary: "set nonsense mem 1",
		args:    []string{"mem=cheese"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "set nonsense mem 2",
		args:    []string{"mem=-1"},
		err:     `bad "mem" constraint: must be a non-negative float with optional M/G/T/P suffix`,
	}, {
		summary: "double set mem together",
		args:    []string{"mem=1G mem=2G"},
		err:     `bad "mem" constraint: already set`,
	}, {
		summary: "double set mem separately",
		args:    []string{"mem=1G", "mem=2G"},
		err:     `bad "mem" constraint: already set`,
	},

	// Everything at once.
	{
		summary: "kitchen sink",
		args:    []string{"mem=2T", "cores=4096"},
	},
}

func (s *ConstraintsValueSuite) TestConstraintsValue(c *C) {
	for i, t := range constraintsValueTests {
		c.Logf("test %d: %s", i, t.summary)
		v1 := constraintsValue{&state.Constraints{}}
		var err error
		for _, arg := range t.args {
			if err = v1.Set(arg); err != nil {
				break
			}
		}
		if t.err == "" {
			c.Assert(err, IsNil)
		} else {
			c.Assert(err, ErrorMatches, t.err)
			continue
		}
		// Depend on state.Constraints.String(), tested in
		// state, to validate actual values via round-trip.
		c.Assert(v1.String(), Equals, v1.c.String())
		v2 := constraintsValue{&state.Constraints{}}
		err = v2.Set(v1.String())
		c.Assert(err, IsNil)
		c.Assert(v1.c, DeepEquals, v2.c)
	}
}

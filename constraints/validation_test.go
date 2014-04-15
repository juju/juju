// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
)

type validationSuite struct{}

var _ = gc.Suite(&validationSuite{})

var validationTests = []struct {
	cons        string
	unsupported []string
	reds        []string
	blues       []string
	err         string
}{
	{
		cons: "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	},
	{
		cons:        "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
		unsupported: []string{"tags"},
	},
	{
		cons:        "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4 instance-type=foo",
		unsupported: []string{"cpu-power", "instance-type"},
		err:         "unsupported constraints: cpu-power,instance-type",
	},
	{
		cons: "root-disk=8G mem=4G arch=amd64 cpu-cores=4 instance-type=foo",
		reds: []string{"mem", "arch"},
		err:  "",
	},
	{
		cons:  "root-disk=8G mem=4G arch=amd64 cpu-cores=4 instance-type=foo",
		blues: []string{"mem", "arch"},
		err:   "",
	},
	{
		cons:  "root-disk=8G mem=4G arch=amd64 cpu-cores=4 instance-type=foo",
		reds:  []string{"mem", "arch"},
		blues: []string{"instance-type"},
		err:   `ambiguous constraints: "arch" overlaps with "instance-type"`,
	},
	{
		cons:  "root-disk=8G mem=4G arch=amd64 cpu-cores=4 instance-type=foo",
		reds:  []string{"instance-type"},
		blues: []string{"mem", "arch"},
		err:   `ambiguous constraints: "arch" overlaps with "instance-type"`,
	},
	{
		cons:  "root-disk=8G mem=4G cpu-cores=4 instance-type=foo",
		reds:  []string{"mem", "arch"},
		blues: []string{"instance-type"},
		err:   `ambiguous constraints: "mem" overlaps with "instance-type"`,
	},
}

func (s *validationSuite) TestValidation(c *gc.C) {
	for i, t := range validationTests {
		c.Logf("test %d", i)
		validator := constraints.NewValidator()
		validator.RegisterUnsupported(t.unsupported)
		validator.RegisterConflicts(t.reds, t.blues)
		cons := constraints.MustParse(t.cons)
		err := validator.Validate(cons)
		if t.err == "" {
			c.Assert(err, gc.IsNil)
		} else {
			c.Assert(err, gc.ErrorMatches, t.err)
		}
	}
}

var mergeTests = []struct {
	consA    string
	consB    string
	reds     []string
	blues    []string
	expected string
}{
	{
		consA:    "root-disk=8G cpu-cores=4 instance-type=foo",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	},
	{
		consB:    "root-disk=8G cpu-cores=4 instance-type=foo",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	},
	{
		consA:    "root-disk=8G instance-type=foo",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	},
	{
		consA:    "root-disk=8G mem=4G",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	},
	{
		consA:    "root-disk=8G arch=amd64",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	},
	{
		consA:    "root-disk=8G cpu-cores=4 instance-type=bar",
		consB:    "root-disk=8G mem=4G",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 mem=4G",
	},
	{
		consA:    "root-disk=8G cpu-cores=4 instance-type=bar",
		consB:    "root-disk=8G arch=amd64 mem=4G",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 arch=amd64 mem=4G",
	},
}

func (s *validationSuite) TestMerge(c *gc.C) {
	for i, t := range mergeTests {
		c.Logf("test %d", i)
		validator := constraints.NewValidator()
		validator.RegisterConflicts(t.reds, t.blues)
		consA := constraints.MustParse(t.consA)
		consB := constraints.MustParse(t.consB)
		merged, err := validator.Merge(consA, consB)
		c.Assert(err, gc.IsNil)
		expected := constraints.MustParse(t.expected)
		c.Check(merged, gc.DeepEquals, expected)
	}
	validator := constraints.NewValidator()
	for i, t := range withFallbacksTests {
		c.Logf("test %d", i+len(mergeTests))
		consA := constraints.MustParse(t.fallbacks)
		consB := constraints.MustParse(t.initial)
		merged, err := validator.Merge(consA, consB)
		c.Assert(err, gc.IsNil)
		expected := constraints.MustParse(t.final)
		c.Check(merged, gc.DeepEquals, expected)
	}
}

func (s *validationSuite) TestMergeError(c *gc.C) {
	validator := constraints.NewValidator()
	validator.RegisterUnsupported([]string{"instance-type"})
	consA := constraints.MustParse("instance-type=foo")
	consB := constraints.MustParse("cpu-cores=2")
	_, err := validator.Merge(consA, consB)
	c.Assert(err, gc.ErrorMatches, "unsupported constraints: instance-type")
	_, err = validator.Merge(consB, consA)
	c.Assert(err, gc.ErrorMatches, "unsupported constraints: instance-type")
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
		desc:    "instance type with empty fallback",
		initial: "instance-type=foo",
		final:   "instance-type=foo",
	}, {
		desc:      "instance type with ignored fallback",
		initial:   "instance-type=foo",
		fallbacks: "instance-type=bar",
		final:     "instance-type=foo",
	}, {
		desc:      "instance type from fallback",
		fallbacks: "instance-type=foo",
		final:     "instance-type=foo",
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
		desc:    "tags with empty fallback",
		initial: "tags=foo,bar",
		final:   "tags=foo,bar",
	}, {
		desc:      "tags with ignored fallback",
		initial:   "tags=foo,bar",
		fallbacks: "tags=baz",
		final:     "tags=foo,bar",
	}, {
		desc:      "tags from fallback",
		fallbacks: "tags=foo,bar",
		final:     "tags=foo,bar",
	}, {
		desc:      "tags inital empty",
		initial:   "tags=",
		fallbacks: "tags=foo,bar",
		final:     "tags=",
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
		desc:    "root-disk with empty fallback",
		initial: "root-disk=4G",
		final:   "root-disk=4G",
	}, {
		desc:      "root-disk with ignored fallback",
		initial:   "root-disk=4G",
		fallbacks: "root-disk=8G",
		final:     "root-disk=4G",
	}, {
		desc:      "root-disk from fallback",
		fallbacks: "root-disk=8G",
		final:     "root-disk=8G",
	}, {
		desc:      "non-overlapping mix",
		initial:   "root-disk=8G mem=4G arch=amd64",
		fallbacks: "cpu-power=1000 cpu-cores=4",
		final:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		desc:      "overlapping mix",
		initial:   "root-disk=8G mem=4G arch=amd64",
		fallbacks: "cpu-power=1000 cpu-cores=4 mem=8G",
		final:     "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	},
}

func (s *validationSuite) TestWithFallbacks(c *gc.C) {
	for i, t := range withFallbacksTests {
		c.Logf("test %d: %s", i, t.desc)
		initial := constraints.MustParse(t.initial)
		fallbacks := constraints.MustParse(t.fallbacks)
		final := constraints.MustParse(t.final)
		c.Check(constraints.WithFallbacks(initial, fallbacks), gc.DeepEquals, final)
	}
}

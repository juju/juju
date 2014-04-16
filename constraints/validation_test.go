// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints_test

import (
	"errors"

	jc "github.com/juju/testing/checkers"
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
		// Ambiguous constraint errors take precedence over unsupported errors.
		cons:        "root-disk=8G mem=4G cpu-cores=4 instance-type=foo",
		reds:        []string{"mem", "arch"},
		blues:       []string{"instance-type"},
		unsupported: []string{"cpu-cores"},
		err:         `ambiguous constraints: "mem" overlaps with "instance-type"`,
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
			if len(t.unsupported) > 0 && len(t.reds) == 0 {
				c.Assert(err, jc.Satisfies, constraints.IsNotSupportedError)
			} else {
				c.Assert(err, gc.Not(jc.Satisfies), constraints.IsNotSupportedError)
			}
		}
	}
}

func (s *validationSuite) TestUnsupportedError(c *gc.C) {
	unsupported := constraints.NewNotSupportedError([]string{"foo", "bar"})
	c.Assert(unsupported, jc.Satisfies, constraints.IsNotSupportedError)
	c.Assert(unsupported, gc.ErrorMatches, "unsupported constraints: foo,bar")
	c.Assert(errors.New("foo"), gc.Not(jc.Satisfies), constraints.IsNotSupportedError)
}

var mergeTests = []struct {
	desc        string
	consA       string
	consB       string
	unsupported []string
	reds        []string
	blues       []string
	expected    string
}{
	{
		desc: "empty all round",
	}, {
		desc:     "container with empty fallback",
		consB:    "container=lxc",
		expected: "container=lxc",
	}, {
		desc:     "container from fallback",
		consA:    "container=lxc",
		expected: "container=lxc",
	}, {
		desc:     "arch with empty fallback",
		consB:    "arch=amd64",
		expected: "arch=amd64",
	}, {
		desc:     "arch with ignored fallback",
		consB:    "arch=amd64",
		consA:    "arch=i386",
		expected: "arch=amd64",
	}, {
		desc:     "arch from fallback",
		consA:    "arch=i386",
		expected: "arch=i386",
	}, {
		desc:     "instance type with empty fallback",
		consB:    "instance-type=foo",
		expected: "instance-type=foo",
	}, {
		desc:     "instance type with ignored fallback",
		consB:    "instance-type=foo",
		consA:    "instance-type=bar",
		expected: "instance-type=foo",
	}, {
		desc:     "instance type from fallback",
		consA:    "instance-type=foo",
		expected: "instance-type=foo",
	}, {
		desc:     "cpu-cores with empty fallback",
		consB:    "cpu-cores=2",
		expected: "cpu-cores=2",
	}, {
		desc:     "cpu-cores with ignored fallback",
		consB:    "cpu-cores=4",
		consA:    "cpu-cores=8",
		expected: "cpu-cores=4",
	}, {
		desc:     "cpu-cores from fallback",
		consA:    "cpu-cores=8",
		expected: "cpu-cores=8",
	}, {
		desc:     "cpu-power with empty fallback",
		consB:    "cpu-power=100",
		expected: "cpu-power=100",
	}, {
		desc:     "cpu-power with ignored fallback",
		consB:    "cpu-power=100",
		consA:    "cpu-power=200",
		expected: "cpu-power=100",
	}, {
		desc:     "cpu-power from fallback",
		consA:    "cpu-power=200",
		expected: "cpu-power=200",
	}, {
		desc:     "tags with empty fallback",
		consB:    "tags=foo,bar",
		expected: "tags=foo,bar",
	}, {
		desc:     "tags with ignored fallback",
		consB:    "tags=foo,bar",
		consA:    "tags=baz",
		expected: "tags=foo,bar",
	}, {
		desc:     "tags from fallback",
		consA:    "tags=foo,bar",
		expected: "tags=foo,bar",
	}, {
		desc:     "tags inital empty",
		consB:    "tags=",
		consA:    "tags=foo,bar",
		expected: "tags=",
	}, {
		desc:     "mem with empty fallback",
		consB:    "mem=4G",
		expected: "mem=4G",
	}, {
		desc:     "mem with ignored fallback",
		consB:    "mem=4G",
		consA:    "mem=8G",
		expected: "mem=4G",
	}, {
		desc:     "mem from fallback",
		consA:    "mem=8G",
		expected: "mem=8G",
	}, {
		desc:     "root-disk with empty fallback",
		consB:    "root-disk=4G",
		expected: "root-disk=4G",
	}, {
		desc:     "root-disk with ignored fallback",
		consB:    "root-disk=4G",
		consA:    "root-disk=8G",
		expected: "root-disk=4G",
	}, {
		desc:     "root-disk from fallback",
		consA:    "root-disk=8G",
		expected: "root-disk=8G",
	}, {
		desc:     "non-overlapping mix",
		consB:    "root-disk=8G mem=4G arch=amd64",
		consA:    "cpu-power=1000 cpu-cores=4",
		expected: "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		desc:     "overlapping mix",
		consB:    "root-disk=8G mem=4G arch=amd64",
		consA:    "cpu-power=1000 cpu-cores=4 mem=8G",
		expected: "root-disk=8G mem=4G arch=amd64 cpu-power=1000 cpu-cores=4",
	}, {
		desc:     "fallback only, no conflicts",
		consA:    "root-disk=8G cpu-cores=4 instance-type=foo",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	}, {
		desc:     "no fallback, no conflicts",
		consB:    "root-disk=8G cpu-cores=4 instance-type=foo",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=foo",
	}, {
		desc:     "conflict value from override",
		consA:    "root-disk=8G instance-type=foo",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		desc:        "unsupported attributes ignored",
		consA:       "root-disk=8G instance-type=foo",
		consB:       "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:        []string{"mem", "arch"},
		blues:       []string{"instance-type"},
		unsupported: []string{"instance-type"},
		expected:    "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		desc:     "red conflict masked from fallback",
		consA:    "root-disk=8G mem=4G",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		desc:     "second red conflict masked from fallback",
		consA:    "root-disk=8G arch=amd64",
		consB:    "root-disk=8G cpu-cores=4 instance-type=bar",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 instance-type=bar",
	}, {
		desc:     "blue conflict masked from fallback",
		consA:    "root-disk=8G cpu-cores=4 instance-type=bar",
		consB:    "root-disk=8G mem=4G",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 mem=4G",
	}, {
		desc:     "both red conflicts used, blue mased from fallback",
		consA:    "root-disk=8G cpu-cores=4 instance-type=bar",
		consB:    "root-disk=8G arch=amd64 mem=4G",
		reds:     []string{"mem", "arch"},
		blues:    []string{"instance-type"},
		expected: "root-disk=8G cpu-cores=4 arch=amd64 mem=4G",
	},
}

func (s *validationSuite) TestMerge(c *gc.C) {
	for i, t := range mergeTests {
		c.Logf("test %d: %s", i, t.desc)
		validator := constraints.NewValidator()
		validator.RegisterConflicts(t.reds, t.blues)
		consA := constraints.MustParse(t.consA)
		consB := constraints.MustParse(t.consB)
		merged, err := validator.Merge(consA, consB)
		c.Assert(err, gc.IsNil)
		expected := constraints.MustParse(t.expected)
		c.Check(merged, gc.DeepEquals, expected)
	}
}

func (s *validationSuite) TestMergeError(c *gc.C) {
	validator := constraints.NewValidator()
	validator.RegisterConflicts([]string{"instance-type"}, []string{"mem"})
	consA := constraints.MustParse("instance-type=foo mem=4G")
	consB := constraints.MustParse("cpu-cores=2")
	_, err := validator.Merge(consA, consB)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "mem" overlaps with "instance-type"`)
	_, err = validator.Merge(consB, consA)
	c.Assert(err, gc.ErrorMatches, `ambiguous constraints: "mem" overlaps with "instance-type"`)
}

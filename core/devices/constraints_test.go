// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devices_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/testing"
)

type ConstraintsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ConstraintsSuite{})

func (*ConstraintsSuite) testParse(c *gc.C, s string, expect devices.Constraints) {
	cons, err := devices.ParseConstraints(s)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cons, gc.DeepEquals, expect)
}

func (*ConstraintsSuite) testParseError(c *gc.C, s, expectErr string) {
	_, err := devices.ParseConstraints(s)
	c.Assert(err, gc.ErrorMatches, expectErr)
}

func (s *ConstraintsSuite) TestParseConstraintsDeviceGood(c *gc.C) {
	s.testParse(c, "nvidia.com/gpu", devices.Constraints{
		Type:  "nvidia.com/gpu",
		Count: 1,
	})
	s.testParse(c, "2,nvidia.com/gpu", devices.Constraints{
		Type:  "nvidia.com/gpu",
		Count: 2,
	})
	s.testParse(c, "3,nvidia.com/gpu,gpu=nvidia-tesla-p100", devices.Constraints{
		Type:  "nvidia.com/gpu",
		Count: 3,
		Attributes: map[string]string{
			"gpu": "nvidia-tesla-p100",
		},
	})
	s.testParse(c, "3,nvidia.com/gpu,gpu=nvidia-tesla-p100;2ndattr=another-attr", devices.Constraints{
		Type:  "nvidia.com/gpu",
		Count: 3,
		Attributes: map[string]string{
			"gpu":     "nvidia-tesla-p100",
			"2ndattr": "another-attr",
		},
	})
}

func (s *ConstraintsSuite) TestParseConstraintsDeviceBad(c *gc.C) {
	s.testParseError(c, "2,nvidia.com/gpu,gpu=nvidia-tesla-p100,a=b", `cannot parse device constraints string, supported format is \[<count>,\]<device-class>|<vendor/type>\[,<key>=<value>;...\]`)
	s.testParseError(c, "2,nvidia.com/gpu,gpu=b=c", `device attribute key/value pair has bad format: \"gpu=b=c\"`)
	s.testParseError(c, "badCount,nvidia.com/gpu", `count must be greater than zero, got \"badCount\"`)
	s.testParseError(c, "0,nvidia.com/gpu", `count must be greater than zero, got \"0\"`)
	s.testParseError(c, "-1,nvidia.com/gpu", `count must be greater than zero, got \"-1\"`)
}

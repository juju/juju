// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devices_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/internal/testing"
)

type ConstraintsSuite struct {
	testing.BaseSuite
}

func TestConstraintsSuite(t *stdtesting.T) { tc.Run(t, &ConstraintsSuite{}) }
func (*ConstraintsSuite) testParse(c *tc.C, s string, expect devices.Constraints) {
	cons, err := devices.ParseConstraints(s)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cons, tc.DeepEquals, expect)
}

func (*ConstraintsSuite) testParseError(c *tc.C, s, expectErr string) {
	_, err := devices.ParseConstraints(s)
	c.Assert(err, tc.ErrorMatches, expectErr)
}

func (s *ConstraintsSuite) TestParseConstraintsDeviceGood(c *tc.C) {
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

func (s *ConstraintsSuite) TestParseConstraintsDeviceBad(c *tc.C) {
	s.testParseError(c, "2,nvidia.com/gpu,gpu=nvidia-tesla-p100,a=b", `cannot parse device constraints string, supported format is \[<count>,\]<device-class>|<vendor/type>\[,<key>=<value>;...\]`)
	s.testParseError(c, "2,nvidia.com/gpu,gpu=b=c", `device attribute key/value pair has bad format: \"gpu=b=c\"`)
	s.testParseError(c, "badCount,nvidia.com/gpu", `count must be greater than zero, got \"badCount\"`)
	s.testParseError(c, "0,nvidia.com/gpu", `count must be greater than zero, got \"0\"`)
	s.testParseError(c, "-1,nvidia.com/gpu", `count must be greater than zero, got \"-1\"`)
}

// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type InstanceTypesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&InstanceTypesSuite{})

func (s *InstanceTypesSuite) TestSupportsClassic(c *gc.C) {
	assertSupportsClassic := func(name string) {
		c.Assert(supportsClassic(name), jc.IsTrue)
	}
	assertDoesNotSupportClassic := func(name string) {
		c.Assert(supportsClassic(name), jc.IsFalse)
	}
	assertSupportsClassic("c1.medium")
	assertSupportsClassic("c3.large")
	assertSupportsClassic("cc2.8xlarge")
	assertSupportsClassic("cg1.4xlarge")
	assertSupportsClassic("cr1.8xlarge")
	assertSupportsClassic("d2.8xlarge")
	assertSupportsClassic("g2.2xlarge")
	assertSupportsClassic("hi1.4xlarge")
	assertSupportsClassic("hs1.8xlarge")
	assertSupportsClassic("i2.2xlarge")
	assertSupportsClassic("m1.medium")
	assertSupportsClassic("m2.medium")
	assertSupportsClassic("m3.medium")
	assertSupportsClassic("r3.8xlarge")
	assertSupportsClassic("t1.micro")
	assertDoesNotSupportClassic("c4.large")
	assertDoesNotSupportClassic("m4.large")
	assertDoesNotSupportClassic("p2.xlarge")
	assertDoesNotSupportClassic("t2.medium")
	assertDoesNotSupportClassic("x1.32xlarge")
}

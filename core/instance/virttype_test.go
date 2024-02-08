// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type VirtTypeSuite struct{}

var _ = gc.Suite(&VirtTypeSuite{})

func (s *VirtTypeSuite) TestParseVirtType(c *gc.C) {
	parseVirtTypeTests := []struct {
		arg   string
		value VirtType
		err   string
	}{{
		arg:   "",
		value: DefaultInstanceType,
	}, {
		arg:   "container",
		value: InstanceTypeContainer,
	}, {
		arg:   "virtual-machine",
		value: InstanceTypeVM,
	}, {
		arg: "foo",
		err: `LXD VirtType "foo" not valid`,
	}}
	for i, t := range parseVirtTypeTests {
		c.Logf("test %d: %s", i, t.arg)
		v, err := ParseVirtType(t.arg)
		if t.err == "" {
			c.Check(err, jc.ErrorIsNil)
			c.Check(v, gc.Equals, t.value)
		} else {
			c.Check(err, gc.ErrorMatches, t.err)
		}
	}
}

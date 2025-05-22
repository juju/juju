// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"testing"

	"github.com/juju/tc"
)

type VirtTypeSuite struct{}

func TestVirtTypeSuite(t *testing.T) {
	tc.Run(t, &VirtTypeSuite{})
}

func (s *VirtTypeSuite) TestParseVirtType(c *tc.C) {
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
			c.Check(err, tc.ErrorIsNil)
			c.Check(v, tc.Equals, t.value)
		} else {
			c.Check(err, tc.ErrorMatches, t.err)
		}
	}
}

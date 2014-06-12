// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/network"
	"github.com/juju/juju/testing"
)

type NetworkSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&NetworkSuite{})

func (s *NetworkSuite) TestIsVLANTag(c *gc.C) {
	tests := []struct {
		value int
		valid bool
	}{{
		value: -100,
		valid: false,
	}, {
		value: -0,
		valid: true,
	}, {
		value: 9999,
		valid: false,
	}, {
		value: 4095,
		valid: false,
	}, {
		value: 4094,
		valid: true,
	}, {
		value: 1,
		valid: true,
	}}

	for i, t := range tests {
		c.Logf("test %d: %d -> %v", i, t.value, t.valid)
		err := network.IsVLANTag(t.value)
		if t.valid {
			c.Check(err, gc.IsNil)
		} else {
			expectErr := "invalid VLAN tag .*: must be between 0 and 4094"
			c.Check(err, gc.ErrorMatches, expectErr)
		}
	}
}

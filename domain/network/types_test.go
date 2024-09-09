// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/internal/database"
)

type typesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestCIDRAddressRangeFromString(c *gc.C) {
	// This is just a sanity check.
	// Extensive parsing tests are in the core network package.

	result, err := network.CIDRAddressRangeFromString("2001:0DB8::/32")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Start, jc.DeepEquals, network.IPAddressBytes{
		MSB: database.Uint64{
			UnsignedValue: 0x20010db800000000,
			Valid:         true,
		},
		LSB: database.Uint64{
			UnsignedValue: 0,
			Valid:         true,
		},
	})
	c.Assert(result.End, jc.DeepEquals, network.IPAddressBytes{
		MSB: database.Uint64{
			UnsignedValue: 0x20010db8ffffffff,
			Valid:         true,
		},
		LSB: database.Uint64{
			UnsignedValue: 0xffffffffffffffff,
			Valid:         true,
		},
	})
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/storage"
	"github.com/juju/juju/testing"
)

type FlagsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&FlagsSuite{})

func (*FlagsSuite) TestDisksFlagErrors(c *gc.C) {
	var disks []storage.Constraints
	f := machine.NewDisksFlag(&disks)
	err := f.Set("-1")
	c.Assert(err, gc.ErrorMatches, `cannot parse disk constraints: cannot parse count: count must be greater than zero, got "-1"`)
	c.Assert(disks, gc.HasLen, 0)
}

func (*FlagsSuite) TestDisksFlag(c *gc.C) {
	var disks []storage.Constraints
	f := machine.NewDisksFlag(&disks)
	err := f.Set("crystal,1G")
	c.Assert(err, jc.ErrorIsNil)
	err = f.Set("2,2G")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(disks, gc.DeepEquals, []storage.Constraints{
		{Pool: "crystal", Size: 1024, Count: 1},
		{Size: 2048, Count: 2},
	})
}

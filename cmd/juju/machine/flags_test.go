// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/machine"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testing"
)

type FlagsSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&FlagsSuite{})

func (*FlagsSuite) TestDisksFlagErrors(c *tc.C) {
	var disks []storage.Directive
	f := machine.NewDisksFlag(&disks)
	err := f.Set("-1")
	c.Assert(err, tc.ErrorMatches, `cannot parse disk storage directives: cannot parse count: count must be greater than zero, got "-1"`)
	c.Assert(disks, tc.HasLen, 0)
}

func (*FlagsSuite) TestDisksFlag(c *tc.C) {
	var disks []storage.Directive
	f := machine.NewDisksFlag(&disks)
	err := f.Set("crystal,1G")
	c.Assert(err, tc.ErrorIsNil)
	err = f.Set("2,2G")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(disks, tc.DeepEquals, []storage.Directive{
		{Pool: "crystal", Size: 1024, Count: 1},
		{Size: 2048, Count: 2},
	})
}

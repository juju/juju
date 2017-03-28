// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package libvirt

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

// gocheck boilerplate.
type domainXMLInternalSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&domainXMLInternalSuite{})

func (domainXMLInternalSuite) TestDeviceID(c *gc.C) {
	table := []struct {
		in     int
		want   string
		errMsg string
	}{
		{0, "vda", ""},
		{4, "vde", ""},
		{15, "vdp", ""},
		{25, "vdz", ""},
		{-1, "", "got -1 but only support devices 0-25"},
		{26, "", "got 26 but only support devices 0-25"},
		{120, "", "got 120 but only support devices 0-25"},
	}
	for i, test := range table {
		c.Logf("test %d for input %d", i+1, test.in)
		got, err := deviceID(test.in)
		if err != nil {
			c.Check(err, gc.ErrorMatches, test.errMsg)
			c.Check(got, gc.Equals, "")
			continue
		}
		c.Check(got, gc.Equals, test.want)
		c.Check(err, jc.ErrorIsNil)
	}
}

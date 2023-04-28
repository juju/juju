// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type archSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&archSuite{})

func (s archSuite) TestArchOrDefault(c *gc.C) {
	a := ArchOrDefault(MustParse("mem=4G"), nil)
	c.Assert(a, gc.Equals, "amd64")
	a = ArchOrDefault(MustParse("arch=arm64"), nil)
	c.Assert(a, gc.Equals, "arm64")
	defaultCons := MustParse("arch=arm64")
	a = ArchOrDefault(MustParse("mem=4G"), &defaultCons)
	c.Assert(a, gc.Equals, "arm64")
	a = ArchOrDefault(MustParse("arch=s390x"), &defaultCons)
	c.Assert(a, gc.Equals, "s390x")
}

// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package constraints

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type archSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&archSuite{})

func (s *archSuite) TestArchOrDefault(c *tc.C) {
	a := ArchOrDefault(MustParse("mem=4G"), nil)
	c.Assert(a, tc.Equals, "amd64")
	a = ArchOrDefault(MustParse("arch=arm64"), nil)
	c.Assert(a, tc.Equals, "arm64")
	defaultCons := MustParse("arch=arm64")
	a = ArchOrDefault(MustParse("mem=4G"), &defaultCons)
	c.Assert(a, tc.Equals, "arm64")
	a = ArchOrDefault(MustParse("arch=s390x"), &defaultCons)
	c.Assert(a, tc.Equals, "s390x")
}

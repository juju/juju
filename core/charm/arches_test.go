// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/juju/core/charm"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type archSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&archSuite{})

func (s archSuite) TestContains(c *gc.C) {
	arches := charm.AllArches()
	c.Assert(arches.Contains(charm.ArchAMD64), jc.IsTrue)
	c.Assert(arches.Contains(charm.Arch("risc")), jc.IsFalse)
}

func (s archSuite) TestStringList(c *gc.C) {
	arches := charm.AllArches()
	c.Assert(arches.StringList(), jc.DeepEquals, []string{"amd64", "arm64", "ppc64", "s390"})
}

func (s archSuite) TestString(c *gc.C) {
	arches := charm.AllArches()
	c.Assert(arches.String(), gc.Equals, "amd64,arm64,ppc64,s390")
}

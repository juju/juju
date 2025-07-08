// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/constraints"
)

type archSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&archSuite{})

func (s archSuite) TestContains(c *gc.C) {
	arches := arch.AllArches()
	c.Assert(arches.Contains(arch.Arch("amd64")), jc.IsTrue)
	c.Assert(arches.Contains(arch.Arch("risc")), jc.IsFalse)
}

func (s archSuite) TestStringList(c *gc.C) {
	arches := arch.AllArches()
	c.Assert(arches.StringList(), jc.DeepEquals, []string{"amd64", "arm64", "armhf", "i386", "ppc64el", "riscv64", "s390x"})
}

func (s archSuite) TestString(c *gc.C) {
	arches := arch.AllArches()
	c.Assert(arches.String(), gc.Equals, "amd64,arm64,armhf,i386,ppc64el,riscv64,s390x")
}

func (s archSuite) TestConstraintArch(c *gc.C) {
	a := arch.ConstraintArch(constraints.MustParse("mem=4G"), nil)
	c.Assert(a, gc.Equals, "amd64")
	a = arch.ConstraintArch(constraints.MustParse("arch=arm64"), nil)
	c.Assert(a, gc.Equals, "arm64")
	defaultCons := constraints.MustParse("arch=arm64")
	a = arch.ConstraintArch(constraints.MustParse("mem=4G"), &defaultCons)
	c.Assert(a, gc.Equals, "arm64")
	a = arch.ConstraintArch(constraints.MustParse("arch=s390x"), &defaultCons)
	c.Assert(a, gc.Equals, "s390x")
}

// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/arch"
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
	c.Assert(arches.StringList(), jc.DeepEquals, []string{"amd64", "arm64", "ppc64el", "riscv64", "s390x"})
}

func (s archSuite) TestString(c *gc.C) {
	arches := arch.AllArches()
	c.Assert(arches.String(), gc.Equals, "amd64,arm64,ppc64el,riscv64,s390x")
}

func (s *archSuite) TestHostArch(c *gc.C) {
	a := arch.HostArch()
	c.Assert(arch.IsSupportedArch(a), jc.IsTrue)
}

func (s *archSuite) TestNormaliseArch(c *gc.C) {
	for _, test := range []struct {
		raw  string
		arch string
	}{
		{"amd64", "amd64"},
		{"x86_64", "amd64"},
		{"aarch64", "arm64"},
		{"arm64", "arm64"},
		{"ppc64el", "ppc64el"},
		{"ppc64le", "ppc64el"},
		{"s390x", "s390x"},
		{"riscv64", "riscv64"},
		{"risc", "riscv64"},
		{"risc-v64", "riscv64"},
		{"risc-V64", "riscv64"},
	} {
		arch := arch.NormaliseArch(test.raw)
		c.Check(arch, gc.Equals, test.arch)
	}
}

func (s *archSuite) TestIsSupportedArch(c *gc.C) {
	for _, a := range arch.AllSupportedArches {
		c.Assert(arch.IsSupportedArch(a), jc.IsTrue)
	}
	c.Assert(arch.IsSupportedArch("invalid"), jc.IsFalse)
}

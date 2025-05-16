// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package arch_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/internal/testhelpers"
)

type archSuite struct {
	testhelpers.IsolationSuite
}

func TestArchSuite(t *stdtesting.T) { tc.Run(t, &archSuite{}) }
func (s *archSuite) TestContains(c *tc.C) {
	arches := arch.AllArches()
	c.Assert(arches.Contains(arch.Arch("amd64")), tc.IsTrue)
	c.Assert(arches.Contains(arch.Arch("risc")), tc.IsFalse)
}

func (s *archSuite) TestStringList(c *tc.C) {
	arches := arch.AllArches()
	c.Assert(arches.StringList(), tc.DeepEquals, []string{"amd64", "arm64", "ppc64el", "riscv64", "s390x"})
}

func (s *archSuite) TestString(c *tc.C) {
	arches := arch.AllArches()
	c.Assert(arches.String(), tc.Equals, "amd64,arm64,ppc64el,riscv64,s390x")
}

func (s *archSuite) TestHostArch(c *tc.C) {
	a := arch.HostArch()
	c.Assert(arch.IsSupportedArch(a), tc.IsTrue)
}

func (s *archSuite) TestNormaliseArch(c *tc.C) {
	for _, test := range []struct {
		raw  string
		arch string
	}{
		{raw: "amd64", arch: "amd64"},
		{raw: "x86_64", arch: "amd64"},
		{raw: "arm64", arch: "arm64"},
		{raw: "aarch64", arch: "arm64"},
		{raw: "ppc64el", arch: "ppc64el"},
		{raw: "ppc64le", arch: "ppc64el"},
		{raw: "s390x", arch: "s390x"},
		{raw: "riscv64", arch: "riscv64"},
		{raw: "risc", arch: "riscv64"},
		{raw: "risc-v64", arch: "riscv64"},
		{raw: "risc-V64", arch: "riscv64"},
	} {
		arch := arch.NormaliseArch(test.raw)
		c.Check(arch, tc.Equals, test.arch)
	}
}

func (s *archSuite) TestIsSupportedArch(c *tc.C) {
	for _, a := range arch.AllSupportedArches {
		c.Assert(arch.IsSupportedArch(a), tc.IsTrue)
	}
	c.Assert(arch.IsSupportedArch("invalid"), tc.IsFalse)
}

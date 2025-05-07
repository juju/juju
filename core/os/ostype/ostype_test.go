// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ostype

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

type osTypeSuite struct{}

var _ = tc.Suite(&osTypeSuite{})

func (s *osTypeSuite) TestEquivalentTo(c *tc.C) {
	c.Check(Ubuntu.EquivalentTo(CentOS), jc.IsTrue)
	c.Check(Ubuntu.EquivalentTo(GenericLinux), jc.IsTrue)
	c.Check(GenericLinux.EquivalentTo(Ubuntu), jc.IsTrue)
	c.Check(CentOS.EquivalentTo(CentOS), jc.IsTrue)
}

func (s *osTypeSuite) TestIsLinux(c *tc.C) {
	c.Check(Ubuntu.IsLinux(), jc.IsTrue)
	c.Check(CentOS.IsLinux(), jc.IsTrue)
	c.Check(GenericLinux.IsLinux(), jc.IsTrue)

	c.Check(Windows.IsLinux(), jc.IsFalse)
	c.Check(Unknown.IsLinux(), jc.IsFalse)

	c.Check(OSX.EquivalentTo(Ubuntu), jc.IsFalse)
	c.Check(OSX.EquivalentTo(Windows), jc.IsFalse)
	c.Check(GenericLinux.EquivalentTo(OSX), jc.IsFalse)
}

func (s *osTypeSuite) TestString(c *tc.C) {
	c.Check(Ubuntu.String(), tc.Equals, "Ubuntu")
	c.Check(Windows.String(), tc.Equals, "Windows")
	c.Check(Unknown.String(), tc.Equals, "Unknown")
}

func (s *osTypeSuite) TestParseOSType(c *tc.C) {
	tests := []struct {
		str string
		t   OSType
	}{
		{str: "uBuntu", t: Ubuntu},
		{str: "winDOwS", t: Windows},
		{str: "OSX", t: OSX},
		{str: "CentOS", t: CentOS},
		{str: "GenericLinux", t: GenericLinux},
		{str: "Kubernetes", t: Kubernetes},
	}
	for i, test := range tests {
		c.Logf("test %d", i)
		t, err := ParseOSType(test.str)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(t, tc.Equals, test.t)
	}
}

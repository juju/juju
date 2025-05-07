// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ostype

import "github.com/juju/tc"

type osTypeSuite struct{}

var _ = tc.Suite(&osTypeSuite{})

func (s *osTypeSuite) TestEquivalentTo(c *tc.C) {
	c.Check(Ubuntu.EquivalentTo(CentOS), tc.IsTrue)
	c.Check(Ubuntu.EquivalentTo(GenericLinux), tc.IsTrue)
	c.Check(GenericLinux.EquivalentTo(Ubuntu), tc.IsTrue)
	c.Check(CentOS.EquivalentTo(CentOS), tc.IsTrue)
}

func (s *osTypeSuite) TestIsLinux(c *tc.C) {
	c.Check(Ubuntu.IsLinux(), tc.IsTrue)
	c.Check(CentOS.IsLinux(), tc.IsTrue)
	c.Check(GenericLinux.IsLinux(), tc.IsTrue)

	c.Check(Windows.IsLinux(), tc.IsFalse)
	c.Check(Unknown.IsLinux(), tc.IsFalse)

	c.Check(OSX.EquivalentTo(Ubuntu), tc.IsFalse)
	c.Check(OSX.EquivalentTo(Windows), tc.IsFalse)
	c.Check(GenericLinux.EquivalentTo(OSX), tc.IsFalse)
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
		c.Assert(err, tc.ErrorIsNil)
		c.Check(t, tc.Equals, test.t)
	}
}

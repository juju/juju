// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ipfamily_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network/ipfamily"
)

type IPFamilySuite struct{}

func TestIPFamilySuite(t *testing.T) {
	tc.Run(t, &IPFamilySuite{})
}

func (s *IPFamilySuite) TestParseIPFamilyValid(c *tc.C) {
	tests := []struct {
		input    string
		expected ipfamily.IPFamily
	}{
		{"ipv4", ipfamily.IPv4},
		{"ipv6", ipfamily.IPv6},
		{"dual", ipfamily.Dual},
	}
	for _, t := range tests {
		got, err := ipfamily.ParseIPFamily(t.input)
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got, tc.Equals, t.expected)
	}
}

func (s *IPFamilySuite) TestParseIPFamilyEmpty(c *tc.C) {
	_, err := ipfamily.ParseIPFamily("")
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err.Error(), tc.Contains, "must be one of")
}

func (s *IPFamilySuite) TestParseIPFamilyUnknown(c *tc.C) {
	_, err := ipfamily.ParseIPFamily("v4")
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err.Error(), tc.Contains, "not recognized")
}

func (s *IPFamilySuite) TestValidateValid(c *tc.C) {
	for _, f := range []ipfamily.IPFamily{
		ipfamily.IPv4,
		ipfamily.IPv6,
		ipfamily.Dual,
	} {
		c.Check(f.Validate(), tc.ErrorIsNil)
	}
}

func (s *IPFamilySuite) TestValidateEmpty(c *tc.C) {
	err := ipfamily.IPFamily("").Validate()
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err.Error(), tc.Contains, "must be one of")
}

func (s *IPFamilySuite) TestValidateUnknown(c *tc.C) {
	err := ipfamily.IPFamily("dual-stack").Validate()
	c.Assert(err, tc.Not(tc.ErrorIsNil))
	c.Check(err.Error(), tc.Contains, "not recognized")
}

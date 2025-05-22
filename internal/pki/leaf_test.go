// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"net"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/testing"
)

type LeafSuite struct {
}

func TestLeafSuite(t *stdtesting.T) {
	tc.Run(t, &LeafSuite{})
}

func (l *LeafSuite) TestLeafHasDNSNames(c *tc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)

	tests := []struct {
		CertDNSNames  []string
		CheckDNSNames []string
		Result        bool
	}{
		{
			CertDNSNames:  []string{"juju-apiserver", "mongodb"},
			CheckDNSNames: []string{"juju-apiserver"},
			Result:        true,
		},
		{
			CertDNSNames:  []string{"juju-apiserver", "mongodb"},
			CheckDNSNames: []string{"juju-apiserver", "mongodb"},
			Result:        true,
		},
		{
			CertDNSNames:  []string{"juju-apiserver", "mongodb"},
			CheckDNSNames: []string{"juju-apiserver1", "mongodb"},
			Result:        false,
		},
	}

	for _, test := range tests {
		leaf, err := authority.LeafRequestForGroup(pki.DefaultLeafGroup).
			AddDNSNames(test.CertDNSNames...).
			Commit()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(pki.LeafHasDNSNames(leaf, test.CheckDNSNames), tc.Equals,
			test.Result)
	}
}

func (l *LeafSuite) TestLeafIPAddresses(c *tc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)

	tests := []struct {
		CertIPAddresses  []net.IP
		CheckIPAddresses []net.IP
		Result           bool
	}{
		{
			CertIPAddresses:  []net.IP{net.ParseIP("fe80::abcd:1")},
			CheckIPAddresses: []net.IP{net.ParseIP("fe80::abcd:1")},
			Result:           true,
		},
	}

	for _, test := range tests {
		leaf, err := authority.LeafRequestForGroup(pki.DefaultLeafGroup).
			AddIPAddresses(test.CertIPAddresses...).
			Commit()
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(leaf.Certificate().IPAddresses, testing.IPsEqual, test.CheckIPAddresses)
	}
}

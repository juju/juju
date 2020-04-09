// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"net"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/testing"
)

type LeafSuite struct {
}

var _ = gc.Suite(&LeafSuite{})

func (l *LeafSuite) TestLeafHasDNSNames(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(pki.LeafHasDNSNames(leaf, test.CheckDNSNames), gc.Equals,
			test.Result)
	}
}

func (l *LeafSuite) TestLeafIPAddresses(c *gc.C) {
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

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
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(leaf.Certificate().IPAddresses, testing.IPsEqual, test.CheckIPAddresses)
	}
}

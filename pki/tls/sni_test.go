// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls_test

import (
	"crypto/tls"
	"net"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	pkitls "github.com/juju/juju/pki/tls"
)

type SNISuite struct {
	authority pki.Authority
	sniGetter func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

var _ = gc.Suite(&SNISuite{})

func (s *SNISuite) SetUpSuite(c *gc.C) {
	pki.DefaultKeyProfile = pkitest.OriginalDefaultKeyProfile
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	s.authority = authority
	s.sniGetter = pkitls.AuthoritySNITLSGetter(authority, loggo.Logger{})
}

func TLSCertificatesEqual(c *gc.C, cert1, cert2 *tls.Certificate) {
	c.Assert(len(cert1.Certificate), gc.Equals, len(cert2.Certificate))
	for i := range cert1.Certificate {
		c.Assert(cert1.Certificate[i], jc.DeepEquals, cert2.Certificate[i])
	}
}

func (s *SNISuite) TestAuthorityTLSGetter(c *gc.C) {
	tests := []struct {
		DNSNames      []string
		ExpectedGroup string
		Group         string
		IPAddresses   []net.IP
		ServerName    string
	}{
		{
			// Testing 1 to 1 mapping
			DNSNames:      []string{"juju.is"},
			ExpectedGroup: "test1",
			Group:         "test1",
			ServerName:    "juju.is",
		},
		{
			// Testing v4 address mapping
			ExpectedGroup: "test2",
			Group:         "test2",
			IPAddresses:   []net.IP{net.ParseIP("10.0.0.1")},
			ServerName:    "10.0.0.1",
		},
		{
			// Testing wild card certificates
			DNSNames:      []string{"*.juju.is"},
			ExpectedGroup: "test3",
			Group:         "test3",
			ServerName:    "tlm.juju.is",
		},
		{
			// Testing v6 address mapping
			ExpectedGroup: "test4",
			Group:         "test4",
			IPAddresses:   []net.IP{net.ParseIP("fe80:abcd::1")},
			ServerName:    "fe80:abcd::1",
		},
		{
			// Test Prevision wild card cert
			ExpectedGroup: "test3",
			Group:         "test5",
			IPAddresses:   []net.IP{net.ParseIP("fe80:abcd::2")},
			ServerName:    "wallyworld.juju.is",
		},
		{
			// Testing default certificate
			DNSNames:      []string{"juju-apiserver"},
			ExpectedGroup: pki.DefaultLeafGroup,
			Group:         pki.DefaultLeafGroup,
			ServerName:    "doesnotexist.juju.example",
		},
		{
			// Regression test for LP1921557
			ExpectedGroup: pki.ControllerIPLeafGroup,
			Group:         pki.ControllerIPLeafGroup,
			ServerName:    "",
		},
	}

	for _, test := range tests {
		_, err := s.authority.LeafRequestForGroup(test.Group).
			AddDNSNames(test.DNSNames...).
			AddIPAddresses(test.IPAddresses...).
			Commit()
		c.Assert(err, jc.ErrorIsNil)

		helloRequest := &tls.ClientHelloInfo{
			ServerName:        test.ServerName,
			SignatureSchemes:  []tls.SignatureScheme{tls.PSSWithSHA256},
			SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
		}

		cert, err := s.sniGetter(helloRequest)
		c.Assert(err, jc.ErrorIsNil)

		leaf, err := s.authority.LeafForGroup(test.ExpectedGroup)
		c.Assert(err, jc.ErrorIsNil)

		TLSCertificatesEqual(c, cert, leaf.TLSCertificate())
	}
}

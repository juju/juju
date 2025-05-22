// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls_test

import (
	"crypto/tls"
	"net"
	stdtesting "testing"

	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	pkitls "github.com/juju/juju/internal/pki/tls"
)

type SNISuite struct {
	authority pki.Authority
	sniGetter func(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

func TestSNISuite(t *stdtesting.T) {
	tc.Run(t, &SNISuite{})
}

func (s *SNISuite) SetUpTest(c *tc.C) {
	pki.DefaultKeyProfile = pkitest.OriginalDefaultKeyProfile
	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)

	s.authority = authority
	s.sniGetter = pkitls.AuthoritySNITLSGetter(authority, loggertesting.WrapCheckLog(c))
}

func TLSCertificatesEqual(c *tc.C, cert1, cert2 *tls.Certificate) {
	c.Assert(len(cert1.Certificate), tc.Equals, len(cert2.Certificate))
	for i := range cert1.Certificate {
		c.Assert(cert1.Certificate[i], tc.DeepEquals, cert2.Certificate[i])
	}
}

func (s *SNISuite) TestAuthorityTLSGetter(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)

		helloRequest := &tls.ClientHelloInfo{
			ServerName:        test.ServerName,
			SignatureSchemes:  []tls.SignatureScheme{tls.PSSWithSHA256},
			SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
		}

		cert, err := s.sniGetter(helloRequest)
		c.Assert(err, tc.ErrorIsNil)

		leaf, err := s.authority.LeafForGroup(test.ExpectedGroup)
		c.Assert(err, tc.ErrorIsNil)

		TLSCertificatesEqual(c, cert, leaf.TLSCertificate())
	}
}

func (s *SNISuite) TestNonExistantIPLeafReturnsDefault(c *tc.C) {
	leaf, err := s.authority.LeafRequestForGroup(pki.DefaultLeafGroup).
		AddDNSNames("juju-app").
		Commit()
	c.Assert(err, tc.ErrorIsNil)

	helloRequest := &tls.ClientHelloInfo{
		ServerName:        "",
		SignatureSchemes:  []tls.SignatureScheme{tls.PSSWithSHA256},
		SupportedVersions: []uint16{tls.VersionTLS13, tls.VersionTLS12},
	}

	cert, err := s.sniGetter(helloRequest)
	c.Assert(err, tc.ErrorIsNil)

	TLSCertificatesEqual(c, cert, leaf.TLSCertificate())
}

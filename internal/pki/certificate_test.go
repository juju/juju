// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/url"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/pki"
)

type CertificateSuite struct {
}

var _ = tc.Suite(&CertificateSuite{})

func (cs *CertificateSuite) VerifyCSRToCertificate(c *tc.C) {
	jujuURL, err := url.Parse("https://discourse.juju.is")
	c.Assert(err, jc.ErrorIsNil)

	subject := pkix.Name{
		CommonName:         "juju test",
		Country:            []string{"Australia"},
		Organization:       []string{"Canonical"},
		OrganizationalUnit: []string{"Juju"},
	}
	csr := x509.CertificateRequest{
		Subject:        subject,
		DNSNames:       []string{"juju.is"},
		EmailAddresses: []string{"juju@juju.is"},
		IPAddresses:    []net.IP{net.ParseIP("fe80:abcd:12")},
		URIs:           []*url.URL{jujuURL},
	}

	expectedCert := x509.Certificate{
		Subject:        csr.Subject,
		DNSNames:       csr.DNSNames,
		EmailAddresses: csr.EmailAddresses,
		IPAddresses:    csr.IPAddresses,
		URIs:           csr.URIs,
	}

	rCert := pki.CSRToCertificate(&csr)
	c.Assert(rCert, jc.DeepEquals, &expectedCert)
}

func (cs *CertificateSuite) CheckPkixNameFromDefaults(c *tc.C) {
	tests := []struct {
		Template pkix.Name
		Request  pkix.Name
		RVal     pkix.Name
	}{
		{
			Template: pkix.Name{
				Country:      []string{"Australia"},
				Organization: []string{"Canonical"},
			},
			Request: pkix.Name{
				Country:    []string{"New Zealand"},
				PostalCode: []string{"4000"},
				CommonName: "Juju Testing",
			},
			RVal: pkix.Name{
				Country:      []string{"New Zealand"},
				Organization: []string{"Canonical"},
				PostalCode:   []string{"4000"},
				CommonName:   "Juju Testing",
			},
		},
	}

	for _, test := range tests {
		rval := pki.MakeX509NameFromDefaults(&test.Template, &test.Request)
		c.Assert(rval, jc.DeepEquals, test.RVal)
	}
}

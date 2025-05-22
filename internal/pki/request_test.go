// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/pki"
)

type RequestSigner struct {
	ca     *x509.Certificate
	signer crypto.Signer
}

func TestRequestSigner(t *testing.T) {
	tc.Run(t, &RequestSigner{})
}

func (r *RequestSigner) SetUpTest(c *tc.C) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	r.signer = signer

	commonName := "juju-test-ca"
	ca, err := pki.NewCA(commonName, r.signer)
	c.Assert(err, tc.ErrorIsNil)
	r.ca = ca
}

func (r *RequestSigner) TestDefaultRequestSigning(c *tc.C) {
	requestSigner := pki.NewDefaultRequestSigner(r.ca, []*x509.Certificate{}, r.signer)

	leafSigner, err := pki.DefaultKeyProfile()
	c.Assert(err, tc.ErrorIsNil)

	dnsNames := []string{"test.juju.is"}
	leafCSR := x509.CertificateRequest{
		DNSNames:  dnsNames,
		PublicKey: leafSigner.Public(),
		Subject: pkix.Name{
			CommonName: "test",
		},
	}

	leafCert, _, err := requestSigner.SignCSR(&leafCSR)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(leafCert.DNSNames, tc.DeepEquals, dnsNames)
	c.Assert(leafCert.Subject.CommonName, tc.Equals, "test")
}

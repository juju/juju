// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"crypto"
	"crypto/x509"
	"crypto/x509/pkix"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pki"
)

type RequestSigner struct {
	ca     *x509.Certificate
	signer crypto.Signer
}

var _ = gc.Suite(&RequestSigner{})

func (r *RequestSigner) SetUpTest(c *gc.C) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	r.signer = signer

	commonName := "juju-test-ca"
	ca, err := pki.NewCA(commonName, r.signer)
	c.Assert(err, jc.ErrorIsNil)
	r.ca = ca
}

func (r *RequestSigner) TestDefaultRequestSigning(c *gc.C) {
	requestSigner := pki.NewDefaultRequestSigner(r.ca, []*x509.Certificate{}, r.signer)

	leafSigner, err := pki.DefaultKeyProfile()
	c.Assert(err, jc.ErrorIsNil)

	dnsNames := []string{"test.juju.is"}
	leafCSR := x509.CertificateRequest{
		DNSNames:  dnsNames,
		PublicKey: leafSigner.Public(),
		Subject: pkix.Name{
			CommonName: "test",
		},
	}

	leafCert, _, err := requestSigner.SignCSR(&leafCSR)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(leafCert.DNSNames, gc.DeepEquals, dnsNames)
	c.Assert(leafCert.Subject.CommonName, gc.Equals, "test")
}

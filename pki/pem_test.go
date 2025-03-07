// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/pki"
)

type PEMSuite struct {
	signer crypto.Signer
}

var _ = gc.Suite(&PEMSuite{})

func (p *PEMSuite) SetUpTest(c *gc.C) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, jc.ErrorIsNil)
	p.signer = signer
}

func (p *PEMSuite) TestCertificateToPemString(c *gc.C) {
	ca, err := pki.NewCA("juju-test-ca", p.signer)
	c.Assert(err, jc.ErrorIsNil)

	certString, err := pki.CertificateToPemString(map[string]string{
		"test": "test-header",
	}, ca)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(certString), jc.GreaterThan, 0)
	//TODO re-enable headers on certificate to pem when Juju upgrade
	//CAAS mongo to something compiled with latest openssl. Currently
	//not all our Openssl versions support pem headers.
	//c.Assert(strings.Contains(certString, "test: test-header"), jc.IsTrue)
	c.Assert(strings.Contains(certString, "BEGIN CERTIFICATE"), jc.IsTrue)
	c.Assert(strings.Contains(certString, "END CERTIFICATE"), jc.IsTrue)
}

func (p *PEMSuite) TestCertificateToPemWriter(c *gc.C) {
	ca, err := pki.NewCA("juju-test-ca", p.signer)
	c.Assert(err, jc.ErrorIsNil)

	builder := strings.Builder{}
	err = pki.CertificateToPemWriter(&builder, map[string]string{
		"test": "test-header",
	}, ca)
	c.Assert(err, jc.ErrorIsNil)
	certString := builder.String()
	c.Assert(len(certString), jc.GreaterThan, 0)
	//TODO re-enable headers on certificate to pem when Juju upgrade
	//CAAS mongo to something compiled with latest openssl. Currently
	//not all our Openssl versions support pem headers.
	//c.Assert(strings.Contains(certString, "test: test-header"), jc.IsTrue)
	c.Assert(strings.Contains(certString, "BEGIN CERTIFICATE"), jc.IsTrue)
	c.Assert(strings.Contains(certString, "END CERTIFICATE"), jc.IsTrue)
}

func (p *PEMSuite) TestSignerToPemString(c *gc.C) {
	pemKey, err := pki.SignerToPemString(p.signer)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(pemKey), jc.GreaterThan, 0)
	c.Assert(strings.Contains(pemKey, "BEGIN PRIVATE KEY"), jc.IsTrue)
	c.Assert(strings.Contains(pemKey, "END PRIVATE KEY"), jc.IsTrue)
}

func (p *PEMSuite) TestSignerToPemWriter(c *gc.C) {
	builder := strings.Builder{}
	err := pki.SignerToPemWriter(&builder, p.signer)
	pemKey := builder.String()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(pemKey), jc.GreaterThan, 0)
	c.Assert(strings.Contains(pemKey, "BEGIN PRIVATE KEY"), jc.IsTrue)
	c.Assert(strings.Contains(pemKey, "END PRIVATE KEY"), jc.IsTrue)
}

func (p *PEMSuite) TestSignerFromPKCS8Pem(c *gc.C) {
	tests := []struct {
		Name     string
		SignerFn func() (crypto.Signer, error)
	}{
		{
			Name:     "ECDSAP224",
			SignerFn: pki.ECDSAP224,
		},
		{
			Name:     "ECDSAP256",
			SignerFn: pki.ECDSAP256,
		},
		{
			Name:     "ECDSAP384",
			SignerFn: pki.ECDSAP384,
		},
		{
			Name:     "RSA2048",
			SignerFn: pki.RSA2048,
		},
		{
			Name:     "RSA3072",
			SignerFn: pki.RSA3072,
		},
	}

	for _, test := range tests {
		signer, err := test.SignerFn()
		c.Assert(err, jc.ErrorIsNil)
		der, err := x509.MarshalPKCS8PrivateKey(signer)
		c.Assert(err, jc.ErrorIsNil)
		block := &pem.Block{
			Type:  pki.PEMTypePKCS8,
			Bytes: der,
		}

		signerPem, err := pki.UnmarshalSignerFromPemBlock(block)
		c.Assert(err, jc.ErrorIsNil)
		mc := jc.NewMultiChecker()
		// Ignore the computed speedups for RSA since comparing byte for byte
		// of bignumbers may fail due to superfluous zeros (despite the numbers matching)
		mc.AddExpr("_.Precomputed", jc.Ignore)
		c.Assert(signerPem, mc, signer)
	}
}

func (p *PEMSuite) TestSignerFromPKCS1Pem(c *gc.C) {
	tests := []struct {
		Name     string
		SignerFn func() (crypto.Signer, error)
	}{
		{
			Name:     "RSA2048",
			SignerFn: pki.RSA2048,
		},
		{
			Name:     "RSA3072",
			SignerFn: pki.RSA3072,
		},
	}

	for _, test := range tests {
		signer, err := test.SignerFn()
		c.Assert(err, jc.ErrorIsNil)
		rsaKey, ok := signer.(*rsa.PrivateKey)
		c.Assert(ok, gc.Equals, true)

		der := x509.MarshalPKCS1PrivateKey(rsaKey)
		block := &pem.Block{
			Type:  pki.PEMTypePKCS1,
			Bytes: der,
		}

		signerPem, err := pki.UnmarshalSignerFromPemBlock(block)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(signerPem, jc.DeepEquals, signer)
	}
}

func (p *PEMSuite) TestFingerprint(c *gc.C) {
	pemCertStr := `-----BEGIN CERTIFICATE-----
MIICBjCCAa2gAwIBAgIVAPIfbVbfSiFAz9eOg1YNuQDakGxuMAoGCCqGSM49BAMC
MCExDTALBgNVBAoTBEp1anUxEDAOBgNVBAMTB2p1anUtY2EwHhcNMjAwNDAxMDQz
NTEwWhcNMzAwNDAxMDQ0MDEwWjBtMQ0wCwYDVQQKEwRKdWp1MS0wKwYDVQQDEyRK
dWp1IHNlcnZlciBjZXJ0aWZpY2F0ZSAtIGNvbnRyb2xsZXIxLTArBgNVBAUTJDUz
NmE4MjBhLWM1Y2QtNGY4Mi04ODkzLTk4OTU1MTNhZmExMjBZMBMGByqGSM49AgEG
CCqGSM49AwEHA0IABBxmIHbv7oMJidNfe4EyKgnVx/NrtjdJ94jro6nCSrLukXuj
tipXQyRUVdjzoVnbJ16YS1m0+WpTbWx8uPDPp2GjdjB0MHIGA1UdEQRrMGmCCWxv
Y2FsaG9zdIIOanVqdS1hcGlzZXJ2ZXKCDGp1anUtbW9uZ29kYoIIYW55dGhpbmeC
NGNvbnRyb2xsZXItc2VydmljZS5jb250cm9sbGVyLW1pY3JvazhzLWxvY2FsaG9z
dC5zdmMwCgYIKoZIzj0EAwIDRwAwRAIgDiRHSIxUmxr+LE+Ja9LENbgv/QmC7XFD
kBzDYg/oNisCIAx2LGL0r19fcn1rrVEMUrSMI4Dy3tJ0UEVgSbYhBGBA
-----END CERTIFICATE-----
`
	fingerPrint, remain, err := pki.Fingerprint([]byte(pemCertStr))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(remain), gc.Equals, 0)
	c.Assert(fingerPrint, gc.Equals, "C7:23:B1:3B:CE:26:BA:55:FF:3B:A0:8F:9D:98:E1:06:9A:70:D1:33:AF:D2:AF:22:F3:28:C0:B4:3B:0E:44:5B")
}

func (p *PEMSuite) TestIsCAPem(c *gc.C) {
	tests := []struct {
		pemStr string
		result bool
	}{
		{
			pemStr: `-----BEGIN CERTIFICATE-----
MIIBaDCCAQ2gAwIBAgIUQ8AAvRcOsUeMNS2JlgcNaHTttqUwCgYIKoZIzj0EAwIw
ITENMAsGA1UEChMESnVqdTEQMA4GA1UEAxMHanVqdS1jYTAeFw0yMDA0MDEwNTE1
MDJaFw0zMDA0MDEwNTIwMDJaMCExDTALBgNVBAoTBEp1anUxEDAOBgNVBAMTB2p1
anUtY2EwWTATBgcqhkjOPQIBBggqhkjOPQMBBwNCAAS2dFyd3vCw+E9xslHp21VZ
DqDzdyVhfOrWnDjgWoIG5mOIyV+KGXtib9ivcP11MUdZ8NbnL0+jVhYpGfU1U2OH
oyMwITAOBgNVHQ8BAf8EBAMCAqQwDwYDVR0TAQH/BAUwAwEB/zAKBggqhkjOPQQD
AgNJADBGAiEAvn0RqCuOGkR/0bng/vUD+VXa0ND7BfPDVm8XH/9QdwkCIQCKuTvK
9dvUdLCxPEraeZLvK12mioTCZzHXQA1crmQskA==
-----END CERTIFICATE-----`,
			result: true,
		},
		{
			pemStr: `-----BEGIN CERTIFICATE-----
MIICBjCCAa2gAwIBAgIVAPIfbVbfSiFAz9eOg1YNuQDakGxuMAoGCCqGSM49BAMC
MCExDTALBgNVBAoTBEp1anUxEDAOBgNVBAMTB2p1anUtY2EwHhcNMjAwNDAxMDQz
NTEwWhcNMzAwNDAxMDQ0MDEwWjBtMQ0wCwYDVQQKEwRKdWp1MS0wKwYDVQQDEyRK
dWp1IHNlcnZlciBjZXJ0aWZpY2F0ZSAtIGNvbnRyb2xsZXIxLTArBgNVBAUTJDUz
NmE4MjBhLWM1Y2QtNGY4Mi04ODkzLTk4OTU1MTNhZmExMjBZMBMGByqGSM49AgEG
CCqGSM49AwEHA0IABBxmIHbv7oMJidNfe4EyKgnVx/NrtjdJ94jro6nCSrLukXuj
tipXQyRUVdjzoVnbJ16YS1m0+WpTbWx8uPDPp2GjdjB0MHIGA1UdEQRrMGmCCWxv
Y2FsaG9zdIIOanVqdS1hcGlzZXJ2ZXKCDGp1anUtbW9uZ29kYoIIYW55dGhpbmeC
NGNvbnRyb2xsZXItc2VydmljZS5jb250cm9sbGVyLW1pY3JvazhzLWxvY2FsaG9z
dC5zdmMwCgYIKoZIzj0EAwIDRwAwRAIgDiRHSIxUmxr+LE+Ja9LENbgv/QmC7XFD
kBzDYg/oNisCIAx2LGL0r19fcn1rrVEMUrSMI4Dy3tJ0UEVgSbYhBGBA
-----END CERTIFICATE-----`,
			result: false,
		},
	}

	for _, test := range tests {
		ok, err := pki.IsPemCA([]byte(test.pemStr))
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ok, gc.Equals, test.result)
	}
}

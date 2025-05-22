// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki_test

import (
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/pki"
)

type PEMSuite struct {
	signer crypto.Signer
}

func TestPEMSuite(t *testing.T) {
	tc.Run(t, &PEMSuite{})
}

func (p *PEMSuite) SetUpTest(c *tc.C) {
	signer, err := pki.DefaultKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	p.signer = signer
}

func (p *PEMSuite) TestCertificateToPemString(c *tc.C) {
	ca, err := pki.NewCA("juju-test-ca", p.signer)
	c.Assert(err, tc.ErrorIsNil)

	certString, err := pki.CertificateToPemString(map[string]string{
		"test": "test-header",
	}, ca)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(certString), tc.GreaterThan, 0)
	//TODO re-enable headers on certificate to pem when Juju upgrade
	//CAAS mongo to something compiled with latest openssl. Currently
	//not all our Openssl versions support pem headers.
	//c.Assert(strings.Contains(certString, "test: test-header"), tc.IsTrue)
	c.Assert(strings.Contains(certString, "BEGIN CERTIFICATE"), tc.IsTrue)
	c.Assert(strings.Contains(certString, "END CERTIFICATE"), tc.IsTrue)
}

func (p *PEMSuite) TestCertificateToPemWriter(c *tc.C) {
	ca, err := pki.NewCA("juju-test-ca", p.signer)
	c.Assert(err, tc.ErrorIsNil)

	builder := strings.Builder{}
	err = pki.CertificateToPemWriter(&builder, map[string]string{
		"test": "test-header",
	}, ca)
	c.Assert(err, tc.ErrorIsNil)
	certString := builder.String()
	c.Assert(len(certString), tc.GreaterThan, 0)
	//TODO re-enable headers on certificate to pem when Juju upgrade
	//CAAS mongo to something compiled with latest openssl. Currently
	//not all our Openssl versions support pem headers.
	//c.Assert(strings.Contains(certString, "test: test-header"), tc.IsTrue)
	c.Assert(strings.Contains(certString, "BEGIN CERTIFICATE"), tc.IsTrue)
	c.Assert(strings.Contains(certString, "END CERTIFICATE"), tc.IsTrue)
}

func (p *PEMSuite) TestSignerToPemString(c *tc.C) {
	pemKey, err := pki.SignerToPemString(p.signer)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(pemKey), tc.GreaterThan, 0)
	c.Assert(strings.Contains(pemKey, "BEGIN PRIVATE KEY"), tc.IsTrue)
	c.Assert(strings.Contains(pemKey, "END PRIVATE KEY"), tc.IsTrue)
}

func (p *PEMSuite) TestSignerToPemWriter(c *tc.C) {
	builder := strings.Builder{}
	err := pki.SignerToPemWriter(&builder, p.signer)
	pemKey := builder.String()
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(pemKey), tc.GreaterThan, 0)
	c.Assert(strings.Contains(pemKey, "BEGIN PRIVATE KEY"), tc.IsTrue)
	c.Assert(strings.Contains(pemKey, "END PRIVATE KEY"), tc.IsTrue)
}

func (p *PEMSuite) TestSignerFromPKCS8Pem(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
		der, err := x509.MarshalPKCS8PrivateKey(signer)
		c.Assert(err, tc.ErrorIsNil)
		block := &pem.Block{
			Type:  pki.PEMTypePKCS8,
			Bytes: der,
		}

		signerPem, err := pki.UnmarshalSignerFromPemBlock(block)
		c.Assert(err, tc.ErrorIsNil)
		mc := tc.NewMultiChecker()
		// Ignore the computed speedups for RSA since comparing byte for byte
		// of bignumbers may fail due to superfluous zeros (despite the numbers matching)
		mc.AddExpr("_.Precomputed", tc.Ignore)
		c.Assert(signerPem, mc, signer)
	}
}

func (p *PEMSuite) TestSignerFromPKCS1Pem(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
		rsaKey, ok := signer.(*rsa.PrivateKey)
		c.Assert(ok, tc.Equals, true)

		der := x509.MarshalPKCS1PrivateKey(rsaKey)
		block := &pem.Block{
			Type:  pki.PEMTypePKCS1,
			Bytes: der,
		}

		signerPem, err := pki.UnmarshalSignerFromPemBlock(block)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(signerPem.Public(), tc.DeepEquals, signer.Public())
	}
}

func (p *PEMSuite) TestFingerprint(c *tc.C) {
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(len(remain), tc.Equals, 0)
	c.Assert(fingerPrint, tc.Equals, "C7:23:B1:3B:CE:26:BA:55:FF:3B:A0:8F:9D:98:E1:06:9A:70:D1:33:AF:D2:AF:22:F3:28:C0:B4:3B:0E:44:5B")
}

func (p *PEMSuite) TestIsCAPem(c *tc.C) {
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
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(ok, tc.Equals, test.result)
	}
}

// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_test

import (
	"bytes"
	"encoding/pem"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd"
	lxdtesting "github.com/juju/juju/container/lxd/testing"
)

var _ = gc.Suite(&certSuite{})

type certSuite struct {
	lxdtesting.BaseSuite
}

func (s *certSuite) TestGenerateClientCertificate(c *gc.C) {
	cert, err := lxd.GenerateClientCertificate()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cert.Validate(), jc.ErrorIsNil)
}

func (s *certSuite) TestValidateMissingCertPEM(c *gc.C) {
	cert := lxd.NewCertificate([]byte(testCertPEM), nil)
	c.Check(cert.Validate(), jc.Satisfies, errors.IsNotValid)
}

func (s *certSuite) TestValidateMissingKeyPEM(c *gc.C) {
	cert := lxd.NewCertificate(nil, []byte(testKeyPEM))
	c.Check(cert.Validate(), jc.Satisfies, errors.IsNotValid)
}

func (s *certSuite) TestWriteCertPEM(c *gc.C) {
	cert := lxd.NewCertificate([]byte(testCertPEM), []byte(testKeyPEM))

	var buf bytes.Buffer
	err := cert.WriteCertPEM(&buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(buf.String(), gc.Equals, testCertPEM)
}

func (s *certSuite) TestWriteKeyPEM(c *gc.C) {
	cert := lxd.NewCertificate([]byte(testCertPEM), []byte(testKeyPEM))

	var buf bytes.Buffer
	err := cert.WriteKeyPEM(&buf)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(buf.String(), gc.Equals, testKeyPEM)
}

func (s *certSuite) TestFingerprint(c *gc.C) {
	cert := lxd.NewCertificate([]byte(testCertPEM), []byte(testKeyPEM))
	fingerprint, err := cert.Fingerprint()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(fingerprint, gc.Equals, testCertFingerprint)
}

func (s *certSuite) TestX509Okay(c *gc.C) {
	cert := lxd.NewCertificate([]byte(testCertPEM), []byte(testKeyPEM))
	x509Cert, err := cert.X509()
	c.Assert(err, jc.ErrorIsNil)

	block, _ := pem.Decode([]byte(testCertPEM))
	c.Assert(block, gc.NotNil)
	c.Check(string(x509Cert.Raw), gc.Equals, string(block.Bytes))
}

func (s *certSuite) TestX509ZeroValue(c *gc.C) {
	cert := &lxd.Certificate{}
	_, err := cert.X509()
	c.Check(err, gc.ErrorMatches, `invalid cert PEM \(0 bytes\)`)
}

func (s *certSuite) TestX509BadPEM(c *gc.C) {
	cert := lxd.NewCertificate([]byte("some-invalid-pem"), nil)
	_, err := cert.X509()
	c.Check(err, gc.ErrorMatches, `invalid cert PEM \(\d+ bytes\)`)
}

func (s *certSuite) TestAsCreateRequestValidCert(c *gc.C) {
	cert := lxd.NewCertificate([]byte(testCertPEM), []byte(testKeyPEM))
	cert.Name = "juju-client-cert"
	req, err := cert.AsCreateRequest()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(req.Name, gc.Equals, "juju-client-cert")
	c.Check(req.Type, gc.Equals, "client")
	c.Check(req.Certificate, gc.Not(gc.Equals), "")
}

func (s *certSuite) TestAsCreateReqInvalidCert(c *gc.C) {
	cert := lxd.NewCertificate([]byte("some-invalid-pem"), nil)
	cert.Name = "juju-client-cert"

	_, err := cert.AsCreateRequest()
	c.Assert(err, gc.ErrorMatches, "failed to decode certificate PEM")
}

const (
	testCertFingerprint = "1c5156027fe71cfd0f7db807123e6873879f0f9754e08eab151f224783b2bff0"
	testCertPEM         = `
-----BEGIN CERTIFICATE-----
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
zxBNMbP3
-----END CERTIFICATE-----
`
	testKeyPEM = `
-----BEGIN CERTIFICATE-----
MIIF0jCCA7qgAwIBAgIQEFjWOkN8qXNbWKtveG5ddTANBgkqhkiG9w0BAQsFADA2
MRwwGgYDVQQKExNsaW51eGNvbnRhaW5lcnMub3JnMRYwFAYDVQQDDA1lc25vd0Bm
dXJpb3VzMB4XDTE1MTAwMTIxMjAyMloXDTI1MDkyODIxMjAyMlowNjEcMBoGA1UE
ChMTbGludXhjb250YWluZXJzLm9yZzEWMBQGA1UEAwwNZXNub3dAZnVyaW91czCC
AiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAMQgSXXaZMWImOP6IFBy/3E6
JFHwrgy5YMqRikoernt5cMr838nNdNLW9woBIVRZfZIFbAjf38PGBQYAs/4G/WIt
not+used+for+anything+really+just+make+sure+it+differs+from+cert
BmxTLbEvz87PrrsoVR9K5R261ciAFdFiE7Jbh15qUm4qXYHT9QgJeXnDtV/bxO+Y
Rrh9WfSP8x0SKrAoO7uhjI9Y276c8+etF0EY8u/+joqS8cZbOLXMuafgtF5E1trd
QNRHwiIhEUVqctdguzMHbhFfKthq6vP8qhWNOF6FowZgSg+Q5Tvm1jaU++BNPqWi
Zxy0qbMLRW8i/ABuTmzqtS3AHTtIFgdHx+BeT4W9LwU2dsO3Ijni2Rutmuz04rT+
zxBNMbP3
-----END CERTIFICATE-----
`
)

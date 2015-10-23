// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient_test

import (
	"bytes"
	"crypto/tls"
	"encoding/pem"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container/lxd/lxdclient"
)

var (
	_ = gc.Suite(&certSuite{})
	_ = gc.Suite(&certFunctionalSuite{})
)

type certSuite struct {
	lxdclient.BaseSuite

	certPEM []byte
	keyPEM  []byte
}

func (s *certSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.certPEM = []byte("<a valid PEM-encoded x.509 cert>")
	s.keyPEM = []byte("<a valid PEM-encoded x.509 key>")
}

func (s *certSuite) genCertAndKey() ([]byte, []byte, error) {
	s.Stub.AddCall("genCertAndKey")
	if err := s.Stub.NextErr(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	return s.certPEM, s.keyPEM, nil
}

func (s *certSuite) TestNewCertificate(c *gc.C) {
	cert := lxdclient.NewCertificate(s.certPEM, s.keyPEM)

	checkCert(c, cert, s.certPEM, s.keyPEM)
}

func (s *certSuite) TestGenerateCertificate(c *gc.C) {
	cert, err := lxdclient.GenerateCertificate(s.genCertAndKey)
	c.Assert(err, jc.ErrorIsNil)

	checkCert(c, cert, s.certPEM, s.keyPEM)
	s.Stub.CheckCallNames(c, "genCertAndKey")
}

func (s *certSuite) TestParsePEMCertFirst(c *gc.C) {
	data := []byte(testCertPEM + testKeyPEM)
	cert, err := lxdclient.ParsePEM(data)
	c.Assert(err, jc.ErrorIsNil)

	checkCert(c, cert, []byte(testCertPEM), []byte(testKeyPEM))
}

func (s *certSuite) TestParsePEMKeyFirst(c *gc.C) {
	data := []byte(testKeyPEM + testCertPEM)
	cert, err := lxdclient.ParsePEM(data)
	c.Assert(err, jc.ErrorIsNil)

	checkCert(c, cert, []byte(testCertPEM), []byte(testKeyPEM))
}

func (s *certSuite) TestParsePEMExtraBlankLines(c *gc.C) {
	data := []byte("\n" + testCertPEM + "\n" + testKeyPEM + "\n")
	cert, err := lxdclient.ParsePEM(data)
	c.Assert(err, jc.ErrorIsNil)

	checkCert(c, cert, []byte(testCertPEM), []byte(testKeyPEM))
}

func (s *certSuite) TestParsePEMExtraSection(c *gc.C) {
	extra := pemEncode("OTHER", "...")
	data := []byte(testCertPEM + extra + testKeyPEM)
	_, err := lxdclient.ParsePEM(data)

	c.Check(err, gc.ErrorMatches, `found unexpected OTHER section in PEM`)
}

func (s *certSuite) TestParsePEMBadData(c *gc.C) {
	extra := "-----BEGIN BOGUS-----\n..."
	data := []byte(testCertPEM + extra + testKeyPEM)
	_, err := lxdclient.ParsePEM(data)

	c.Check(err, gc.ErrorMatches, `no key found in PEM`)
}

func (s *certSuite) TestParsePEMLeadingData(c *gc.C) {
	data := []byte("...\n" + testCertPEM + testKeyPEM)
	cert, err := lxdclient.ParsePEM(data)
	c.Assert(err, jc.ErrorIsNil)

	checkCert(c, cert, []byte(testCertPEM), []byte(testKeyPEM))
}

func (s *certSuite) TestParsePEMTrailingData(c *gc.C) {
	data := []byte(testCertPEM + testKeyPEM + "...")
	cert, err := lxdclient.ParsePEM(data)
	c.Assert(err, jc.ErrorIsNil)

	checkCert(c, cert, []byte(testCertPEM), []byte(testKeyPEM))
}

func (s *certSuite) TestParsePEMMultipleCert(c *gc.C) {
	extra := pemEncode("CERTIFICATE", "...")
	data := []byte(testCertPEM + testKeyPEM + extra)
	_, err := lxdclient.ParsePEM(data)

	c.Check(err, gc.ErrorMatches, `found multiple CERTIFICATE sections in PEM`)
}

func (s *certSuite) TestParseMultipleKey(c *gc.C) {
	extra := pemEncode("RSA PRIVATE KEY", "...")
	data := []byte(testCertPEM + testKeyPEM + extra)
	_, err := lxdclient.ParsePEM(data)

	c.Check(err, gc.ErrorMatches, `found multiple RSA PRIVATE KEY sections in PEM`)
}

func (s *certSuite) TestParseMissingCert(c *gc.C) {
	data := []byte(testKeyPEM)
	_, err := lxdclient.ParsePEM(data)

	c.Check(err, gc.ErrorMatches, `no certificate found in PEM`)
}

func (s *certSuite) TestParseMissingKey(c *gc.C) {
	data := []byte(testCertPEM)
	_, err := lxdclient.ParsePEM(data)

	c.Check(err, gc.ErrorMatches, `no key found in PEM`)
}

func (s *certSuite) TestValidateOkay(c *gc.C) {
	cert := lxdclient.NewCertificate(s.certPEM, s.keyPEM)
	err := cert.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (s *certSuite) TestValidateMissingCertPEM(c *gc.C) {
	cert := lxdclient.NewCertificate(nil, s.keyPEM)
	err := cert.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *certSuite) TestValidateMissingKeyPEM(c *gc.C) {
	cert := lxdclient.NewCertificate(s.certPEM, nil)
	err := cert.Validate()

	c.Check(err, jc.Satisfies, errors.IsNotValid)
}

func (s *certSuite) TestWriteCertPEM(c *gc.C) {
	cert := lxdclient.NewCertificate(s.certPEM, s.keyPEM)
	var pemfile bytes.Buffer
	err := cert.WriteCertPEM(&pemfile)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(pemfile.String(), gc.Equals, string(s.certPEM))
}

func (s *certSuite) TestWriteKeyPEM(c *gc.C) {
	cert := lxdclient.NewCertificate(s.certPEM, s.keyPEM)
	var pemfile bytes.Buffer
	err := cert.WriteKeyPEM(&pemfile)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(pemfile.String(), gc.Equals, string(s.keyPEM))
}

func (s *certSuite) TestWritePEMs(c *gc.C) {
	cert := lxdclient.NewCertificate(s.certPEM, s.keyPEM)
	var pemfile bytes.Buffer
	err := cert.WriteCertPEM(&pemfile)
	c.Assert(err, jc.ErrorIsNil)
	err = cert.WriteKeyPEM(&pemfile)
	c.Assert(err, jc.ErrorIsNil)

	expected := string(s.certPEM) + string(s.keyPEM)
	c.Check(pemfile.String(), gc.Equals, expected)
}

type certFunctionalSuite struct {
	lxdclient.BaseSuite
}

func (s *certFunctionalSuite) TestGenerateCertificate(c *gc.C) {
	// This test involves the filesystem.
	cert, err := lxdclient.GenerateCertificate(lxdclient.GenCertAndKey)
	c.Assert(err, jc.ErrorIsNil)

	_, err = tls.X509KeyPair(cert.CertPEM, cert.KeyPEM)
	c.Check(err, jc.ErrorIsNil)
	parsed, err := lxdclient.ParsePEM(append(cert.CertPEM, cert.KeyPEM...))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(parsed, jc.DeepEquals, cert)
}

func checkCert(c *gc.C, cert *lxdclient.Certificate, certPEM, keyPEM []byte) {
	c.Check(cert, jc.DeepEquals, &lxdclient.Certificate{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	})
	c.Check(string(cert.CertPEM), gc.Equals, string(certPEM))
	c.Check(string(cert.KeyPEM), gc.Equals, string(keyPEM))
}

func pemEncode(blockType, data string) string {
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  blockType,
		Bytes: []byte(data),
	}))
}

const (
	testCertPEM = `-----BEGIN CERTIFICATE-----
MIIHJzCCBQ+gAwIBAgIQQDh3B6SKRTF1TJqWIZUpXjANBgkqhkiG9w0BAQsFADA2
MRwwGgYDVQQKExNsaW51eGNvbnRhaW5lcnMub3JnMRYwFAYDVQQDDA1lc25vd0Bm
dXJpb3VzMB4XDTE1MTAyNDE3NDI1MFoXDTI1MTAyMTE3NDI1MFowNjEcMBoGA1UE
ChMTbGludXhjb250YWluZXJzLm9yZzEWMBQGA1UEAwwNZXNub3dAZnVyaW91czCC
AiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAO8v8uwq3kj6gorfkIwDtaW0
aVYl6HGHoxnmYbjYjxBHc0NjQSTzQBu0ei7bwGncPc5piCInia6y0bZXkfgDZp2V
u92K3Eknqep6fnQmrkW91PKSTliwr1oDEmRSFHw/aedqHU1Fa0c4Z7/AQ/KC7uQ1
xjvkPLXQ20CK5pRb0wIhask21mlEKBtwelAJbrIcOuJuX9NdZYkNlE0p3vJV6l52
P74CDIQ+xaLrwWXwjeNPpZM2maUzkBhX1ihY+8pisZk98XCL3yPElKjHiO5l57nP
5ecsL6QWvOPoF2XswkeLUiGIW/izi29oaw/TkhjmFNDz2kF944IoQl/8ntaecFGn
Z9vQqZ6YCjtYywdXHSY4IsJDsGrWRDenvDZDPiACh/SfclDUSGB/YzrD4Qx8Z2ME
gEM6mc/D/Hc5Q9aFtgBdNTlI/Ar/IJiwyeA+c9Rb9rI6GGrvtBkVel1PI1cZeZlW
URxtgp9Nj5QAxCeHF7dFD5pl1R9sGJySbaVdDsN6RW8jIBFKWTo6d4iCf8Uf/g6I
dGFFWe5PyL9ltNLRvSoJV92pkOh86IITF68avcsHCkPZf+NahVK0ze7X+ln7GVZr
kf23xGWP7BGUMydTgcbHomOXChiao7Y0dbhwtTs+YwiK08Jk3FC7EHKkMMEldMPb
q/qADA5MGemp5a8VAXuDAgMBAAGjggIvMIICKzAOBgNVHQ8BAf8EBAMCBaAwEwYD
VR0lBAwwCgYIKwYBBQUHAwEwDAYDVR0TAQH/BAIwADCCAfQGA1UdEQSCAeswggHn
ggdmdXJpb3VzghExOTIuMTY4LjI5LjEwMi8yNIIoMjYwMTo2ODE6MjAwOmQzNjM6
MjA1MjozNGEzOmUxNzk6MjkzZi82NIIoMjYwMTo2ODE6MjAwOmQzNjM6N2Q2ZDo4
MTZiOmQxNTc6NmRiNi82NIInMjYwMTo2ODE6MjAwOmQzNjM6Mzg5NTphOWQyOmFm
NjpiOGZhLzY0gigyNjAxOjY4MToyMDA6ZDM2Mzo2MDI5OjczZmE6ZTQxMzpkOGRk
LzY0gigyNjAxOjY4MToyMDA6ZDM2MzozOGY2OmZiMjg6MzQ5ZjoyYjQyLzY0gigy
NjAxOjY4MToyMDA6ZDM2MzplNGI0OjgyZjM6NWRmZjoxYjJlLzY0gigyNjAxOjY4
MToyMDA6ZDM2Mzo5NDU2OmFhZjY6NzllNTo3MjgxLzY0gigyNjAxOjY4MToyMDA6
ZDM2Mzo1ZTUxOjRmZmY6ZmVkYzpjNWZkLzY0ghxmZTgwOjo1ZTUxOjRmZmY6ZmVk
YzpjNWZkLzY0ggsxMC4wLjMuMS8yNIIcZmU4MDo6YjBjZDo1ZGZmOmZlZjI6MzRl
Ny82NIIQMTkyLjE2OC4xMjIuMS8yNIIPMTkyLjE2OC42NC4xLzI0gg4xNzIuMTcu
NDIuMS8xNjANBgkqhkiG9w0BAQsFAAOCAgEAX5aXPCppFGwCfRcAGN+JlqS2dXhw
XHMn/qpig89rReiVOXWmpx7CNkcE2q5gRS1lGQYmWyBu+1dDKNsBlClJJux4eCun
+dnPA1SOsHsfjyovkk2EP7IuOq7ugFPV2uP8WvzszWifAdGJGF5eVD4x2NBHFuH3
2QXoGywEZ36JypSHSwgejLt/h2i/CrN8s3kXGPrUgDDxyyR1NP7TmmmSdFOn004h
OghCIHoKCLb+IsrQFY64bBtmJKCf+pymrP9aXML9zB/e+yh4hvg9qnC3DtCYrKGo
aVq46mVEc1a3t+P+Q+RPAp8hwu4zFKWxjB55xaj2dIHI1u2PrQDBzq9wovpZ3Ywq
sO7ECdLmN0WZjmB5AvB4cYqrVO0qoloNb9ev9KoOGzk6qF/s+F8qzmRqCAu6glO7
cwCLAmY+JJAYbbEQj33qMC2lDa7p9WhBairwbybhsG9/624IphnR1SOtzLZTHtxF
7wBndWjgDH+Ucqht245kKCbdjW6OiCjL+gJS1TEehyA8SkaFMIcpeYCRdDLbEWGm
oR5CESCrme2MQyv7v6ZLY+y1sR7cZMnxknAERmTQxVs4WsCG8U7e0xE9dpKlqSHC
ob9Z+fBgukOfQVIwpNAyQkeISdsvkbpkxf5cJqNAI3qzg43aefdknTARIsGIZAO9
03K21l3K34p2tdM=
-----END CERTIFICATE-----
`
	testKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----
`
)

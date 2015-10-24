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
	block, remainder := pem.Decode(cert.CertPEM)
	c.Check(block.Type, gc.Equals, "CERTIFICATE")
	c.Check(remainder, gc.HasLen, 0)
	block, remainder = pem.Decode(cert.KeyPEM)
	c.Check(block.Type, gc.Equals, "RSA PRIVATE KEY")
	c.Check(remainder, gc.HasLen, 0)
}

func checkCert(c *gc.C, cert *lxdclient.Certificate, certPEM, keyPEM []byte) {
	c.Check(cert, jc.DeepEquals, &lxdclient.Certificate{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	})
	c.Check(string(cert.CertPEM), gc.Equals, string(certPEM))
	c.Check(string(cert.KeyPEM), gc.Equals, string(keyPEM))
}

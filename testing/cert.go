// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"time"

	"launchpad.net/juju-core/cert"
)

func init() {
	if err := verifyCertificates(); err != nil {
		panic(err)
	}
}

// CACert and CAKey make up a CA key pair.
// CACertX509 and CAKeyRSA hold their parsed equivalents.
// ServerCert and ServerKey hold a CA-signed server cert/key.
var (
	CACert, CAKey = mustNewCA()

	CACertX509, CAKeyRSA = mustParseCertAndKey(CACert, CAKey)

	ServerCert, ServerKey = mustNewServer()
)

func verifyCertificates() error {
	_, err := tls.X509KeyPair([]byte(CACert), []byte(CAKey))
	if err != nil {
		return fmt.Errorf("bad CA cert key pair: %v", err)
	}
	_, err = tls.X509KeyPair([]byte(ServerCert), []byte(ServerKey))
	if err != nil {
		return fmt.Errorf("bad server cert key pair: %v", err)
	}
	return cert.Verify(ServerCert, CACert, time.Now())
}

func mustNewCA() (string, string) {
	cert.KeyBits = 512
	caCert, caKey, err := cert.NewCA("juju testing", time.Now().AddDate(10, 0, 0))
	if err != nil {
		panic(err)
	}
	return string(caCert), string(caKey)
}

func mustNewServer() (string, string) {
	cert.KeyBits = 512
	var hostnames []string
	srvCert, srvKey, err := cert.NewServer(CACert, CAKey, time.Now().AddDate(10, 0, 0), hostnames)
	if err != nil {
		panic(err)
	}
	return string(srvCert), string(srvKey)
}

func mustParseCert(pemData string) *x509.Certificate {
	cert, err := cert.ParseCert(pemData)
	if err != nil {
		panic(err)
	}
	return cert
}

func mustParseCertAndKey(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey) {
	cert, key, err := cert.ParseCertAndKey(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return cert, key
}

package testing

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"launchpad.net/juju-core/cert"
	"time"
)

func init() {
	if err := verifyCertificates(); err != nil {
		panic(err)
	}
}

func verifyCertificates() error {
	_, err := tls.X509KeyPair([]byte(CACertPEM), []byte(CAKeyPEM))
	if err != nil {
		return fmt.Errorf("bad CA cert key pair: %v", err)
	}
	_, err = tls.X509KeyPair([]byte(serverCertPEM), []byte(serverKeyPEM))
	if err != nil {
		return fmt.Errorf("bad server cert key pair: %v", err)
	}
	return cert.Verify([]byte(serverCertPEM), []byte(CACertPEM), time.Now())
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
	srvCert, srvKey, err := cert.NewServer("testing-env", []byte(CACertPEM), []byte(CAKeyPEM), time.Now().AddDate(10, 0, 0))
	if err != nil {
		panic(err)
	}
	return string(srvCert), string(srvKey)
}

// CACertPEM and CAKeyPEM make up a CA key pair.
// CACertX509 and CAKeyRSA hold their parsed equivalents.
var (
	CACertPEM, CAKeyPEM = mustNewCA()

	CACertX509 = mustParseCertPEM(CACertPEM)
	CAKeyRSA   = mustParseKeyPEM(CAKeyPEM)

	serverCertPEM, serverKeyPEM = mustNewServer()
)

func mustParseCertPEM(pemData string) *x509.Certificate {
	b, _ := pem.Decode([]byte(pemData))
	if b.Type != "CERTIFICATE" {
		panic("unexpected type")
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		panic(err)
	}
	return cert
}

func mustParseKeyPEM(pemData string) *rsa.PrivateKey {
	b, _ := pem.Decode([]byte(pemData))
	if b.Type != "RSA PRIVATE KEY" {
		panic("unexpected type")
	}
	key, err := x509.ParsePKCS1PrivateKey(b.Bytes)
	if key != nil {
		return key
	}
	key1, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		panic(err)
	}
	return key1.(*rsa.PrivateKey)
}

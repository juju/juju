// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/rand"
	"time"

	gitjujutesting "github.com/juju/testing"
	utilscert "github.com/juju/utils/cert"

	"github.com/juju/juju/cert"
)

func init() {
	if err := verifyCertificates(); err != nil {
		panic(err)
	}
}

// CACert and CAKey make up a CA key pair.
// CACertX509 and CAKeyRSA hold their parsed equivalents.
// ServerCert and ServerKey hold a CA-signed server cert/key.
// Certs holds the certificates and keys required to make a secure
// connection to a Mongo database.
var (
	CACert, CAKey, ServerCert, ServerKey = chooseGeneratedCA()

	CACertX509, CAKeyRSA = mustParseCertAndKey(CACert, CAKey)

	ServerTLSCert = mustParseServerCert(ServerCert, ServerKey)

	Certs = serverCerts()

	// Other valid test certs different from the default.
	OtherCACert, OtherCAKey        = chooseGeneratedOtherCA()
	OtherCACertX509, OtherCAKeyRSA = mustParseCertAndKey(OtherCACert, OtherCAKey)
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

func chooseGeneratedCA() (string, string, string, string) {
	index := rand.Intn(len(generatedCA))
	if len(generatedCA) != len(generatedServer) {
		// This should never happen.
		panic("generatedCA and generatedServer have mismatched length")
	}
	ca := generatedCA[index]
	server := generatedServer[index]
	return ca.certPEM, ca.keyPEM, server.certPEM, server.keyPEM
}

func chooseGeneratedOtherCA() (string, string) {
	index := rand.Intn(len(otherCA))
	ca := otherCA[index]
	return ca.certPEM, ca.keyPEM
}

func mustNewCA() (string, string) {
	caCert, caKey, err := cert.NewCA(
		"juju testing",
		"1234-ABCD-IS-NOT-A-REAL-UUID",
		time.Now().AddDate(10, 0, 0))
	if err != nil {
		panic(err)
	}
	return caCert, caKey
}

func mustNewServer() (*tls.Certificate, string, string) {
	var hostnames []string
	srvCert, srvKey, err := cert.NewServer(CACert, CAKey, time.Now().AddDate(10, 0, 0), hostnames)
	if err != nil {
		panic(err)
	}
	tlsCert := mustParseServerCert(srvCert, srvKey)
	return tlsCert, srvCert, srvKey
}

func mustParseServerCert(srvCert string, srvKey string) *tls.Certificate {
	tlsCert, err := tls.X509KeyPair([]byte(srvCert), []byte(srvKey))
	if err != nil {
		panic(err)
	}
	x509Cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		panic(err)
	}
	tlsCert.Leaf = x509Cert
	return &tlsCert
}

func mustParseCertAndKey(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey) {
	cert, key, err := utilscert.ParseCertAndKey(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}
	return cert, key
}

func serverCerts() *gitjujutesting.Certs {
	serverCert, serverKey := mustParseCertAndKey(ServerCert, ServerKey)
	return &gitjujutesting.Certs{
		CACert:     CACertX509,
		ServerCert: serverCert,
		ServerKey:  serverKey,
	}
}

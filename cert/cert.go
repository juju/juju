// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cert

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/errgo/errgo"
)

var KeyBits = 1024

// ParseCert parses the given PEM-formatted X509 certificate.
func ParseCert(certPEM []byte) (*x509.Certificate, error) {
	for len(certPEM) > 0 {
		var certBlock *pem.Block
		certBlock, certPEM = pem.Decode(certPEM)
		if certBlock == nil {
			break
		}
		if certBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			return cert, err
		}
	}
	return nil, errors.New("no certificates found")
}

// ParseCert parses the given PEM-formatted X509 certificate
// and RSA private key.
func ParseCertAndKey(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, err
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, nil, err
	}

	key, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("private key with unexpected type %T", key)
	}
	return cert, key, nil
}

// Verify verifies that the given server certificate is valid with
// respect to the given CA certificate at the given time.
func Verify(srvCertPEM, caCertPEM []byte, when time.Time) error {
	caCert, err := ParseCert(caCertPEM)
	if err != nil {
		return errgo.Annotate(err, "cannot parse CA certificate")
	}
	srvCert, err := ParseCert(srvCertPEM)
	if err != nil {
		return errgo.Annotate(err, "cannot parse server certificate")
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	opts := x509.VerifyOptions{
		DNSName:     "anyServer",
		Roots:       pool,
		CurrentTime: when,
	}
	_, err = srvCert.Verify(opts)
	return err
}

// NewCA generates a CA certificate/key pair suitable for signing server
// keys for an environment with the given name.
func NewCA(envName string, expiry time.Time) (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("juju-generated CA for environment %q", envName),
			Organization: []string{"juju"},
		},
		NotBefore:             now.UTC().Add(-5 * time.Minute),
		NotAfter:              expiry.UTC(),
		SubjectKeyId:          bigIntHash(key.N),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		MaxPathLen:            0, // Disallow delegation for now.
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("canot create certificate: %v", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return certPEM, keyPEM, nil
}

// NewServer generates a certificate/key pair suitable for use by a server.
func NewServer(caCertPEM, caKeyPEM []byte, expiry time.Time, hostnames []string) (certPEM, keyPEM []byte, err error) {
	return newLeaf(caCertPEM, caKeyPEM, expiry, hostnames, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
}

// NewClient generates a certificate/key pair suitable for client authentication.
func NewClient(caCertPEM, caKeyPEM []byte, expiry time.Time) (certPEM, keyPEM []byte, err error) {
	return newLeaf(caCertPEM, caKeyPEM, expiry, nil, []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth})
}

// newLeaf generates a certificate/key pair suitable for use by a leaf node.
func newLeaf(caCertPEM, caKeyPEM []byte, expiry time.Time, hostnames []string, extKeyUsage []x509.ExtKeyUsage) (certPEM, keyPEM []byte, err error) {
	tlsCert, err := tls.X509KeyPair(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	if len(tlsCert.Certificate) != 1 {
		return nil, nil, fmt.Errorf("more than one certificate for CA")
	}
	caCert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	if !caCert.BasicConstraintsValid || !caCert.IsCA {
		return nil, nil, fmt.Errorf("CA certificate is not a valid CA")
	}
	caKey, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("CA private key has unexpected type %T", tlsCert.PrivateKey)
	}
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return nil, nil, fmt.Errorf("cannot generate key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			// This won't match host names with dots. The hostname
			// is hardcoded when connecting to avoid the issue.
			CommonName:   "*",
			Organization: []string{"juju"},
		},
		NotBefore: now.UTC().Add(-5 * time.Minute),
		NotAfter:  expiry.UTC(),

		SubjectKeyId: bigIntHash(key.N),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageKeyAgreement,
		ExtKeyUsage:  extKeyUsage,
	}
	for _, hostname := range hostnames {
		if ip := net.ParseIP(hostname); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, hostname)
		}
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return certPEM, keyPEM, nil
}

func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}

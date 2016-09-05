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
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/juju/errors"
)

var KeyBits = 2048

// ParseCert parses the given PEM-formatted X509 certificate.
func ParseCert(certPEM string) (*x509.Certificate, error) {
	certPEMData := []byte(certPEM)
	for len(certPEMData) > 0 {
		var certBlock *pem.Block
		certBlock, certPEMData = pem.Decode(certPEMData)
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

// ParseCertAndKey parses the given PEM-formatted X509 certificate
// and RSA private key.
func ParseCertAndKey(certPEM, keyPEM string) (*x509.Certificate, *rsa.PrivateKey, error) {
	tlsCert, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
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
func Verify(srvCertPEM, caCertPEM string, when time.Time) error {
	caCert, err := ParseCert(caCertPEM)
	if err != nil {
		return errors.Annotate(err, "cannot parse CA certificate")
	}
	srvCert, err := ParseCert(srvCertPEM)
	if err != nil {
		return errors.Annotate(err, "cannot parse server certificate")
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
func NewCA(envName, UUID string, expiry time.Time) (certPEM, keyPEM string, err error) {
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return "", "", err
	}
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()

	serialNumber, err := newSerialNumber()
	if err != nil {
		return "", "", errors.Trace(err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   fmt.Sprintf("juju-generated CA for model %q", envName),
			Organization: []string{"juju"},
			SerialNumber: UUID,
		},
		NotBefore:             now.UTC().AddDate(0, 0, -7),
		NotAfter:              expiry.UTC(),
		SubjectKeyId:          bigIntHash(key.N),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:                  true,
		MaxPathLen:            0, // Disallow delegation for now.
		BasicConstraintsValid: true,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("cannot create certificate: %v", err)
	}
	certPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return string(certPEMData), string(keyPEMData), nil
}

// newSerialNumber returns a new random serial number suitable
// for use in a certificate.
func newSerialNumber() (*big.Int, error) {
	// A serial number can be up to 20 octets in size.
	// https://tools.ietf.org/html/rfc5280#section-4.1.2.2
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 8*20))
	if err != nil {
		return nil, errors.Annotatef(err, "failed to generate serial number")
	}
	return n, nil
}

// NewDefaultServer generates a certificate/key pair suitable for use by a server, with an
// expiry time of 10 years.
func NewDefaultServer(caCertPEM, caKeyPEM string, hostnames []string) (certPEM, keyPEM string, err error) {
	// TODO(perrito666) 2016-05-02 lp:1558657
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	return newLeaf(caCertPEM, caKeyPEM, expiry, hostnames, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
}

// NewServer generates a certificate/key pair suitable for use by a server.
func NewServer(caCertPEM, caKeyPEM string, expiry time.Time, hostnames []string) (certPEM, keyPEM string, err error) {
	return newLeaf(caCertPEM, caKeyPEM, expiry, hostnames, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth})
}

// newLeaf generates a certificate/key pair suitable for use by a leaf node.
func newLeaf(caCertPEM, caKeyPEM string, expiry time.Time, hostnames []string, extKeyUsage []x509.ExtKeyUsage) (certPEM, keyPEM string, err error) {
	tlsCert, err := tls.X509KeyPair([]byte(caCertPEM), []byte(caKeyPEM))
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if len(tlsCert.Certificate) != 1 {
		return "", "", fmt.Errorf("more than one certificate for CA")
	}
	caCert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if !caCert.BasicConstraintsValid || !caCert.IsCA {
		return "", "", errors.Errorf("CA certificate is not a valid CA")
	}
	caKey, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return "", "", errors.Errorf("CA private key has unexpected type %T", tlsCert.PrivateKey)
	}
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return "", "", errors.Errorf("cannot generate key: %v", err)
	}

	serialNumber, err := newSerialNumber()
	if err != nil {
		return "", "", errors.Trace(err)
	}
	// TODO(perrito666) 2016-05-02 lp:1558657
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			// This won't match host names with dots. The hostname
			// is hardcoded when connecting to avoid the issue.
			CommonName:   "*",
			Organization: []string{"juju"},
		},
		NotBefore: now.UTC().AddDate(0, 0, -7),
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
		return "", "", err
	}
	certPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	keyPEMData := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	return string(certPEMData), string(keyPEMData), nil
}

func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}

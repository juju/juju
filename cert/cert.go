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
	"time"
)

var KeyBits = 1024

func ParseCertificate(certPEM []byte) (*x509.Certificate, error) {
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

// NewCACert generates a CA certificate/key pair suitable for
// signing server keys for an environment with the
// given name.
func NewCA(envName string, expiry time.Time) (certPEM, keyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, KeyBits)
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			// TODO quote the environment name when we start using
			// Go version 1.1. See Go issue 3791.
			CommonName:   fmt.Sprintf("juju-generated CA for environment %s", envName),
			Organization: []string{"juju"},
		},
		NotBefore:             now.UTC().Add(-5 * time.Minute),
		NotAfter:              expiry,
		SubjectKeyId:          bigIntHash(key.N),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:       true,
		MaxPathLen: 0, // Disallow delegation for now.
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

// NewServer generates a certificate/key pair suitable
// for using by a server for an environment with the given
// name.
func NewServer(envName string, caCertPEM, caKeyPEM []byte, expiry time.Time) (certPEM, keyPEM []byte, err error) {
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
		NotAfter:  now.UTC().AddDate(10, 0, 0), // 10 years hence.

		SubjectKeyId: bigIntHash(key.N),
		KeyUsage:     x509.KeyUsageDataEncipherment,
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

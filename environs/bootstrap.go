package environs

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Bootstrap bootstraps the given environment.  If the environment does
// not contain a CA certificate, a new certificate and key pair are
// generated, added to the environment configuration, and writeCertAndKey
// will be called to save them.  If writeCertFile is nil, the generated
// certificate and key will be saved to ~/.juju/<environ-name>-cert.pem
// and ~/.juju/<environ-name>-private-key.pem.
//
// If uploadTools is true, the current version of the juju tools will be
// uploaded, as documented in Environ.Bootstrap.
func Bootstrap(environ Environ, uploadTools bool, writeCertAndKey func(environName string, cert, key []byte) error) error {
	if writeCertAndKey == nil {
		writeCertAndKey = writeCertAndKeyToHome
	}
	cfg := environ.Config()
	caCert, hasCACert := cfg.CACert()
	caKey, hasCAKey := cfg.CAPrivateKey()
	if !hasCACert {
		if hasCAKey {
			return fmt.Errorf("environment configuration with CA private key but no certificate")
		}
		var err error
		caCert, caKey, err = generateCACert(environ.Name())
		if err != nil {
			return err
		}
		m := cfg.AllAttrs()
		m["ca-cert"] = string(caCert)
		m["ca-private-key"] = string(caKey)
		cfg, err = config.New(m)
		if err != nil {
			return fmt.Errorf("cannot create environment configuration with new CA: %v", err)
		}
		if err := environ.SetConfig(cfg); err != nil {
			return fmt.Errorf("cannot set environment configuration with CA: %v", err)
		}
		if err := writeCertAndKey(environ.Name(), caCert, caKey); err != nil {
			return fmt.Errorf("cannot write CA certificate and key: %v", err)
		}
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	cert, key, err := generateCert(environ.Name(), caCert, caKey)
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, cert, key)
}

func writeCertAndKeyToHome(name string, cert, key []byte) error {
	path := filepath.Join(os.Getenv("HOME"), ".juju", name)
	if err := ioutil.WriteFile(path+"-cert.pem", cert, 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path+"-private-key.pem", key, 0600); err != nil {
		return err
	}
	return nil
}

const keyBits = 1024

func generateCACert(envName string) (certPEM, keyPEM []byte, err error) {
	log.Printf("generating new CA certificate")
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
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
		NotAfter:              now.UTC().AddDate(10, 0, 0), // 10 years hence.
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

func generateCert(envName string, caCertPEM, caKeyPEM []byte) (certPEM, keyPEM []byte, err error) {
	tlsCert, err := tls.X509KeyPair(caCertPEM, caKeyPEM)
	if err != nil {
		return nil, nil, err
	}
	if len(tlsCert.Certificate) != 1 {
		return nil, nil, fmt.Errorf("CA key pair must have 1 certificate, not %d", len(tlsCert.Certificate))
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
	key, err := rsa.GenerateKey(rand.Reader, keyBits)
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

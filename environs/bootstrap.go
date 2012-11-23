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
// generated, added to the environment configuration, and writeCertFile
// will be called to save them.  If writeCertFile is nil, the generated
// certificate and key pair will be saved to ~/.juju.
//
// If uploadTools is true, the current version of the juju tools will be
// uploaded, as documented in Environ.Bootstrap.
func Bootstrap(environ Environ, uploadTools bool, writeCertFile func(name string, data []byte) error) error {
	if writeCertFile == nil {
		writeCertFile = writeCertFileToHome
	}
	cfg := environ.Config()
	caCertPEM, hasCACert := cfg.CACertPEM()
	caKeyPEM, hasCAKey := cfg.CAPrivateKeyPEM()
	if !hasCACert {
		if hasCAKey {
			return fmt.Errorf("environment config has private key without CA certificate")
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
			return fmt.Errorf("cannot create config with added CA certificate: %v", err)
		}
		if err := environ.SetConfig(cfg); err != nil {
			return fmt.Errorf("cannot add CA certificate to environ: %v", err)
		}
		if err := writeCertFile(environ.Name() + "-cert.pem", caCert); err != nil {
			return fmt.Errorf("cannot save CA certificate: %v", err)
		}
		if err := writeCertFile(environ.Name() + "-private-key.pem", caKey); err != nil {
			return fmt.Errorf("cannot save CA key: %v", err)
		}
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	certPEM, keyPEM, err := generateCert(environ.Name(), caCert, caKey)
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, certPEM, keyPEM)
}

func writeCertFileToHome(name string, data []byte) error {
	path := filepath.Join(os.Getenv("HOME"), ".juju", name)
	return ioutil.WriteFile(path, data, 0600)
}

const keyBits = 1024

func generateCACert(envName string) (*x509.Certificate, *rsa.PrivateKey, error)
certPEM, keyPEM []byte, err error) {
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
		BasicConstraintsValid: true,
		IsCA:       true,
		MaxPathLen: 0, // Disallow delegation for now.
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("canot create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func certToPEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&bytes.Buffer{
		Type: "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

func keyToPEM(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}


func generateCert(envName string, caCertPEM, caKeyPEM string) (certPEM, keyPEM string, err error) {
	if !caCert.BasicConstraintsValid || !caCert.IsCA {
		return nil, nil, fmt.Errorf("CA certificate is not a valid CA")
	}
	tlsCert, err := tls.X509KeyPair(certToPEM(caCert), keyToPEM(caKey))
	if err != nil {
		return nil, nil, err
	}
	if len(tlsCert.Certificate) != 1 {
		return nil, nil, fmt.Errorf("more than one certificate for CA")
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

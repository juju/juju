package juju

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/log"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// Bootstrap bootstraps the given environment.  The CA certificate and
// private key in PEM format can be given in caPEM; if this is nil,
// they are read from $HOME/.juju/<environ-name>.pem, or generated and
// written there if the file does not exist.  If uploadTools is true,
// the current version of the juju tools will be uploaded, as documented
// in environs.Environ.Bootstrap.
func Bootstrap(environ environs.Environ, uploadTools bool, caPEM []byte) error {
	if caPEM == nil {
		var err error
		caPEM, err = generateCACert(environ.Name())
		if err != nil {
			return fmt.Errorf("cannot generate CA certificate: %v", err)
		}
	}
	caCert, caKey, err := parseCAPEM(caPEM, true)
	if err != nil {
		return fmt.Errorf("bad CA PEM: %v", err)
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	cert, err := generateCert(environ.Name(), caCert, caKey)
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, cert)
}

const keyBits = 1024

func generateCACert(envName string) ([]byte, error) {
	path := filepath.Join(os.Getenv("HOME"), ".juju", envName+".pem")
	data, err := ioutil.ReadFile(path)
	if err == nil {
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			// TODO quote the environment name when we start using
			// Go version 1.1.
			CommonName:   fmt.Sprintf("juju-generated CA for environment %s", envName),
			Organization: []string{"juju"},
		},
		NotBefore:             now.UTC().Add(-5 * time.Minute),
		NotAfter:              now.UTC().AddDate(10, 0, 0), // 10 years hence.
		SubjectKeyId:          bigIntHash(priv.N),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:       true,
		MaxPathLen: 0, // Disallow delegation for now.
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return nil, fmt.Errorf("canot create certificate: %v", err)
	}
	var b bytes.Buffer
	pem.Encode(&b, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	pem.Encode(&b, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	if err := ioutil.WriteFile(path, b.Bytes(), 0600); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func generateCert(envName string, caCert *x509.Certificate, caKey *rsa.PrivateKey) ([]byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, fmt.Errorf("cannot generate key: %v", err)
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

		SubjectKeyId: bigIntHash(priv.N),
		KeyUsage:     x509.KeyUsageDataEncipherment,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	pem.Encode(&b, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})
	pem.Encode(&b, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	return b.Bytes(), nil
}

func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}

func parseCAPEM(caPEM []byte, requirePrivateKey bool) (*x509.Certificate, *rsa.PrivateKey, error) {
	var certBlock, keyBlock *pem.Block
	// We split the CA certificate pem into certificate
	// blocks and non-certificate blocks so that
	// it's amenable to checking with tls.X509KeyPair.
	for {
		var b *pem.Block
		b, caPEM = pem.Decode(caPEM)
		if b == nil {
			break
		}
		switch b.Type {
		case "CERTIFICATE":
			if certBlock != nil {
				return nil, nil, fmt.Errorf("more than one certificate found in CA certificate PEM")
			}
			certBlock = b
		case "RSA PRIVATE KEY":
			if keyBlock != nil {
				return nil, nil, fmt.Errorf("more than one key found in CA certificate PEM")
			}
			keyBlock = b
		default:
			log.Printf("juju: unknown PEM block type %q found in CA certificate", b.Type)
		}
	}

	if certBlock == nil {
		return nil, nil, fmt.Errorf("CA PEM holds no certificate")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	if !cert.BasicConstraintsValid || !cert.IsCA {
		return nil, nil, fmt.Errorf("CA certificate is not a valid CA")
	}
	if keyBlock == nil {
		if requirePrivateKey {
			return nil, nil, fmt.Errorf("CA PEM holds no private key")
		}
		return cert, nil, nil
	}
	tlsCert, err := tls.X509KeyPair(pem.EncodeToMemory(certBlock), pem.EncodeToMemory(keyBlock))
	if err != nil {
		return nil, nil, err
	}
	priv, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("CA private key has unexpected type %T", tlsCert.PrivateKey)
	}
	return cert, priv, nil
}

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

// Bootstrap bootstraps the given environment.  The root certifying
// authority certificate and private key in PEM format can be given in
// rootPEM; if this is nil, the root CA certificate and key pair is read
// from $HOME/.juju/<environ-name>-cert.pem, or generated and written
// there if the file does not exist.  If uploadTools is true, the
// current version of the juju tools will be uploaded, as documented in
// environs.Environ.Bootstrap.
func Bootstrap(environ environs.Environ, uploadTools bool, rootPEM []byte) error {
	if rootPEM == nil {
		var err error
		rootPEM, err = generateRootCert(environ.Name())
		if err != nil {
			return fmt.Errorf("cannot generate root certificate: %v", err)
		}
	}
	rootCert, rootKey, err := parseRootPEM(rootPEM, true)
	if err != nil {
		return fmt.Errorf("bad root CA PEM: %v", err)
	}
	// Generate a new key pair and certificate for
	// the newly bootstrapped instance.
	bootstrapCert, err := generateBootstrapCert(environ.Name(), rootCert, rootKey)
	if err != nil {
		return fmt.Errorf("cannot generate bootstrap certificate: %v", err)
	}
	return environ.Bootstrap(uploadTools, bootstrapCert)
}

const keyBits = 1024

func generateRootCert(envName string) ([]byte, error) {
	// TODO make sure that the environment name cannot
	// contain slashes.
	path := filepath.Join(os.Getenv("HOME"), ".juju", envName+"-cert.pem")
	data, err := ioutil.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		return data, nil
	}
	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			CommonName:   "juju-generated root CA for environment " + envName,
			Organization: []string{"juju"},
		},
		NotBefore:             now.Add(-5 * time.Minute).UTC(),
		NotAfter:              now.UTC().AddDate(10, 0, 0), // 10 years hence.
		SubjectKeyId:          bigIntHash(priv.N),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:       true,
		MaxPathLen: 1,
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

func generateBootstrapCert(envName string, rootCert *x509.Certificate, rootKey *rsa.PrivateKey) ([]byte, error) {
	priv, err := rsa.GenerateKey(rand.Reader, keyBits)
	if err != nil {
		return nil, fmt.Errorf("cannot generate key: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			// Note: this is important - we use the same
			// name for the host name when we connect
			// later, so that we can avoid hostname verification,
			// because we don't initially know the name
			// of the bootstrap server.
			CommonName:   "anyServer",
			Organization: []string{"juju"},
		},
		NotBefore: now.Add(-5 * time.Minute).UTC(),
		NotAfter:  now.UTC().AddDate(10, 0, 0), // 10 years hence.

		SubjectKeyId: bigIntHash(priv.N),
		KeyUsage:     x509.KeyUsageDataEncipherment,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, rootCert, &priv.PublicKey, rootKey)
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

func parseRootPEM(rootPEM []byte, requirePrivateKey bool) (cert *x509.Certificate, priv *rsa.PrivateKey, err error) {
	var certBlock, keyBlock *pem.Block
	// We split the root certificate pem into certificate
	// blocks and non-certificate blocks so that
	// it's amenable to checking with tls.X509KeyPair.
	for {
		var b *pem.Block
		b, rootPEM = pem.Decode(rootPEM)
		if b == nil {
			break
		}
		switch b.Type {
		case "CERTIFICATE":
			if certBlock != nil {
				return nil, nil, fmt.Errorf("more than one certificate found in root certificate")
			}
			certBlock = b
		case "RSA PRIVATE KEY":
			if keyBlock != nil {
				return nil, nil, fmt.Errorf("more than one key found in root certificate")
			}
			keyBlock = b
		default:
			log.Printf("juju: unknown PEM block type %q found", b.Type)
		}
	}
	if keyBlock == nil {
		if requirePrivateKey {
			return nil, nil, fmt.Errorf("root PEM holds no private key")
		}
		cert, err := x509.ParseCertificate(certBlock.Bytes)
		if err != nil {
			return nil, nil, err
		}
		if !cert.BasicConstraintsValid || !cert.IsCA {
			return nil, nil, fmt.Errorf("root certificate is not a valid CA")
		}
		return cert, nil, nil
	}
	if certBlock == nil {
		return nil, nil, fmt.Errorf("root PEM holds no certificate")
	}
	tlsCert, err := tls.X509KeyPair(pem.EncodeToMemory(certBlock), pem.EncodeToMemory(keyBlock))
	if err != nil {
		return nil, nil, err
	}
	priv, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, fmt.Errorf("certificate private key has unexpected type %T", tlsCert.PrivateKey)
	}
	if len(tlsCert.Certificate) != 1 {
		return nil, nil, fmt.Errorf("expected 1 certificate, got %d", len(tlsCert.Certificate))
	}
	cert, err = x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, nil, err
	}
	if !cert.BasicConstraintsValid || !cert.IsCA {
		return nil, nil, fmt.Errorf("root certificate is not a valid CA")
	}
	return cert, priv, nil
}

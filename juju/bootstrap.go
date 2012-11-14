package juju
import (
	"os/exec"
	"bufio"
	"bytes"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/tls"
	"crypto/x509"
	"crypto/rand"
	"crypto/x509/pkix"
	"labix.org/v2/mgo"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"net"
	"syscall"
	"sync"
	"time"
	"crypto/md5"
)

// Bootstrap bootstraps the given environment.
// The root certifying authority certificate and private key in PEM
// format can be given in rootCert; if this is nil,
// the root CA certificate and key pair is
// read from $HOME/.juju/<environ-name>-cert.pem,
// or generated and written there if the file does not exist.
func Bootstrap(environ environs.Environ, rootCert []byte) error {
	if rootCert == nil {
		rootCert, err = generateRootCert(environ.Name())
		if err != nil {
			return fmt.Errorf("cannot generate root certificate: %v", err)
		}
	}
	if err := verifyRootCert(environ.Name()); err != nil {
		return err
	}
}

const (
	rootKeyBits = 1024
)

func generateRootCert(envName string) ([]byte, error) {
	// TODO make sure that the environment name cannot
	// contain slashes.
	path := filepath.Join(os.Getenv("HOME"), ".juju", envName + "-cert.pem")
	data, err := ioutil.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil {
		return data, nil
	}
	priv, err := rsa.GenerateKey(rand.Reader, rootKeyBits)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: new(big.Int),
		Subject: pkix.Name{
			CommonName:   "juju-generated root CA for environment "+envName,
			Organization: []string{"juju"},
		},
		NotBefore: now.Add(-5 * time.Minute).UTC(),
		NotAfter:  now.Add(50 * 365 * 24 * time.Hour).UTC(),	// 50 years
		SubjectKeyId: bigIntHash(priv.N),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA: true,
		MaxPathLen: 1,
	}
	derBytes, err = x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("canot create certificate: %v", err)
	}
	var b bytes.Buffer
	pem.Encode(&b, &pem.Block{
		Type: "CERTIFICATE",
		Bytes: derBytes,
	})
	pem.Encode(&b, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	return b.Bytes(), nil
}

func verifyRootCert(pemData []byte) error {
	for {
		var b *pem.Block
		b, pemData = pem.Decode(pemData)
		if b == nil {
			break
		}
	}
}


func bigIntHash(n *big.Int) []byte {
	h := sha1.New()
	h.Write(n.Bytes())
	return h.Sum(nil)
}

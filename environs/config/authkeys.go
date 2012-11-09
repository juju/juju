package config

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"launchpad.net/juju-core/log"
	"os"
	"path/filepath"
	"strings"
)

func expandTilde(f string) string {
	// TODO expansion of other user's home directories.
	// Q what characters are valid in a user name?
	if strings.HasPrefix(f, "~"+string(filepath.Separator)) {
		return os.Getenv("HOME") + f[1:]
	}
	return f
}

// authorizedKeys implements the standard juju behaviour for finding
// authorized_keys. It returns a set of keys in in authorized_keys format
// (see sshd(8) for a description).  If path is non-empty, it names the
// file to use; otherwise the user's .ssh directory will be searched.
// Home directory expansion will be performed on the path if it starts with
// a ~; if the expanded path is relative, it will be interpreted relative
// to $HOME/.ssh.
func readAuthorizedKeys(path string) (string, error) {
	var files []string
	if path == "" {
		files = []string{"id_dsa.pub", "id_rsa.pub", "identity.pub"}
	} else {
		files = []string{path}
	}
	var firstError error
	var keyData []byte
	for _, f := range files {
		f = expandTilde(f)
		if !filepath.IsAbs(f) {
			f = filepath.Join(os.Getenv("HOME"), ".ssh", f)
		}
		data, err := ioutil.ReadFile(f)
		if err != nil {
			if firstError == nil && !os.IsNotExist(err) {
				firstError = err
			}
			continue
		}
		keyData = append(keyData, bytes.Trim(data, "\n")...)
		keyData = append(keyData, '\n')
	}
	if len(keyData) == 0 {
		if firstError == nil {
			firstError = fmt.Errorf("no public ssh keys found")
		}
		return "", firstError
	}
	return string(keyData), nil
}

func makeRootCertPath(path string) string {
	if path == "" {
		path = "rootcert.pem"
	}
	path = expandTilde(path)
	if !filepath.IsAbs(path) {
		path = filepath.Join(os.Getenv("HOME"), ".juju", path)
	}
	return path
}

// readRootCert reads a certificate and key for the root
// certifying authority from the given path.
func readRootCert(path string) (cert, key []byte, err error) {
	pemData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var certPEM bytes.Buffer
	var keyDERBlock *pem.Block
	// Separate any certificates from the private key.
	for len(pemData) > 0 {
		var b *pem.Block
		b, pemData = pem.Decode(pemData)
		switch b.Type {
		case "CERTIFICATE":
			pem.Encode(&certPEM, b)
		case "RSA PRIVATE KEY":
			if keyDERBlock != nil {
				return nil, nil, fmt.Errorf("more than one private key found")
			}
			keyDERBlock = b
		default:
			log.Printf("environs/config: unexpected PEM type %q", b.Type)
		}
	}
	cert = certPEM.Bytes()
	if len(cert) == 0 {
		return nil, nil, fmt.Errorf("no certificate found")
	}
	if keyDERBlock != nil {
		key = pem.EncodeToMemory(keyDERBlock)
	}
	return cert, key, nil
}

// verifyKeyPair verifies that the certificate and key parse correctly.
// The key is optional - if it is provided, we also check that the key
// matches the certificate.
func verifyKeyPair(certPEM, keyPEM []byte) error {
	log.Printf("verify ok? cert %d, key %d", len(certPEM), len(keyPEM))
	if len(keyPEM) > 0 {
		_, err := tls.X509KeyPair(certPEM, keyPEM)
		log.Printf("verify key pair -> %v", err)
		return err
	}
	for len(certPEM) > 0 {
		certBlock, _ := pem.Decode(certPEM)
		if certBlock != nil && certBlock.Type == "CERTIFICATE" {
			_, err := x509.ParseCertificate(certBlock.Bytes)
			return err
		}
	}
	return fmt.Errorf("no certificates found")
}

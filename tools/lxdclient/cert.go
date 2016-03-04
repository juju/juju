// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/juju/errors"
)

const (
	tempPrefix = "juju-lxd-"

	pemBlockTypeCert = "CERTIFICATE"
	pemBlockTypeKey  = "RSA PRIVATE KEY"

	certDefaultName = "juju-client-certificate"
)

// Cert holds the information for a single certificate a client
// may use to connect to a remote server.
type Cert struct {
	// Name is the name that LXD will use for the cert.
	Name string

	// CertPEM is the PEM-encoded x.509 cert.
	CertPEM []byte

	// KeyPEM is the PEM-encoded x.509 private key.
	KeyPEM []byte
}

// NewCertificate creates a new Certificate for the given cert and key.
func NewCert(certPEM, keyPEM []byte) Cert {
	return Cert{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
}

// WithDefaults updates a copy of the remote with default values
// where needed.
func (cert Cert) WithDefaults() (Cert, error) {
	// Note that cert is a value receiver, so it is an implicit copy.

	if cert.Name == "" {
		// The certificate Name will get overridden later by code that
		// knows what environment this certificate is being used for.
		cert.Name = certDefaultName
	}

	// WithDefaults doesn't populate CertPEM or KeyPEM because those aren't
	// used when contacting the LXD service via the unix socket.
	return cert, nil
}

func (cert *Cert) isZero() bool {
	if cert == nil {
		return true
	}
	return len(cert.CertPEM) == 0 && len(cert.KeyPEM) == 0
}

// Validate ensures that the cert is valid.
func (cert Cert) Validate() error {
	if len(cert.CertPEM) == 0 {
		return errors.NotValidf("missing cert PEM")
	}
	if len(cert.KeyPEM) == 0 {
		return errors.NotValidf("missing key PEM")
	}

	// TODO(ericsnow) Ensure cert and key are valid?

	return nil
}

// WriteCertPEM writes the cert's x.509 PEM data to the given writer.
func (cert Cert) WriteCertPEM(out io.Writer) error {
	if _, err := out.Write(cert.CertPEM); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WriteKeytPEM writes the key's x.509 PEM data to the given writer.
func (cert Cert) WriteKeyPEM(out io.Writer) error {
	if _, err := out.Write(cert.KeyPEM); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Fingerprint returns the cert's LXD fingerprint.
func (cert Cert) Fingerprint() (string, error) {
	// See: https://github.com/lxc/lxd/blob/master/lxd/certificates.go
	x509Cert, err := cert.X509()
	if err != nil {
		return "", errors.Trace(err)
	}
	data := sha256.Sum256(x509Cert.Raw)
	return fmt.Sprintf("%x", data), nil
}

// X509 returns the x.509 certificate.
func (cert Cert) X509() (*x509.Certificate, error) {
	block, _ := pem.Decode(cert.CertPEM)
	if block == nil {
		return nil, errors.Errorf("invalid cert PEM (%d bytes)", len(cert.CertPEM))
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return x509Cert, nil
}

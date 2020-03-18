// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/api"
)

// Certificate holds the information for a single certificate that a client may
// use to connect to a remote server.
type Certificate struct {
	// Name is the name that LXD will use for the cert.
	Name string
	// CertPEM is the PEM-encoded x.509 cert.
	CertPEM []byte
	// KeyPEM is the PEM-encoded x.509 private key.
	KeyPEM []byte
}

// GenerateClientCertificate creates and returns a new certificate for client
// communication with an LXD server.
func GenerateClientCertificate() (*Certificate, error) {
	cert, key, err := shared.GenerateMemCert(true, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewCertificate(cert, key), nil
}

// NewCertificate creates a new Certificate for the given cert and key.
func NewCertificate(certPEM, keyPEM []byte) *Certificate {
	return &Certificate{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
}

// Validate ensures that the cert is valid.
func (c *Certificate) Validate() error {
	if len(c.CertPEM) == 0 {
		return errors.NotValidf("missing cert PEM")
	}
	if len(c.KeyPEM) == 0 {
		return errors.NotValidf("missing key PEM")
	}
	return nil
}

// WriteCertPEM writes the cert's x.509 PEM data to the given writer.
func (c *Certificate) WriteCertPEM(out io.Writer) error {
	_, err := out.Write(c.CertPEM)
	return errors.Trace(err)
}

// WriteKeyPEM writes the key's x.509 PEM data to the given writer.
func (c *Certificate) WriteKeyPEM(out io.Writer) error {
	_, err := out.Write(c.KeyPEM)
	return errors.Trace(err)
}

// Fingerprint returns the cert's LXD fingerprint.
func (c *Certificate) Fingerprint() (string, error) {
	x509Cert, err := c.X509()
	if err != nil {
		return "", errors.Trace(err)
	}
	data := sha256.Sum256(x509Cert.Raw)
	return fmt.Sprintf("%x", data), nil
}

// X509 returns the x.509 certificate.
func (c *Certificate) X509() (*x509.Certificate, error) {
	block, _ := pem.Decode(c.CertPEM)
	if block == nil {
		return nil, errors.Errorf("invalid cert PEM (%d bytes)", len(c.CertPEM))
	}

	x509Cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return x509Cert, nil
}

// AsCreateRequest creates a payload for the LXD API, suitable for posting the
// client certificate to an LXD server.
func (c *Certificate) AsCreateRequest() (api.CertificatesPost, error) {
	block, _ := pem.Decode(c.CertPEM)
	if block == nil {
		return api.CertificatesPost{}, errors.New("failed to decode certificate PEM")
	}

	return api.CertificatesPost{
		Certificate: base64.StdEncoding.EncodeToString(block.Bytes),
		CertificatePut: api.CertificatePut{
			Name: c.Name,
			Type: "client",
		},
	}, nil
}

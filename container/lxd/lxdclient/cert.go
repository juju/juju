// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared"
)

const (
	tempPrefix = "juju-lxd-"

	// See readMyCert() in:
	//  https://github.com/lxc/lxd/blob/master/client.go
	authCertFile = "client.crt"
	authKeyFile  = "client.key"

	pemBlockTypeCert = "CERTIFICATE"
	pemBlockTypeKey  = "RSA PRIVATE KEY"
)

// Certificate holds the information for a single certificate a client
// may use to connect to a remote server.
type Certificate struct {
	// CertPEM is the PEM-encoded x.509 cert.
	CertPEM []byte

	// KeyPEM is the PEM-encoded x.509 private key.
	KeyPEM []byte
}

// NewCertificate creates a new Certificate for the given cert and key.
func NewCertificate(certPEM, keyPEM []byte) *Certificate {
	return &Certificate{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
}

// GenerateCertificate creates a new LXD client certificate. It uses
// the provided function to generate the raw data.
func GenerateCertificate(genCertAndKey func() ([]byte, []byte, error)) (*Certificate, error) {
	certPEM, keyPEM, err := genCertAndKey()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewCertificate(certPEM, keyPEM), nil
}

func genCertAndKey() ([]byte, []byte, error) {
	// See GenCert() in:
	//  https://github.com/lxc/lxd/blob/master/shared/cert.go
	// TODO(ericsnow) Split up GenCert so it is more re-usable.
	tempdir, err := ioutil.TempDir("", tempPrefix)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer os.RemoveAll(tempdir)
	certFile := filepath.Join(tempdir, authCertFile)
	keyFile := filepath.Join(tempdir, authKeyFile)
	if err := shared.GenCert(certFile, keyFile); err != nil {
		return nil, nil, errors.Trace(err)
	}

	certPEM, err := ioutil.ReadFile(certFile)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	keyPEM, err := ioutil.ReadFile(keyFile)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return certPEM, keyPEM, nil
}

// Validate ensures that the cert is valid.
func (cert Certificate) Validate() error {
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
func (cert Certificate) WriteCertPEM(out io.Writer) error {
	if _, err := out.Write(cert.CertPEM); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// WriteKeytPEM writes the key's x.509 PEM data to the given writer.
func (cert Certificate) WriteKeyPEM(out io.Writer) error {
	if _, err := out.Write(cert.KeyPEM); err != nil {
		return errors.Trace(err)
	}
	return nil
}

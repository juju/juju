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

	pemBlockTypeCert = "CERTIFICATE"
	pemBlockTypeKey  = "RSA PRIVATE KEY"
)

// Cert holds the information for a single certificate a client
// may use to connect to a remote server.
type Cert struct {
	// CertPEM is the PEM-encoded x.509 cert.
	CertPEM []byte

	// KeyPEM is the PEM-encoded x.509 private key.
	KeyPEM []byte
}

// NewCert creates a new Cert for the given cert and key.
func NewCert(certPEM, keyPEM []byte) *Cert {
	return &Cert{
		CertPEM: certPEM,
		KeyPEM:  keyPEM,
	}
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

func genCertAndKey() ([]byte, []byte, error) {
	// See GenCert() in:
	//  https://github.com/lxc/lxd/blob/master/shared/cert.go
	// TODO(ericsnow) Split up GenCert so it is more re-usable.
	tempdir, err := ioutil.TempDir("", tempPrefix)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	defer os.RemoveAll(tempdir)
	certFile := filepath.Join(tempdir, configCertFile)
	keyFile := filepath.Join(tempdir, configKeyFile)
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

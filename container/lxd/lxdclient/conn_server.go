// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"crypto/x509"

	"github.com/juju/errors"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

type rawClientServerMethods interface {
	WaitForSuccess(waitURL string) error

	SetServerConfig(key string, value string) (*lxd.Response, error)

	CertificateList() ([]shared.CertInfo, error)
	CertificateAdd(cert *x509.Certificate, name string) error
	CertificateRemove(fingerprint string) error
}

// clientServerMethods implements the Client methods related
// to server operations.
type clientServerMethods struct {
	raw rawServerMethods
}

// SetConfig sets the given value in the server's config.
func (c clientServerMethods) SetConfig(key, value string) error {
	resp, err := c.raw.SetServerConfig(key, value)
	if err != nil {
		return errors.Trace(err)
	}

	if resp.Operation != "" {
		if err := c.raw.WaitForSuccess(resp.Operation); err != nil {
			// TODO(ericsnow) Handle different failures (from the async
			// operation) differently?
			return errors.Trace(err)
		}
	}

	return nil
}

// AddCert adds the given certificate to the server.
func (c clientServerMethods) AddCert(cert Certificate, name string) error {
	x509Cert, err := cert.X509()
	if err != nil {
		return errors.Trace(err)
	}

	if err := c.raw.CertificateAdd(x509Cert, name); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// ListCerts returns the list of cert fingerprints from the server.
func (c clientServerMethods) ListCerts() ([]string, error) {
	certs, err := c.raw.CertificateList()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fingerprints []string
	for _, cert := range certs {
		fingerprints = append(fingerprints, cert.Fingerprint)
	}
	return fingerprints, nil
}

// RemoveCert removes the cert from the server.
func (c clientServerMethods) RemoveCert(cert *Certificate) error {
	fingerprint, err := cert.Fingerprint()
	if err != nil {
		return errors.Trace(err)
	}

	if err := c.raw.CertificateRemove(fingerprint); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RemoveCertByFingerprint removes the cert from the server.
func (c clientServerMethods) RemoveCertByFingerprint(fingerprint string) error {
	if err := c.raw.CertificateRemove(fingerprint); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"crypto/x509"
	"net/http"

	"github.com/juju/errors"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
)

type rawCertClient interface {
	CertificateList() ([]shared.CertInfo, error)
	CertificateAdd(cert *x509.Certificate, name string) error
	CertificateRemove(fingerprint string) error
}

type certClient struct {
	raw rawCertClient
}

// AddCert adds the given certificate to the server.
func (c certClient) AddCert(cert Cert) error {
	x509Cert, err := cert.X509()
	if err != nil {
		return errors.Trace(err)
	}

	if err := c.raw.CertificateAdd(x509Cert, cert.Name); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// RemoveCertByFingerprint removes the cert from the server.
func (c certClient) RemoveCertByFingerprint(fingerprint string) error {
	if err := c.raw.CertificateRemove(fingerprint); err != nil {
		if err == lxd.LXDErrors[http.StatusNotFound] {
			return errors.NotFoundf("certificate with fingerprint %q", fingerprint)
		}
		return errors.Trace(err)
	}
	return nil
}

// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdclient

import (
	"encoding/base64"
	"encoding/pem"

	"github.com/juju/errors"
	"github.com/lxc/lxd/shared/api"
)

type rawCertClient interface {
	GetCertificates() (certificates []api.Certificate, err error)
	CreateCertificate(certificate api.CertificatesPost) (err error)
	DeleteCertificate(fingerprint string) (err error)
}

type certClient struct {
	raw rawCertClient
}

// AddCert adds the given certificate to the server.
func (c certClient) AddCert(cert Cert) error {
	block, _ := pem.Decode(cert.CertPEM)
	if block == nil {
		return errors.New("failed to decode certificate PEM")
	}

	req := api.CertificatesPost{
		Certificate: base64.StdEncoding.EncodeToString(block.Bytes),
		CertificatePut: api.CertificatePut{
			Name: cert.Name,
		},
	}
	if err := c.raw.CreateCertificate(req); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// RemoveCertByFingerprint removes the cert from the server.
func (c certClient) RemoveCertByFingerprint(fingerprint string) error {
	if err := c.raw.DeleteCertificate(fingerprint); err != nil {
		if isLXDNotFound(err) {
			return errors.NotFoundf("certificate with fingerprint %q", fingerprint)
		}
		return errors.Trace(err)
	}
	return nil
}

// CertByFingerprint returns information about a certificate with the
// matching fingerprint. If there is no such certificate, an error
// satisfying errors.IsNotFound is returned.
func (c certClient) CertByFingerprint(fingerprint string) (api.Certificate, error) {
	certs, err := c.raw.GetCertificates()
	if err != nil {
		return api.Certificate{}, errors.Trace(err)
	}
	for _, cert := range certs {
		if cert.Fingerprint == fingerprint {
			return cert, nil
		}
	}
	return api.Certificate{}, errors.NotFoundf("certificate with fingerprint %q", fingerprint)
}

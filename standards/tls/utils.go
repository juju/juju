// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls

import (
	"crypto/x509"
	"encoding/pem"
	"time"

	"github.com/juju/errors"
)

// ParseCert parses the given PEM-formatted X509 certificate.
func ParseCert(certPEM string) (*x509.Certificate, error) {
	certPEMData := []byte(certPEM)
	for len(certPEMData) > 0 {
		var certBlock *pem.Block
		certBlock, certPEMData = pem.Decode(certPEMData)
		if certBlock == nil {
			break
		}
		if certBlock.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(certBlock.Bytes)
			return cert, errors.Trace(err)
		}
	}
	return nil, errors.Errorf("no certificates found")
}

// verifyCertCA ensures that the given certificate is valid with respect
// to the given CA certificate at the given time.
func verifyCertCA(cert, caCert *x509.Certificate, when time.Time) error {
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	opts := x509.VerifyOptions{
		DNSName:     "anyServer",
		Roots:       pool,
		CurrentTime: when,
	}
	if _, err := cert.Verify(opts); err != nil {
		return errors.NewNotValid(err, "cert does not match CA cert")
	}
	return nil
}

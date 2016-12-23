// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cert

import (
	"crypto/x509"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/cert"
)

// Verify verifies that the given server certificate is valid with
// respect to the given CA certificate at the given time.
func Verify(srvCertPEM, caCertPEM string, when time.Time) error {
	caCert, err := cert.ParseCert(caCertPEM)
	if err != nil {
		return errors.Annotate(err, "cannot parse CA certificate")
	}
	srvCert, err := cert.ParseCert(srvCertPEM)
	if err != nil {
		return errors.Annotate(err, "cannot parse server certificate")
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	opts := x509.VerifyOptions{
		DNSName:     "anyServer",
		Roots:       pool,
		CurrentTime: when,
	}
	_, err = srvCert.Verify(opts)
	return err
}

// NewDefaultServer generates a certificate/key pair suitable for use by a server, with an
// expiry time of 10 years.
func NewDefaultServer(caCertPEM, caKeyPEM string, hostnames []string) (certPEM, keyPEM string, err error) {
	// TODO(perrito666) 2016-05-02 lp:1558657
	expiry := time.Now().UTC().AddDate(10, 0, 0)
	return cert.NewLeaf(&cert.Config{
		CommonName:  "*",
		CA:          []byte(caCertPEM),
		CAKey:       []byte(caKeyPEM),
		Expiry:      expiry,
		Hostnames:   hostnames,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
}

// NewServer generates a certificate/key pair suitable for use by a server.
func NewServer(caCertPEM, caKeyPEM string, expiry time.Time, hostnames []string) (certPEM, keyPEM string, err error) {
	return cert.NewLeaf(&cert.Config{
		CommonName:  "*",
		CA:          []byte(caCertPEM),
		CAKey:       []byte(caKeyPEM),
		Expiry:      expiry,
		Hostnames:   hostnames,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
}

// NewCA generates a CA certificate/key pair suitable for signing server
// keys for an environment with the given name.
// wrapper arount utils/cert#NewCA
func NewCA(commonName, UUID string, expiry time.Time) (certPEM, keyPEM string, err error) {
	return cert.NewCA(
		fmt.Sprintf("juju-generated CA for model %q", commonName),
		UUID, expiry, 0)
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"time"

	"github.com/juju/errors"
)

// RawCert holds the x.509-encoded PEM blocks for a TLS cert.
type RawCert struct {
	// CertPEM is the TLS certificate (x.509, PEM-encoded) to use
	// when connecting.
	CertPEM string

	// KeyPEM is the TLS private key (x.509, PEM-encoded) to use
	// when connecting.
	KeyPEM string

	// CACertPEM is the CA cert PEM to use to validate the server cert.
	// It is not necessary if the system already has the CA cert that
	// signed the server's cert.
	CACertPEM string
}

// Cert returns the TLS cert that the raw PEM data represents.
func (raw RawCert) Cert() (tls.Certificate, error) {
	cert, err := tls.X509KeyPair([]byte(raw.CertPEM), []byte(raw.KeyPEM))
	if err != nil {
		return tls.Certificate{}, errors.Trace(err)
	}
	return cert, nil
}

// X509Cert returns the decoded certificate.
func (raw RawCert) X509Cert() (*x509.Certificate, error) {
	cert, err := raw.Cert()
	if err != nil {
		// Fall back to manual extraction.
		cert, err := ParseCert(raw.CertPEM)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return cert, nil
	}
	xCert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, errors.Trace(err)
	}
	return xCert, nil
}

// RSAKey returns the decoded RSA private key that goes with the cert.
func (raw RawCert) RSAKey() (*rsa.PrivateKey, error) {
	cert, err := raw.Cert()
	if err != nil {
		return nil, errors.Trace(err)
	}
	key, ok := cert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.Errorf("private key with unexpected type %T", key)
	}
	return key, nil
}

// CACert is the decoded CA certificate for the cert. It might not
// be set.
func (raw RawCert) CACert() (*x509.Certificate, error) {
	caCert, err := ParseCert(raw.CACertPEM)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return caCert, nil
}

// Validate ensures that the cert is correct.
func (raw RawCert) Validate() error {
	// Check the cert.
	if raw.CertPEM == "" {
		return errors.NewNotValid(nil, "empty CertPEM")
	}
	cert, err := raw.X509Cert()
	if err != nil {
		return errors.NewNotValid(err, "invalid CertPEM")
	}

	// Check the key.
	if raw.KeyPEM == "" {
		return errors.NewNotValid(nil, "empty KeyPEM")
	}
	if _, err := raw.RSAKey(); err != nil {
		err = errors.NewNotValid(err, "bad key or key does not match certificate")
		return errors.Annotate(err, "invalid ClientKey")
	}

	// Check the CA cert.
	if raw.CACertPEM != "" {
		caCert, err := raw.CACert()
		if err != nil {
			return errors.NewNotValid(err, "invalid ClientCACert")
		}

		if err := verifyCertCA(cert, caCert, time.Now()); err != nil {
			return errors.Annotate(err, "invalid ClientCert")
		}
	}

	return nil
}

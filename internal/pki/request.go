// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"crypto/rand"
	"crypto/x509"
	"time"

	"github.com/juju/errors"
)

// CertificateRequestSigner is an interface for signing CSR's under a CA
type CertificateRequestSigner interface {
	SignCSR(*x509.CertificateRequest) (*x509.Certificate, []*x509.Certificate, error)
}

// CertificateRequestSignerFn implements CertificateRequestSigner
type CertificateRequestSignerFn func(*x509.CertificateRequest) (*x509.Certificate, []*x509.Certificate, error)

// DefaultRequestSigner is a default implementation of CertificateRequestSigner
type DefaultRequestSigner struct {
	authority *x509.Certificate
	chain     []*x509.Certificate
	privKey   interface{}
	validity  time.Duration
}

const (
	// DefaultValidityYears is the max age a certificate is signed for using the
	// DefaultRequestSigner
	DefaultValidityYears = 10
)

var (
	// NotBeforeJitter is the amount of time before now that a certificate is
	// valid for
	NotBeforeJitter = time.Minute * -5
)

// NewDefaultRequestSigner creates a new DefaultRequestSigner for the supplied
// CA and key
func NewDefaultRequestSigner(
	authority *x509.Certificate,
	chain []*x509.Certificate,
	privKey interface{},
	validity time.Duration) *DefaultRequestSigner {
	return &DefaultRequestSigner{
		authority: authority,
		chain:     chain,
		privKey:   privKey,
		validity:  validity,
	}
}

// SignCSR implements CertificateRequestSigner SignCSR
func (c CertificateRequestSignerFn) SignCSR(r *x509.CertificateRequest) (*x509.Certificate, []*x509.Certificate, error) {
	return c(r)
}

// SignCSR implements CertificateRequestSigner SignCSR
func (d *DefaultRequestSigner) SignCSR(csr *x509.CertificateRequest) (*x509.Certificate, []*x509.Certificate, error) {
	template := CSRToCertificate(csr)
	if err := assetTagCertificate(template); err != nil {
		return nil, nil, errors.Annotate(err, "failed tagging certificate")
	}

	now := time.Now()
	template.NotBefore = now.Add(NotBeforeJitter)
	var expiry time.Time
	if d.validity > 0 {
		expiry = now.Add(d.validity)
	} else {
		expiry = now.AddDate(DefaultValidityYears, 0, 0)
	}
	template.NotAfter = expiry

	der, err := x509.CreateCertificate(rand.Reader, template, d.authority,
		csr.PublicKey, d.privKey)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	reqCert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return reqCert, d.chain, nil
}

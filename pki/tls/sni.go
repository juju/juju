// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls

import (
	"crypto/tls"

	"github.com/juju/errors"

	"github.com/juju/juju/pki"
)

type Logger interface {
	Debugf(string, ...interface{})
}

// AuthoritySNITLSGetter is responsible for performing the crypto/tls get
// function. It allows support for SNI by selecting a certificate from a
// supplied  authority that best matches the client hellow message.
func AuthoritySNITLSGetter(authority pki.Authority, logger Logger) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		logger.Debugf("received tls client hello for server name %s", hello.ServerName)
		var cert *tls.Certificate
		authority.LeafRange(func(leaf pki.Leaf) bool {
			if err := hello.SupportsCertificate(leaf.TLSCertificate()); err == nil {
				cert = leaf.TLSCertificate()
				return false
			}
			return true
		})

		if cert == nil {
			logger.Debugf("no matching certificate found for tls client hello %s, using default certificate", hello.ServerName)
			leaf, err := authority.LeafForGroup(pki.DefaultLeafGroup)
			if err != nil {
				return nil, errors.New("tls: no certificates configured")
			}
			cert = leaf.TLSCertificate()
		}

		return cert, nil
	}
}

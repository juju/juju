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

		// NOTE: This was added in response to bug lp:1921557. If we get an
		// empty server name here we assume the the connection is being made
		// with an ip address as the host.
		if hello.ServerName == "" {
			logger.Debugf("tls client hello server name is empty. Attempting to provide ip address certificate")
			leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
			if err != nil && !errors.IsNotFound(err) {
				return nil, errors.Annotate(err, "fetching ip address certificate")
			}
			cert = leaf.TLSCertificate()
		} else {
			authority.LeafRange(func(leaf pki.Leaf) bool {
				if err := hello.SupportsCertificate(leaf.TLSCertificate()); err == nil {
					cert = leaf.TLSCertificate()
					return false
				}
				return true
			})
		}

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

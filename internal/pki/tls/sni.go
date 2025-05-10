// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tls

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pki"
)

// AuthoritySNITLSGetter is responsible for performing the crypto/tls get
// function. It allows support for SNI by selecting a certificate from a
// supplied  authority that best matches the client hello message.
func AuthoritySNITLSGetter(authority pki.Authority, logger logger.Logger) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		var cert *tls.Certificate

		// NOTE: This was added in response to bug lp:1921557. If we get an
		// empty server name here we assume the the connection is being made
		// with an ip address as the host.
		if hello.ServerName == "" {
			logger.Debugf(context.Background(), "tls client hello server name is empty. Attempting to provide ip address certificate")
			leaf, err := authority.LeafForGroup(pki.ControllerIPLeafGroup)
			if err == nil {
				cert = leaf.TLSCertificate()
			} else if !errors.Is(err, errors.NotFound) {
				return nil, fmt.Errorf("getting ip address based certificate: %w", err)
			}
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
			logger.Debugf(context.Background(), "no matching certificate found for server name %s, using default certificate", hello.ServerName)
			leaf, err := authority.LeafForGroup(pki.DefaultLeafGroup)
			if err != nil {
				return nil, fmt.Errorf("getting default certificate: %w", err)
			}
			cert = leaf.TLSCertificate()
		}

		return cert, nil
	}
}

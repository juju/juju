// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pki

import (
	"crypto/tls"

	"github.com/juju/errors"
)

// AuthoritySNITLSGetter is responsible for performing the crypto/tls get
// function. It allows support for SNI by selecting a certificate from a
// supplied  authority that best matches the client hellow message.
func AuthoritySNITLSGetter(authority Authority) func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		var cert *tls.Certificate
		authority.LeafRange(func(leaf Leaf) bool {
			if err := hello.SupportsCertificate(leaf.TLSCertificate()); err == nil {
				cert = leaf.TLSCertificate()
				return true
			}
			return false
		})

		if cert == nil {
			leaf, err := authority.LeafForGroup(DefaultLeafGroup)
			if err != nil {
				return nil, errors.New("tls: no certificates configured")
			}
			cert = leaf.TLSCertificate()
		}

		return cert, nil
	}
}

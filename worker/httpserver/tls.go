// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"crypto/tls"
	"net"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/juju/juju/state"
)

// NewTLSConfig returns the TLS configuration for the HTTP server to use
// based on controller configuration stored in the state database.
func NewTLSConfig(st *state.State, getCertificate func() *tls.Certificate) (*tls.Config, error) {
	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newTLSConfig(
		controllerConfig.AutocertDNSName(),
		controllerConfig.AutocertURL(),
		st.AutocertCache(),
		getCertificate,
	), nil
}

func newTLSConfig(
	autocertDNSName, autocertURL string,
	autocertCache autocert.Cache,
	getLocalCertificate func() *tls.Certificate,
) *tls.Config {
	// localCertificate calls getLocalCertificate, returning the result
	// and reporting whether it should be used to serve a connection
	// addressed to the given server name.
	localCertificate := func(serverName string) (*tls.Certificate, bool) {
		cert := getLocalCertificate()
		if net.ParseIP(serverName) != nil {
			// IP address connections always use the local certificate.
			return cert, true
		}
		if !strings.Contains(serverName, ".") {
			// If the server name doesn't contain a period there's no
			// way we can obtain a certificate for it.
			// This applies to the common case where "juju-apiserver" is
			// used as the server name.
			return cert, true
		}
		// Perhaps the server name is explicitly mentioned by the server certificate.
		for _, name := range cert.Leaf.DNSNames {
			if name == serverName {
				return cert, true
			}
		}
		return cert, false
	}

	tlsConfig := utils.SecureTLSConfig()
	if autocertDNSName == "" {
		// No official DNS name, no certificate.
		tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			cert, _ := localCertificate(clientHello.ServerName)
			return cert, nil
		}
		return tlsConfig
	}
	m := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocertCache,
		HostPolicy: autocert.HostWhitelist(autocertDNSName),
	}
	if autocertURL != "" {
		m.Client = &acme.Client{
			DirectoryURL: autocertURL,
		}
	}
	tlsConfig.GetCertificate = func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		logger.Infof("getting certificate for server name %q", clientHello.ServerName)
		// Get the locally created certificate and whether it's appropriate
		// for the SNI name. If not, we'll try to get an acme cert and
		// fall back to the local certificate if that fails.
		cert, shouldUse := localCertificate(clientHello.ServerName)
		if shouldUse {
			return cert, nil
		}
		acmeCert, err := m.GetCertificate(clientHello)
		if err == nil {
			return acmeCert, nil
		}
		logger.Errorf("cannot get autocert certificate for %q: %v", clientHello.ServerName, err)
		return cert, nil
	}
	return tlsConfig
}

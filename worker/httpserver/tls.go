// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"crypto/tls"

	"github.com/juju/errors"
	"github.com/juju/http/v2"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

type SNIGetterFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

func aggregateSNIGetter(getters ...SNIGetterFunc) SNIGetterFunc {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		lastErr := errors.Errorf("unable to find certificate for %s",
			hello.ServerName)
		for _, getter := range getters {
			cert, err := getter(hello)
			if err != nil {
				lastErr = err
				continue
			}
			if cert != nil {
				return cert, nil
			}
		}
		return nil, lastErr
	}
}

// NewTLSConfig returns the TLS configuration for the HTTP server to use
// based on controller configuration stored in the state database.
func NewTLSConfig(certDNSName, certURL string, certCache autocert.Cache, defaultSNI SNIGetterFunc, logger Logger) *tls.Config {
	return newTLSConfig(
		certDNSName,
		certURL,
		certCache,
		defaultSNI,
		logger,
	)
}

func newTLSConfig(
	autocertDNSName, autocertURL string,
	autocertCache autocert.Cache,
	defaultSNI SNIGetterFunc,
	logger Logger,
) *tls.Config {
	tlsConfig := http.SecureTLSConfig()
	if autocertDNSName == "" {
		// No official DNS name, no certificate.
		tlsConfig.GetCertificate = defaultSNI
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
	certLogger := SNIGetterFunc(func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		logger.Infof("getting certificate for server name %q", h.ServerName)
		return nil, nil
	})

	autoCertGetter := SNIGetterFunc(func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := m.GetCertificate(h)
		if err != nil {
			// We only log on debug since this is not really an
			// error.
			logger.Debugf("cannot get autocert certificate for %q: %v",
				h.ServerName, err)
		}
		return c, err
	})

	tlsConfig.GetCertificate = aggregateSNIGetter(
		certLogger, autoCertGetter, defaultSNI)
	tlsConfig.NextProtos = []string{
		"h2", "http/1.1", // Enable HTTP/2.
		acme.ALPNProto, // Enable TLS-ALPN ACME challenges.
	}
	return tlsConfig
}

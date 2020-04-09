// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"crypto/tls"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/juju/juju/pki"
	"github.com/juju/juju/state"
)

type SNIGetter interface {
	GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error)
}

type SNIGetterFn func(*tls.ClientHelloInfo) (*tls.Certificate, error)

func (fn SNIGetterFn) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return fn(hello)
}

func aggregateSNIGetter(getters ...SNIGetter) SNIGetter {
	return SNIGetterFn(func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		lastErr := errors.Errorf("unable to find certificate for %s",
			hello.ServerName)
		for _, getter := range getters {
			cert, err := getter.GetCertificate(hello)
			if err != nil {
				lastErr = err
				continue
			}
			if cert != nil {
				return cert, nil
			}
		}
		return nil, lastErr
	})
}

func authoritySNIGetter(authority pki.Authority) SNIGetter {
	return SNIGetterFn(func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		var cert *tls.Certificate
		if hello.ServerName != "" {
			authority.LeafRange(func(leaf pki.Leaf) bool {
				// TODO when juju is upgraded to go 1.14 we should change this to
				// use ClientHelloInfo.SupportsCertificate

				tlsCert := leaf.TLSCertificate()
				if tlsCert.Leaf == nil {
					return true
				} else if err := tlsCert.Leaf.VerifyHostname(hello.ServerName); err == nil {
					cert = leaf.TLSCertificate()
					return false
				}
				return true
			})
		}

		if cert == nil {
			leaf, err := authority.LeafForGroup(pki.DefaultLeafGroup)
			if err != nil {
				return nil, errors.New("tls: no certificates configured")
			}
			cert = leaf.TLSCertificate()
		}
		return cert, nil
	})
}

// NewTLSConfig returns the TLS configuration for the HTTP server to use
// based on controller configuration stored in the state database.
func NewTLSConfig(st *state.State, defaultSNI SNIGetter) (*tls.Config, error) {
	controllerConfig, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newTLSConfig(
		controllerConfig.AutocertDNSName(),
		controllerConfig.AutocertURL(),
		st.AutocertCache(),
		defaultSNI,
	), nil
}

func newTLSConfig(
	autocertDNSName, autocertURL string,
	autocertCache autocert.Cache,
	defaultSNI SNIGetter,
) *tls.Config {
	tlsConfig := utils.SecureTLSConfig()
	if autocertDNSName == "" {
		// No official DNS name, no certificate.
		tlsConfig.GetCertificate = defaultSNI.GetCertificate
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
	certLogger := SNIGetterFn(func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		logger.Infof("getting certificate for server name %q", h.ServerName)
		return nil, nil
	})

	autoCertGetter := SNIGetterFn(func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := m.GetCertificate(h)
		if err != nil {
			logger.Errorf("cannot get autocert certificate for %q: %v",
				h.ServerName, err)
		}
		return c, err
	})

	tlsConfig.GetCertificate = aggregateSNIGetter(
		certLogger, autoCertGetter, defaultSNI).GetCertificate
	tlsConfig.NextProtos = []string{
		"h2", "http/1.1", // Enable HTTP/2.
		acme.ALPNProto, // Enable TLS-ALPN ACME challenges.
	}
	return tlsConfig
}

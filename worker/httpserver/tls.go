// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/http/v2"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// SNIGetterFunc is a helper function that aids the TLS SNI process by working
// out if a certificate can be provided that satisfies the TLS ClientHello
// message. If no the function has no matches then a error that satisfies
// NotFound should be returned. Alternatively if the function can not help in
// the process of finding a certificate a error that satisfies NotImplemented
// should be returned.
type SNIGetterFunc func(*tls.ClientHelloInfo) (*tls.Certificate, error)

const (
	// errorHostWhiteList is an error to indicate that the requested host name
	// is not in the approved list.
	errorHostWhitelist = errors.ConstError("host not in whitelist")
)

// autocertHostWhitelist is a wrapper around the autocert host policy func. We
// wrap this, so we can return typed errors that Juju can switch on.
func autocertHostWhitelist(hosts ...string) autocert.HostPolicy {
	return func(ctx context.Context, host string) error {
		fn := autocert.HostWhitelist(hosts...)
		err := fn(ctx, host)
		if err != nil {
			err = fmt.Errorf("%w %q: %w", errorHostWhitelist, host, err)
		}
		return err
	}
}

func aggregateSNIGetter(logger Logger, getters ...SNIGetterFunc) SNIGetterFunc {
	return func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		for _, getter := range getters {
			cert, err := getter(hello)
			if err != nil && !errors.Is(err, errors.NotFound) && !errors.Is(err, errors.NotImplemented) {
				logger.Errorf("finding certificate with SNI getter: %v", err)
			}
			if cert != nil {
				return cert, nil
			}
		}
		logger.Warningf("unable to find certificate for server name %q", hello.ServerName)
		return nil, fmt.Errorf("%w certificate for server name %q", errors.NotFound, hello.ServerName)
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

	autoCertManager := autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocertCache,
		HostPolicy: autocertHostWhitelist(autocertDNSName),
	}
	if autocertURL != "" {
		autoCertManager.Client = &acme.Client{
			DirectoryURL: autocertURL,
		}
	}
	certLogger := SNIGetterFunc(func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		logger.Debugf("getting certificate for server name %q", h.ServerName)
		return nil, errors.NotImplemented
	})

	autoCertGetter := SNIGetterFunc(func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// If the ServerName doesn't contain any '.' then it's not a valid dns
		// name and we can't use autocert with it. Bail out early.
		if !strings.Contains(strings.Trim(h.ServerName, "."), ".") {
			return nil, fmt.Errorf(
				"autocert not able to provide certificate for %q%w",
				h.ServerName,
				errors.Hide(errors.NotImplemented),
			)
		}

		c, err := autoCertManager.GetCertificate(h)
		if errors.Is(err, errorHostWhitelist) {
			err = fmt.Errorf(
				"server name %q not in auto cert whitelist%w",
				h.ServerName,
				errors.Hide(errors.NotFound),
			)
		} else if err != nil {
			err = fmt.Errorf("getting autocert certificate for %q: %w",
				h.ServerName,
				err,
			)
		}
		return c, err
	})

	tlsConfig.GetCertificate = aggregateSNIGetter(
		logger,
		certLogger,
		autoCertGetter,
		defaultSNI,
	)
	tlsConfig.NextProtos = []string{
		"h2", "http/1.1", // Enable HTTP/2.
		acme.ALPNProto, // Enable TLS-ALPN ACME challenges.
	}
	return tlsConfig
}

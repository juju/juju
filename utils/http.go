// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sync"

	"launchpad.net/juju-core/cert"
)

var (
	insecureClient      = (*http.Client)(nil)
	insecureClientMutex = sync.Mutex{}
	secureClient        = (*http.Client)(nil)
	secureClientMutex   = sync.Mutex{}
)

// GetNonValidatingHTTPClient returns a *http.Client that does not
// perform verification of TLS server certificates when connecting.
func GetNonValidatingHTTPClient() *http.Client {
	insecureClientMutex.Lock()
	defer insecureClientMutex.Unlock()
	if insecureClient == nil {
		insecureConfig := &tls.Config{InsecureSkipVerify: true}
		insecureTransport := NewHttpTLSTransport(insecureConfig)
		insecureClient = &http.Client{Transport: insecureTransport}
	}
	return insecureClient
}

// GetHTTPClientFromCert returns a *http.Client, which includes the
// given CA certificate in its trusted list, so it can be used to
// connect to an API server
func GetHTTPClientFromCert(caCert []byte) (*http.Client, error) {
	secureClientMutex.Lock()
	defer secureClientMutex.Unlock()
	if secureClient == nil {
		pool := x509.NewCertPool()
		xcert, err := cert.ParseCert(caCert)
		if err != nil {
			return nil, err
		}
		pool.AddCert(xcert)
		secureConfig := &tls.Config{
			RootCAs:    pool,
			ServerName: "anything",
		}
		secureTransport := NewHttpTLSTransport(secureConfig)
		secureClient = &http.Client{Transport: secureTransport}
	}
	return secureClient, nil
}

// NewHttpTLSTransport returns a new http.Transport constructed with the TLS config
// and the necessary parameters for Juju.
func NewHttpTLSTransport(tlsConfig *tls.Config) *http.Transport {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	return &http.Transport{
		TLSClientConfig:   tlsConfig,
		DisableKeepAlives: true,
	}
}

// NewHttpTransport returns a new http.Transport constructed with the necessary
// parameters for Juju.
func NewHttpTransport() *http.Transport {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	return &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DisableKeepAlives: true,
	}
}

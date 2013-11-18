// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"net/http"
	"sync"
)

var insecureClient = (*http.Client)(nil)
var insecureClientMutex = sync.Mutex{}

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

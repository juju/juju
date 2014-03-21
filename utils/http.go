// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"net/http"
	"sync"
)

var clientMutex = sync.Mutex{}
var insecureClient = (*http.Client)(nil)
var secureClient = (*http.Client)(nil)

func init() {
	// We expect GetHTTPClient() to be used normally but in case
	// calls http.Get() are used we set up the transport here too.
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	defaultTransport := http.DefaultTransport.(*http.Transport)
	defaultTransport.DisableKeepAlives = true
	//	registerFileProtocol(defaultTransport)
}

// registerFileProtocol registers support for file:// URLs on the
// given transport.
func registerFileProtocol(transport *http.Transport) {
	transport.RegisterProtocol("file", http.NewFileTransport(http.Dir("/")))
}

// SSLHostnameVerification is used as a switch for when a given provider might
// use self-signed credentials and we should not try to verify the hostname on
// the TLS/SSL certificates
type SSLHostnameVerification bool

const (
	// VerifySSLHostnames ensures we verify the hostname on the certificate
	// matches the host we are connecting and is signed
	VerifySSLHostnames = SSLHostnameVerification(true)
	// NoVerifySSLHostnames informs us to skip verifying the hostname
	// matches a valid certificate
	NoVerifySSLHostnames = SSLHostnameVerification(false)
)

// GetHTTPClient returns either a standard http client or
// non validating client depending on the value of verify.
func GetHTTPClient(verify SSLHostnameVerification) *http.Client {
	if verify == VerifySSLHostnames {
		return GetValidatingHTTPClient()
	}
	return GetNonValidatingHTTPClient()
}

func GetValidatingHTTPClient() *http.Client {
	logger.Infof("hostname SSL verification enabled")
	clientMutex.Lock()
	defer clientMutex.Unlock()
	if secureClient == nil {
		secureTransport := NewHttpTransport()
		secureClient = &http.Client{Transport: secureTransport}
	}
	return secureClient
}

func GetNonValidatingHTTPClient() *http.Client {
	logger.Infof("hostname SSL verification disabled")
	clientMutex.Lock()
	defer clientMutex.Unlock()
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
	transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		DisableKeepAlives: true,
	}
	registerFileProtocol(transport)
	return transport
}

// NewHttpTransport returns a new http.Transport constructed with the necessary
// parameters for Juju.
func NewHttpTransport() *http.Transport {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	transport := &http.Transport{
		Proxy:             http.ProxyFromEnvironment,
		DisableKeepAlives: true,
	}
	registerFileProtocol(transport)
	return transport
}

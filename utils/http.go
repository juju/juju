// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"sync"
)

var insecureClient = (*http.Client)(nil)
var insecureClientMutex = sync.Mutex{}

func init() {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	defaultTransport := http.DefaultTransport.(*http.Transport)
	defaultTransport.DisableKeepAlives = true
	defaultTransport.Dial = dial
	registerFileProtocol(defaultTransport)
}

// registerFileProtocol registers support for file:// URLs on the given transport.
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

// GetValidatingHTTPClient returns a new http.Client that
// verifies the server's certificate chain and hostname.
func GetValidatingHTTPClient() *http.Client {
	logger.Infof("hostname SSL verification enabled")
	return http.DefaultClient
}

// GetNonValidatingHTTPClient returns a new http.Client that
// does not verify the server's certificate chain and hostname.
func GetNonValidatingHTTPClient() *http.Client {
	logger.Infof("hostname SSL verification disabled")
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
	transport := &http.Transport{
		TLSClientConfig:   tlsConfig,
		DisableKeepAlives: true,
		Dial:              dial,
	}
	registerFileProtocol(transport)
	return transport
}

// BasicAuthHeader creates a header that contains just the "Authorization"
// entry.  The implementation was originally taked from net/http but this is
// needed externally from the http request object in order to use this with
// our websockets. See 2 (end of page 4) http://www.ietf.org/rfc/rfc2617.txt
// "To receive authorization, the client sends the userid and password,
// separated by a single colon (":") character, within a base64 encoded string
// in the credentials."
func BasicAuthHeader(username, password string) http.Header {
	auth := username + ":" + password
	encoded := "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
	return http.Header{
		"Authorization": {encoded},
	}
}

// OutgoingAccessAllowed determines whether connections other than
// localhost can be dialled.
var OutgoingAccessAllowed = true

// Override for tests.
var netDial = net.Dial

func dial(network, addr string) (net.Conn, error) {
	if !OutgoingAccessAllowed {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return netDial(network, addr)
		}
		if host != "localhost" {
			ip := net.ParseIP(host)
			if ip == nil || !ip.IsLoopback() {
				return nil, fmt.Errorf("access to address %q not allowed", addr)
			}
		}
	}
	return netDial(network, addr)
}

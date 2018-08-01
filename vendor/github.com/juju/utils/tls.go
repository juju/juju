// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package utils

import (
	"crypto/tls"
	"net/http"
	"time"
)

// NewHttpTLSTransport returns a new http.Transport constructed with the TLS config
// and the necessary parameters for Juju.
func NewHttpTLSTransport(tlsConfig *tls.Config) *http.Transport {
	// See https://code.google.com/p/go/issues/detail?id=4677
	// We need to force the connection to close each time so that we don't
	// hit the above Go bug.
	transport := &http.Transport{
		Proxy:               http.ProxyFromEnvironment,
		TLSClientConfig:     tlsConfig,
		DisableKeepAlives:   true,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	installHTTPDialShim(transport)
	registerFileProtocol(transport)
	return transport
}

// knownGoodCipherSuites contains the list of secure cipher suites to use
// with tls.Config. This list matches those that Go 1.6 implements from
// https://wiki.mozilla.org/Security/Server_Side_TLS#Recommended_configurations.
//
// https://tools.ietf.org/html/rfc7525#section-4.2 excludes RSA exchange completely
// so we could be more strict if all our clients will support
// TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256/384. Unfortunately Go's crypto library
// is limited and doesn't support DHE-RSA-AES256-GCM-SHA384 and
// DHE-RSA-AES256-SHA256, which are part of the recommended set.
//
// Unfortunately we can't drop the RSA algorithms because our servers aren't
// generating ECDHE keys.
var knownGoodCipherSuites = []uint16{
	// These are technically useless for Juju, since we use an RSA certificate,
	// but they also don't hurt anything, and supporting an ECDSA certificate
	// could be useful in the future.
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,

	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,

	// Windows doesn't support GCM currently, so we need these for RSA support.
	tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
	tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,

	// We need this so that we have at least one suite in common
	// with the default gnutls installed for precise and trusty.
	tls.TLS_RSA_WITH_AES_256_CBC_SHA,
}

// SecureTLSConfig returns a tls.Config that conforms to Juju's security
// standards, so as to avoid known security vulnerabilities in certain
// configurations.
//
// Currently it excludes RC4 implementations from the available ciphersuites,
// requires ciphersuites that provide forward secrecy, and sets the minimum TLS
// version to 1.2.
func SecureTLSConfig() *tls.Config {
	return &tls.Config{
		CipherSuites: knownGoodCipherSuites,
		MinVersion:   tls.VersionTLS12,
	}
}

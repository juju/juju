// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logging

// LokiConfig holds the controller-wide Loki push API configuration.
type LokiConfig struct {
	// Endpoint is the Loki push API URL.
	Endpoint string
	// CACertificate is the CA certificate used to validate the Loki endpoint.
	CACertificate string
	// InsecureSkipVerify controls whether a client verifies the
	// server's certificate chain and host name. If InsecureSkipVerify
	// is true, TLS accepts any certificate returned by the server and
	// any host name in that certificate. This must be pointer so we
	// can distinguish unset from bool values.
	InsecureSkipVerify *bool
}

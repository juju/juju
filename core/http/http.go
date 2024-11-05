// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"context"
	"net/http"

	"github.com/juju/errors"
)

const (
	// ErrHTTPClientDying is used to indicate to *third parties* that the
	// http client worker is dying, instead of catacomb.ErrDying, which is
	// unsuitable for propagating inter-worker.
	// This error indicates to consuming workers that their dependency has
	// become unmet and a restart by the dependency engine is imminent.
	ErrHTTPClientDying = errors.ConstError("http client worker is dying")
)

// HTTPClientGetter is the interface that is used to get a http clients.
type HTTPClientGetter interface {
	GetHTTPClient(context.Context, string) (HTTPClient, error)
}

// HTTPClient is the interface that is used to do http requests.
type HTTPClient interface {
	// Do sends an HTTP request and returns an HTTP response. The client will
	// follow policy (such as redirects, cookies, auth) as configured on the
	// client.
	Do(*http.Request) (*http.Response, error)
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apihttp

import (
	"net/http"
)

// Endpoint describes a single HTTP endpoint.
type Endpoint struct {
	// Pattern is the pattern to match for the endpoint.
	Pattern string

	// Method is the HTTP method to use (e.g. GET).
	Method string

	// Handler is the HTTP handler to use.
	Handler http.Handler
}

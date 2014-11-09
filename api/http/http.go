// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"net/http"
)

// HTTPClient is an API-specific HTTP client.
type HTTPClient interface {
	// Do sends the HTTP request, returning the subsequent response.
	Do(req *http.Request) (*http.Response, error)
}

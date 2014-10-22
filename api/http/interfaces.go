// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"net/http"
)

// Request is a wrapper around an HTTP request that has been
// prepared for use in API HTTP calls.
type Request struct {
	http.Request
}

// HTTPClient is an API-specific HTTP client.
type HTTPClient interface {
	// Do sends the HTTP request, returning the subsequent response.
	Do(req *http.Request) (*http.Response, error)
}

// Client exposes direct HTTP request functionality for the juju state
// API.  This is significant for upload and download of files, which the
// websockets-based RPC does not support.
type Client interface {
	// NewHTTPRequest returns a new API-specific HTTP request.  Callers
	// should finish the request (setting headers, body) before sending.
	NewHTTPRequest(method, path string) (*Request, error)
	// SendHTTPRequest returns the HTTP response from the API server.
	// The caller is then responsible for handling the response.
	SendHTTPRequest(req *Request) (*http.Response, error)
}

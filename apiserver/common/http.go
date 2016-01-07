// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"net/http"

	"github.com/juju/juju/state"
)

// NewHTTPHandlerArgs holds the args to the func in the NewHTTPHandler
// field of HTTPHandlerSpec.
type NewHTTPHandlerArgs struct {
	// Connect is the function that is used to connect to Juju's state
	// for the given HTTP request.
	Connect func(*http.Request) (*state.State, error)
}

// HTTPHandlerConstraints describes the conditions under which
// a handler is valid.
type HTTPHandlerConstraints struct {
	// AuthKind defines the kind of authenticated "user" that the
	// handler supports. This correlates directly to entities, as
	// identified by tag kinds (e.g. names.UserTagKind). The empty
	// string indicates support for unauthenticated requests.
	AuthKind string

	// StrictValidation is the value that will be used for the handler's
	// httpContext (see apiserver/httpcontext.go).
	StrictValidation bool

	// StateServerEnvOnly is the value that will be used for the handler's
	// httpContext (see apiserver/httpcontext.go).
	StateServerEnvOnly bool
}

// HTTPHandlerSpec defines an HTTP handler for a specific endpoint
// on Juju's HTTP server. Such endpoints facilitate behavior that is
// not supported through normal (websocket) RPC. That includes file
// transfer.
type HTTPHandlerSpec struct {
	// Constraints holds the handler's constraints.
	Constraints HTTPHandlerConstraints

	// NewHTTPHandler returns a new HTTP handler for the given args.
	NewHTTPHandler func(NewHTTPHandlerArgs) http.Handler
}

// HTTPEndpoint describes a single HTTP endpoint.
type HTTPEndpoint struct {
	// Pattern is the URL path pattern for the endpoint.
	// (See use of pat.PatternServeMux in apiserver/apiserver.go.)
	Pattern string

	// Method is the HTTP method (e.g. GET, PUT) for the endpoint.
	Method string

	// Handler is the HTTP handler to use for the endpoint.
	Handler http.Handler
}

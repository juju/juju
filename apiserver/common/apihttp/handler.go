// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apihttp

import (
	"net/http"

	"github.com/juju/juju/state"
)

// NewHandlerArgs holds the args to the func in the NewHandler
// field of HandlerSpec.
type NewHandlerArgs struct {
	// Connect is the function that is used to connect to Juju's state
	// for the given HTTP request. It is the caller's responsibility
	// to call the release function returned. If the error arg is nil
	// the release function will always be a valid function.
	Connect func(*http.Request) (*state.State, func(), state.Entity, error)
}

// HandlerConstraints describes conditions under which a handler
// may operate.
type HandlerConstraints struct {
	// AuthKinds defines the kinds of authenticated "user" that the
	// handler supports. This correlates directly to entities, as
	// identified by tag kinds (e.g. names.UserTagKind). An empty list
	// will block all authentication.
	AuthKinds []string

	// StrictValidation is the value that will be used for the handler's
	// httpContext (see apiserver/httpcontext.go).
	StrictValidation bool

	// ControllerModelOnly is the value that will be used for the handler's
	// httpContext (see apiserver/httpcontext.go).
	ControllerModelOnly bool
}

// HandlerSpec defines an HTTP handler for a specific endpoint
// on Juju's HTTP server. Such endpoints facilitate behavior that is
// not supported through normal (websocket) RPC. That includes file
// transfer.
type HandlerSpec struct {
	// Constraints are the handler's constraints.
	Constraints HandlerConstraints

	// NewHandler returns a new HTTP handler for the given args.
	// The function is idempotent--if given the same args, it will
	// produce an equivalent handler each time.
	NewHandler func(NewHandlerArgs) http.Handler
}

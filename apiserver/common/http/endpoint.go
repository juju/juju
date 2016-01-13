// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
)

// EndpointSpec describes potential HTTP endpoints.
type EndpointSpec struct {
	// pattern is the URL path pattern to match for this endpoint.
	// (See use of pat.PatternServeMux in apiserver/apiserver.go.)
	pattern string

	// methodHandlers associates each supported HTTP method (e.g. GET, PUT)
	// with the handler spec that supports it.
	methodHandlers map[string]HandlerSpec

	// orderedMethods tracks the order in which methods were added.
	orderedMethods []string

	// defaultHandler is the handler spec to use for unrecognized HTTP methods.
	defaultHandler *HandlerSpec
}

// NewEndpointSpec composes a new HTTP endpoint spec for the given
// URL path pattern and handler spec. If any methods are provided, the
// handler spec is associated with each of them for the endpoint.
// Otherwise the handler spec is used as the default for all HTTP
// methods.
func NewEndpointSpec(pattern string, hSpec HandlerSpec, methods ...string) (EndpointSpec, error) {
	pattern = NormalizePath(pattern)

	spec := EndpointSpec{
		pattern:        pattern,
		methodHandlers: make(map[string]HandlerSpec),
	}

	if len(methods) == 0 {
		// Short-circuit for the "all available" case.
		spec.defaultHandler = &hSpec
		return spec, nil
	}

	for _, method := range methods {
		if err := spec.Add(method, hSpec); err != nil {
			return spec, errors.Trace(err)
		}
	}
	return spec, nil
}

// Add adds the handler spec to the endpoint spec for the given HTTP
// method. If the method already has a handler then the call will fail.
func (spec *EndpointSpec) Add(method string, hSpec HandlerSpec) error {
	if method == "" {
		return errors.NewNotValid(nil, "missing method")
	}
	method = strings.ToUpper(method)
	if _, ok := spec.methodHandlers[method]; ok {
		msg := fmt.Sprintf("HTTP method %q already added", method)
		return errors.NewAlreadyExists(nil, msg)
	}
	// TODO(ericsnow) Fail if not one of the "supported" HTTP methods?
	spec.methodHandlers[method] = hSpec
	spec.orderedMethods = append(spec.orderedMethods, method)
	return nil
}

// Pattern returns the spec's URL path pattern.
func (spec EndpointSpec) Pattern() string {
	return spec.pattern
}

// Methods returns the set of methods that have handlers
// for this endpoint.
func (spec EndpointSpec) Methods() []string {
	return append([]string{}, spec.orderedMethods...) // a copy
}

// Default returns a copy of the default handler spec, if there is one.
// If not then false is returned.
func (spec EndpointSpec) Default() (HandlerSpec, bool) {
	if spec.defaultHandler == nil {
		return HandlerSpec{}, false
	}
	return *spec.defaultHandler, true
}

// Resolve returns the HTTP handler spec for the given HTTP method.
// The returned spec is guaranteed to have a valid NewHandler.
// In the cases that the HTTP method is not supported, the provided
// "unhandled" handler will be returned from NewHandler.
func (spec EndpointSpec) Resolve(method string, unhandled http.Handler) HandlerSpec {
	hSpec := spec.resolve(method)

	// Handle the nil NewHandler/handler cases, treating them
	// as "unhandled".
	if unhandled == nil {
		unhandled = unsupportedMethodHandler()
	}
	newHandler := hSpec.NewHandler
	hSpec.NewHandler = func(args NewHandlerArgs) http.Handler {
		if newHandler == nil {
			return unhandled
		}
		handler := newHandler(args)
		if handler == nil {
			return unhandled
		}
		return handler
	}

	return hSpec
}

// resolve returns the handler spec for the given HTTP method. If no
// handler has been added for the method then the default (method "")
// is returned. If no default has been set then a zero-value spec is
// returned.
func (spec EndpointSpec) resolve(method string) HandlerSpec {
	if method != "" {
		if hSpec, ok := spec.methodHandlers[method]; ok {
			return hSpec
		}
		// Otherwise fall back to the default, if any.
	}

	if spec.defaultHandler != nil {
		return *spec.defaultHandler
	}

	// No match and no default, so return an "unhandled" handler spec.
	return HandlerSpec{}
}

// Endpoint describes a single HTTP endpoint.
type Endpoint struct {
	// Pattern is the URL path pattern to match for this endpoint.
	// (See use of pat.PatternServeMux in apiserver/apiserver.go.)
	Pattern string

	// Method is the HTTP method for the endpoint (e.g. GET, PUT).
	// An empty string means "all supported".
	Method string

	// Handler is the HTTP handler for the endpoint.
	Handler http.Handler
}

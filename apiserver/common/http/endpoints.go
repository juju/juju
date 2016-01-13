// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
)

// Endpoints holds an ordered set of endpoint definitions.
// The order of insertion is preserved (for now).
type Endpoints struct {
	// patternSpecs is the set of endpoint specs, mapping patterns to specs.
	patternSpecs map[string]EndpointSpec

	// orderedPatterns holds the flat order of endpoint patterns.
	orderedPatterns []string

	// UnsupportedMethodHandler is the HTTP handler that is used
	// for unsupported HTTP methods.
	UnsupportedMethodHandler http.Handler

	// TODO(ericsnow) Support an ordering function field?
}

// NewEndpoints returns a newly initialized Endpoints.
func NewEndpoints() Endpoints {
	return Endpoints{
		patternSpecs:             make(map[string]EndpointSpec),
		UnsupportedMethodHandler: unsupportedMethodHandler(),
	}
}

// Add adds an endpoint spec to the set for the provided pattern
// and handler. Order is preserved. A pattern collision results
// in a failure.
func (hes *Endpoints) Add(spec EndpointSpec) error {
	if spec.pattern == "" {
		return errors.NewNotValid(nil, "spec missing pattern")
	}
	if _, ok := hes.patternSpecs[spec.pattern]; ok {
		// TODO(ericsnow) Merge if strictly different HTTP Methods.
		msg := fmt.Sprintf("endpoint %q already registered", spec.pattern)
		return errors.NewAlreadyExists(nil, msg)
	}
	hes.patternSpecs[spec.pattern] = spec

	// TODO(ericsnow) Order by the flattened hierarchy of URL path
	// elements, alphabetical with deepest elements first. Depth matters
	// because the first pattern match is the one used.
	hes.orderedPatterns = append(hes.orderedPatterns, spec.pattern)
	return nil
}

// Specs returns the spec for each endpoint, in order.
func (hes *Endpoints) Specs() []EndpointSpec {
	var specs []EndpointSpec
	for _, pattern := range hes.orderedPatterns {
		spec := hes.patternSpecs[pattern]
		specs = append(specs, spec)
	}
	return specs
}

// Resolve builds the list of endpoints, preserving order.
func (hes Endpoints) Resolve(newArgs func(HandlerConstraints) NewHandlerArgs) []Endpoint {
	var endpoints []Endpoint
	for _, pattern := range hes.orderedPatterns {
		spec := hes.patternSpecs[pattern]
		// Note that the spec's default handler (if any) is not used.
		for _, method := range spec.Methods() {
			hSpec := spec.Resolve(method, hes.UnsupportedMethodHandler)
			args := newArgs(hSpec.Constraints)
			handler := hSpec.NewHandler(args)

			endpoints = append(endpoints, Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}
	return endpoints
}

// ResolveForMethods builds the list of endpoints, preserving order.
func (hes Endpoints) ResolveForMethods(methods []string, newArgs func(HandlerConstraints) NewHandlerArgs) []Endpoint {
	var endpoints []Endpoint
	for _, pattern := range hes.orderedPatterns {
		spec := hes.patternSpecs[pattern]
		for _, method := range methods {
			hSpec := spec.Resolve(method, hes.UnsupportedMethodHandler)
			args := newArgs(hSpec.Constraints)
			handler := hSpec.NewHandler(args)

			endpoints = append(endpoints, Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}
	return endpoints
}

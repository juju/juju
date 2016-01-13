// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Move this to its own package or even to another repo?

package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.common.http")

// NewHandlerArgs holds the args to the func in the NewHandler
// field of HandlerSpec.
type NewHandlerArgs struct {

	// TODO(ericsnow) Return an interface instead of state.State?

	// Connect is the function that is used to connect to Juju's state
	// for the given HTTP request.
	Connect func(*http.Request) (*state.State, state.Entity, error)

	// TODO(ericsnow) Other fields:
	//DataDir string
	//LogDir string
	//Stop <-chan struct{}
	//State EnvConfigger
}

// HandlerConstraints describes conditions under which a handler
// may operate.
type HandlerConstraints struct {
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

// unsupportedMethodHandler returns an HTTP handler that returns
// an API error response indicating that the method is not supported.
func unsupportedMethodHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := errors.MethodNotAllowedf("unsupported method: %q", req.Method)
		status := http.StatusMethodNotAllowed
		sendStatusAndJSON(w, status, &params.Error{
			Message: err.Error(),
			Code:    params.CodeMethodNotAllowed,
		})
	})
}

// EndpointSpec describes potential HTTP endpoints.
type EndpointSpec struct {
	// pattern is the URL path pattern to match for this endpoint.
	// (See use of pat.PatternServeMux in apiserver/apiserver.go.)
	pattern string

	// methodHandlers associates each supported HTTP method (e.g. GET, PUT)
	// with the handler spec that supports it.
	methodHandlers map[string]HandlerSpec
}

// NormalizePath cleans up the provided URL path and makes it absolute.
func NormalizePath(pth string) string {
	return path.Clean(path.Join("/", pth))
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
		spec.methodHandlers[""] = hSpec
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
	return nil
}

// Pattern returns the spec's URL path pattern.
func (spec EndpointSpec) Pattern() string {
	return spec.pattern
}

// Methods returns the set of  methods that have handlers
// for this endpoint.
func (spec EndpointSpec) Methods() []string {
	var methods []string
	for method := range spec.methodHandlers {
		methods = append(methods, method)
	}
	return methods
}

// Resolve returns the HTTP handler spec for the given HTTP method.
// The returned spec is guaranteed to have a valid NewHandler.
// In the cases that the HTTP method is not supported, the provided
// "unhandled" handler will be returned from NewHandler.
func (spec EndpointSpec) Resolve(method string, unhandled http.Handler) HandlerSpec {
	if unhandled == nil {
		unhandled = unsupportedMethodHandler()
	}
	hSpec := spec.resolve(method)

	// Handle the nil NewHandler/handler cases, treating them
	// as "unhandled".
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
	if hSpec, ok := spec.methodHandlers[method]; ok {
		return hSpec
	}
	if method != "" {
		// Fall back to the default, if any.
		return spec.resolve("")
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

// Endpoints holds an ordered set of endpoint definitions.
// The order of insertion is preserved (for now).
type Endpoints struct {
	// patternSpecs is the set of endpoint specs, mapping patterns to specs.
	patternSpecs map[string]EndpointSpec

	// orderedPatterns holds the flat order of endpoint patterns.
	orderedPatterns []string

	// unsupportedMethodHandler
	unsupportedMethodHandler http.Handler

	// TODO(ericsnow) Support an ordering function field?
}

// NewEndpoints returns a newly initialized Endpoints.
func NewEndpoints() Endpoints {
	return Endpoints{
		patternSpecs:             make(map[string]EndpointSpec),
		unsupportedMethodHandler: unsupportedMethodHandler(),
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
		for _, method := range spec.Methods() {
			if method == "" {
				// The default is discarded.
				continue
			}

			hSpec := spec.Resolve(method, hes.unsupportedMethodHandler)
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
			hSpec := spec.Resolve(method, hes.unsupportedMethodHandler)
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

// TODO(ericsnow) This is copied from apiserver/httpcontext.go...

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) {
	body, err := json.Marshal(response)
	if err != nil {
		logger.Errorf("cannot marshal JSON result %#v: %v", response, err)
		return
	}

	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	w.Write(body)
}

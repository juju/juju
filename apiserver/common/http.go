// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// NewHTTPHandlerArgs holds the args to the func in the NewHTTPHandler
// field of HTTPHandlerSpec.
type NewHTTPHandlerArgs struct {

	// TODO(ericsnow) Return an interface instead of state.State?

	// Connect is the function that is used to connect to Juju's state
	// for the given HTTP request.
	Connect func(*http.Request) (*state.State, error)

	// TODO(ericsnow) Other fields:
	//DataDir string
	//LogDir string
	//Stop <-chan struct{}
	//State EnvConfigger
}

// HTTPHandlerConstraints describes conditions under which a handler
// may operate.
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
	// Constraints are the handler's constraints.
	Constraints HTTPHandlerConstraints

	// NewHTTPHandler returns a new HTTP handler for the given args.
	// The function is idempotent--if given the same args, it will
	// produce an equivalent handler each time.
	NewHTTPHandler func(NewHTTPHandlerArgs) http.Handler
}

// unsupportedHTTPMethodHandler returns an HTTP handler that returns
// an API error response indicating that the method is not supported.
func unsupportedHTTPMethodHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		err := errors.MethodNotAllowedf("unsupported method: %q", req.Method)
		err, status := ServerErrorAndStatus(err)
		sendStatusAndJSON(w, status, err)
	})
}

// HTTPEndpointSpec describes potential HTTP endpoints.
type HTTPEndpointSpec struct {
	// pattern is the URL path pattern to match for this endpoint.
	// (See use of pat.PatternServeMux in apiserver/apiserver.go.)
	pattern string

	// methodHandlers associates each supported HTTP method (e.g. GET, PUT)
	// with the handler spec that supports it.
	methodHandlers map[string]HTTPHandlerSpec
}

// NewHTTPEndpointSpec composes a new HTTP endpoint spec for the given
// URL path pattern and handler spec. If any methods are provided, the
// handler spec is associated with each of them for the endpoint.
// Otherwise the handler spec is used as the default for all HTTP
// methods.
func NewHTTPEndpointSpec(pattern string, hSpec HTTPHandlerSpec, methods ...string) (HTTPEndpointSpec, error) {
	pattern = path.Clean(path.Join("/", pattern))

	spec := HTTPEndpointSpec{
		pattern:        pattern,
		methodHandlers: make(map[string]HTTPHandlerSpec),
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
func (spec *HTTPEndpointSpec) Add(method string, hSpec HTTPHandlerSpec) error {
	if method == "" {
		return errors.NewNotValid(nil, "missing method")
	}
	method = strings.ToUpper(method)
	if _, ok := spec.methodHandlers[method]; ok {
		msg := fmt.Sprintf("HTTP method %q already added")
		return errors.NewAlreadyExists(nil, msg)
	}
	// TODO(ericsnow) Fail if not one of the "supported" HTTP methods?
	spec.methodHandlers[method] = hSpec
	return nil
}

// methods returns the set of HTTP methods that have handlers
// for this endpoint.
func (spec HTTPEndpointSpec) methods() []string {
	var methods []string
	for method := range spec.methodHandlers {
		methods = append(methods, method)
	}
	return methods
}

// resolve returns the HTTP handler spec for the given HTTP method.
func (spec HTTPEndpointSpec) resolve(method string, unhandled http.Handler) HTTPHandlerSpec {
	if hSpec, ok := spec.methodHandlers[method]; ok {
		return hSpec
	}
	if method != "" {
		return spec.resolve("", unhandled)
	}
	if unhandled == nil {
		unhandled = unsupportedHTTPMethodHandler()
	}
	return HTTPHandlerSpec{
		NewHTTPHandler: func(NewHTTPHandlerArgs) http.Handler {
			return unhandled
		},
	}
}

// HTTPEndpoint describes a single HTTP endpoint.
type HTTPEndpoint struct {
	// Pattern is the URL path pattern to match for this endpoint.
	// (See use of pat.PatternServeMux in apiserver/apiserver.go.)
	Pattern string

	// Method is the HTTP method for the endpoint (e.g. GET, PUT).
	// An empty string means "all supported".
	Method string

	// handlerSpecs is the list of handler specs for the endpoint.
	// An endpoint may have more than one handler (e.g. one for GET
	// and another for SET).
	Handler http.Handler
}

// HTTPEndpoints holds an ordered set of endpoint definitions.
// The order of insertion is preserved (for now).
type HTTPEndpoints struct {
	// specs is the set of endpoint specs, mapping patterns to specs.
	specs map[string]HTTPEndpointSpec

	// ordered holds the flat order of endpoint patterns.
	ordered []string

	// unsupportedMethodHandler
	unsupportedMethodHandler http.Handler

	// TODO(ericsnow) Support an ordering function field?
}

// NewHTTPEndpoints returns a newly initialized HTTPEndpoints.
func NewHTTPEndpoints() HTTPEndpoints {
	return HTTPEndpoints{
		specs: make(map[string]HTTPEndpointSpec),
		unsupportedMethodHandler: unsupportedHTTPMethodHandler(),
	}
}

// add adds an endpoint spec to the set for the provided pattern
// and handler. Order is preserved. A pattern collision results
// in a failure.
func (hes *HTTPEndpoints) add(spec HTTPEndpointSpec) error {
	if spec.pattern == "" {
		return errors.NewNotValid(nil, "spec missing pattern")
	}
	if _, ok := hes.specs[spec.pattern]; ok {
		// TODO(ericsnow) Merge if strictly different HTTP Methods.
		msg := fmt.Sprintf("endpoint %q already registered")
		return errors.NewAlreadyExists(nil, msg)
	}
	hes.specs[spec.pattern] = spec

	// TODO(ericsnow) Order by the flattened hierarchy of URL path
	// elements, alphabetical with deepest elements first. Depth matters
	// because the first pattern match is the one used.
	hes.ordered = append(hes.ordered, spec.pattern)
	return nil
}

// resolve builds the list of endpoints, preserving order.
func (hes HTTPEndpoints) resolve(newArgs func(HTTPHandlerConstraints) NewHTTPHandlerArgs) []HTTPEndpoint {
	var endpoints []HTTPEndpoint
	for _, pattern := range hes.ordered {
		spec := hes.specs[pattern]
		for _, method := range spec.methods() {
			if method == "" {
				// The default is discarded.
				continue
			}

			hSpec := spec.resolve(method, hes.unsupportedMethodHandler)
			args := newArgs(hSpec.Constraints)
			handler := hSpec.NewHTTPHandler(args)

			endpoints = append(endpoints, HTTPEndpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}
	return endpoints
}

// resolveForMethods builds the list of endpoints, preserving order.
func (hes HTTPEndpoints) resolveForMethods(methods []string, newArgs func(HTTPHandlerConstraints) NewHTTPHandlerArgs) []HTTPEndpoint {
	var endpoints []HTTPEndpoint
	for _, pattern := range hes.ordered {
		spec := hes.specs[pattern]
		for _, method := range methods {
			hSpec := spec.resolve(method, hes.unsupportedMethodHandler)
			args := newArgs(hSpec.Constraints)
			handler := hSpec.NewHTTPHandler(args)

			endpoints = append(endpoints, HTTPEndpoint{
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

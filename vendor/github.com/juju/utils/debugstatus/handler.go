// Copyright 2015 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package debugstatus

import (
	"net/http"

	"golang.org/x/net/trace"
	"gopkg.in/errgo.v1"

	pprof "github.com/juju/httpprof"
	"golang.org/x/net/context"
	"gopkg.in/httprequest.v1"
)

// Version describes the current version of the code being run.
type Version struct {
	GitCommit string
	Version   string
}

// Handler implements a type that can be used with httprequest.Handlers
// to serve a standard set of /debug endpoints, including
// the version of the system, its current health status
// the runtime profiling information.
type Handler struct {
	// Check will be called to obtain the current health of the
	// system. It should return a map as returned from the
	// Check function. If this is nil, an empty result will
	// always be returned from /debug/status.
	Check func(context.Context) map[string]CheckResult

	// Version should hold the current version
	// of the binary running the server, served
	// from the /debug/info endpoint.
	Version Version

	// CheckPprofAllowed will be used to check whether the
	// given pprof request should be allowed.
	// It should return an error if not, which will not be masked.
	// If this is nil, no access will be allowed to any
	// of the endpoints under /debug/pprof - the
	// error returned will be ErrNoPprofConfigured.
	CheckPprofAllowed func(req *http.Request) error

	// CheckTraceAllowed will be used to check whether the given
	// trace request should be allowed. It should return an error if
	// not, which will not be masked. If this is nil, no access will
	// be allowed to either /debug/events or /debug/requests - the
	// error returned will be ErrNoTraceConfigured. If access is
	// allowed, the sensitive value specifies whether sensitive trace
	// events will be shown.
	CheckTraceAllowed func(req *http.Request) (sensitive bool, err error)
}

// DebugStatusRequest describes the /debug/status endpoint.
type DebugStatusRequest struct {
	httprequest.Route `httprequest:"GET /debug/status"`
}

// DebugStatus returns the current status of the server.
func (h *Handler) DebugStatus(p httprequest.Params, _ *DebugStatusRequest) (map[string]CheckResult, error) {
	if h.Check == nil {
		return map[string]CheckResult{}, nil
	}
	return h.Check(p.Context), nil
}

// DebugInfoRequest describes the /debug/info endpoint.
type DebugInfoRequest struct {
	httprequest.Route `httprequest:"GET /debug/info"`
}

// DebugInfo returns version information on the current server.
func (h *Handler) DebugInfo(*DebugInfoRequest) (Version, error) {
	return h.Version, nil
}

// DebugPprofRequest describes the /debug/pprof/ endpoint.
type DebugPprofRequest struct {
	httprequest.Route `httprequest:"GET /debug/pprof/"`
}

// DebugPprof serves index information on the available pprof endpoints.
func (h *Handler) DebugPprof(p httprequest.Params, _ *DebugPprofRequest) error {
	if err := h.checkPprofAllowed(p.Request); err != nil {
		return err
	}
	pprof.Index(p.Response, p.Request)
	return nil
}

// DebugPprofEndpointsRequest describes the endpoints under /debug/prof.
type DebugPprofEndpointsRequest struct {
	httprequest.Route `httprequest:"GET /debug/pprof/:name"`
	Name              string `httprequest:"name,path"`
}

// DebugPprofEndpoints serves all the endpoints under DebugPprof.
func (h *Handler) DebugPprofEndpoints(p httprequest.Params, r *DebugPprofEndpointsRequest) error {
	if err := h.checkPprofAllowed(p.Request); err != nil {
		return err
	}
	switch r.Name {
	case "cmdline":
		pprof.Cmdline(p.Response, p.Request)
	case "profile":
		pprof.Profile(p.Response, p.Request)
	case "symbol":
		pprof.Symbol(p.Response, p.Request)
	default:
		pprof.Handler(r.Name).ServeHTTP(p.Response, p.Request)
	}
	return nil
}

// ErrNoPprofConfigured is the error returned on access
// to endpoints when Handler.CheckPprofAllowed is nil.
var ErrNoPprofConfigured = errgo.New("no pprof access configured")

// checkPprofAllowed is used instead of h.CheckPprofAllowed
// so that we don't panic if that is nil.
func (h *Handler) checkPprofAllowed(req *http.Request) error {
	if h.CheckPprofAllowed == nil {
		return ErrNoPprofConfigured
	}
	return h.CheckPprofAllowed(req)
}

// DebugEventsRequest describes the /debug/events endpoint.
type DebugEventsRequest struct {
	httprequest.Route `httprequest:"GET /debug/events"`
}

// DebugEvents serves the /debug/events endpoint.
func (h *Handler) DebugEvents(p httprequest.Params, r *DebugEventsRequest) error {
	sensitive, err := h.checkTraceAllowed(p.Request)
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	trace.RenderEvents(p.Response, p.Request, sensitive)
	return nil
}

// DebugRequestsRequest describes the /debug/requests endpoint.
type DebugRequestsRequest struct {
	httprequest.Route `httprequest:"GET /debug/requests"`
}

// DebugRequests serves the /debug/requests endpoint.
func (h *Handler) DebugRequests(p httprequest.Params, r *DebugRequestsRequest) error {
	sensitive, err := h.checkTraceAllowed(p.Request)
	if err != nil {
		return errgo.Mask(err, errgo.Any)
	}
	trace.Render(p.Response, p.Request, sensitive)
	return nil
}

// ErrNoTraceConfigured is the error returned on access
// to endpoints when Handler.CheckTraceAllowed is nil.
var ErrNoTraceConfigured = errgo.New("no trace access configured")

// checkTraceAllowed is used instead of h.CheckTraceAllowed
// so that we don't panic if that is nil.
func (h *Handler) checkTraceAllowed(req *http.Request) (bool, error) {
	if h.CheckTraceAllowed == nil {
		return false, ErrNoTraceConfigured
	}
	return h.CheckTraceAllowed(req)
}

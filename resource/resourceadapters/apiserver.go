// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/resource"
	corestate "github.com/juju/juju/state"
)

// APIHTTPRequestExtractor provides the functionality to extract an API
// request from an HTTP request.
type APIHTTPRequestExtractor struct {
	Deps APIHTTPRequestExtractorDeps
}

// NewResourceOpener returns a new resource.Opener for the given
// HTTP request.
func (ex APIHTTPRequestExtractor) NewResourceOpener(req *http.Request) (resource.Opener, error) {
	apiReq, err := ex.extractForUnit(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceID := apiReq.unit.ServiceName()
	cURL, _ := apiReq.unit.CharmURL()
	opener, err := apiReq.NewResourceOpener(serviceID, cURL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return opener, nil
}

func (ex APIHTTPRequestExtractor) extractForUnit(req *http.Request) (*apiHTTPRequest, error) {
	st, unit, err := ex.Deps.ConnectForUnitAgent(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiReq := &apiHTTPRequest{
		st:     resources,
		userID: unit.Tag(),
		unit:   unit,
	}

	return apiReq, nil
}

// APIHTTPRequestExtractorDeps exposes the external dependencies
// of APIHTTPRequestExtractor.
type APIHTTPRequestExtractorDeps interface {
	ConnectForUser(*http.Request) (*corestate.State, corestate.Entity, error)

	ConnectForUnitAgent(*http.Request) (*corestate.State, *corestate.Unit, error)
}

// APIHTTPRequest represents the information in an HTTP API request
// for resources.
type APIHTTPRequest interface {
	// NewResourceOpener returns a new resource.Opener for the given service.
	NewResourceOpener(serviceID string, cURL *charm.URL) (resource.Opener, error)

	// Username returns the username the requestor.
	Username() string
}

// apiHTTPRequest is an implementation of APIHTTPRequest.
type apiHTTPRequest struct {
	st     corestate.Resources
	userID names.Tag
	unit   resource.Unit // (may be not set)
}

// NewResourceOpener implements APIHTTPRequest.
func (req *apiHTTPRequest) NewResourceOpener(serviceID string, cURL *charm.URL) (resource.Opener, error) {
	unit := req.unit
	if unit == nil {
		return nil, errors.NotImplementedf("")
	}

	ro := &resourceOpener{
		st:     req.st,
		userID: req.userID,
		unit:   unit,
	}
	return ro, nil
}

// Username implements APIHTTPRequest.
func (req *apiHTTPRequest) Username() string {
	switch tag := req.userID.(type) {
	case *names.UserTag:
		return tag.Name()
	default:
		return tag.Id()
	}
}

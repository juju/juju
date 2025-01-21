// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	handlersresource "github.com/juju/juju/apiserver/internal/handlers/resource"
	coreresource "github.com/juju/juju/core/resource"
	internalerrors "github.com/juju/juju/internal/errors"
)

type resourceServiceGetter struct {
	ctxt httpContext
}

func (a *resourceServiceGetter) Resource(r *http.Request) (handlersresource.ResourceService, error) {
	domainServices, err := a.ctxt.domainServicesForRequest(r.Context())
	if err != nil {
		return nil, internalerrors.Capture(err)
	}

	return domainServices.Resource(), nil
}

type migratingResourceServicesGetter struct {
	ctxt httpContext
}

func (a *migratingResourceServicesGetter) Resource(r *http.Request) (handlersresource.ResourceService, error) {
	domainServices, err := a.ctxt.domainServicesDuringMigrationForRequest(r)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return domainServices.Resource(), nil
}

func (a *migratingResourceServicesGetter) Application(r *http.Request) (handlersresource.ApplicationService, error) {
	domainServices, err := a.ctxt.domainServicesDuringMigrationForRequest(r)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	return domainServices.Application(), nil
}

type resourceOpenerGetter func(r *http.Request, tagKinds ...string) (coreresource.Opener, error)

func (rog resourceOpenerGetter) Opener(r *http.Request, tagKinds ...string) (coreresource.Opener, error) {
	return rog(r, tagKinds...)
}

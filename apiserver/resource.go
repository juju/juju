// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Remove this file once we add a registration mechanism.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"

	internalserver "github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/state"
)

type resourcesHandlerDeps struct {
	httpCtxt httpContext
}

// ConnectForUser connects to state for an API user.
func (deps resourcesHandlerDeps) ConnectForUser(req *http.Request) (*state.State, state.Entity, error) {
	return deps.httpCtxt.stateForRequestAuthenticatedUser(req)
}

// ConnectForUnitAgent connects to state for a unit agent.
func (deps resourcesHandlerDeps) ConnectForUnitAgent(req *http.Request) (*state.State, *state.Unit, error) {
	st, ent, err := deps.httpCtxt.stateForRequestAuthenticatedAgent(req)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	unit, ok := ent.(*state.Unit)
	if !ok {
		logger.Errorf("unexpected type: %T", ent)
		return nil, nil, errors.Errorf("unexpected type: %T", ent)
	}
	return st, unit, nil
}

// TODO(ericsnow) Move these functions to resourceadapters?

func newResourceHandler(httpCtxt httpContext) http.Handler {
	deps := resourcesHandlerDeps{httpCtxt}
	return server.NewLegacyHTTPHandler(
		func(req *http.Request) (server.DataStore, names.Tag, error) {
			st, entity, err := deps.ConnectForUser(req)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			resources, err := st.Resources()
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			ds := resourceadapters.DataStore{
				Resources: resources,
				State:     st,
			}
			return ds, entity.Tag(), nil
		},
	)
}

func newUnitResourceHandler(httpCtxt httpContext) http.Handler {
	extractor := resourceadapters.HTTPDownloadRequestExtractor{
		Connector: &resourcesHandlerDeps{httpCtxt},
	}
	deps := internalserver.NewLegacyHTTPHandlerDeps(extractor)
	return internalserver.NewLegacyHTTPHandler(deps)
}

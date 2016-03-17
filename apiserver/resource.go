// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Remove this file once we add a registration mechanism.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"

	internalserver "github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/resourceadapters"
	"github.com/juju/juju/state"
)

type resourcesHandlerDeps struct {
	httpCtxt httpContext
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

func newUnitResourceHandler(httpCtxt httpContext) http.Handler {
	extractor := resourceadapters.HTTPDownloadRequestExtractor{
		Connector: &resourcesHandlerDeps{httpCtxt},
	}
	deps := internalserver.NewLegacyHTTPHandlerDeps(extractor)
	return internalserver.NewLegacyHTTPHandler(deps)
}

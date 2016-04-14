// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/resource"
	internalserver "github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/api/server"
	corestate "github.com/juju/juju/state"
)

// NewPublicFacade provides the public API facade for resources. It is
// passed into common.RegisterStandardFacade.
func NewPublicFacade(st *corestate.State, _ *common.Resources, authorizer common.Authorizer) (*server.Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	rst, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newClient := func() (server.CharmStore, error) {
		return newCharmStoreClient(st)
	}
	facade, err := server.NewFacade(rst, newClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// NewUploadHandler returns a new HTTP handler for the given args.
func NewUploadHandler(args apihttp.NewHandlerArgs) http.Handler {
	return server.NewLegacyHTTPHandler(
		func(req *http.Request) (server.DataStore, names.Tag, error) {
			st, entity, err := args.Connect(req)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			resources, err := st.Resources()
			if err != nil {
				return nil, nil, errors.Trace(err)
			}

			return resources, entity.Tag(), nil
		},
	)
}

// NewDownloadHandler returns a new HTTP handler for the given args.
func NewDownloadHandler(args apihttp.NewHandlerArgs) http.Handler {
	extractor := &httpDownloadRequestExtractor{
		connect: args.Connect,
	}
	deps := internalserver.NewLegacyHTTPHandlerDeps(extractor)
	return internalserver.NewLegacyHTTPHandler(deps)
}

// stateConnector exposes ways to connect to Juju's state.
type stateConnector interface {
	// Connect connects to state for a unit agent.
}

// httpDownloadRequestExtractor provides the functionality needed to
// handle a resource download HTTP request.
type httpDownloadRequestExtractor struct {
	connect func(*http.Request) (*corestate.State, corestate.Entity, error)
}

// NewResourceOpener returns a new resource.Opener for the given
// HTTP request.
func (ex *httpDownloadRequestExtractor) NewResourceOpener(req *http.Request) (resource.Opener, error) {
	st, ent, err := ex.connect(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit, ok := ent.(*corestate.Unit)
	if !ok {
		logger.Errorf("unexpected type: %T", ent)
		return nil, errors.Errorf("unexpected type: %T", ent)
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	opener := &resourceOpener{
		st:     st,
		res:    resources,
		userID: unit.Tag(),
		unit:   unit,
	}
	return opener, nil
}

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api/server"
	corestate "github.com/juju/juju/state"
)

// StateConnector exposes ways to connect to Juju's state.
type StateConnector interface {
	// ConnectForUnitAgent connects to state for a unit agent.
	ConnectForUnitAgent(*http.Request) (*corestate.State, *corestate.Unit, error)
}

// HTTPDownloadRequestExtractor provides the functionality needed to
// handle a resource download HTTP request.
type HTTPDownloadRequestExtractor struct {
	// Connector provides connections to Juju's state.
	Connector StateConnector
}

// NewResourceOpener returns a new resource.Opener for the given
// HTTP request.
func (ex HTTPDownloadRequestExtractor) NewResourceOpener(req *http.Request) (resource.Opener, error) {
	st, unit, err := ex.Connector.ConnectForUnitAgent(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	opener := &resourceOpener{
		st:     resources,
		userID: unit.Tag(),
		unit:   unit,
	}
	return opener, nil
}

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
	ds := DataStore{
		Resources: rst,
		State:     st,
	}
	newClient := func(cURL *charm.URL, csMac *macaroon.Macaroon) (server.CharmStore, error) {
		opener := newCharmstoreOpener(cURL, csMac)
		return opener.NewClient()
	}
	return server.NewFacade(ds, newClient), nil
}

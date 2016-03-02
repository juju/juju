// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/resource"
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

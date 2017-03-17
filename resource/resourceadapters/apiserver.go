// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/resource"
	internalserver "github.com/juju/juju/resource/api/private/server"
	corestate "github.com/juju/juju/state"
)

// NewDownloadHandler returns a new HTTP handler for the given args.
func NewDownloadHandler(args apihttp.NewHandlerArgs) http.Handler {
	extractor := &httpDownloadRequestExtractor{
		connect: args.Connect,
	}
	deps := internalserver.NewHTTPHandlerDeps(extractor)
	return internalserver.NewHTTPHandler(deps)
}

// stateConnector exposes ways to connect to Juju's state.
type stateConnector interface {
	// Connect connects to state for a unit agent.
}

// httpDownloadRequestExtractor provides the functionality needed to
// handle a resource download HTTP request.
type httpDownloadRequestExtractor struct {
	connect func(*http.Request) (*corestate.State, func(), corestate.Entity, error)
}

// NewResourceOpener returns a new resource.Opener for the given
// HTTP request.
func (ex *httpDownloadRequestExtractor) NewResourceOpener(req *http.Request) (opener resource.Opener, err error) {
	st, releaser, _, err := ex.connect(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	closer := func() error {
		releaser()
		return nil
	}

	defer func() {
		if err != nil {
			closer()
		}
	}()

	unitTagStr := req.URL.Query().Get(":unit")
	unitTag, err := names.ParseUnitTag(unitTagStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	unit, err := st.Unit(unitTag.Id())
	if err != nil {
		return nil, errors.Annotate(err, "loading unit")
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	opener = &resourceOpener{
		st:     st,
		res:    resources,
		userID: unitTag,
		unit:   unit,
		closer: closer,
	}
	return opener, nil
}

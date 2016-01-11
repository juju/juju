// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Remove this file once we add a registration mechanism.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/resource/api/server"
)

func newResourceHandler(httpCtxt httpContext) http.Handler {
	return server.NewLegacyHTTPHandler(
		func(req *http.Request) (server.DataStore, names.Tag, error) {
			st, entity, err := httpCtxt.stateForRequestAuthenticatedUser(req)
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

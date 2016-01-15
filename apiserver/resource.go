// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// TODO(ericsnow) Remove this file once we add a registration mechanism.

package apiserver

import (
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/resource"
	internalserver "github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/state"
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

func newUnitResourceHandler(httpCtxt httpContext) http.Handler {
	return internalserver.NewLegacyHTTPHandler(
		func(req *http.Request) (internalserver.UnitDataStore, error) {
			st, ent, err := httpCtxt.stateForRequestAuthenticatedAgent(req)
			if err != nil {
				return nil, errors.Trace(err)
			}
			resources, err := st.Resources()
			if err != nil {
				return nil, errors.Trace(err)
			}

			var svcName string
			switch ent := ent.(type) {
			case *state.Unit:
				svcName = ent.ServiceName()
			default:
				logger.Criticalf("unexpected type: %T", ent)
				return nil, errors.Errorf("unexpected type: %T", ent)
			}
			st2 := &resourceUnitState{
				state:     resources,
				serviceID: svcName,
			}

			return st2, nil
		},
	)
}

type resourceUnitState struct {
	state     state.Resources
	serviceID string
}

func (s *resourceUnitState) ListResources() ([]resource.Resource, error) {
	return s.state.ListResources(s.serviceID)
}

func (s *resourceUnitState) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	return s.state.OpenResource(s.serviceID, name)
}

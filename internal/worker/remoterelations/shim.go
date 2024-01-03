// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/internal/worker/apicaller"
)

func NewRemoteRelationsFacade(apiCaller base.APICaller) RemoteRelationsFacade {
	return remoterelations.NewClient(apiCaller)
}

func NewWorker(config Config) (worker.Worker, error) {
	w, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// remoteRelationsFacadeForModelFunc returns a function that
// can be used to construct instances which manage remote relation
// changes for a given model.

// For now we use a facade, but in future this may evolve into a REST caller.
func remoteRelationsFacadeForModelFunc(
	connectionFunc apicaller.NewExternalControllerConnectionFunc,
) newRemoteRelationsFacadeFunc {
	return func(apiInfo *api.Info) (RemoteModelRelationsFacadeCloser, error) {
		apiInfo.Tag = names.NewUserTag(api.AnonymousUsername)
		conn, err := connectionFunc(apiInfo)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return crossmodelrelations.NewClient(conn), nil
	}
}

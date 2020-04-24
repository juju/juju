// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/crossmodelrelations"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/worker/apicaller"
)

func NewRemoteRelationsFacade(apiCaller base.APICaller) (RemoteRelationsFacade, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

func NewRemoteModelRelationsFacade(apiCaller base.APICallCloser) (RemoteModelRelationsFacade, error) {
	facade := crossmodelrelations.NewClient(apiCaller)
	return facade, nil
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
		facade, err := NewRemoteModelRelationsFacade(conn)
		if err != nil {
			conn.Close()
			return nil, errors.Trace(err)
		}
		return &remoteModelRelationsFacadeCloser{facade, conn}, nil
	}
}

type remoteModelRelationsFacadeCloser struct {
	RemoteModelRelationsFacade
	conn io.Closer
}

func (p *remoteModelRelationsFacadeCloser) Close() error {
	return p.conn.Close()
}

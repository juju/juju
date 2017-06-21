// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"

	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/crossmodelrelations"
	"github.com/juju/juju/api/remoterelations"
)

func NewRemoteRelationsFacade(apiCaller base.APICaller) (RemoteRelationsFacade, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

func NewRemoteModelRelationsFacade(apiCaller base.APICaller) (RemoteModelRelationsFacade, error) {
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
// can be used be construct instances which manage remote relation
// changes for a given model.

// For now we use a facade, but in future this may evolve into a REST caller.
func remoteRelationsFacadeForModelFunc(
	apiConnForModelFunc func(string) (api.Connection, error),
) func(string) (RemoteModelRelationsFacadeCloser, error) {
	return func(modelUUID string) (RemoteModelRelationsFacadeCloser, error) {
		conn, err := apiConnForModelFunc(modelUUID)
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

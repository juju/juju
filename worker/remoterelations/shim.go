// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"io"

	"github.com/juju/errors"
	worker "gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/remoterelations"
)

func NewRemoteRelationsFacade(apiCaller base.APICaller) (RemoteRelationsFacade, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

func NewWorker(config Config) (worker.Worker, error) {
	w, err := New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// relationChangePublisherForModelFunc returns a function that
// can be used be construct instances which publish remote relation
// changes for a given model.

// For now we use a facade on the same controller, but in future this
// may evolve into a REST caller.
func relationChangePublisherForModelFunc(
	apiConnForModelFunc func(string) (api.Connection, error),
) func(string) (RemoteRelationChangePublisherCloser, error) {
	return func(modelUUID string) (RemoteRelationChangePublisherCloser, error) {
		conn, err := apiConnForModelFunc(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		facade, err := NewRemoteRelationsFacade(conn)
		if err != nil {
			conn.Close()
			return nil, errors.Trace(err)
		}
		return &publisherCloser{facade, conn}, nil
	}
}

type publisherCloser struct {
	RemoteRelationChangePublisher
	conn io.Closer
}

func (p *publisherCloser) Close() error {
	return p.conn.Close()
}

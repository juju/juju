// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/crossmodelrelations"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remoterelations"
)

// NewRemoteRelationsFacade creates a remote relations API facade.
func NewRemoteRelationsFacade(apiCaller base.APICaller) (*remoterelations.Client, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

// NewFirewallerFacade creates a firewaller API facade.
func NewFirewallerFacade(apiCaller base.APICaller) (FirewallerAPI, error) {
	facade := firewaller.NewState(apiCaller)
	return facade, nil
}

// NewWorker creates a firewaller worker.
func NewWorker(cfg Config) (worker.Worker, error) {
	w, err := NewFirewaller(cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// crossmodelFirewallerFacadeFunc returns a function that
// can be used to construct instances which manage remote relation
// firewall changes for a given model.

// For now we use a facade, but in future this may evolve into a REST caller.
func crossmodelFirewallerFacadeFunc(
	apiConnForModelFunc func(string) (api.Connection, error),
) func(string) (CrossModelFirewallerFacadeCloser, error) {
	return func(modelUUID string) (CrossModelFirewallerFacadeCloser, error) {
		conn, err := apiConnForModelFunc(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		facade := crossmodelrelations.NewClient(conn)
		return &crossModelFirewallerFacadeCloser{facade, conn}, nil
	}
}

type crossModelFirewallerFacadeCloser struct {
	CrossModelFirewallerFacade
	conn io.Closer
}

func (p *crossModelFirewallerFacadeCloser) Close() error {
	return p.conn.Close()
}

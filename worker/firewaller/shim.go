// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"io"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remotefirewaller"
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

// remoteFirewallerAPIFunc returns a function that
// can be used be construct instances which provide a
// remote firewaller API facade for a given (remote) model.
// For now we use a facade on the same controller.
func remoteFirewallerAPIFunc(
	apiConnForModelFunc func(string) (api.Connection, error),
) func(string) (RemoteFirewallerAPICloser, error) {
	return func(modelUUID string) (RemoteFirewallerAPICloser, error) {
		conn, err := apiConnForModelFunc(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		facade := remotefirewaller.NewClient(conn)
		return &firewallerAPICloser{facade, conn}, nil
	}
}

type firewallerAPICloser struct {
	RemoteFirewallerAPI
	conn io.Closer
}

func (p *firewallerAPICloser) Close() error {
	return p.conn.Close()
}

// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/crossmodelrelations"
	"github.com/juju/juju/api/firewaller"
	"github.com/juju/juju/api/remoterelations"
	"github.com/juju/juju/worker/apicaller"
)

// NewRemoteRelationsFacade creates a remote relations API facade.
func NewRemoteRelationsFacade(apiCaller base.APICaller) (*remoterelations.Client, error) {
	facade := remoterelations.NewClient(apiCaller)
	return facade, nil
}

// NewFirewallerFacade creates a firewaller API facade.
func NewFirewallerFacade(apiCaller base.APICaller) (FirewallerAPI, error) {
	facade, err := firewaller.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
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
	connectionFunc apicaller.NewExternalControllerConnectionFunc,
) newCrossModelFacadeFunc {
	return func(apiInfo *api.Info) (CrossModelFirewallerFacadeCloser, error) {
		apiInfo.Tag = names.NewUserTag(api.AnonymousUsername)
		conn, err := connectionFunc(apiInfo)
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

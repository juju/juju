// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/crossmodelrelations"
	"github.com/juju/juju/api/controller/firewaller"
	"github.com/juju/juju/api/controller/remoterelations"
	"github.com/juju/juju/internal/worker/apicaller"
)

// NewRemoteRelationsFacade creates a remote relations API facade.
func NewRemoteRelationsFacade(apiCaller base.APICaller) *remoterelations.Client {
	return remoterelations.NewClient(apiCaller)
}

// NewFirewallerFacade creates a firewaller API facade.
func NewFirewallerFacade(apiCaller base.APICaller) (FirewallerAPI, error) {
	facade, err := firewaller.NewClient(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &firewallerShim{facade}, nil
}

type firewallerShim struct {
	*firewaller.Client
}

func (s *firewallerShim) Machine(tag names.MachineTag) (Machine, error) {
	return s.Client.Machine(tag)
}

func (s *firewallerShim) Unit(tag names.UnitTag) (Unit, error) {
	u, err := s.Client.Unit(tag)
	if err != nil {
		return nil, err
	}
	return &unitShim{u}, nil
}

type unitShim struct {
	*firewaller.Unit
}

func (s *unitShim) Application() (Application, error) {
	return s.Unit.Application()
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
		return crossmodelrelations.NewClient(conn), nil
	}
}

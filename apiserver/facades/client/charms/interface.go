// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/state"
)

type stateShim struct {
	*state.State
}

func newStateShim(st *state.State) interfaces.BackendState {
	return stateShim{
		State: st,
	}
}

func (s stateShim) Application(name string) (interfaces.Application, error) {
	app, err := s.State.Application(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return app, nil
}

// StoreCharm represents a store charm.
type StoreCharm interface {
	charm.Charm
	charm.LXDProfiler
	Version() string
}

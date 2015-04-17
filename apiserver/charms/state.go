// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/state"
)

type charmsAccess interface {
	Charm(curl *charm.URL) (*state.Charm, error)
	AllCharms() ([]*state.Charm, error)
}

type stateShim struct {
	state *state.State
}

func (s stateShim) Charm(curl *charm.URL) (*state.Charm, error) {
	return s.state.Charm(curl)
}

func (s stateShim) AllCharms() ([]*state.Charm, error) {
	return s.state.AllCharms()
}

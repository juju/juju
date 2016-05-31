// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
)

// CharmMetadata returns the charm metadata for the identified service.
func CharmMetadata(st *state.State, serviceID string) (*charm.Meta, error) {
	return charmMetadata(NewCharmState(st), serviceID)
}

func charmMetadata(st CharmState, serviceID string) (*charm.Meta, error) {
	service, err := st.Service(serviceID)
	if err != nil {
		return nil, errors.Annotatef(err, "while looking up service %q", serviceID)
	}

	ch, err := service.Charm()
	if err != nil {
		return nil, errors.Annotatef(err, "while looking up charm info for service %q", serviceID)
	}

	meta := ch.Meta()

	return meta, nil
}

// CharmState exposes the methods of state.State used here.
type CharmState interface {
	Service(id string) (CharmService, error)
}

// CharmService exposes the methods of state.Service used here.
type CharmService interface {
	Charm() (Charm, error)
}

// Charm exposes the methods of state.Charm used here.
type Charm interface {
	Meta() *charm.Meta
}

// NewCharmState returns a new CharmState for the given state.State.
func NewCharmState(st *state.State) CharmState {
	return &charmState{raw: st}
}

type charmState struct {
	raw *state.State
}

// Service implements CharmState.
func (st charmState) Service(id string) (CharmService, error) {
	raw, err := st.raw.Service(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &charmService{raw: raw}, nil
}

type charmService struct {
	raw *state.Service
}

// Charm implements CharmService.
func (svc charmService) Charm() (Charm, error) {
	raw, _, err := svc.raw.Charm()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &charmInfo{raw}, nil
}

type charmInfo struct {
	*state.Charm
}

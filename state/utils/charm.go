// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
)

func charmMetadata(st CharmState, applicationID string) (*charm.Meta, error) {
	application, err := st.Service(applicationID)
	if err != nil {
		return nil, errors.Annotatef(err, "while looking up application %q", applicationID)
	}

	ch, err := application.Charm()
	if err != nil {
		return nil, errors.Annotatef(err, "while looking up charm info for application %q", applicationID)
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
	raw, err := st.raw.Application(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &charmService{raw: raw}, nil
}

type charmService struct {
	raw *state.Application
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

// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version"

	"github.com/juju/juju/state"
)

// backend wraps a *state.State to implement Backend.
// It is untested, but is simple enough to be verified by inspection.
type backend struct {
	*state.State
	state.ExternalControllers
}

func newBacked(st *state.State) Backend {
	return &backend{
		State:               st,
		ExternalControllers: state.NewExternalControllers(st),
	}
}

// ModelName implements Backend.
func (s *backend) ModelName() (string, error) {
	model, err := s.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.Name(), nil
}

// ModelOwner implements Backend.
func (s *backend) ModelOwner() (names.UserTag, error) {
	model, err := s.Model()
	if err != nil {
		return names.UserTag{}, errors.Trace(err)
	}
	return model.Owner(), nil
}

// AgentVersion implements Backend.
func (s *backend) AgentVersion() (version.Number, error) {
	m, err := s.Model()
	if err != nil {
		return version.Zero, errors.Trace(err)
	}

	cfg, err := m.ModelConfig()
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	vers, ok := cfg.AgentVersion()
	if !ok {
		return version.Zero, errors.New("no agent version")
	}
	return vers, nil
}

// AllOfferConnections (Backend) returns all CMR offer consumptions
// for the model.
func (s *backend) AllOfferConnections() ([]OfferConnection, error) {
	conns, err := s.State.AllOfferConnections()
	if err != nil {
		return nil, errors.Trace(err)
	}
	out := make([]OfferConnection, len(conns))
	for i, conn := range conns {
		out[i] = conn
	}
	return out, nil
}

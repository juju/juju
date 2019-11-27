// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/version"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/state"
)

// backendShim wraps a *state.State to implement Backend. It is
// untested, but is simple enough to be verified by inspection.
type backendShim struct {
	*state.State
}

// ModelName implements Backend.
func (s *backendShim) ModelName() (string, error) {
	model, err := s.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.Name(), nil
}

// ModelOwner implements Backend.
func (s *backendShim) ModelOwner() (names.UserTag, error) {
	model, err := s.Model()
	if err != nil {
		return names.UserTag{}, errors.Trace(err)
	}
	return model.Owner(), nil
}

// AgentVersion implements Backend.
func (s *backendShim) AgentVersion() (version.Number, error) {
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
func (s *backendShim) AllOfferConnections() ([]OfferConnection, error) {
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

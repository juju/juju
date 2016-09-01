// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
	"github.com/juju/version"
)

// newAPIForRegistration exists to provide the required signature for
// RegisterStandardFacade, converting st to backend.
func newAPIForRegistration(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	return NewAPI(&backendShim{st}, migration.PrecheckShim(st), resources, authorizer)
}

// backendShim wraps a *state.State to implement Backend. It is
// untested, but is simple enough to be verified by inspection.
type backendShim struct {
	*state.State
}

func (s *backendShim) ModelName() (string, error) {
	model, err := s.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.Name(), nil
}

func (s *backendShim) AgentVersion() (version.Number, error) {
	cfg, err := s.ModelConfig()
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	vers, ok := cfg.AgentVersion()
	if !ok {
		return version.Zero, errors.New("no agent version")
	}
	return vers, nil
}

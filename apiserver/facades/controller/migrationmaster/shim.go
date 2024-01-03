// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/version/v2"

	"github.com/juju/juju/state"
)

// backend wraps a *state.State to implement Backend.
// It is untested, but is simple enough to be verified by inspection.
type backend struct {
	*state.State
}

func newBacked(st *state.State) Backend {
	return &backend{State: st}
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

	cfg, err := m.ModelConfig(context.TODO())
	if err != nil {
		return version.Zero, errors.Trace(err)
	}
	vers, ok := cfg.AgentVersion()
	if !ok {
		return version.Zero, errors.New("no agent version")
	}
	return vers, nil
}

// AllLocalRelatedModels returns all models on this controller to which
// another hosted model has a consuming cross model relation.
func (s *backend) AllLocalRelatedModels() ([]string, error) {
	uuids, err := s.AllModelUUIDs()
	if err != nil {
		return nil, errors.Trace(err)
	}
	localUUIDs := set.NewStrings(uuids...)
	apps, err := s.AllRemoteApplications()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var crossModelUUIDs []string
	for _, app := range apps {
		if app.IsConsumerProxy() {
			continue
		}
		if localUUIDs.Contains(app.SourceModel().Id()) {
			crossModelUUIDs = append(crossModelUUIDs, app.SourceModel().Id())
		}
	}
	return crossModelUUIDs, nil
}

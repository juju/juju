// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/collections/set"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/domain/application"
)

// ApplicationExposed returns whether the provided application is exposed or
// not.
func (s *State) ApplicationExposed(ctx context.Context, appID coreapplication.ID) (bool, error) {
	return false, nil
}

// GetExposedEndpoints returns map where keys are endpoint names (or the ""
// value which represents all endpoints) and values are ExposedEndpoint
// instances that specify which sources (spaces or CIDRs) can access the
// opened ports for each endpoint once the application is exposed.
func (s *State) GetExposedEndpoints(ctx context.Context, appID coreapplication.ID) (map[string]application.ExposedEndpoint, error) {
	return nil, nil
}

// UnsetExposeSettings removes the expose settings for the provided list of
// endpoint names. If the resulting exposed endpoints map for the application
// becomes empty after the settings are removed, the application will be
// automatically unexposed.
func (s *State) UnsetExposeSettings(ctx context.Context, appID coreapplication.ID, exposedEndpoints set.Strings) error {
	return nil
}

// MergeExposeSettings marks the application as exposed and merges the provided
// ExposedEndpoint details into the current set of expose settings. The merge
// operation will overwrite expose settings for each existing endpoint name.
func (s *State) MergeExposeSettings(ctx context.Context, appID coreapplication.ID, exposedEndpoints map[string]application.ExposedEndpoint) error {
	return nil
}

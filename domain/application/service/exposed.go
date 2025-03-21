// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
)

// ApplicationExposed returns whether the provided application is exposed or not.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) ApplicationExposed(ctx context.Context, appName string) (bool, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return false, errors.Capture(err)
	}

	return s.st.ApplicationExposed(ctx, appID)
}

// GetExposedEndpoints returns map where keys are endpoint names (or the ""
// value which represents all endpoints) and values are ExposedEndpoint
// instances that specify which sources (spaces or CIDRs) can access the
// opened ports for each endpoint once the application is exposed.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) GetExposedEndpoints(ctx context.Context, appName string) (map[string]application.ExposedEndpoint, error) {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return s.st.GetExposedEndpoints(ctx, appID)
}

// UnsetExposeSettings removes the expose settings for the provided list of
// endpoint names. If the resulting exposed endpoints map for the application
// becomes empty after the settings are removed, the application will be
// automatically unexposed.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) UnsetExposeSettings(ctx context.Context, appName string, exposedEndpoints set.Strings) error {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.UnsetExposeSettings(ctx, appID, exposedEndpoints)
}

// MergeExposeSettings marks the application as exposed and merges the provided
// ExposedEndpoint details into the current set of expose settings. The merge
// operation will overwrite expose settings for each existing endpoint name.
//
// If no application is found, an error satisfying
// [applicationerrors.ApplicationNotFound] is returned.
func (s *Service) MergeExposeSettings(ctx context.Context, appName string, exposedEndpoints map[string]application.ExposedEndpoint) error {
	appID, err := s.st.GetApplicationIDByName(ctx, appName)
	if err != nil {
		return errors.Capture(err)
	}

	return s.st.MergeExposeSettings(ctx, appID, exposedEndpoints)
}

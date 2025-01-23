// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The code in this file provides a temporary implementation for handling
// the environ version, which is currently needed for export functionality.
//
// This implementation fetches the version directly from the environ provider
// instead of relying on the underlying data model. This is a placeholder solution,
// and the design might evolve as we decide how to model it properly.
//
// We should revisit and refactor this code when a clearer approach is established.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
)

// EnvironVersionProvider defines a minimal subset of the EnvironProvider interface
// that focuses specifically on the provider's versioning capabilities.
type EnvironVersionProvider interface {
	// Version returns the version of the provider. This is recorded as the
	// environ version for each model, and used to identify which upgrade
	// operations to run when upgrading a model's environ.
	Version() int
}

// EnvironVersionProviderFunc describes a type that is able to return a
// [EnvironVersionProvider] for the specified cloud type. If no
// environ version provider exists for the supplied cloud type then a
// [coreerrors.NotFound] error is returned. If the cloud type provider does not support
// the EnvironVersionProvider interface then a [coreerrors.NotSupported] error is returned.
type EnvironVersionProviderFunc func(string) (EnvironVersionProvider, error)

// EnvironVersionProviderGetter returns a [EnvironVersionProviderFunc]
// for retrieving an EnvironVersionProvider
func EnvironVersionProviderGetter() EnvironVersionProviderFunc {
	return func(cloudType string) (EnvironVersionProvider, error) {
		environProvider, err := environs.GlobalProviderRegistry().Provider(cloudType)
		if errors.Is(err, coreerrors.NotFound) {
			return nil, errors.Errorf(
				"no environ version provider exists for cloud type %q", cloudType,
			).Add(coreerrors.NotFound)
		}

		environVersionProvider, supports := environProvider.(EnvironVersionProvider)
		if !supports {
			return nil, errors.Errorf(
				"environ version provider not supported for cloud type %q", cloudType,
			).Add(coreerrors.NotSupported)
		}

		return environVersionProvider, nil
	}
}

// GetEnvironVersion retrieves the version of the environment provider associated with the model.
//
// The following error types can be expected:
// - [modelerrors.NotFound]: Returned if the model does not exist.
func (s *ModelService) GetEnvironVersion(ctx context.Context) (int, error) {
	modelCloudType, err := s.modelSt.GetModelCloudType(ctx)
	if err != nil {
		return 0, errors.Errorf(
			"getting model cloud type from state: %w", err,
		)
	}

	envProvider, err := s.environProviderGetter(modelCloudType)
	if err != nil {
		return 0, errors.Errorf(
			"getting environment provider for cloud type %q: %w", modelCloudType, err,
		)
	}

	return envProvider.Version(), nil
}

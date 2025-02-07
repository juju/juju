// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"

	"github.com/juju/errors"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// EnvironConfigGetter exposes a model configuration to its clients.
type EnvironConfigGetter interface {
	ModelConfig(context.Context) (*config.Config, error)
	CloudSpec(context.Context) (environscloudspec.CloudSpec, error)
}

// EnvironCredentialsInvalidator is an interface that provides a way to
// invalidate the credentials for a model.
type EnvironCredentialsInvalidator interface {
	CredentialInvalidator() ModelCredentialInvalidator
}

// NewEnvironFunc is the type of a function that, given a model config,
// returns an Environ. This will typically be environs.New.
type NewEnvironFunc func(context.Context, OpenParams) (Environ, error)

// GetEnviron returns the environs.Environ ("provider") associated
// with the model.
func GetEnviron(ctx context.Context, st EnvironConfigGetter, invalidator EnvironCredentialsInvalidator, newEnviron NewEnvironFunc) (Environ, error) {
	env, _, err := GetEnvironAndCloud(ctx, st, invalidator, newEnviron)
	return env, err
}

// GetEnvironAndCloud returns the environs.Environ ("provider") and cloud associated
// with the model.
func GetEnvironAndCloud(ctx context.Context, getter EnvironConfigGetter, invalidator EnvironCredentialsInvalidator, newEnviron NewEnvironFunc) (Environ, *environscloudspec.CloudSpec, error) {
	modelConfig, err := getter.ModelConfig(ctx)
	if err != nil {
		return nil, nil, errors.Annotate(err, "retrieving model config")
	}

	cloudSpec, err := getter.CloudSpec(ctx)
	if err != nil {
		return nil, nil, errors.Annotatef(
			err, "retrieving cloud spec for model %q (%s)", modelConfig.Name(), modelConfig.UUID())
	}

	env, err := newEnviron(ctx, OpenParams{
		Cloud:                 cloudSpec,
		Config:                modelConfig,
		CredentialInvalidator: invalidator.CredentialInvalidator(),
	})
	if err != nil {
		return nil, nil, errors.Annotatef(
			err, "creating environ for model %q (%s)", modelConfig.Name(), modelConfig.UUID())
	}
	return env, &cloudSpec, nil
}

type NoopEnvironCredentialInvalidator struct{}

func (NoopEnvironCredentialInvalidator) CredentialInvalidator() ModelCredentialInvalidator {
	return NoopModelCredentialInvalidator{}
}

type NoopModelCredentialInvalidator struct{}

func (NoopModelCredentialInvalidator) InvalidateModelCredential(context.Context, InvalidationReason) error {
	return nil
}

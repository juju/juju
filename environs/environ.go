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
	// ModelConfig returns the current config for the model, this is used
	// to create the environ.
	ModelConfig(context.Context) (*config.Config, error)

	// CloudSpec returns the cloud spec for the model, this is used to create
	// the environ.
	CloudSpec(context.Context) (environscloudspec.CloudSpec, error)

	// ControllerUUID returns the UUID of the controller.
	ControllerUUID(context.Context) (string, error)
}

// CredentialInvalidReason is an enumeration of reasons why credentials
// might be invalidated.
type CredentialInvalidReason string

// CredentialInvalidator is an interface that can invalidate a provider
// credentials.
type CredentialInvalidator interface {
	// InvalidateCredentials invalidates the credentials for the provider.
	// The reason argument indicates why the credentials are being invalidated.
	InvalidateCredentials(context.Context, CredentialInvalidReason) error
}

// NewEnvironFunc is the type of a function that, given a model config,
// returns an Environ. This will typically be environs.New.
type NewEnvironFunc func(context.Context, OpenParams, CredentialInvalidator) (Environ, error)

// GetEnvironAndCloud returns the environs.Environ ("provider") and cloud associated
// with the model.
func GetEnvironAndCloud(ctx context.Context, getter EnvironConfigGetter, invalidator CredentialInvalidator, newEnviron NewEnvironFunc) (Environ, *environscloudspec.CloudSpec, error) {
	modelConfig, err := getter.ModelConfig(ctx)
	if err != nil {
		return nil, nil, errors.Annotate(err, "retrieving model config")
	}

	cloudSpec, err := getter.CloudSpec(ctx)
	if err != nil {
		return nil, nil, errors.Annotatef(
			err, "retrieving cloud spec for model %q (%s)", modelConfig.Name(), modelConfig.UUID())
	}

	controllerUUID, err := getter.ControllerUUID(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	env, err := newEnviron(ctx, OpenParams{
		Cloud:          cloudSpec,
		Config:         modelConfig,
		ControllerUUID: controllerUUID,
	}, invalidator)
	if err != nil {
		return nil, nil, errors.Annotatef(
			err, "creating environ for model %q (%s)", modelConfig.Name(), modelConfig.UUID())
	}
	return env, &cloudSpec, nil
}

// NoopCredentialInvalidator returns a CredentialInvalidator that does nothing.
func NoopCredentialInvalidator() CredentialInvalidator {
	return noopCredentialInvalidator{}
}

type noopCredentialInvalidator struct{}

func (noopCredentialInvalidator) InvalidateCredentials(context.Context, CredentialInvalidReason) error {
	return nil
}

// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/uuid"
)

// Provider in the intersection of a environs.Environ and a caas.Broker.
//
// We ideally don't want to import the environs package here, but I've not
// sure how to avoid it.
type Provider interface {
	// InstancePrechecker provides a means of "prechecking" placement
	// arguments before recording them in state.
	environs.InstancePrechecker

	// BootstrapEnviron defines methods for bootstrapping a controller.
	environs.BootstrapEnviron

	// ResourceAdopter defines methods for adopting resources.
	environs.ResourceAdopter
}

// NonTrackedProvider returns a provider that is not tracked by the worker.
// It is created and then discarded. No credential invalidation is enforced
// during the call to the provider.
type NonTrackedProvider interface {
	// Provider returns the provider.
	Provider() (Provider, error)

	// Kill will shut down the provider and release any resources.
	Kill()
}

// NonTrackedProviderConfig is a struct that contains the necessary information
// to create a provider.
type NonTrackedProviderConfig struct {
	// ModelType is the type of the model.
	ModelType model.ModelType

	// ModelConfig is the model configuration for the provider.
	ModelConfig *config.Config

	// CloudSpec is the cloud spec for the provider.
	CloudSpec cloudspec.CloudSpec

	// ControllerUUID is the UUID of the controller that the provider is
	// associated with. This is currently only used for k8s providers.
	ControllerUUID uuid.UUID
}

// ProviderFactory is an interface that provides a way to get a provider
// for a given model namespace. It will continue to be updated in the background
// for as long as the Worker continues to run.
type ProviderFactory interface {
	// ProviderForModel returns the encapsulated provider for a given model
	// namespace. It will continue to be updated in the background for as long
	// as the Worker continues to run. If the worker is not a singular worker,
	// then an error will be returned.
	ProviderForModel(ctx context.Context, namespace string) (Provider, error)

	// ProviderFromConfig returns a provider for a given configuration.
	// The provider is not tracked, instead is created and then discarded.
	ProviderFromConfig(ctx context.Context, config NonTrackedProviderConfig) (NonTrackedProvider, error)
}

// ProviderGetter is a function that returns a provider for a given type.
// It's generic type any because it can return any type of provider, this should
// be used in conjunction with the ProviderRunner function.
type ProviderGetter[T any] func(ctx context.Context) (T, error)

// ProviderRunner returns the ProviderGetter function for a given generic type.
// If the returned provider is not of the expected type, a not supported
// error will be returned.
func ProviderRunner[T any](providerFactory ProviderFactory, namespace string) func(context.Context) (T, error) {
	var zero T
	return func(ctx context.Context) (T, error) {
		p, err := providerFactory.ProviderForModel(ctx, namespace)
		if err != nil {
			return zero, errors.Trace(err)
		}
		if v, ok := p.(T); ok {
			return v, nil
		}
		return zero, errors.NotSupportedf("provider type %T", zero)
	}
}

// NonTrackedProviderRunnerFromConfig returns the ProviderGetter function for a
// given generic type. This is useful for ad-hoc providers that are not tracked,
// but instead created and discarded. Credential invalidation is not enforced
// during the call to the provider. For that reason along, a closure is returned
// and the provider is created and discarded on each call.
//
// The name of this function is long, but that is intentional to make it clear
// that this is a non-tracked provider.
func NonTrackedProviderRunnerFromConfig[T any](providerFactory ProviderFactory, config NonTrackedProviderConfig) func(context.Context, func(context.Context, T) error) error {
	return func(ctx context.Context, fn func(context.Context, T) error) error {
		result, err := providerFactory.ProviderFromConfig(ctx, config)
		if err != nil {
			return errors.Trace(err)
		}

		// The provider will be discarded after the function is called. This
		// ensures that the provider is not leaked.
		defer result.Kill()

		provider, err := result.Provider()
		if err != nil {
			return errors.Trace(err)
		}

		if v, ok := provider.(T); ok {
			return fn(ctx, v)
		}

		var zero T
		return errors.NotSupportedf("provider type %T", zero)
	}
}

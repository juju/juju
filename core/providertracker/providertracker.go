// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/environs"
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

// ProviderFactory is an interface that provides a way to get a provider
// for a given model namespace. It will continue to be updated in the background
// for as long as the Worker continues to run.
type ProviderFactory interface {
	// ProviderForModel returns the encapsulated provider for a given model
	// namespace. It will continue to be updated in the background for as long
	// as the Worker continues to run. If the worker is not a singular worker,
	// then an error will be returned.
	ProviderForModel(ctx context.Context, namespace string) (Provider, error)
}

// GenericProviderFactory is an interface that provides a way to get a provider
// for a given model namespace. It will continue to be updated in the background
// for as long as the Worker continues to run.
type GenericProviderFactory[T Provider] interface {
	// ProviderForModel returns the encapsulated provider for a given model
	// namespace. It will continue to be updated in the background for as long
	// as the Worker continues to run. If the worker is not a singular worker,
	// then an error will be returned.
	ProviderForModel(ctx context.Context, namespace string) (T, error)
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

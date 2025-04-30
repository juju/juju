// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
)

// BootstrapAddressFinderFunc is responsible for finding the network provider
// addresses for a bootstrap instance.
type BootstrapAddressFinderFunc func(context.Context, instance.Id) (network.ProviderAddresses, error)

// getInstanceAddresses retrieves the instance from the instance lister and
// returns the addresses associated with the instance.
func getInstanceAddresses(
	ctx context.Context,
	lister environs.InstanceLister,
	instanceID instance.Id,
) (network.ProviderAddresses, error) {
	instances, err := lister.Instances(ctx, []instance.Id{instanceID})
	if err != nil {
		return nil, fmt.Errorf(
			"cannot get instance %q from instance lister: %w",
			instanceID, err,
		)
	}
	if len(instances) != 1 {
		return nil, fmt.Errorf(
			"requested instance %q from instance lister and got %d instances, unable to determine correct result",
			instanceID,
			len(instances),
		)
	}
	addrs, err := instances[0].Addresses(ctx)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot get bootstrap instance %q provider addresses: %w",
			instanceID, err,
		)
	}
	return addrs, nil
}

// BootstrapAddressFinder is responsible for finding the bootstrap network
// addresses for a bootstrap instance. If the provider does not implement the
// [environs.InstanceLister] interface then "localhost" will be returned. This
// is the case for CAAS providers.
func BootstrapAddressFinder(
	providerGetter providertracker.ProviderGetter[environs.InstanceLister],
) BootstrapAddressFinderFunc {
	return func(
		ctx context.Context,
		bootstrapInstance instance.Id,
	) (network.ProviderAddresses, error) {

		lister, err := providerGetter(ctx)
		if errors.Is(err, errors.NotSupported) {
			return network.NewMachineAddresses([]string{"localhost"}).AsProviderAddresses(), nil
		} else if err != nil {
			return network.ProviderAddresses{}, fmt.Errorf(
				"cannot get instance lister from provider for finding bootstrap addresses: %w",
				err,
			)
		}

		return getInstanceAddresses(ctx, lister, bootstrapInstance)
	}
}

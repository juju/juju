// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/errors"
	k8sconstants "github.com/juju/juju/internal/provider/kubernetes/constants"
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

// IAASAddressFinder is responsible for finding the network addresses for a
// bootstrap instance.
func IAASAddressFinder(
	providerFactory providertracker.ProviderFactory, namespace string,
) BootstrapAddressFinderFunc {
	return func(
		ctx context.Context,
		bootstrapInstance instance.Id,
	) (network.ProviderAddresses, error) {
		providerGetter := providertracker.ProviderRunner[environs.InstanceLister](
			providerFactory, namespace,
		)

		lister, err := providerGetter(ctx)
		if err != nil {
			return network.ProviderAddresses{}, fmt.Errorf(
				"cannot get instance lister from provider for finding bootstrap addresses: %w",
				err,
			)
		}

		return getInstanceAddresses(ctx, lister, bootstrapInstance)
	}
}

// K8sAddressFinder is responsible for finding the network addresses for a
// bootstrap instance.
func K8sAddressFinder(
	providerFactory providertracker.ProviderFactory, namespace string,
) BootstrapAddressFinderFunc {
	return func(
		ctx context.Context,
		bootstrapInstance instance.Id,
	) (network.ProviderAddresses, error) {
		providerGetter := providertracker.ProviderRunner[caas.ServiceManager](
			providerFactory, namespace,
		)

		svcManager, err := providerGetter(ctx)
		if err != nil {
			return network.ProviderAddresses{}, fmt.Errorf(
				"cannot get service manager from provider for finding bootstrap addresses: %w",
				err,
			)
		}

		// Retrieve the k8s service from the k8s broker.
		svc, err := svcManager.GetService(ctx, k8sconstants.JujuControllerStackName, true)
		if err != nil {
			return nil, errors.Capture(err)
		}
		if svc == nil || len(svc.Addresses) == 0 {
			return nil, errors.New("controller API service has no addresses")
		}

		return svc.Addresses, nil
	}
}

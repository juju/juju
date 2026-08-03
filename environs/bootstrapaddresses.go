// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
)

// BootstrapAddressFinder returns the stable provider addresses for the
// initial controller.
type BootstrapAddressFinder interface {
	// BootstrapControllerAddresses returns the provider addresses for the
	// initial controller.
	BootstrapControllerAddresses(context.Context) (network.ProviderAddresses, error)
}

// NewBootstrapAddressFinder returns the bootstrap address implementation for
// an opened bootstrap environment.
func NewBootstrapAddressFinder(
	env BootstrapEnviron, instanceID instance.Id,
) (BootstrapAddressFinder, error) {
	if finder, ok := env.(BootstrapAddressFinder); ok {
		return finder, nil
	}
	if lister, ok := env.(InstanceLister); ok {
		return instanceBootstrapAddressFinder{
			lister:     lister,
			instanceID: instanceID,
		}, nil
	}
	return nil, errors.NotSupportedf("bootstrap controller addresses for environ %T", env)
}

type instanceBootstrapAddressFinder struct {
	lister     InstanceLister
	instanceID instance.Id
}

// BootstrapControllerAddresses returns the provider addresses for the
// requested bootstrap instance.
func (f instanceBootstrapAddressFinder) BootstrapControllerAddresses(
	ctx context.Context,
) (network.ProviderAddresses, error) {
	instances, err := f.lister.Instances(ctx, []instance.Id{f.instanceID})
	if err != nil && !errors.Is(err, ErrPartialInstances) {
		return nil, errors.Annotatef(err, "getting bootstrap instance %q", f.instanceID)
	}
	if len(instances) != 1 || instances[0] == nil {
		return nil, errors.NotFoundf("bootstrap instance %q", f.instanceID)
	}
	if returnedID := instances[0].Id(); returnedID != f.instanceID {
		return nil, errors.NotFoundf(
			"bootstrap instance %q (provider returned %q)", f.instanceID, returnedID,
		)
	}

	addresses, err := instances[0].Addresses(ctx)
	if err != nil {
		return nil, errors.Annotatef(err, "getting bootstrap instance %q addresses", f.instanceID)
	}
	return addresses, nil
}

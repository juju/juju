// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"

	"github.com/packethost/packngo"
)

type equinixDevice struct {
	e *environ

	*packngo.Device
}

var _ instances.Instance = (*equinixDevice)(nil)

func (device *equinixDevice) String() string {
	return device.ID
}

func (device *equinixDevice) Id() instance.Id {
	return instance.Id(device.ID)
}

func (device *equinixDevice) Status(ctx context.ProviderCallContext) instance.Status {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

// Addresses implements network.Addresses() returning generic address
// details for the instance, and requerying the equinix api if required.
func (device *equinixDevice) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	panic(errors.NewNotImplemented(nil, "not implemented"))
}

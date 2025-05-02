// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package manual

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

var _ environs.NetworkingEnviron = &manualEnviron{}

// SupportsSpaces implements environs.NetworkingEnviron.
func (e *manualEnviron) SupportsSpaces() (bool, error) {
	return true, nil
}

// Subnets implements environs.NetworkingEnviron.
func (e *manualEnviron) Subnets(context.Context, []network.Id) ([]network.SubnetInfo, error) {
	return nil, errors.NotSupportedf("subnets")
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (e *manualEnviron) NetworkInterfaces(
	context.Context, []instance.Id,
) ([]network.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("network interfaces")
}

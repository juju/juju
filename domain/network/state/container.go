// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// GetMachineSpaceConstraints returns the positive and negative
// space constraints for the machine with the input UUID.
func (st *State) GetMachineSpaceConstraints(
	ctx context.Context, machineUUID string,
) ([]internal.SpaceName, []internal.SpaceName, error) {
	return nil, nil, errors.Errorf("implement me")
}

// GetMachineAppBindings returns the bound spaces for applications
// with units assigned to the machine with the input UUID.
func (st *State) GetMachineAppBindings(ctx context.Context, machineUUID string) ([]internal.SpaceName, error) {
	return nil, errors.Errorf("implement me")
}

// NICsInSpaces returns the link-layer devices on the machine with the
// input net node UUID, indexed by the spaces that they are in.
func (st *State) NICsInSpaces(ctx context.Context, netNode string) (map[string][]network.NetInterface, error) {
	return nil, errors.Errorf("implement me")
}

// GetContainerNetworkingMethod returns the model's configured value
// for container-networking-method.
func (st *State) GetContainerNetworkingMethod(ctx context.Context) (string, error) {
	return "", errors.Errorf("implement me")
}

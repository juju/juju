// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"context"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
)

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// GetAllSubnets returns all the subnets for the model.
	GetAllSubnets(ctx context.Context) (network.SubnetInfos, error)
}

// ApplicationService is the interface that is used to interact with the
// applications.
type ApplicationService interface {
	// GetUnitMachineName gets the name of the unit's machine.
	//
	// The following errors may be returned:
	//   - [applicationerrors.UnitNotFound] if the unit cannot be found.
	GetUnitMachineName(context.Context, unit.Name) (machine.Name, error)
}

// ModelConfigService is an interface that provides access to the
// model configuration.
type ModelConfigService interface {
	ModelConfig(ctx context.Context) (*config.Config, error)
}

// ModelProviderService providers access to the model provider service.
type ModelProviderService interface {
	// GetCloudSpecForSSH returns the cloud spec for sshing into a k8s pod.
	GetCloudSpecForSSH(ctx context.Context) (cloudspec.CloudSpec, error)
}

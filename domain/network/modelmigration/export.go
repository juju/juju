// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v5"
	"github.com/juju/loggo"

	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/service"
	"github.com/juju/juju/domain/network/state"
)

var logger = loggo.GetLogger("juju.migration.")

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator) {
	coordinator.Add(&exportOperation{})
}

// ExportSpaceService provides a subset of the network domain
// service methods needed for spaces export.
type ExportSpaceService interface {
	AddSpace(ctx context.Context, name string, providerID network.Id, subnetIDs []string) (network.Id, error)
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
}

// ExportSpaceService provides a subset of the network domain
// service methods needed for subnets export.
type ExportSubnetService interface {
	AddSubnet(ctx context.Context, args network.SubnetInfo) (network.Id, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	spaceService  ExportSpaceService
	subnetService ExportSubnetService
}

// Setup implements Operation.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.spaceService = service.NewSpaceService(
		state.NewState(scope.ModelDB()),
		logger,
	)
	e.subnetService = service.NewSubnetService(
		state.NewState(scope.ModelDB()),
	)
	return nil
}

// Execute the migration export, which adds the spaces and subnets to the model.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	return nil
}
